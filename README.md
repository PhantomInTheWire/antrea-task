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

I went with `tcpdump host <PodIP>` running on the host via `hostNetwork: true`. No runtime dependencies, simple code, just works. It's not perfectly isolated but good enough for this task.

## What I Considered

Some other approaches I tried or thought about:

**Before this** (it's still in older commits on this repo) I was doing /proc scanning + nsenter (PID lookup by cgroup scan), basically just scan /proc/*/cgroup to find the container ID, get the PID, then nsenter -n and run tcpdump in the pod netns.

It was more accurate pod-specific capture but it felt very brittle by comparison to the above mentioned approach and seemed "overengineered".

**CRI lookup + nsenter** was another option - talk to container runtime over CRI socket, get container status, extract PID, then nsenter -n. It was clean and felt like the "correct" way to do things but for a screening task this seemed even more "overengineered" than /proc scanning, added a lot more code and complexity.

**Containerd API** to get PID, then nsenter - this would make the solution dependent on the containerd environment so I didn't pick it, but it would have been a correct and minimal solution.

## Deliverables

| Deliverable | Location |
|-------------|----------|
| Go source code | `antrea-capture/` (main.go, controller.go)
| Dockerfile & Makefile | `Dockerfile`, `Makefile` |
| DaemonSet manifest | `deployment/daemonset.yaml` |
| Test Pod manifest | `test-pod.yaml` |
| Pod describe, pods list, capture files, pcap file, tcpdump output | `outputs/` (`pod-describe.txt`, `pods.txt`, `capture-files.txt`, `capture-file.pcap`, `capture-output.txt`) |
| README | `README.md` |
