# Makefile for k8s example using Kind (Kubernetes in Docker)
#
# Prerequisites:
#   - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
#   - kubectl: https://kubernetes.io/docs/tasks/tools/
#   - Earthly: https://earthly.dev/get-earthly
#   - grpcurl: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest (for make test)
#
# Quick start:
#   make cluster-create    # Create Kind cluster (one-time)
#   make deploy            # Build and deploy
#   make port-forward      # Expose API (in another terminal)
#   make test              # Run gRPC tests
#
# Run from: goakt-actors-cluster/k8s/

.DEFAULT_GOAL := help

CLUSTER_NAME ?= goakt-k8s
IMAGE_NAME ?= accounts:dev-k8s
K8S_DIR := ./k8s
REPO_ROOT := ../..

.PHONY: help
help:
	@echo "k8s GoAkt Example - Kind (Kubernetes in Docker)"
	@echo ""
	@echo "Targets:"
	@echo "  deploy            Build image, load into Kind, and deploy"
	@echo "  cluster-create    Create a new Kind cluster"
	@echo "  cluster-delete    Delete the Kind cluster"
	@echo "  image             Build Docker image and load into Kind"
	@echo "  cluster-up        Deploy accounts and nginx"
	@echo "  cluster-down      Remove all deployments"
	@echo "  status            Show cluster and pod status"
	@echo "  port-forward      Forward nginx to localhost:8080 (gRPC)"
	@echo "  dashboard         Access Kubernetes dashboard"
	@echo "  dashboard-install Install Kubernetes dashboard (one-time)"
	@echo "  logs              Tail logs from accounts pods"
	@echo "  test              Run gRPC integration tests"
	@echo "  test-resilience   Create accounts, verify, kill a random node, re-verify"

.PHONY: cluster-create
cluster-create:
	@echo "Creating Kind cluster '$(CLUSTER_NAME)'..."
	kind create cluster --name $(CLUSTER_NAME) --wait 2m
	@echo "Kind cluster created. Use 'make deploy' to deploy."

.PHONY: cluster-delete
cluster-delete:
	@echo "Deleting Kind cluster '$(CLUSTER_NAME)'..."
	kind delete cluster --name $(CLUSTER_NAME)
	@echo "Kind cluster deleted."

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
	@echo "Deploying accounts service..."
	kubectl apply -f $(K8S_DIR)/k8s.yaml
	@echo "Waiting for accounts StatefulSet to be ready..."
	kubectl wait --for=condition=ready --timeout=120s pod -l app.kubernetes.io/name=accounts
	@echo "Deploying nginx..."
	kubectl apply -f $(K8S_DIR)/nginx-config.yaml
	kubectl apply -f $(K8S_DIR)/nginx-deployment.yaml
	kubectl apply -f $(K8S_DIR)/nginx-service.yaml
	@echo "Deployed. Run 'make port-forward' to access the gRPC API."

.PHONY: cluster-down
cluster-down:
	@echo "Removing deployments..."
	kubectl delete -f $(K8S_DIR)/nginx-service.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-config.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/k8s.yaml --ignore-not-found
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
	kubectl logs -l app.kubernetes.io/name=accounts -f --tail=100

.PHONY: port-forward
port-forward:
	@echo "Forwarding nginx (port 80) to localhost:8080..."
	@echo "gRPC API available at localhost:8080"
	@echo "Use grpcurl: grpcurl -plaintext -import-path ../../protos -proto sample/service.proto localhost:8080 list"
	kubectl port-forward service/nginx 8080:80

.PHONY: test
test:
	@echo "Running gRPC API tests..."
	@REPO_ROOT="$(REPO_ROOT)" sh ./scripts/test-api.sh || true

.PHONY: test-resilience
test-resilience:
	@echo "Running resilience test (create → verify → kill node → re-verify)..."
	@chmod +x scripts/test-resilience.sh 2>/dev/null || true
	@REPO_ROOT="$(REPO_ROOT)" bash ./scripts/test-resilience.sh || true

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
