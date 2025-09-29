.DEFAULT_GOAL := all

IMAGE ?= quay.io/$(USER)/bpfman-operator-catalog:latest
BUILD_STREAM ?= dev

# Image building tool - podman only for catalog generation
OCI_BIN_PATH := $(shell which podman)
OCI_BIN ?= $(shell basename ${OCI_BIN_PATH})
export OCI_BIN

LOCALBIN ?= $(shell pwd)/bin

# OPM configuration - v1.52 for local install (v1.54 has go.mod replace issues)
OPM_VERSION := v1.52.0
OPM_IMAGE := quay.io/operator-framework/opm:$(OPM_VERSION)
YQ_VERSION  := v4.35.2

OPM ?= $(LOCALBIN)/opm
YQ  ?= $(LOCALBIN)/yq

# Define go-install macro for installing Go tools.
# $1 = tool binary path
# $2 = tool package
# $3 = tool version
define go-install
	@[ -f $(1) ] || { \
		echo "Downloading $(notdir $(1)) $(3)..." ; \
		GOBIN=$(LOCALBIN) go install $(2)@$(3) ; \
	}
endef

.PHONY: opm
opm: $(OPM) ## Download opm locally if necessary.
$(OPM):
	$(call go-install,$(OPM),github.com/operator-framework/operator-registry/cmd/opm,$(OPM_VERSION))

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ):
	$(call go-install,$(YQ),github.com/mikefarah/yq/v4,$(YQ_VERSION))

.PHONY: prereqs
prereqs: opm yq

# Template files
TEMPLATES := $(wildcard templates/*.yaml)
CATALOGS := $(patsubst templates/%.yaml,auto-generated/catalog/%.yaml,$(TEMPLATES))

auto-generated/catalog:
	mkdir -p $@

##@ Build

.PHONY: generate-catalogs
generate-catalogs: prereqs ## Generate catalogs from templates using local OPM.
	@for template in $(TEMPLATES); do \
		catalog="auto-generated/catalog/$$(basename $$template)" ; \
		echo "Generating $$catalog (local)..." ; \
		$(OPM) alpha render-template basic --migrate-level=bundle-object-to-csv-metadata -o yaml $$template > $$catalog ; \
	done

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
		$(OCI_BIN) build --quiet \
			--secret id=dockerconfig,src=$${XDG_RUNTIME_DIR}/containers/auth.json \
			--build-arg TEMPLATE_FILE=$$template_name \
			-f Dockerfile.generate -t temp-catalog:$$template_name . && \
		$(OCI_BIN) create --name temp-$$template_name temp-catalog:$$template_name >/dev/null && \
		$(OCI_BIN) cp temp-$$template_name:/catalog.yaml $$catalog && \
		$(OCI_BIN) rm temp-$$template_name >/dev/null && \
		$(OCI_BIN) rmi temp-catalog:$$template_name >/dev/null || exit 1 ; \
	done
	@echo "Catalogs generated successfully"

.PHONY: build-image
build-image: ## Build catalog container image.
	$(OCI_BIN) build --build-arg INDEX_FILE="./auto-generated/catalog/$(BUILD_STREAM).yaml" -t $(IMAGE) -f Dockerfile .

.PHONY: push-image
push-image: ## Push catalog container image.
	$(OCI_BIN) push ${IMAGE}

##@ Deployment

.PHONY: deploy
deploy: yq ## Deploy catalog to OpenShift cluster.
	$(YQ) '.spec.image="$(IMAGE)"' ./catalog-source.yaml | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Remove catalog from OpenShift cluster.
	kubectl delete -f ./catalog-source.yaml

##@ Cleanup

.PHONY: clean-generated-catalogs
clean-generated-catalogs: ## Remove generated catalogs.
	rm -rf auto-generated/catalog/*.yaml

.PHONY: clean-tools
clean-tools: ## Remove downloaded tools.
	rm -rf $(LOCALBIN)

.PHONY: clean
clean: clean-generated-catalogs clean-tools ## Remove all generated files and tools.

##@ General

.PHONY: all
all: help ## Default target: display help.

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  %-28s %s\n", $$1, $$2 } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) }' \
		$(MAKEFILE_LIST)
