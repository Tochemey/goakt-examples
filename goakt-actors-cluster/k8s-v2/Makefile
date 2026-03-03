# Makefile for k8s-v2 example using Kind (Kubernetes in Docker)
#
# Prerequisites:
#   - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
#   - kubectl: https://kubernetes.io/docs/tasks/tools/
#   - Earthly: https://earthly.dev/get-earthly (for building the image)
#
# Quick start:
#   make cluster-create    # Create Kind cluster (one-time)
#   make image             # Build and load image into Kind
#   make cluster-up        # Deploy all components
#   make test              # Run API tests
#   make cluster-down      # Tear down deployments
#
# Run from: goakt-actors-cluster/k8s-v2/

.DEFAULT_GOAL := help

CLUSTER_NAME ?= goakt-k8s-v2
IMAGE_NAME ?= accounts:dev-k8s-v2
K8S_DIR := ./k8s
REPO_ROOT := ../..

.PHONY: help
help:
	@echo "k8s-v2 GoAkt Example - Kind (Kubernetes in Docker)"
	@echo ""
	@echo "Targets:"
	@echo "  deploy           Build image, load into Kind, and deploy (recommended)"
	@echo "  cluster-create   Create a new Kind cluster"
	@echo "  cluster-delete   Delete the Kind cluster"
	@echo "  image            Build the Docker image and load into Kind"
	@echo "  cluster-up       Deploy PostgreSQL, accounts, and nginx"
	@echo "  cluster-down     Remove all deployments"
	@echo "  status           Show cluster and pod status"
	@echo "  test             Run API integration tests"
	@echo "  logs             Tail logs from accounts pods"
	@echo "  port-forward     Forward nginx service to localhost:8080"
	@echo "  dashboard        Access Kubernetes dashboard (workloads, pods)"
	@echo "  dashboard-install Install Kubernetes dashboard (one-time)"

.PHONY: cluster-create
cluster-create:
	@echo "Creating Kind cluster '$(CLUSTER_NAME)'..."
	kind create cluster --name $(CLUSTER_NAME) --wait 2m
	@echo "Kind cluster created. Use 'make cluster-up' to deploy."

.PHONY: cluster-delete
cluster-delete:
	@echo "Deleting Kind cluster '$(CLUSTER_NAME)'..."
	kind delete cluster --name $(CLUSTER_NAME)
	@echo "Kind cluster deleted."

.PHONY: image
image:
	@echo "Building Docker image..."
	cd $(REPO_ROOT) && earthly +k8s-v2-image
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
	@echo "Deploying accounts service..."
	kubectl apply -f $(K8S_DIR)/k8s.yaml
	@echo "Waiting for accounts StatefulSet to be ready..."
	kubectl wait --for=condition=ready --timeout=120s pod -l app.kubernetes.io/name=accounts
	@echo "Deploying nginx ingress..."
	kubectl apply -f $(K8S_DIR)/nginx-config.yaml
	kubectl apply -f $(K8S_DIR)/nginx-deployment.yaml
	kubectl apply -f $(K8S_DIR)/nginx-service.yaml
	@echo "All components deployed. Run 'make port-forward' to access the API."

.PHONY: cluster-down
cluster-down:
	@echo "Removing deployments..."
	kubectl delete -f $(K8S_DIR)/nginx-service.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-deployment.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/nginx-config.yaml --ignore-not-found
	kubectl delete -f $(K8S_DIR)/k8s.yaml --ignore-not-found
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

.PHONY: logs
logs:
	kubectl logs -l app.kubernetes.io/name=accounts -f --tail=100

.PHONY: port-forward
port-forward:
	@echo "Forwarding nginx (port 80) to localhost:8080..."
	@echo "API available at http://localhost:8080"
	@echo "Swagger UI at http://localhost:8080/docs"
	kubectl port-forward service/nginx 8080:80

.PHONY: test
test:
	@echo "Running API tests..."
	@chmod +x scripts/test-api.sh 2>/dev/null || true
	@sh ./scripts/test-api.sh || true

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
