# forail-operator — build & deploy targets.
#
# All commands assume Go 1.23+ and kubectl on PATH (or running inside the
# k8s-m1 vagrant VM where they're installed).

IMAGE ?= ghcr.io/forail-platform/forail-operator:2026.06.0

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: generate
generate:
	# Regenerates zz_generated.deepcopy.go for the API types.
	# Requires `controller-gen` (install: go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5).
	controller-gen object paths="./api/..."

.PHONY: manifests
manifests:
	# Re-generate the CRD YAML from kubebuilder markers in api/v1alpha1.
	controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/manager ./cmd

.PHONY: test
test: envtest-assets
	KUBEBUILDER_ASSETS="$(shell setup-envtest use 1.30 -p path)" go test ./... -v -count=1 -timeout=120s

# Install setup-envtest tool and download envtest binaries (apiserver,
# etcd, kubectl) for the targeted Kubernetes version. Stored under
# ~/.local/share/kubebuilder-envtest by default.
.PHONY: envtest-assets
envtest-assets:
	@command -v setup-envtest >/dev/null || go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
	setup-envtest use 1.30 -p path >/dev/null

.PHONY: vet
vet:
	go vet ./...

.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE) .

.PHONY: install-crd
install-crd:
	kubectl apply -f config/crd/bases/

.PHONY: deploy
deploy: install-crd
	kubectl apply -f config/rbac/rbac.yaml
	kubectl apply -f config/manager/manager.yaml

.PHONY: undeploy
undeploy:
	-kubectl delete -f config/manager/manager.yaml
	-kubectl delete -f config/rbac/rbac.yaml
	-kubectl delete -f config/crd/bases/

.PHONY: run
run:
	# Run the operator out-of-cluster against the kubeconfig in $KUBECONFIG.
	go run ./cmd \
		--forail-url=$$FORAIL_URL \
		--forail-user=$$FORAIL_USER \
		--forail-password=$$FORAIL_PASSWORD \
		--forail-insecure-skip-verify

# --- OLM bundle + catalog ---
#
# Bundle wraps the operator manifests (CSV + CRDs) into an OCI image that
# an OLM CatalogSource can serve. Catalog is a file-based catalog (FBC)
# image listing one or more bundle versions.

VERSION     ?= 1.0.0
BUNDLE_IMG  ?= ghcr.io/forail-platform/forail-operator-bundle:v$(VERSION)
CATALOG_IMG ?= ghcr.io/forail-platform/forail-operator-catalog:v$(VERSION)

.PHONY: bundle
bundle: manifests
	# Refresh bundle/manifests with the latest CRDs and the CSV base.
	rm -rf bundle/manifests
	mkdir -p bundle/manifests
	cp config/crd/bases/*.yaml bundle/manifests/
	cp config/manifests/bases/forail-operator.clusterserviceversion.yaml bundle/manifests/

.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push:
	docker push $(BUNDLE_IMG)

# Validate bundle layout (requires operator-sdk).
.PHONY: bundle-validate
bundle-validate:
	@command -v operator-sdk >/dev/null || { echo "operator-sdk required"; exit 1; }
	operator-sdk bundle validate ./bundle --select-optional name=operatorhub

# Build a file-based catalog image referencing the bundle. For a
# multi-version catalog, re-run with --bundles "$(BUNDLE_IMG_v1.0.0),$(BUNDLE_IMG_v1.1.0)".
.PHONY: catalog-build
catalog-build:
	@command -v opm >/dev/null || { echo "opm required (https://github.com/operator-framework/operator-registry)"; exit 1; }
	opm index add --container-tool docker \
		--bundles $(BUNDLE_IMG) \
		--tag $(CATALOG_IMG)

.PHONY: catalog-push
catalog-push:
	docker push $(CATALOG_IMG)
