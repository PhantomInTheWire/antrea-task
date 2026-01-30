# Antrea Packet Capture Controller Makefile

BINARY_NAME := antrea-capture
IMAGE_NAME := antrea-capture
KIND_CLUSTER := antrea-capture
GO_DIR := antrea-capture
BUILD_DIR := build
KIND_CONFIG := kind-config.yaml
DEPLOY_DIR := deployment
LDFLAGS := -w -s

.PHONY: all build verify cluster-setup cleanup deploy test e2e help

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

cleanup:
	@echo "=== Cleaning up everything ==="
	@echo "Removing test pod..."
	@kubectl delete pod test-traffic-pod -n default --ignore-not-found 2>/dev/null || true
	@echo "Undeploying from cluster..."
	@kubectl delete -f $(DEPLOY_DIR) --ignore-not-found 2>/dev/null || true
	@echo "Uninstalling Antrea..."
	@if helm list -n kube-system | grep -q antrea 2>/dev/null; then \
		helm uninstall antrea -n kube-system; \
	else \
		echo "Antrea not installed, skipping"; \
	fi
	@echo "Deleting Kind cluster..."
	@if kind get clusters | grep -q "^$(KIND_CLUSTER)$$" 2>/dev/null; then \
		kind delete cluster --name $(KIND_CLUSTER); \
	else \
		echo "Kind cluster does not exist, skipping"; \
	fi
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(GO_DIR)/$(BINARY_NAME)
	@echo "=== Cleanup complete ==="

build:
	@mkdir -p $(BUILD_DIR)
	cd $(GO_DIR) && go mod tidy
	cd $(GO_DIR) && go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"



verify:
	@echo "=== Running code quality checks (fmt,vet, golangci-lint) ==="
	@cd $(GO_DIR) && go fmt ./...
	@cd $(GO_DIR) && go vet ./...
	@cd $(GO_DIR) && (golangci-lint run ./... 2>/dev/null || echo "Warning: golangci-lint not installed or failed")
	@echo "=== Verification complete ==="

docker-build:
	@echo "=== Building Docker image ==="
	docker build -t $(IMAGE_NAME):latest .
	@echo "=== Docker image built: $(IMAGE_NAME):latest ==="

kind-load:
	@echo "=== Loading image into Kind cluster ==="
	kind load docker-image $(IMAGE_NAME):latest --name $(KIND_CLUSTER)
	@echo "=== Image loaded into Kind cluster ==="

deploy: docker-build kind-load
	@echo "=== Deploying to cluster ==="
	kubectl apply -f $(DEPLOY_DIR)/rbac.yaml
	kubectl apply -f $(DEPLOY_DIR)/daemonset.yaml
	@echo "=== Waiting for DaemonSet to be ready ==="
	kubectl rollout status daemonset/antrea-capture -n antrea-capture --timeout=120s
	@echo "=== Deployment complete ==="



logs:
	@echo "=== Showing logs (ctrl+c to exit) ==="
	kubectl logs -l app=antrea-capture -n antrea-capture -f

test: test-deploy test-annotate test-collect
	@echo ""
	@echo "=== TEST COMPLETE ==="
	@echo "All outputs saved to outputs/ directory"
	@echo "Test pod 'test-traffic-pod' is still running for manual inspection"
	@echo "Run 'make cleanup' to remove all resources when done"

test-deploy:
	@echo "=== Deploying test pod ==="
	kubectl apply -f test-pod.yaml
	@echo "=== Waiting for test pod to be ready ==="
	kubectl wait --for=condition=ready pod/test-traffic-pod -n default --timeout=60s

test-annotate:
	@echo "=== Annotating test pod with tcpdump.antrea.io=5 ==="
	kubectl annotate pod test-traffic-pod -n default tcpdump.antrea.io="5"
	@echo "=== Waiting 5 seconds for traffic capture ==="
	@sleep 5

test-collect:
	@echo "=== Collecting test outputs ==="
	@mkdir -p outputs
	@echo "Saving pod description..."
	kubectl describe pod test-traffic-pod -n default > outputs/pod-describe.txt
	@echo "Saving all pods list..."
	kubectl get pods -A > outputs/pods.txt
	@echo "Finding capture pod on same node..."
	@TEST_NODE=$$(kubectl get pod test-traffic-pod -n default -o jsonpath='{.spec.nodeName}') && \
	CAPTURE_POD=$$(kubectl get pods -n antrea-capture -l app=antrea-capture -o jsonpath="{.items[?(@.spec.nodeName=='$$TEST_NODE')].metadata.name}") && \
	echo "Capture pod: $$CAPTURE_POD on node $$TEST_NODE" && \
	echo "Saving capture files listing..." && \
	(kubectl exec $$CAPTURE_POD -n antrea-capture -- sh -c "ls -l /capture-*.pcap* 2>/dev/null" > outputs/capture-files.txt 2>&1 && echo "Files found") || echo "No capture files found" > outputs/capture-files.txt
	@echo "Extracting pcap file..."
	@TEST_NODE=$$(kubectl get pod test-traffic-pod -n default -o jsonpath='{.spec.nodeName}') && \
	CAPTURE_POD=$$(kubectl get pods -n antrea-capture -l app=antrea-capture -o jsonpath="{.items[?(@.spec.nodeName=='$$TEST_NODE')].metadata.name}") && \
	PCAP_FILE=$$(kubectl exec $$CAPTURE_POD -n antrea-capture -- sh -c "ls /capture-*.pcap* 2>/dev/null | head -1" 2>/dev/null) && \
	if [ -n "$$PCAP_FILE" ]; then \
		echo "Found pcap file: $$PCAP_FILE" && \
		kubectl cp antrea-capture/$$CAPTURE_POD:$$PCAP_FILE outputs/capture-file.pcap && \
		echo "Pcap file saved to outputs/capture-file.pcap" && \
		echo "Converting pcap to text output..." && \
		kubectl exec $$CAPTURE_POD -n antrea-capture -- tcpdump -r $$PCAP_FILE -n > outputs/capture-output.txt 2>&1 || true && \
		echo "Text output saved to outputs/capture-output.txt"; \
	else \
		echo "No pcap files found" && \
		echo "No pcap files found" > outputs/capture-output.txt; \
	fi



e2e: cluster-setup deploy test
	@echo ""
	@echo "=== E2E TEST COMPLETE ==="
	@echo "All deliverables saved to outputs/ directory"
	@echo "Run 'make cleanup' to remove all resources when done"

help:
	@echo "Antrea Packet Capture Controller"
	@echo ""
	@echo "Cluster Management:"
	@echo "  make cluster-setup   - Create Kind cluster and install Antrea"
	@echo "  make cleanup         - Remove all resources (cluster, deployment, build artifacts)"
	@echo ""
	@echo "Build:"
	@echo "  make build           - Build the Go binary to build/"
	@echo "  make docker-build    - Build Docker image"
	@echo "  make verify          - Run fmt, vet, and lint"
	@echo ""
	@echo "Deployment:"
	@echo "  make deploy          - Build image and deploy to cluster"
	@echo "  make logs            - Show controller logs"
	@echo ""
	@echo "Testing:"
	@echo "  make test            - Full test: deploy, annotate, collect outputs"
	@echo "  make e2e             - Complete E2E: setup cluster, deploy, and test"
	@echo ""
	@echo "Help:"
	@echo "  make help            - Show this help message"
