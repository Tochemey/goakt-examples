# MIT License
#
# Copyright (c) 2022-2026 GoAkt Team
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.
#
# Makefile for goakt-ai example using Kind (Kubernetes in Docker)
#
# Prerequisites:
#   - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
#   - kubectl: https://kubernetes.io/docs/tasks/tools/
#   - Earthly: https://earthly.dev/get-earthly (for building the image)
#
# Quick start:
#   make cluster-create    # Create Kind cluster (one-time)
#   make deploy           # Build image, load into Kind, deploy
#   make port-forward     # Expose load balancer (run in background)
#   make query QUERY="your question here"
#   make cluster-down     # Tear down deployments
#
# Run from: goakt-ai/

.DEFAULT_GOAL := help

CLUSTER_NAME ?= goakt-ai
IMAGE_NAME ?= goakt-ai:dev
K8S_DIR := ./k8s
REPO_ROOT := ..

# Query target: required and optional args
# Required: QUERY - the question to submit
# Optional: ENDPOINT, PROVIDER, OPENAI_KEY, ANTHROPIC_KEY, GOOGLE_KEY, MISTRAL_KEY
ENDPOINT ?= http://localhost:8080
PROVIDER ?=

.PHONY: help
help:
	@echo "goakt-ai - Distributed AI Agents (Kind/Kubernetes)"
	@echo ""
	@echo "Targets:"
	@echo "  deploy              Build image, load into Kind, and deploy (recommended)"
	@echo "  cluster-create      Create a new Kind cluster"
	@echo "  cluster-recreate    Delete and recreate the cluster"
	@echo "  cluster-delete      Delete the Kind cluster"
	@echo "  image               Build the Docker image and load into Kind"
	@echo "  cluster-up          Deploy PostgreSQL, goakt-ai, nginx, tracing"
	@echo "  cluster-down        Remove all deployments"
	@echo "  status              Show cluster and pod status"
	@echo "  query               Submit a query (requires QUERY=...)"
	@echo "  set-llm-keys        Set API keys in cluster (run after deploy)"
	@echo "  test                Run CLI integration tests"
	@echo "  logs                Tail logs from goakt-ai pods"
	@echo "  port-forward        Forward nginx service to localhost:8080"
	@echo "  port-forward-jaeger Forward Jaeger UI to localhost:16686"
	@echo ""
	@echo "Query target (required):"
	@echo "  QUERY=\"your question\"   The question to submit to the AI agents"
	@echo ""
	@echo "Query target (optional):"
	@echo "  ENDPOINT=URL         Load balancer endpoint (default: http://localhost:8080)"
	@echo ""
	@echo "Set LLM keys (run after deploy, no Makefile editing):"
	@echo "  make set-llm-keys PROVIDER=openai OPENAI_API_KEY=sk-xxx"
	@echo "  make set-llm-keys PROVIDER=anthropic ANTHROPIC_API_KEY=sk-ant-xxx"
	@echo "  make set-llm-keys PROVIDER=google GOOGLE_API_KEY=xxx"
	@echo "  make set-llm-keys PROVIDER=mistral MISTRAL_API_KEY=xxx"
	@echo ""
	@echo "Examples:"
	@echo "  make set-llm-keys PROVIDER=openai OPENAI_API_KEY=sk-your-key"
	@echo "  make query QUERY=\"Summarize the benefits of distributed systems\""
	@echo "  make query QUERY=\"What is 15%% of 1000?\""
	@echo "  make query QUERY=\"...\" ENDPOINT=http://192.168.1.10:30080"

.PHONY: query
query:
ifndef QUERY
	@echo "Error: QUERY is required. Usage: make query QUERY=\"your question here\""
	@exit 1
endif
	@echo "Submitting query to $(ENDPOINT)..."
	@cd $(REPO_ROOT) && go run ./goakt-ai query "$(QUERY)" --endpoint $(ENDPOINT)

.PHONY: set-llm-keys
set-llm-keys:
ifndef OPENAI_API_KEY
ifndef ANTHROPIC_API_KEY
ifndef GOOGLE_API_KEY
ifndef MISTRAL_API_KEY
	@echo "Error: At least one API key is required."
	@echo "Usage: make set-llm-keys PROVIDER=openai OPENAI_API_KEY=sk-xxx"
	@echo "   or: make set-llm-keys PROVIDER=anthropic ANTHROPIC_API_KEY=sk-ant-xxx"
	@exit 1
endif
endif
endif
endif
	@echo "Updating llm-api-keys secret in cluster..."
	@kubectl create secret generic llm-api-keys \
		--from-literal=LLM_PROVIDER=$(or $(PROVIDER),openai) \
		$(if $(OPENAI_API_KEY),--from-literal=OPENAI_API_KEY=$(OPENAI_API_KEY)) \
		$(if $(ANTHROPIC_API_KEY),--from-literal=ANTHROPIC_API_KEY=$(ANTHROPIC_API_KEY)) \
		$(if $(GOOGLE_API_KEY),--from-literal=GOOGLE_API_KEY=$(GOOGLE_API_KEY)) \
		$(if $(MISTRAL_API_KEY),--from-literal=MISTRAL_API_KEY=$(MISTRAL_API_KEY)) \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "Restarting goakt-ai pods to pick up new keys..."
	@kubectl rollout restart statefulset/goakt-ai
	@echo "Waiting for all pods to be ready..."
	@kubectl rollout status statefulset/goakt-ai --timeout=120s
	@echo "Done. Run 'make query QUERY=\"...\"'"

.PHONY: cluster-create
cluster-create:
	@echo "Creating Kind cluster '$(CLUSTER_NAME)'..."
	kind create cluster --name $(CLUSTER_NAME) --config $(K8S_DIR)/kind-config.yaml --wait 2m
	@echo "Kind cluster created. Use 'make deploy' to deploy."

.PHONY: cluster-delete
cluster-delete:
	@echo "Deleting Kind cluster '$(CLUSTER_NAME)'..."
	kind delete cluster --name $(CLUSTER_NAME)
	@echo "Kind cluster deleted."

.PHONY: cluster-recreate
cluster-recreate: cluster-delete cluster-create
	@echo "Cluster recreated."

.PHONY: image
image:
	@echo "Building Docker image..."
	cd $(REPO_ROOT) && earthly +goakt-ai-image
	@echo "Loading image into Kind cluster..."
	kind load docker-image $(IMAGE_NAME) --name $(CLUSTER_NAME)
	@echo "Image loaded successfully."

.PHONY: deploy
deploy: image cluster-up

.PHONY: cluster-up
cluster-up:
	@echo "Deploying PostgreSQL..."
	kubectl apply -f $(K8S_DIR)/postgres-secret.yaml
	kubectl apply -f $(K8S_DIR)/postgres-configmap.yaml
	kubectl apply -f $(K8S_DIR)/postgres-deployment.yaml
	@echo "Waiting for PostgreSQL to be ready..."
	kubectl wait --for=condition=available --timeout=120s deployment/postgres
	@echo "Deploying tracing stack (Jaeger + OTEL Collector)..."
	kubectl apply -f $(K8S_DIR)/jaeger-deployment.yaml
	kubectl create configmap otel-collector-config --from-file=config.yaml=$(K8S_DIR)/otel-collector-config.yaml --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f $(K8S_DIR)/otel-collector-deployment.yaml
	@echo "Waiting for OTEL Collector to be ready..."
	kubectl wait --for=condition=available --timeout=60s deployment/otel-collector
	@echo "Creating LLM API keys secret (update with your keys: kubectl edit secret llm-api-keys)..."
	kubectl apply -f $(K8S_DIR)/llm-secret.yaml
	@echo "Deploying goakt-ai service..."
	kubectl apply -f $(K8S_DIR)/k8s.yaml
	@echo "Waiting for goakt-ai pods to be ready..."
	@for i in 0 1 2; do \
		echo "  Waiting for goakt-ai-$$i..."; \
		elapsed=0; \
		until [ "$$(kubectl get pod goakt-ai-$$i -o jsonpath='{.status.containerStatuses[?(@.name=="goakt-ai")].ready}' 2>/dev/null)" = "true" ]; do \
			sleep 5; elapsed=$$((elapsed+5)); \
			if [ $$elapsed -ge 300 ]; then \
				echo "  Timeout waiting for goakt-ai-$$i."; \
				kubectl get pod goakt-ai-$$i -o wide 2>/dev/null; \
				exit 1; \
			fi; \
		done; \
		echo "  goakt-ai-$$i is ready"; \
	done
	@echo "Deploying nginx load balancer..."
	kubectl apply -f $(K8S_DIR)/nginx-config.yaml
	kubectl apply -f $(K8S_DIR)/nginx-deployment.yaml
	kubectl apply -f $(K8S_DIR)/nginx-service.yaml
	@echo "All components deployed. Run 'make port-forward' then 'make query QUERY=\"...\"'"

.PHONY: cluster-down
cluster-down:
	@echo "Removing deployments..."
	kubectl delete -f $(K8S_DIR)/nginx-service.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-config.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/k8s.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/otel-collector-deployment.yaml --ignore-not-found
	kubectl delete configmap otel-collector-config --ignore-not-found
	kubectl delete -f $(K8S_DIR)/jaeger-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-configmap.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-secret.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/llm-secret.yaml --ignore-not-found
	@echo "Deployments removed."

.PHONY: status
status:
	@echo "=== Cluster Info ==="
	kubectl cluster-info --context kind-$(CLUSTER_NAME)
	@echo ""
	@echo "=== Pods ==="
	kubectl get pods -o wide
	@echo ""
	@echo "=== Services ==="
	kubectl get svc

.PHONY: logs
logs:
	kubectl logs -l app.kubernetes.io/name=goakt-ai -f --tail=100

.PHONY: port-forward
port-forward:
	@echo "Waiting for nginx to be ready..."
	kubectl wait --for=condition=available --timeout=60s deployment/nginx
	@echo "Forwarding nginx (port 80) to localhost:8080..."
	@echo "CLI endpoint: http://localhost:8080"
	kubectl port-forward service/nginx 8080:80

.PHONY: port-forward-jaeger
port-forward-jaeger:
	@echo "Forwarding Jaeger UI to localhost:16686..."
	@echo "Jaeger UI at http://localhost:16686"
	kubectl port-forward service/jaeger-collector 16686:16686

.PHONY: test
test:
	@echo "Running CLI integration tests..."
	@chmod +x scripts/test-cli.sh 2>/dev/null || true
	@sh ./scripts/test-cli.sh
