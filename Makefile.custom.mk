
# Image URL to use all building/pushing image targets
IMG ?= docker.io/giantswarm/aws-network-topology-operator:dev

# Substitute colon with space - this creates a list.
# Word selects the n-th element of the list
IMAGE_REPO = $(word 1,$(subst :, ,$(IMG)))
IMAGE_TAG = $(word 2,$(subst :, ,$(IMG)))

CLUSTER ?= acceptance
MANAGEMENT_CLUSTER_NAME ?= test-mc
MANAGEMENT_CLUSTER_NAMESPACE ?= test


##@ Development

.PHONY: lint-imports
lint-imports: goimports ## Run go vet against code.
	./scripts/check-imports.sh

.PHONY: create-acceptance-cluster
create-acceptance-cluster: kind
	CLUSTER=$(CLUSTER) IMG=$(IMG) MANAGEMENT_CLUSTER_NAMESPACE=$(MANAGEMENT_CLUSTER_NAMESPACE) ./scripts/ensure-kind-cluster.sh

.PHONY: install-cluster-api
install-cluster-api: clusterctl
	$(CLUSTERCTL) init --kubeconfig "$(KUBECONFIG)" --infrastructure=aws --wait-providers || true

.PHONY: deploy-acceptance-cluster
deploy-acceptance-cluster: docker-build create-acceptance-cluster install-cluster-api deploy

.PHONY: clear-envtest-cache
clear-envtest-cache: ## Clear envtest ports cache
	rm -rf "$(HOME)/.cache/kubebuilder-envtest/"

.PHONY: test-unit
test-unit: ginkgo generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GINKGO) -p --nodes 8 --cover -r -randomize-all --randomize-suites --skip-package=tests ./...

.PHONY: test-integration
test-integration: ginkgo envtest ## Run integration tests
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GINKGO) -p --nodes 8 -r -randomize-all --randomize-suites --slow-spec-threshold "30s" tests/integration/

.PHONY: test-acceptance
test-acceptance: KUBECONFIG=$(HOME)/.kube/$(CLUSTER).yml
test-acceptance: ginkgo deploy-acceptance-cluster ## Run acceptance testst
	KUBECONFIG="$(KUBECONFIG)" \
	MANAGEMENT_CLUSTER_NAME="$(MANAGEMENT_CLUSTER_NAME)" \
	MANAGEMENT_CLUSTER_NAMESPACE="$(MANAGEMENT_CLUSTER_NAMESPACE)" \
	$(GINKGO) -r -randomize-all --randomize-suites --slow-spec-threshold "30s" tests/acceptance

.PHONY: test-all
test-all: lint lint-imports test-unit test-integration test-acceptance ## Run all tests and litner

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: render
render: architect
	mkdir -p $(shell pwd)/helm/rendered
	cp -r $(shell pwd)/helm/aws-network-topology-operator $(shell pwd)/helm/rendered/
	$(ARCHITECT) helm template --dir $(shell pwd)/helm/rendered/aws-network-topology-operator

.PHONY: deploy
deploy: manifests render ensure-deploy-envs ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	KUBECONFIG=$(KUBECONFIG) helm upgrade --install \
		--namespace giantswarm \
		--set image.tag=$(IMAGE_TAG) \
		--set managementClusterName=$(MANAGEMENT_CLUSTER_NAME) \
		--set managementClusterNamespace=$(MANAGEMENT_CLUSTER_NAMESPACE) \
		--wait \
		aws-network-topology-operator helm/rendered/aws-network-topology-operator

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s  specified in ~/.kube/config.
	KUBECONFIG="$(KUBECONFIG)" helm uninstall \
		--namespace giantswarm \
		aws-network-topology-operator

##@ App

ensure-schema-gen:
	@helm schema-gen --help &>/dev/null || helm plugin install https://github.com/mihaisee/helm-schema-gen.git

.PHONY: schema-gen
schema-gen: ensure-schema-gen ## Generates the values schema file
	@cd helm/aws-network-topology-operator && helm schema-gen values.yaml > values.schema.json

##@ Build Dependencies

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.0)

ENVTEST = $(shell pwd)/bin/setup-envtest
.PHONY: envtest
envtest: clear-envtest-cache ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

GINKGO = $(shell pwd)/bin/ginkgo
.PHONY: ginkgo
ginkgo: ## Download ginkgo locally if necessary.
	$(call go-get-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo@latest)

ARCHITECT = $(shell pwd)/bin/architect
.PHONY: architect
architect: ## Download architect locally if necessary.
	$(call go-get-tool,$(ARCHITECT),github.com/giantswarm/architect@latest)

KIND = $(shell pwd)/bin/kind
.PHONY: kind
kind: ## Download kind locally if necessary.
	$(call go-get-tool,$(KIND),sigs.k8s.io/kind@latest)

GOIMPORTS = $(shell pwd)/bin/goimports
.PHONY: goimports
goimports: ## Download kind locally if necessary.
	$(call go-get-tool,$(GOIMPORTS),golang.org/x/tools/cmd/goimports@latest)

CLUSTERCTL = $(shell pwd)/bin/clusterctl
.PHONY: clusterctl
clusterctl: ## Download clusterctl locally if necessary.
	$(eval LATEST_RELEASE = $(shell curl -s https://api.github.com/repos/kubernetes-sigs/cluster-api/releases/latest | jq -r '.tag_name'))
	curl -sL "https://github.com/kubernetes-sigs/cluster-api/releases/download/$(LATEST_RELEASE)/clusterctl-linux-amd64" -o $(CLUSTERCTL)
	chmod +x $(CLUSTERCTL)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
