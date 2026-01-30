# Antrea Packet Capture Controller

Poor-man's version of the Antrea's PacketCapture feature. Kubernetes controller running as a DaemonSet that performs packet captures on demand.

## How It Works

- Watches Pods running on the same Node
- When a Pod is annotated with `tcpdump.antrea.io: "<N>"`, starts tcpdump capture
- Runs: `tcpdump -C 1 -W <N> -w /capture-<pod>.pcap -i any -n`
- Capture stops and pcap files are cleaned up when annotation is removed

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

## Verification

All deliverables are in `outputs/`:
- `pod-describe.txt` - Test pod with annotation
- `pods.txt` - All pods in cluster
- `capture-files.txt` - Pcap files listing
- `capture-file.pcap` - Extracted capture file
- `capture-output.txt` - Human-readable tcpdump output
