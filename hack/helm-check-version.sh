#!/usr/bin/env bash
# helm-check-version.sh: Fail when the in-repo Helm chart no longer describes the
# operator this repo actually ships.
#
# Three assertions, in order of how loudly they fail for a user:
#
#   1. The rendered operator image tag equals the chart appVersion. This is the
#      user-visible symptom of #347: `helm install ./charts/...` deployed a 1.3.1
#      operator because values.image.tag is empty and falls through to appVersion.
#   2. Every place that carries a version carries the SAME version — the umbrella
#      chart's version and appVersion, the CRD sub-chart's version and appVersion,
#      the dependency pin, Chart.lock, and the vendored sub-chart .tgz filename.
#      This is what actually rots: bumping one and forgetting the rest.
#   3. The chart version is not OLDER than the latest published release. Ahead is
#      fine (the release commit bumps the chart before the tag exists); behind
#      means the documented local install ships a stale operator.
#
# Assertion 3 needs the latest release tag. CI passes it in via
# LATEST_RELEASE_TAG; locally the script falls back to `gh` and then to git tags,
# and warns rather than failing if it cannot resolve one offline.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART_DIR="${REPO_ROOT}/charts/aerospike-ce-kubernetes-operator"
CRDS_CHART_DIR="${REPO_ROOT}/charts/aerospike-ce-kubernetes-operator-crds"

fail() {
  echo "ERROR: $*" >&2
  echo "       Run 'make helm-set-version VERSION=<x.y.z>' to re-sync the chart." >&2
  exit 1
}

# yaml_field returns a top-level scalar field, with surrounding quotes stripped.
yaml_field() {
  sed -n "s/^$2:[[:space:]]*//p" "$1" | head -1 | tr -d '"' | tr -d "'" | tr -d '\r'
}

CHART_VERSION="$(yaml_field "${CHART_DIR}/Chart.yaml" version)"
CHART_APP_VERSION="$(yaml_field "${CHART_DIR}/Chart.yaml" appVersion)"
CRDS_VERSION="$(yaml_field "${CRDS_CHART_DIR}/Chart.yaml" version)"
CRDS_APP_VERSION="$(yaml_field "${CRDS_CHART_DIR}/Chart.yaml" appVersion)"

# The dependency pin is the version line following the dependency's name.
DEP_VERSION="$(sed -n '/name: aerospike-ce-kubernetes-operator-crds/{n;s/.*version:[[:space:]]*//p;}' \
  "${CHART_DIR}/Chart.yaml" | head -1 | tr -d '"')"

LOCK_VERSION="$(sed -n 's/^[[:space:]]*version:[[:space:]]*//p' "${CHART_DIR}/Chart.lock" | head -1 | tr -d '"')"

shopt -s nullglob
VENDORED=("${CHART_DIR}"/charts/aerospike-ce-kubernetes-operator-crds-*.tgz)
shopt -u nullglob

[[ -n "${CHART_VERSION}" ]] || fail "could not read version from ${CHART_DIR}/Chart.yaml"

# --- 1. Rendered image tag matches appVersion -------------------------------
# crds.install=false keeps this from depending on the sub-chart being vendored,
# so a broken .tgz reports as assertion 2 rather than as a confusing render error.
RENDERED_TAG="$(helm template acko "${CHART_DIR}" --set crds.install=false 2>/dev/null |
  grep -oE 'ghcr\.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator:[^[:space:]"]+' |
  head -1 | cut -d: -f2)"

if [[ -z "${RENDERED_TAG}" ]]; then
  fail "could not find the operator image in the rendered chart"
fi
if [[ "${RENDERED_TAG}" != "${CHART_APP_VERSION}" ]]; then
  fail "rendered operator image tag (${RENDERED_TAG}) != chart appVersion (${CHART_APP_VERSION})"
fi
echo "OK: rendered operator image tag = ${RENDERED_TAG}"

# --- 2. Internal consistency ------------------------------------------------
for pair in \
  "Chart.yaml appVersion:${CHART_APP_VERSION}" \
  "crds Chart.yaml version:${CRDS_VERSION}" \
  "crds Chart.yaml appVersion:${CRDS_APP_VERSION}" \
  "Chart.yaml dependency pin:${DEP_VERSION}" \
  "Chart.lock version:${LOCK_VERSION}"; do
  label="${pair%%:*}"
  value="${pair#*:}"
  if [[ "${value}" != "${CHART_VERSION}" ]]; then
    fail "${label} is ${value}, but Chart.yaml version is ${CHART_VERSION}"
  fi
done

if [[ ${#VENDORED[@]} -ne 1 ]]; then
  fail "expected exactly one vendored CRD sub-chart .tgz, found ${#VENDORED[@]}"
fi
VENDORED_VERSION="$(basename "${VENDORED[0]}" .tgz)"
VENDORED_VERSION="${VENDORED_VERSION#aerospike-ce-kubernetes-operator-crds-}"
if [[ "${VENDORED_VERSION}" != "${CHART_VERSION}" ]]; then
  fail "vendored sub-chart is ${VENDORED_VERSION}, but Chart.yaml version is ${CHART_VERSION}"
fi
echo "OK: chart version ${CHART_VERSION} is consistent across Chart.yaml, Chart.lock, the CRD sub-chart and the vendored .tgz"

# --- 3. Documented chart-version pins match ---------------------------------
# `helm install oci://... --version 1.3.1` in the docs installs a stale chart
# whatever Chart.yaml says, so the pins are checked too.
for doc in \
  "${CHART_DIR}/README.md" \
  "${REPO_ROOT}/docs/i18n/ko/docusaurus-plugin-content-docs/current/getting-started/install.md"; do
  [[ -f "${doc}" ]] || continue
  stale="$(grep -nE -- "(--version|targetRevision:|version:)[[:space:]]*\"?[0-9]+\.[0-9]+\.[0-9]+\"?" "${doc}" |
    grep -v "${CHART_VERSION}" || true)"
  if [[ -n "${stale}" ]]; then
    echo "${stale}" >&2
    fail "${doc#"${REPO_ROOT}/"} pins a chart version other than ${CHART_VERSION} (lines above)"
  fi
done
echo "OK: documented chart-version pins are all ${CHART_VERSION}"

# --- 4. Not older than the latest release -----------------------------------
LATEST="${LATEST_RELEASE_TAG:-}"
if [[ -z "${LATEST}" ]] && command -v gh >/dev/null 2>&1; then
  LATEST="$(gh release view --json tagName -q .tagName 2>/dev/null || true)"
fi
if [[ -z "${LATEST}" ]]; then
  LATEST="$(git -C "${REPO_ROOT}" describe --tags --abbrev=0 2>/dev/null || true)"
fi
LATEST="${LATEST#v}"

if [[ -z "${LATEST}" ]]; then
  echo "WARNING: could not resolve the latest release tag; skipping the staleness check." >&2
  echo "         Set LATEST_RELEASE_TAG to enforce it." >&2
  exit 0
fi

OLDEST="$(printf '%s\n%s\n' "${CHART_VERSION}" "${LATEST}" | sort -V | head -1)"
if [[ "${CHART_VERSION}" != "${LATEST}" && "${OLDEST}" == "${CHART_VERSION}" ]]; then
  fail "chart version ${CHART_VERSION} is older than the latest release ${LATEST}; the documented local 'helm install ./charts/...' would deploy a stale operator"
fi
echo "OK: chart version ${CHART_VERSION} is not older than the latest release ${LATEST}"
