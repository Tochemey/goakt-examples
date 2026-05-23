# goakt-scrabble Makefile.
#
# Run `make help` for a one-screen summary of every target.

SHELL := bash

# ---------------------------------------------------------------- variables

BIN      := bin/goakt-scrabble
BIN_DIR  := bin

TS_SRC   := web/main.ts
JS_OUT   := web/main.js
TS_CFG   := tsconfig.json

# Image + cluster configuration. Override on the command line, e.g.
#   make image-build IMAGE=goakt-scrabble:v1.2.3
IMAGE              ?= goakt-scrabble:latest
KIND_CLUSTER       ?= goakt-scrabble
KIND_CONFIG        ?= kind/cluster.yaml
K8S_NAMESPACE      ?= scrabble
K8S_MANIFESTS      ?= k8s
INGRESS_VERSION    ?= controller-v1.11.2
INGRESS_MANIFEST   ?= https://raw.githubusercontent.com/kubernetes/ingress-nginx/$(INGRESS_VERSION)/deploy/static/provider/kind/deploy.yaml

# ---------------------------------------------------------------- help

.DEFAULT_GOAL := help

##@ General

.PHONY: help
help: ## Show this help.
	@printf "\n\033[1mgoakt-scrabble\033[0m — make targets\n"
	@awk 'BEGIN {FS = ":.*?## "} \
	  /^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
	  /^[a-zA-Z0-9_.-]+:.*?## / {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' \
	  $(MAKEFILE_LIST)
	@printf "\nQuick starts:\n"
	@printf "  Single-process:   \033[36mmake build\033[0m          (compile only; clustering requires kind)\n"
	@printf "  3-node cluster:   \033[36mmake k8s-up\033[0m         (http://localhost via nginx)\n\n"

##@ Local development

.PHONY: web build clean

web: $(JS_OUT) ## Compile web/main.ts → web/main.js.

$(JS_OUT): $(TS_SRC) $(TS_CFG)
	npx --package=typescript@5.6 -y -- tsc -p .

build: web ## Build the binary (single static Go binary with web + dict embedded).
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) .

clean: ## Remove the build output + generated JS.
	rm -rf $(BIN_DIR) $(JS_OUT)

##@ Docker image

.PHONY: image-build image-push

image-build: ## Build the goakt-scrabble Docker image (distroless multi-stage).
	@docker info >/dev/null 2>&1 || { echo "Docker is not running."; exit 1; }
	docker build -f Dockerfile -t $(IMAGE) ..

image-push: ## Push the image to a remote registry (set IMAGE=<registry>/<name>:<tag>).
	docker push $(IMAGE)

##@ Kubernetes (kind + nginx ingress)

.PHONY: kind-up kind-down kind-ingress image-load deploy undeploy k8s-up k8s-down k8s-logs k8s-status k8s-port-forward

kind-up: ## Create the local kind cluster (1 control-plane + 3 workers) and install nginx ingress.
	@command -v kind >/dev/null 2>&1 || { echo "kind is not installed: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { echo "kubectl is not installed."; exit 1; }
	kind create cluster --name $(KIND_CLUSTER) --config $(KIND_CONFIG)
	$(MAKE) kind-ingress

kind-ingress: ## (Re-)install the NGINX ingress controller into the existing kind cluster.
	kubectl apply -f $(INGRESS_MANIFEST)
	@echo "Waiting for ingress-nginx admission webhook jobs to complete..."
	@until kubectl -n ingress-nginx get job/ingress-nginx-admission-create >/dev/null 2>&1; do sleep 2; done
	kubectl -n ingress-nginx wait --for=condition=complete --timeout=180s \
		job/ingress-nginx-admission-create \
		job/ingress-nginx-admission-patch
	@echo "Waiting for ingress controller deployment to become available..."
	@until kubectl -n ingress-nginx get deployment/ingress-nginx-controller >/dev/null 2>&1; do sleep 2; done
	kubectl -n ingress-nginx rollout status deployment/ingress-nginx-controller --timeout=300s

kind-down: ## Destroy the local kind cluster.
	kind delete cluster --name $(KIND_CLUSTER)

image-load: image-build ## Build the image and load it into every kind node.
	kind load docker-image $(IMAGE) --name $(KIND_CLUSTER)

deploy: ## Apply the Kubernetes manifests (namespace + service + StatefulSet + ingress).
	kubectl apply -k $(K8S_MANIFESTS)
	@echo "Waiting for scrabble pods to become ready..."
	kubectl -n $(K8S_NAMESPACE) rollout status statefulset/scrabble --timeout=180s

undeploy: ## Remove the Kubernetes manifests (keep the cluster running).
	kubectl delete -k $(K8S_MANIFESTS) --ignore-not-found

k8s-up: kind-up image-load deploy ## One-shot: create kind cluster, install ingress, build+load image, deploy app.
	@printf "\n\033[32m✓ Cluster ready.\033[0m\n"
	@printf "  Browser:          http://localhost\n"
	@printf "  Live pod status:  make k8s-status\n"
	@printf "  Stream logs:      make k8s-logs\n"
	@printf "  Tear down:        make k8s-down\n\n"

k8s-down: ## Tear it all down: undeploy + delete the kind cluster.
	-$(MAKE) undeploy
	$(MAKE) kind-down

k8s-logs: ## Tail logs from all three scrabble pods, prefixed with pod name.
	kubectl logs -n $(K8S_NAMESPACE) -l app=scrabble --tail=200 --prefix --follow

k8s-status: ## Show pod / service / ingress / endpoint status for the scrabble namespace.
	@printf "\n\033[1mPods\033[0m\n"
	@kubectl get pods -n $(K8S_NAMESPACE) -o wide
	@printf "\n\033[1mServices\033[0m\n"
	@kubectl get svc -n $(K8S_NAMESPACE)
	@printf "\n\033[1mEndpoints (peer DNS)\033[0m\n"
	@kubectl get endpoints -n $(K8S_NAMESPACE) scrabble-headless
	@printf "\n\033[1mIngress\033[0m\n"
	@kubectl get ingress -n $(K8S_NAMESPACE)
	@printf "\n"

k8s-port-forward: ## Forward localhost:8080 to a single pod (alternative if :80 is in use).
	kubectl -n $(K8S_NAMESPACE) port-forward statefulset/scrabble 8080:8080
