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

## Architecture

I went with **CRI lookup + nsenter**: as in talk to the container runtime over the CRI socket, get the container's PID from the status, then `nsenter -n -t <pid>` to run tcpdump inside the pod's network namespace.

This gives perfect isolation (capturing exactly what the pod sees) without the brittleness of scanning `/proc/*/cgroup` manually. The CRI client library handles the gRPC communication with containerd/cri-o, so it's both robust and runtime-agnostic.

## What I Considered

**Before this**, I had a simpler version using `tcpdump host <PodIP>` running on the host via `hostNetwork: true`. No runtime dependencies, simple code, just works. But it's not perfectly isolated - you're still capturing from the host's perspective, and you miss pod-internal traffic.

I also had  **proc scanning** implemented at one point - basically scan `/proc/*/cgroup` to find the container ID, get the PID, then `nsenter -n`. It worked but felt very brittle.

**Containerd API directly** was another option - bypass CRI and talk containerd gRPC directly. Would work but makes the solution containerd-specific.

## Deliverables

| Deliverable | Location |
|-------------|----------|
| Go source code | `antrea-capture/` (main.go, controller.go)
| Dockerfile & Makefile | `Dockerfile`, `Makefile` |
| DaemonSet manifest | `deployment/daemonset.yaml` |
| Test Pod manifest | `test-pod.yaml` |
| Pod describe, pods list, capture files, pcap file, tcpdump output | `outputs/` (`pod-describe.txt`, `pods.txt`, `capture-files.txt`, `capture-file.pcap`, `capture-output.txt`) |
| README | `README.md` |
