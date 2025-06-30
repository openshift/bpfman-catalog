
IMAGE ?= quay.io/$(USER)/bpfman-operator-catalog:latest
BUILD_STREAM ?= y-stream
OCI_BIN_PATH := $(shell which docker 2>/dev/null || which podman)
OCI_BIN ?= $(shell basename ${OCI_BIN_PATH})

.PHONY: prereqs
prereqs:
	go install github.com/operator-framework/operator-registry/cmd/opm@v1.51.0
	go install github.com/mikefarah/yq/v4@v4.35.2

.PHONY: generate
generate: prereqs
	rm -f ./auto-generated/catalog/*
	rm -f ./auto-generated/legacy-catalog/*
	for i in $(shell ls ./templates/); do \
		opm alpha render-template basic --migrate-level=bundle-object-to-csv-metadata  -o yaml ./templates/$$i > ./auto-generated/catalog/$$i; \
		opm alpha render-template basic -o yaml ./templates/$$i > ./auto-generated/legacy-catalog/$$i; \
		sed -i -e 's#quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-zstream#registry.redhat.io/bpfman/bpfman-operator-bundle#g' ./auto-generated/catalog/$$i; \
		sed -i -e 's#quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-zstream#registry.redhat.io/bpfman/bpfman-operator-bundle#g' ./auto-generated/legacy-catalog/$$i; \
	done

.PHONY: build-image
build-image:
	$(OCI_BIN) build --build-arg INDEX_FILE="./auto-generated/catalog/$(BUILD_STREAM).yaml" -t $(IMAGE) -f upstream.Dockerfile .

.PHONY: push-image
push-image:
	$(OCI_BIN) push ${IMAGE}

.PHONY: deploy
deploy:
	yq '.spec.image="$(IMAGE)"' ./catalog-source.yaml | kubectl apply -f -

.PHONY: undeploy
undeploy:
	kubectl delete -f ./catalog-source.yaml
