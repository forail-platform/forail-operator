# forge-operator — build & deploy targets.
#
# All commands assume Go 1.23+ and kubectl on PATH (or running inside the
# k8s-m1 vagrant VM where they're installed).

IMAGE ?= krlex/forge-operator:latest

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
		--forge-url=$$FORGE_URL \
		--forge-user=$$FORGE_USER \
		--forge-password=$$FORGE_PASSWORD \
		--forge-insecure-skip-verify
