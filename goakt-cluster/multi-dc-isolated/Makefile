# Makefile for multi-dc-isolated example
# Uses TWO separate Kind clusters to simulate real network isolation between DCs.
#
# Prerequisites:
#   - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation
#   - kubectl: https://kubernetes.io/docs/tasks/tools/
#   - Docker: https://docs.docker.com/get-docker/
#   - Earthly: https://earthly.dev/get-earthly (for building the image)
#
# Quick start:
#   make setup      # Create Docker network, NATS, and 2 Kind clusters
#   make deploy     # Build image and deploy to both DCs
#   make test       # Run cross-DC integration tests
#   make teardown   # Clean up everything

.DEFAULT_GOAL := help

REPO_ROOT := ../..
IMAGE_NAME := accounts:dev-multi-dc-isolated

.PHONY: help
help:
	@echo "multi-dc-isolated GoAkt Example — Two Separate Kind Clusters"
	@echo ""
	@echo "Targets:"
	@echo "  setup          Create Docker network, NATS container, and 2 Kind clusters"
	@echo "  deploy         Build image and deploy to both DCs"
	@echo "  deploy-dc1     Deploy to DC-1 only"
	@echo "  deploy-dc2     Deploy to DC-2 only"
	@echo "  teardown       Delete everything (clusters, NATS, Docker network)"
	@echo "  image          Build the Docker image"
	@echo "  status         Show pod status in both DCs"
	@echo "  test           Run cross-DC API integration tests"
	@echo "  logs-dc1       Tail logs from DC-1 pods"
	@echo "  logs-dc2       Tail logs from DC-2 pods"
	@echo "  k9s-dc1        Launch k9s for DC-1"
	@echo "  k9s-dc2        Launch k9s for DC-2"

.PHONY: setup
setup:
	@chmod +x scripts/*.sh
	@./scripts/setup.sh

.PHONY: image
image:
	@echo "Building Docker image..."
	cd $(REPO_ROOT) && earthly +multi-dc-isolated-image
	@echo "Image built: $(IMAGE_NAME)"

.PHONY: deploy
deploy: image deploy-dc1 deploy-dc2
	@echo ""
	@echo "Both DCs deployed. Run 'make test' to verify cross-DC communication."

.PHONY: deploy-dc1
deploy-dc1:
	@./scripts/deploy-dc.sh dc1

.PHONY: deploy-dc2
deploy-dc2:
	@./scripts/deploy-dc.sh dc2

.PHONY: teardown
teardown:
	@./scripts/teardown.sh

.PHONY: status
status:
	@echo "=== DC-1 (kind-dc1) ==="
	kubectl get pods -o wide --context kind-dc1 2>/dev/null || echo "  Cluster not running"
	@echo ""
	@echo "=== DC-2 (kind-dc2) ==="
	kubectl get pods -o wide --context kind-dc2 2>/dev/null || echo "  Cluster not running"

.PHONY: test
test:
	@chmod +x scripts/test-api.sh
	@./scripts/test-api.sh

.PHONY: logs-dc1
logs-dc1:
	kubectl logs -l dc=dc-1 -f --tail=100 --context kind-dc1

.PHONY: logs-dc2
logs-dc2:
	kubectl logs -l dc=dc-2 -f --tail=100 --context kind-dc2

.PHONY: k9s-dc1
k9s-dc1:
	@command -v k9s >/dev/null 2>&1 || { echo "k9s not found. Install with: brew install k9s"; exit 1; }
	k9s --context kind-dc1

.PHONY: k9s-dc2
k9s-dc2:
	@command -v k9s >/dev/null 2>&1 || { echo "k9s not found. Install with: brew install k9s"; exit 1; }
	k9s --context kind-dc2
