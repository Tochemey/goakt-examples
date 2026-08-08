# Makefile for the k8s example using Kind (Kubernetes in Docker)
#
# Prerequisites:
#   - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
#   - kubectl: https://kubernetes.io/docs/tasks/tools/
#   - Earthly: https://earthly.dev/get-earthly (for building the image)
#   - grpcurl (proto mode only): go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
#
# Quick start (CBOR / HTTP — default):
#   make cluster-create
#   make deploy
#   make test
#
# Full protobuf stack:
#   make deploy CODEC=proto
#   make test CODEC=proto
#
# Run from: goakt-cluster/k8s/

.DEFAULT_GOAL := help

CLUSTER_NAME ?= goakt-k8s
IMAGE_NAME ?= accounts:dev-k8s
# Exclusive stack: cbor (HTTP/JSON + CBOR remoting) or proto (Connect/gRPC + protobuf remoting).
# All pods must use the same value.
CODEC ?= cbor
K8S_DIR := ./deploy
REPO_ROOT := ../..

ifeq ($(CODEC),proto)
NGINX_CONFIG := nginx-config-proto.yaml
TEST_SCRIPT := scripts/test-api-proto.sh
TEST_RESILIENCE_SCRIPT := scripts/test-resilience-proto.sh
else
NGINX_CONFIG := nginx-config.yaml
TEST_SCRIPT := scripts/test-api.sh
TEST_RESILIENCE_SCRIPT := scripts/test-resilience.sh
endif

.PHONY: help
help:
	@echo "k8s GoAkt Example - Kind (Kubernetes in Docker)"
	@echo ""
	@echo "CODEC=$(CODEC)  (cbor | proto) — exclusive process-wide stack"
	@echo ""
	@echo "Targets:"
	@echo "  deploy           Build image, load into Kind, and deploy (recommended)"
	@echo "  cluster-create   Create a new Kind cluster"
	@echo "  cluster-recreate Delete and recreate the cluster (needed after kind-config changes)"
	@echo "  cluster-delete   Delete the Kind cluster"
	@echo "  image            Build the Docker image and load into Kind"
	@echo "  cluster-up       Deploy PostgreSQL, accounts, nginx (respects CODEC)"
	@echo "  cluster-down     Remove all deployments"
	@echo "  status           Show cluster and pod status"
	@echo "  test             Run API integration tests for the selected CODEC"
	@echo "  test-resilience  Create accounts, verify, kill a random node, re-verify"
	@echo "  logs             Tail logs from accounts pods"
	@echo "  port-forward     Forward nginx service to localhost:8080"
	@echo "  port-forward-jaeger  Forward Jaeger UI to localhost:16686 (traces)"
	@echo "  dashboard        Access Kubernetes dashboard (workloads, pods)"
	@echo "  dashboard-install Install Kubernetes dashboard (one-time)"

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
	cd $(REPO_ROOT) && earthly +k8s-image
	@echo "Loading image into Kind cluster..."
	kind load docker-image $(IMAGE_NAME) --name $(CLUSTER_NAME)
	@echo "Image loaded successfully."

.PHONY: deploy
deploy: image cluster-up

.PHONY: cluster-up
cluster-up:
	@echo "Deploying with CODEC=$(CODEC)..."
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
	@echo "Deploying accounts service..."
	kubectl apply -f $(K8S_DIR)/k8s.yaml
	kubectl set env statefulset/accounts CODEC=$(CODEC)
	@echo "Waiting for accounts pods to be ready..."
	@for i in 0 1 2; do \
		echo "  Waiting for accounts-$$i..."; \
		elapsed=0; \
		until [ "$$(kubectl get pod accounts-$$i -o jsonpath='{.status.containerStatuses[?(@.name=="accounts")].ready}' 2>/dev/null)" = "true" ]; do \
			sleep 5; elapsed=$$((elapsed+5)); \
			if [ $$elapsed -ge 300 ]; then \
				echo "  Timeout waiting for accounts-$$i. Pod status:"; \
				kubectl get pod accounts-$$i -o wide 2>/dev/null; \
				kubectl describe pod accounts-$$i 2>/dev/null | tail -20; \
				exit 1; \
			fi; \
		done; \
		echo "  accounts-$$i is ready"; \
	done
	@echo "Deploying nginx ingress ($(NGINX_CONFIG))..."
	kubectl apply -f $(K8S_DIR)/$(NGINX_CONFIG)
	kubectl apply -f $(K8S_DIR)/nginx-deployment.yaml
	kubectl apply -f $(K8S_DIR)/nginx-service.yaml
	@echo "All components deployed (CODEC=$(CODEC)). Run 'make port-forward' to access the API."
	@echo "Run 'make port-forward-jaeger' to view traces in Jaeger UI."

.PHONY: cluster-down
cluster-down:
	@echo "Removing deployments..."
	kubectl delete -f $(K8S_DIR)/nginx-service.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-config.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-config-proto.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/k8s.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/otel-collector-deployment.yaml --ignore-not-found
	kubectl delete configmap otel-collector-config --ignore-not-found
	kubectl delete -f $(K8S_DIR)/jaeger-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-configmap.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/postgres-secret.yaml --ignore-not-found
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
	@echo ""
	@echo "=== CODEC on accounts StatefulSet ==="
	@kubectl get statefulset accounts -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="CODEC")].value}' 2>/dev/null; echo

.PHONY: logs
logs:
	kubectl logs -l app.kubernetes.io/name=accounts -f --tail=100

.PHONY: port-forward
port-forward:
	@echo "Waiting for nginx to be ready..."
	kubectl wait --for=condition=available --timeout=60s deployment/nginx
	@echo "Forwarding nginx (port 80) to localhost:8080..."
	@echo "API available at http://localhost:8080"
	@if [ "$(CODEC)" = "cbor" ]; then echo "Swagger UI at http://localhost:8080/docs"; fi
	kubectl port-forward service/nginx 8080:80

.PHONY: port-forward-jaeger
port-forward-jaeger:
	@echo "Forwarding Jaeger UI to localhost:16686..."
	@echo "Jaeger UI at http://localhost:16686"
	kubectl port-forward service/jaeger-collector 16686:16686

.PHONY: test
test:
	@echo "Running API tests (CODEC=$(CODEC))..."
	@chmod +x $(TEST_SCRIPT) 2>/dev/null || true
	@sh ./$(TEST_SCRIPT) || true

.PHONY: test-resilience
test-resilience:
	@echo "Running resilience test (CODEC=$(CODEC))..."
	@chmod +x $(TEST_RESILIENCE_SCRIPT) 2>/dev/null || true
	@bash ./$(TEST_RESILIENCE_SCRIPT) || true

.PHONY: dashboard-install
dashboard-install:
	@echo "Installing Kubernetes dashboard..."
	kubectl apply -f https://raw.githubusercontent.com/kubernetes/dashboard/v2.7.0/aio/deploy/recommended.yaml
	@echo "Creating dashboard admin user..."
	-kubectl create serviceaccount dashboard-admin -n kubernetes-dashboard 2>/dev/null || true
	-kubectl create clusterrolebinding dashboard-admin --clusterrole=cluster-admin --serviceaccount=kubernetes-dashboard:dashboard-admin 2>/dev/null || true
	@echo "Waiting for dashboard to be ready..."
	kubectl wait --for=condition=available --timeout=120s deployment/kubernetes-dashboard -n kubernetes-dashboard
	@echo "Dashboard installed. Run 'make dashboard' to access it."

.PHONY: dashboard
dashboard: dashboard-install
	@echo ""
	@echo "=========================================="
	@echo "Kubernetes Dashboard"
	@echo "=========================================="
	@echo "URL: http://localhost:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/"
	@echo ""
	@echo "Login token (copy and paste at the dashboard login):"
	@kubectl -n kubernetes-dashboard create token dashboard-admin 2>/dev/null || \
	  kubectl -n kubernetes-dashboard get secret $$(kubectl -n kubernetes-dashboard get sa dashboard-admin -o jsonpath='{.secrets[0].name}') -o jsonpath='{.data.token}' 2>/dev/null | base64 -d; echo
	@echo ""
	@echo "Starting kubectl proxy (Ctrl+C to stop)..."
	kubectl proxy
