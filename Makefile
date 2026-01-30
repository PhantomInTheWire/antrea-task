BINARY := antrea-capture
IMAGE := antrea-capture
CLUSTER := antrea-capture
GO_DIR := antrea-capture
BUILD_DIR := build
DEPLOY_DIR := deployment
LDFLAGS := -w -s

.PHONY: build docker-build cluster-setup deploy test e2e cleanup help

help:
	@echo "Targets: build, docker-build, cluster-setup, deploy, test, e2e, cleanup"

build:
	@mkdir -p $(BUILD_DIR)
	@cd $(GO_DIR) && go mod tidy && go build -ldflags "$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY) .
	@echo "Built: $(BUILD_DIR)/$(BINARY)"

docker-build:
	@docker build -t $(IMAGE):latest .

cluster-setup:
	@kind get clusters | grep -q $(CLUSTER) || kind create cluster --config kind-config.yaml --name $(CLUSTER)
	@helm repo list | grep -q antrea || helm repo add antrea https://charts.antrea.io
	@helm repo update
	@helm list -n kube-system | grep -q antrea || (helm install antrea antrea/antrea -n kube-system --create-namespace && kubectl wait --for=condition=ready pod -l app=antrea -n kube-system --timeout=120s)
	@kubectl get nodes

deploy: docker-build
	@kind load docker-image $(IMAGE):latest --name $(CLUSTER)
	@kubectl apply -f $(DEPLOY_DIR)/rbac.yaml && kubectl apply -f $(DEPLOY_DIR)/daemonset.yaml
	@kubectl rollout status daemonset/$(BINARY) -n $(BINARY) --timeout=120s

test:
	@kubectl apply -f test-pod.yaml
	@kubectl wait --for=condition=ready pod/test-traffic-pod -n default --timeout=60s
	@kubectl annotate pod test-traffic-pod -n default tcpdump.antrea.io="5"
	@sleep 5
	@mkdir -p outputs
	@kubectl describe pod test-traffic-pod -n default > outputs/pod-describe.txt
	@kubectl get pods -A > outputs/pods.txt
	@TEST_NODE=$$(kubectl get pod test-traffic-pod -n default -o jsonpath='{.spec.nodeName}') && \
	CAPTURE_POD=$$(kubectl get pods -n $(BINARY) -l app=$(BINARY) -o jsonpath="{.items[?(@.spec.nodeName=='$$TEST_NODE')].metadata.name}") && \
	kubectl exec $$CAPTURE_POD -n $(BINARY) -- sh -c 'ls -l /capture-* 2>/dev/null || echo "No capture files"' > outputs/capture-files.txt && \
	PCAP_FILE=$$(kubectl exec $$CAPTURE_POD -n $(BINARY) -- sh -c 'ls /capture-*.pcap* 2>/dev/null | head -1') && \
	if [ -n "$$PCAP_FILE" ]; then \
		kubectl cp $(BINARY)/$$CAPTURE_POD:$$PCAP_FILE outputs/capture-file.pcap && \
		kubectl exec $$CAPTURE_POD -n $(BINARY) -- tcpdump -r $$PCAP_FILE -n > outputs/capture-output.txt 2>&1 || true; \
	else \
		echo "No pcap files found" > outputs/capture-output.txt; \
	fi

e2e: cluster-setup deploy test
	@echo "E2E complete. Run 'make cleanup' to remove resources."

cleanup:
	@kubectl delete pod test-traffic-pod -n default --ignore-not-found 2>/dev/null || true
	@kubectl delete -f $(DEPLOY_DIR) --ignore-not-found 2>/dev/null || true
	@helm list -n kube-system 2>/dev/null | grep -q antrea && helm uninstall antrea -n kube-system || true
	@kind get clusters | grep -q $(CLUSTER) && kind delete cluster --name $(CLUSTER) || true
	@rm -rf $(BUILD_DIR) $(GO_DIR)/$(BINARY)
