.DEFAULT_GOAL := all

# Username for image references - priority: BPFMAN_CATALOG_QUAY_USER > USER > fallback.
QUAY_USER ?= $(or $(BPFMAN_CATALOG_QUAY_USER),$(USER),$(shell echo $$USER))
IMAGE ?= quay.io/$(QUAY_USER)/bpfman-operator-catalog:latest
BUILD_STREAM ?= y-stream
BASE_IMAGE ?= registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.20
BUILDVERSION ?= 4.20.0
COMMIT ?= $(shell git rev-parse HEAD)

# Image building tool - defaults to podman, override with OCI_BIN.
OCI_BIN_PATH := $(shell which podman)
OCI_BIN ?= $(shell if [ -n "${OCI_BIN_PATH}" ]; then basename ${OCI_BIN_PATH}; else echo "podman"; fi)
export OCI_BIN

LOCALBIN ?= $(shell pwd)/bin

# Go build configuration - tags required for operator-registry library integration.
export GOFLAGS := -tags=json1,containers_image_openpgp
export CGO_ENABLED := 0

# Tool versions (for dependency scanning).
# OPM_VERSION: for local go install (v1.53+ has go install issues, keep at v1.52.0)
# OPM_CONTAINER_VERSION: for container image in Dockerfile.generate-catalog (can be newer)
OPM_VERSION := v1.52.0
OPM_CONTAINER_VERSION := v1.60.0
YQ_VERSION  := v4.35.2

# Pattern rule: install Go tools.
$(LOCALBIN)/%:
	@echo "Downloading $*..."
	@GOBIN=$(LOCALBIN) go install $(TOOL_PKG_$*)@$(TOOL_VERSION_$*)

# Tool package and version mappings.
TOOL_PKG_opm := github.com/operator-framework/operator-registry/cmd/opm
TOOL_VERSION_opm := $(OPM_VERSION)
TOOL_PKG_yq := github.com/mikefarah/yq/v4
TOOL_VERSION_yq := $(YQ_VERSION)

# OPM image for container-based catalog generation.
OPM_IMAGE := quay.io/operator-framework/opm:$(OPM_CONTAINER_VERSION)

OPM_BIN ?= $(LOCALBIN)/opm
YQ_BIN  ?= $(LOCALBIN)/yq

.PHONY: download-prerequisites
download-prerequisites: $(OPM_BIN) $(YQ_BIN) ## Download all required tools.

# Template files.
TEMPLATES := $(wildcard templates/*.yaml)
CATALOGS := $(patsubst templates/%.yaml,auto-generated/catalog/%.yaml,$(TEMPLATES))

auto-generated/catalog:
	mkdir -p $@

##@ Build

# Pattern rule: generate catalog from template.
auto-generated/catalog/%.yaml: templates/%.yaml | auto-generated/catalog $(OPM_BIN)
	$(OPM_BIN) alpha render-template basic --migrate-level=bundle-object-to-csv-metadata -o yaml $< > $@

.PHONY: generate-catalogs
generate-catalogs: $(CATALOGS) ## Generate catalogs from templates using local OPM.

# Alternative catalog generation using containerised OPM. Useful for
# using newer OPM versions without local build issues (opm v1.53+ has
# go install problems). Requires Podman with BuildKit secret mounting
# for registry authentication.
.PHONY: generate-catalogs-container
generate-catalogs-container: | auto-generated/catalog ## Generate catalogs using OPM container (requires Podman auth).
	@if [ -z "$(OCI_BIN_PATH)" ]; then \
		echo "ERROR: Podman is required but not found in PATH" ; \
		echo "Please install podman first" ; \
		exit 1 ; \
	fi
	@if [ ! -f "$${XDG_RUNTIME_DIR}/containers/auth.json" ]; then \
		echo "ERROR: No Podman registry authentication found." ; \
		echo "Please login to registry first:" ; \
		echo "  podman login registry.redhat.io" ; \
		exit 1 ; \
	fi
	@echo "Generating catalogs using container approach..."
	@for template in $(TEMPLATES); do \
		template_name=$$(basename $$template) ; \
		catalog="auto-generated/catalog/$$template_name" ; \
		echo "Processing $$template_name..." ; \
		podman build --quiet \
			--secret id=dockerconfig,src=$${XDG_RUNTIME_DIR}/containers/auth.json \
			--build-arg TEMPLATE_FILE=$$template_name \
			-f Dockerfile.generate-catalog -t bpfman-catalog-cli-temp:$$template_name . && \
		podman create --name bpfman-catalog-cli-temp-$$template_name bpfman-catalog-cli-temp:$$template_name >/dev/null && \
		podman cp bpfman-catalog-cli-temp-$$template_name:/catalog.yaml $$catalog && \
		podman rm bpfman-catalog-cli-temp-$$template_name >/dev/null && \
		podman rmi bpfman-catalog-cli-temp:$$template_name >/dev/null || exit 1 ; \
	done
	@echo "Catalogs generated successfully"

.PHONY: build-image
build-image: ## Build catalog container image.
	$(OCI_BIN) build --build-arg INDEX_FILE="./auto-generated/catalog/$(BUILD_STREAM).yaml" --build-arg BASE_IMAGE="$(BASE_IMAGE)" --build-arg COMMIT="$(COMMIT)" --build-arg BUILDVERSION="$(BUILDVERSION)" -t $(IMAGE) -f Dockerfile .

.PHONY: push-image
push-image: ## Push catalog container image.
	$(OCI_BIN) push ${IMAGE}

##@ Deployment

.PHONY: deploy
deploy: $(YQ_BIN) ## Deploy catalog to OpenShift cluster.
	$(YQ_BIN) '.spec.image="$(IMAGE)"' ./catalog-source.yaml | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Remove catalog from OpenShift cluster.
	kubectl delete -f ./catalog-source.yaml

.PHONY: purge-bpfman-catalog-cli-resources
purge-bpfman-catalog-cli-resources: ## Remove all bpfman-catalog-cli resources from current cluster.
	@echo "Removing all bpfman-catalog-cli resources from cluster..."
	kubectl delete all -l app.kubernetes.io/created-by=bpfman-catalog-cli --all-namespaces --ignore-not-found
	kubectl delete catalogsource,operatorgroup,subscription -l app.kubernetes.io/created-by=bpfman-catalog-cli --all-namespaces --ignore-not-found
	kubectl delete imagedigestmirrorset -l app.kubernetes.io/created-by=bpfman-catalog-cli --ignore-not-found
	kubectl delete namespace -l app.kubernetes.io/created-by=bpfman-catalog-cli --ignore-not-found

##@ Cleanup

.PHONY: clean-catalogs
clean-catalogs: ## Remove generated catalogs.
	rm -rf auto-generated/catalog/*.yaml

.PHONY: clean-bin
clean-bin: ## Remove bin directory (tools and built binaries).
	rm -rf $(LOCALBIN)

.PHONY: clean
clean: clean-catalogs clean-bin ## Remove all generated files and binaries.

##@ CLI Tool

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: test
test: ## Run go unit tests.
	go test ./...

.PHONY: build-cli
build-cli: fmt vet test $(OPM_BIN) ## Build the bpfman-catalog CLI tool.
	go build -o $(LOCALBIN)/bpfman-catalog ./cmd/bpfman-catalog

# Define test-cli-run macro for running CLI tests
# $1 = workflow name
# $2 = output directory
# $3 = CLI command and arguments
define test-cli-run
	@echo "Testing $(1)..."
	@rm -rf $(2)
	PATH="$(LOCALBIN):$$PATH" $(LOCALBIN)/bpfman-catalog $(3) --output-dir $(2)
	@echo "Artefacts generated in $(2)"
	@ls -la $(2)/
endef

.PHONY: test-cli
test-cli: test-cli-bundle test-cli-yaml test-cli-image ## Test the CLI with all three workflow examples.

.PHONY: test-cli-bundle
test-cli-bundle: test-cli-bundle-opm-library test-cli-bundle-opm-binary ## Test workflow 1: Build catalog from bundle (both OPM methods).

.PHONY: test-cli-bundle-opm-library
test-cli-bundle-opm-library: build-cli ## Test workflow 1a: Build catalog from bundle (OPM library mode).
	$(call test-cli-run,workflow 1a: Build catalog from bundle (OPM library mode),/tmp/bpfman-catalog-cli-test-bundle-opm-library,prepare-catalog-build-from-bundle quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest)
	echo "Building catalog image to verify artefacts..."
	make -C /tmp/bpfman-catalog-cli-test-bundle-opm-library build-catalog-image IMAGE=bpfman-catalog-cli-test:opm-library
	echo "Image built successfully, cleaning up..."
	$(OCI_BIN) rmi bpfman-catalog-cli-test:opm-library

.PHONY: test-cli-bundle-opm-binary
test-cli-bundle-opm-binary: build-cli ## Test workflow 1b: Build catalog from bundle (OPM binary mode).
	$(call test-cli-run,workflow 1b: Build catalog from bundle (OPM binary mode),/tmp/bpfman-catalog-cli-test-bundle-opm-binary,prepare-catalog-build-from-bundle quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest --opm-bin $(LOCALBIN)/opm)
	echo "Building catalog image to verify artefacts..."
	make -C /tmp/bpfman-catalog-cli-test-bundle-opm-binary build-catalog-image IMAGE=bpfman-catalog-cli-test:opm-binary
	echo "Image built successfully, cleaning up..."
	$(OCI_BIN) rmi bpfman-catalog-cli-test:opm-binary

.PHONY: test-cli-yaml
test-cli-yaml: build-cli generate-catalogs ## Test workflow 2: Build catalog from catalog.yaml.
	$(call test-cli-run,workflow 2: Build catalog from catalog.yaml,/tmp/bpfman-catalog-cli-test-yaml,prepare-catalog-build-from-yaml auto-generated/catalog/y-stream.yaml)
	echo "Building catalog image to verify artefacts..."
	make -C /tmp/bpfman-catalog-cli-test-yaml build-catalog-image IMAGE=bpfman-catalog-cli-test:yaml
	echo "Image built successfully, cleaning up..."
	$(OCI_BIN) rmi bpfman-catalog-cli-test:yaml

.PHONY: test-cli-image
test-cli-image: build-cli ## Test workflow 3: Deploy existing catalog image.
	$(call test-cli-run,workflow 3: Deploy existing catalog image,/tmp/bpfman-catalog-cli-test-manifests,prepare-catalog-deployment-from-image quay.io/redhat-user-workloads/ocp-bpfman-tenant/catalog-ystream:latest)
	@ls -la /tmp/bpfman-catalog-cli-test-manifests/catalog/

##@ General

.PHONY: all
all: help ## Default target: display help.

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  %-28s %s\n", $$1, $$2 } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) }' \
		$(MAKEFILE_LIST)
