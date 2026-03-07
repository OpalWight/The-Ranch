# TODO

## Cross-Node DNS Resolution Issue

**Priority:** Medium (workaround in place)
**Workaround:** All filesync pods pinned to head node via `nodeSelector`

### Problem
DNS resolution from pods on `worker-2` times out when trying to reach CoreDNS on the `head` node.
CoreDNS runs only on `head` (10.42.0.11). Pods on worker-2 (10.42.2.x subnet) can't reach
the CoreDNS service IP (10.43.0.10) — UDP packets time out.

`worker-1` is `NotReady` and has been for some time.

All node internal IPs are Tailscale IPs (100.x.x.x), which means k3s cross-node networking
goes over the Tailscale mesh. The flannel/VXLAN overlay may not be routing service ClusterIPs
properly across the Tailscale tunnel.

### Investigation Steps
1. Check flannel status on worker-2: `journalctl -u k3s-agent | grep flannel`
2. Check if VXLAN interface exists on worker-2: `ip link show flannel.1`
3. Test direct pod-to-pod connectivity across nodes (bypass service IP)
4. Check iptables rules on worker-2 for service IP NAT
5. Consider if k3s needs `--flannel-iface` set to the Tailscale interface
6. Check if restarting k3s-agent on worker-2 fixes the issue
7. Consider draining worker-1 and removing it if it's permanently offline

### References
- CoreDNS pod: `coredns-7f496c8d7d-6n4gb` on `head` (10.42.0.11)
- CoreDNS service: `10.43.0.10:53`
- Node IPs: head=100.119.28.52, worker-1=100.109.193.25 (NotReady), worker-2=100.64.205.37
