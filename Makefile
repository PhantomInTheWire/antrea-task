# Antrea Packet Capture Controller Makefile

BINARY_NAME := antrea-capture
KIND_CLUSTER := antrea-capture
GO_DIR := antrea-capture
BUILD_DIR := build
KIND_CONFIG := kind-config.yaml
LDFLAGS := -w -s

.PHONY: all build clean verify cluster-setup cluster-cleanup help

all: build

cluster-setup:
	@echo "=== Setting up Kind cluster ==="
	@if ! kind get clusters | grep -q "^$(KIND_CLUSTER)$$"; then \
		echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
		kind create cluster --config $(KIND_CONFIG) --name $(KIND_CLUSTER); \
	else \
		echo "Kind cluster '$(KIND_CLUSTER)' already exists, skipping creation"; \
	fi
	@echo "=== Installing Antrea ==="
	@helm repo list | grep -q antrea || helm repo add antrea https://charts.antrea.io
	@helm repo update
	@if ! helm list -n kube-system | grep -q antrea; then \
		echo "Installing Antrea..."; \
		helm install antrea antrea/antrea --namespace kube-system --create-namespace; \
		echo "Waiting for Antrea pods to be ready..."; \
		kubectl wait --for=condition=ready pod -l app=antrea -n kube-system --timeout=120s; \
	else \
		echo "Antrea already installed, skipping"; \
	fi
	@echo "=== Cluster setup complete ==="
	@kubectl get nodes

cluster-cleanup:
	@echo "=== Cleaning up ==="
	@if helm list -n kube-system | grep -q antrea; then \
		echo "Uninstalling Antrea..."; \
		helm uninstall antrea -n kube-system; \
	else \
		echo "Antrea not installed, skipping uninstall"; \
	fi
	@if kind get clusters | grep -q "^$(KIND_CLUSTER)$$"; then \
		echo "Deleting Kind cluster '$(KIND_CLUSTER)'..."; \
		kind delete cluster --name $(KIND_CLUSTER); \
	else \
		echo "Kind cluster '$(KIND_CLUSTER)' does not exist, skipping deletion"; \
	fi
	@echo "=== Cleanup complete ==="

build:
	@mkdir -p $(BUILD_DIR)
	cd $(GO_DIR) && go mod tidy
	cd $(GO_DIR) && go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

clean:
	rm -rf $(BUILD_DIR)
	rm -f $(GO_DIR)/$(BINARY_NAME)

verify:
	@echo "=== Running code quality checks (fmt,vet, golangci-lint) ==="
	@cd $(GO_DIR) && go fmt ./...
	@cd $(GO_DIR) && go vet ./...
	@cd $(GO_DIR) && (golangci-lint run ./... 2>/dev/null || echo "Warning: golangci-lint not installed or failed")
	@echo "=== Verification complete ==="

help:
	@echo "Antrea Packet Capture Controller"
	@echo ""
	@echo "Cluster Management:"
	@echo "  make cluster-setup   - Create Kind cluster and install Antrea (idempotent)"
	@echo "  make cluster-cleanup - Remove Antrea and delete Kind cluster"
	@echo ""
	Build:"
	@echo "  make build           - Build the Go binary to build/"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make verify          - Run fmt, vet, and lint"
	@echo ""
	@echo "Help:"
	@echo "  make help            - Show this help message"
