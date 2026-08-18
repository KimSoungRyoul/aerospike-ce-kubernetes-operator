#!/usr/bin/env bash
# helm-set-version.sh: Set the in-repo Helm chart version to VERSION.
#
# The chart version, the chart appVersion (which is what the operator image tag
# falls through to when values.image.tag is empty), the CRD sub-chart version and
# the dependency pin in the umbrella chart all have to move together. Bumping
# only some of them is how the chart ended up rendering a 1.3.1 operator image
# while the release was 1.10.x (#347), so this script moves all of them at once
# and regenerates Chart.lock plus the vendored sub-chart .tgz.
#
# Usage:
#   hack/helm-set-version.sh 1.10.3
#   hack/helm-set-version.sh v1.10.3   # leading v is stripped
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART_DIR="${REPO_ROOT}/charts/aerospike-ce-kubernetes-operator"
CRDS_CHART_DIR="${REPO_ROOT}/charts/aerospike-ce-kubernetes-operator-crds"

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version>" >&2
  exit 2
fi

VERSION="${1#v}"

if [[ ! "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
  echo "ERROR: '${VERSION}' is not a semver version" >&2
  exit 2
fi

# sed -i is not portable between GNU and BSD; write through a temp file instead.
# Takes sed arguments followed by the file to rewrite in place.
sed_inplace() {
  local file="${!#}"
  local args=("${@:1:$#-1}")
  local tmp
  tmp="$(mktemp)"
  sed "${args[@]}" "${file}" > "${tmp}"
  mv "${tmp}" "${file}"
}

# 1. CRD sub-chart: version + appVersion.
sed_inplace "s/^version:.*/version: ${VERSION}/"        "${CRDS_CHART_DIR}/Chart.yaml"
sed_inplace "s/^appVersion:.*/appVersion: \"${VERSION}\"/" "${CRDS_CHART_DIR}/Chart.yaml"

# 2. Umbrella chart: version + appVersion.
sed_inplace "s/^version:.*/version: ${VERSION}/"        "${CHART_DIR}/Chart.yaml"
sed_inplace "s/^appVersion:.*/appVersion: \"${VERSION}\"/" "${CHART_DIR}/Chart.yaml"

# 3. Umbrella chart: the pinned version of the CRD dependency, which is the line
#    immediately after the dependency's name.
sed_inplace "/name: aerospike-ce-kubernetes-operator-crds/{n;s/version: \".*\"/version: \"${VERSION}\"/;}" \
  "${CHART_DIR}/Chart.yaml"

# 4. Documented chart-version pins. `helm install oci://... --version 1.3.1` in
#    the docs installs a stale chart just as surely as a stale in-repo Chart.yaml
#    does, so the pins move with the chart. The patterns are anchored to the flag
#    or key that carries a chart version, never to a bare version string.
for doc in \
  "${CHART_DIR}/README.md" \
  "${REPO_ROOT}/docs/i18n/ko/docusaurus-plugin-content-docs/current/getting-started/install.md"; do
  [[ -f "${doc}" ]] || continue
  sed_inplace -E "s/--version [0-9]+\.[0-9]+\.[0-9]+/--version ${VERSION}/g" "${doc}"
  sed_inplace -E "s/targetRevision: \"[0-9]+\.[0-9]+\.[0-9]+\"/targetRevision: \"${VERSION}\"/g" "${doc}"
  sed_inplace -E "s/^([[:space:]]*)version: \"[0-9]+\.[0-9]+\.[0-9]+\"/\1version: \"${VERSION}\"/g" "${doc}"
done

# 5. Drop the stale vendored sub-chart archive before rebuilding, so an old
#    version's .tgz is not left behind next to the new one (helm would then have
#    two candidate sub-charts in charts/).
rm -f "${CHART_DIR}"/charts/aerospike-ce-kubernetes-operator-crds-*.tgz

# 6. Regenerate Chart.lock and the vendored .tgz from the local sub-chart.
helm dep update "${CHART_DIR}"

echo "Chart version set to ${VERSION}."
