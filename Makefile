# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# The default is Podman. Override with CONTAINER_TOOL=docker if needed.
CONTAINER_TOOL ?= podman
# Podman requires --format docker-archive for kind image loading; Docker does not support it.
SAVE_FORMAT_FLAG = $(if $(filter podman,$(CONTAINER_TOOL)),--format docker-archive,)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test-unit
test-unit: fmt vet ## Run unit tests (no envtest required).
	go test $$(go list ./... | grep -v /e2e) -coverprofile cover-unit.out

.PHONY: test-integration
test-integration: manifests generate fmt vet setup-envtest ## Run integration tests (envtest required).
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test -tags=integration $$(go list ./... | grep -v /e2e) -coverprofile cover-integration.out

.PHONY: test
test: test-unit test-integration ## Run all tests (unit + integration).

.PHONY: coverage
coverage: manifests generate fmt vet setup-envtest ## Run tests with coverage report.
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out
	go tool cover -html=cover.out -o cover.html
	go tool cover -func=cover.out | tail -1

.PHONY: clean
clean: ## Remove build artifacts, coverage files, and tool binaries.
	rm -rf bin/
	rm -f cover*.out
	rm -f cover.html

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the manager container image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= aerospike-ce-kubernetes-operator-test-e2e

.PHONY: setup-kind
setup-kind: ## Delete existing Kind cluster and create a fresh one with kind-config.yaml (3-worker, zone labels)
	@$(KIND) delete cluster --name kind 2>/dev/null || true
	KIND_EXPERIMENTAL_PROVIDER=$(KIND_PROVIDER) $(KIND) create cluster --config kind-config.yaml --name kind

##@ Local Development (Full Stack)

.PHONY: run-local
run-local: manifests helm-sync-crds ## Deploy operator + cluster-manager UI into a fresh Kind cluster via Helm
	@echo ""
	@echo "==> [0/7] Refreshing Helm sub-chart dependency (CRD bundle)..."
	helm dep update charts/aerospike-ce-kubernetes-operator
	@echo ""
	@echo "==> [1/7] Creating fresh Kind cluster..."
	-@$(KIND) delete cluster --name kind 2>/dev/null || true
	KIND_EXPERIMENTAL_PROVIDER=$(KIND_PROVIDER) $(KIND) create cluster --config kind-config.yaml --name kind
	@echo ""
	@echo "==> [2/7] Building operator image..."
	$(CONTAINER_TOOL) build --build-arg VERSION=$(VERSION) -t $(IMG) .
	@echo ""
	@echo "==> [3/7] Building cluster-manager image..."
	$(CONTAINER_TOOL) build -t $(CLUSTER_MANAGER_IMG):latest aerospike-cluster-manager/
	@echo ""
	@echo "==> [4/7] Loading images into Kind cluster..."
	$(CONTAINER_TOOL) save $(SAVE_FORMAT_FLAG) $(IMG) -o /tmp/acko-operator.tar
	$(KIND) load image-archive /tmp/acko-operator.tar --name kind
	$(CONTAINER_TOOL) save $(SAVE_FORMAT_FLAG) $(CLUSTER_MANAGER_IMG):latest -o /tmp/acko-cluster-manager.tar
	$(KIND) load image-archive /tmp/acko-cluster-manager.tar --name kind
	@rm -f /tmp/acko-operator.tar /tmp/acko-cluster-manager.tar
	@echo ""
	@echo "==> [5/7] Installing cert-manager..."
	helm install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace --set crds.enabled=true
	@echo "    Waiting for cert-manager webhook..."
	kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
	@echo ""
	@echo "==> [6/7] Deploying operator and UI via Helm..."
	helm upgrade -i aerospike-ce-kubernetes-operator ./charts/aerospike-ce-kubernetes-operator \
		-n aerospike-operator --create-namespace \
		--set ui.enabled=true \
		--set image.tag=latest \
		--set ui.image.tag=latest
	@echo ""
	@echo "==> [7/7] Waiting for operator deployment to be ready..."
	kubectl -n aerospike-operator wait --for=condition=Available deployment --all --timeout=180s
	@echo ""
	@echo "==> ACKO local development stack is running!"
	@echo "    Operator:  deployed in namespace 'aerospike-operator'"
	@echo "    UI:        kubectl port-forward -n aerospike-operator svc/aerospike-ce-kubernetes-operator-ui 3000:3000"
	@echo ""

.PHONY: stop-local
stop-local: ## Delete the Kind cluster used for local development
	-@$(KIND) delete cluster --name kind
	@echo "==> Kind cluster 'kind' deleted."

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)' with provider '$(KIND_PROVIDER)'..."; \
			KIND_EXPERIMENTAL_PROVIDER=$(KIND_PROVIDER) $(KIND) create cluster --config kind-config.yaml --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) CONTAINER_TOOL=$(CONTAINER_TOOL) KIND_EXPERIMENTAL_PROVIDER=$(KIND_PROVIDER) \
	  go test -tags=e2e -timeout 30m ./test/e2e/ -v -ginkgo.v $(GINKGO_FLAGS); \
	  test_exit=$$?; $(MAKE) cleanup-test-e2e; exit $$test_exit

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

##@ Build

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "unknown")
LDFLAGS = -X github.com/ksr/aerospike-ce-kubernetes-operator/internal/version.Version=$(VERSION)

# Cluster Manager image settings
CLUSTER_MANAGER_IMG ?= ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager
CLUSTER_MANAGER_KIND_CLUSTER ?= kind
CLUSTER_MANAGER_NAMESPACE ?= aerospike-operator
NO_CACHE ?=
BUILD_NO_CACHE_FLAG = $(if $(NO_CACHE),--no-cache,)

.PHONY: reload-cluster-manager
reload-cluster-manager: ## Build operator + cluster-manager images, load into Kind, and redeploy via helm upgrade (use NO_CACHE=1 to disable cache)
	@echo ">>> [1/5] Building images..."
	$(CONTAINER_TOOL) build $(BUILD_NO_CACHE_FLAG) --build-arg VERSION=$(VERSION) -t $(IMG) .
	$(CONTAINER_TOOL) build $(BUILD_NO_CACHE_FLAG) -t $(CLUSTER_MANAGER_IMG):latest aerospike-cluster-manager/
	@echo ">>> [2/5] Loading images into Kind cluster '$(CLUSTER_MANAGER_KIND_CLUSTER)'"
	$(CONTAINER_TOOL) save $(SAVE_FORMAT_FLAG) $(IMG) -o /tmp/acko-operator.tar
	$(KIND) load image-archive /tmp/acko-operator.tar --name $(CLUSTER_MANAGER_KIND_CLUSTER)
	$(CONTAINER_TOOL) save $(SAVE_FORMAT_FLAG) $(CLUSTER_MANAGER_IMG):latest -o /tmp/acko-cluster-manager.tar
	$(KIND) load image-archive /tmp/acko-cluster-manager.tar --name $(CLUSTER_MANAGER_KIND_CLUSTER)
	@rm -f /tmp/acko-operator.tar /tmp/acko-cluster-manager.tar
	@echo ">>> [3/5] Upgrading Helm release"
	helm upgrade aerospike-ce-kubernetes-operator ./charts/aerospike-ce-kubernetes-operator \
		-n $(CLUSTER_MANAGER_NAMESPACE) --reuse-values \
		--set ui.enabled=true --set image.tag=latest --set ui.image.tag=latest
	@echo ">>> [4/5] Restarting deployments"
	kubectl -n $(CLUSTER_MANAGER_NAMESPACE) rollout restart deployment
	@echo ">>> [5/5] Waiting for rollout to complete"
	kubectl -n $(CLUSTER_MANAGER_NAMESPACE) wait --for=condition=Available deployment --all --timeout=180s
	@echo ">>> Done. Operator and Cluster Manager reloaded successfully."

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -ldflags "$(LDFLAGS)" -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run -ldflags "$(LDFLAGS)" ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build --build-arg VERSION=$(VERSION) -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name aerospike-ce-kubernetes-operator-builder
	$(CONTAINER_TOOL) buildx use aerospike-ce-kubernetes-operator-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm aerospike-ce-kubernetes-operator-builder
	rm Dockerfile.cross

## Aliases (Podman terminology)
.PHONY: container-build container-push
container-build: docker-build  ## Alias for docker-build (Podman compatible)
container-push: docker-push    ## Alias for docker-push (Podman compatible)

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default > dist/install.yaml

HELM_PACKAGE_DIR ?= dist/charts
CHART_REGISTRY ?= oci://ghcr.io/aerospike-ce-ecosystem/charts

.PHONY: helm-sync-crds
helm-sync-crds: manifests ## Sync generated CRDs into aerospike-ce-kubernetes-operator-crds chart templates/ and refresh bundled tgz.
	bash hack/helm-sync-crds.sh
	helm dep update charts/aerospike-ce-kubernetes-operator

.PHONY: helm-lint
helm-lint: ## Lint both Helm charts (aerospike-ce-kubernetes-operator-crds and aerospike-ce-kubernetes-operator).
	helm lint charts/aerospike-ce-kubernetes-operator-crds
	helm lint charts/aerospike-ce-kubernetes-operator --set crds.install=false

.PHONY: helm-package
helm-package: helm-sync-crds ## Package both Helm charts into dist/charts/.
	mkdir -p $(HELM_PACKAGE_DIR)
	helm package charts/aerospike-ce-kubernetes-operator-crds --destination $(HELM_PACKAGE_DIR)
	helm dep update charts/aerospike-ce-kubernetes-operator
	helm package charts/aerospike-ce-kubernetes-operator --destination $(HELM_PACKAGE_DIR)
	@echo "Packaged charts:"
	@ls -1 $(HELM_PACKAGE_DIR)/*.tgz

.PHONY: helm-push
helm-push: helm-package ## Push packaged Helm charts to OCI registry (aerospike-ce-kubernetes-operator-crds first, then aerospike-ce-kubernetes-operator).
	helm push $(HELM_PACKAGE_DIR)/aerospike-ce-kubernetes-operator-crds-*.tgz $(CHART_REGISTRY)
	@# Use [0-9] prefix to avoid matching aerospike-ce-kubernetes-operator-crds-*.tgz again
	helm push $(HELM_PACKAGE_DIR)/aerospike-ce-kubernetes-operator-[0-9]*.tgz $(CHART_REGISTRY)

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply --server-side -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && "$(KUSTOMIZE)" edit set image controller=${IMG}
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" apply --server-side -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) -f -

##@ Documentation

.PHONY: docs-generate-api
docs-generate-api: ## Generate API reference from Go types
	bash docs/scripts/generate-api-reference.sh

.PHONY: docs-install
docs-install: ## Install docs dependencies
	cd docs && npm install

.PHONY: docs-build
docs-build: docs-generate-api docs-install ## Build docs site
	cd docs && npm run build

.PHONY: docs-serve
docs-serve: docs-install ## Start local docs dev server
	cd docs && npm run start

.PHONY: docs-serve-ko
docs-serve-ko: docs-install ## Start local docs dev server (Korean)
	cd docs && npm run start -- --locale ko

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
# KIND_PROVIDER sets the container provider for Kind.
KIND_PROVIDER ?= podman
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.8.1
CONTROLLER_TOOLS_VERSION ?= v0.20.1

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

GOLANGCI_LINT_VERSION ?= v2.8.0
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))
	@test -f .custom-gcl.yml && { \
		echo "Building custom golangci-lint with plugins..." && \
		$(GOLANGCI_LINT) custom --destination $(LOCALBIN) --name golangci-lint-custom && \
		mv -f $(LOCALBIN)/golangci-lint-custom $(GOLANGCI_LINT); \
	} || true

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
