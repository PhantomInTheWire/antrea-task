# Antrea Packet Capture Controller

Poor-man's version of the Antrea's PacketCapture feature. Kubernetes controller running as a DaemonSet that performs packet captures on demand.

## Quick Start

```bash
# Complete E2E setup (cluster + deploy + test)
make e2e

# Or step by step:
make cluster-setup  # Create Kind cluster + install Antrea
make deploy         # Build image and deploy DaemonSet
make test           # Deploy test pod, annotate, collect outputs
make cleanup        # Teardown everything
```

## Deliverables

| Deliverable | Location |
|-------------|----------|
| Go source code | `antrea-capture/` (main.go, controller.go)
| Dockerfile & Makefile | `Dockerfile`, `Makefile` |
| DaemonSet manifest | `deployment/daemonset.yaml` |
| Test Pod manifest | `test-pod.yaml` |
| Pod describe, pods list, capture files, pcap file, tcpdump output | `outputs/` (`pod-describe.txt`, `pods.txt`, `capture-files.txt`, `capture-file.pcap`, `capture-output.txt`) |
| README | `README.md` |
