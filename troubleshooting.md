# Homelab Deployment Troubleshooting

## Overview

Deploy **The Ranch** (filesync / Dropbox clone) to a 3-node K3s cluster using **FluxCD GitOps**.
Stack: Go API + Svelte SPA + Postgres + Redis + MinIO + KEDA + Tailscale ingress.
Cluster: `head` (control-plane), `worker-1`, `worker-2` — all joined via Tailscale (100.x.x.x IPs).

---

## Issues & Fixes (Chronological)

### 1. Missing Backend Infrastructure (Postgres, Redis, MinIO)

**Problem:** `filesync-api` crash-loops — can't find `postgres-postgresql.database.svc.cluster.local`.
**Cause:** Helm values existed in `helm/values/` but no Flux `HelmRelease` objects were defined.
**Fix:** Created `helmrelease-postgres.yaml`, `helmrelease-redis.yaml`, `helmrelease-minio.yaml` + `helmrepository.yaml` in `deploy/k8s/base/`. Commit `d0448bf`.

---

### 2. Image Pull Failures (GHCR Authentication)

**Problem:** `ImagePullBackOff` on api/worker pods — images in private GHCR.
**Cause:** No `imagePullSecrets` in deployments, no `ghcr-auth` secret in cluster.
**Fix:**
- User manually created `ghcr-auth` secret via `kubectl create secret docker-registry`.
- Added `imagePullSecrets` to deployment manifests (commit `aa22477`).
- Later sealed the secret with kubeseal and added to GitOps (commit `845aee9`).

---

### 3. Database Migration Init Container Failures

**Problem:** `Init:CrashLoopBackOff` on API pod — migrate container can't reach Postgres.
**Cause:** Postgres wasn't deployed yet.
**Fix:** Resolved once HelmRelease for Postgres was applied (issue #1).

---

### 4. Sealed Secrets Controller Namespace

**Problem:** `kubeseal` commands failed — couldn't find sealed-secrets controller.
**Cause:** Controller is in `kube-system` (not `flux-system` which is a common default).
**Fix:** Always use `--controller-namespace=kube-system` with kubeseal. Discovery command: `kubectl get pods -A | grep sealed`.

---

### 5. Namespace Clobbering by Kustomize

**Problem:** Sealed secrets encrypted for specific namespaces (e.g. `database`, `cache`) failed to decrypt after being deployed.
**Cause:** `deploy/k8s/overlays/homelab/kustomization.yaml` had `namespace: default` which Kustomize applies to ALL resources, overriding their metadata namespace. A sealed secret encrypted for namespace `database` was deployed to `default` and couldn't be decrypted.
**Fix:** Removed global `namespace: default` from the overlay kustomization. Added explicit `namespace: default` to each app-level resource that needed it. Commit `cef188d`.

---

### 6. Missing Namespace Resources

**Problem:** Sealed secrets for `database`, `cache`, `storage`, `tailscale-system` namespaces failed because the namespaces didn't exist yet.
**Cause:** HelmReleases had `install.createNamespace: true` but sealed secrets were applied before HelmReleases ran.
**Fix:** Created explicit `Namespace` resources in `namespaces.yaml` (database, cache, storage, tailscale-system). Commit `69e5baa`.

---

### 7. Flux Image Automation Controllers Missing

**Problem:** `ImageRepository`, `ImagePolicy`, `ImageUpdateAutomation` CRDs not recognized.
**Cause:** Flux's image automation controllers are NOT included in default `flux bootstrap` — they must be added via `--components-extra`.
**Fix:** Added image-reflector-controller and image-automation-controller to `gotk-components.yaml`. Commit `e99a714`.

---

### 8. ImageUpdateAutomation API Version

**Problem:** `ImageUpdateAutomation` resource rejected — unknown API version.
**Cause:** Used `v1beta1` but Flux v2.8.1 uses `v1beta2`.
**Fix:** Changed `image.toolkit.fluxcd.io/v1beta1` to `image.toolkit.fluxcd.io/v1beta2`. Commit `e81a6f1`.

---

### 9. Bitnami Helm Repository Deprecated

**Problem:** Flux couldn't fetch Bitnami charts — 404 errors.
**Cause:** Bitnami deprecated their classic HTTP chart repo (`https://charts.bitnami.com/bitnami`). Charts are now OCI-only.
**Fix:** Changed HelmRepository to `type: oci` with `url: oci://registry-1.docker.io/bitnamicharts`. Commit `b7b5e55`.

---

### 10. Dockerfile Go Version Mismatch

**Problem:** CI build failed — Go compilation errors.
**Cause:** Dockerfile used `golang:1.23-alpine` but `go.mod` declares Go 1.25.6.
**Fix:** Changed base image to `golang:1.25-alpine`. Commit `86ec3c4`.

---

### 11. Helm Release Service Hostname Mismatch

**Problem:** API pods couldn't connect to Postgres/Redis/MinIO after HelmReleases deployed.
**Cause:** When a Flux HelmRelease named `postgres` targets `targetNamespace: database`, the Helm release is created as `database-postgres`. Bitnami charts generate service names from the release name, so:
- Postgres: `database-postgres-postgresql.database.svc.cluster.local` (not `postgres-postgresql`)
- Redis: `cache-redis-master.cache.svc.cluster.local` (not `redis-master`)
- MinIO: `storage-minio.storage.svc.cluster.local` (not `minio`)
**Fix:** Updated configmap, KEDA ScaledObject, and re-sealed DB/Redis secrets with correct hostnames. Commit `f640285`.

---

### 12. Cross-Node DNS Broken (CoreDNS Unreachable from Workers)

**Problem:** Pods on `worker-2` (and `worker-1`) can't resolve DNS — UDP timeouts to CoreDNS ClusterIP `10.43.0.10`.
**Cause:** CoreDNS runs only on `head`. All node internal IPs are Tailscale IPs (100.x.x.x). K3s cross-node networking goes over Tailscale mesh — flannel/VXLAN overlay doesn't route service ClusterIPs properly across the tunnel. `worker-1` was also `NotReady`.
**Workaround:** Pinned ALL pods to `head` node via `nodeSelector: node-role.kubernetes.io/control-plane: "true"` on every deployment + HelmRelease values. Commits `8327c30`, `8802905`.
**Status:** ROOT CAUSE UNRESOLVED. See `TODO.md` for investigation steps.

---

### 13. CI: GHCR Rejects Mixed-Case Image Tags

**Problem:** `docker push` failed in GitHub Actions — "invalid reference format".
**Cause:** `${{ github.repository }}` resolves to `OpalWight/The-Ranch` (mixed case). Docker/GHCR requires all-lowercase image references.
**Fix:** Added a CI step that lowercases the repo name via `echo "$REPO" | tr '[:upper:]' '[:lower:]'` into `$GITHUB_ENV` as `IMAGE_BASE`. All image refs use `${{ env.IMAGE_BASE }}`. Commit `f80bc75`.

---

### 14. Tailscale Operator ACL Tag Permissions

**Problem:** Tailscale operator logs: `"requested tags [tag:k8s] are invalid or not permitted"`. No proxy pod created for ingress.
**Cause:** The operator uses `tag:k8s-operator` for itself, but creates ingress proxy devices with `tag:k8s` by default. Both tags must exist in the tailnet ACL, and the OAuth client must be scoped to both.
**Fix:**
- Updated tailnet ACL: `"tagOwners": {"tag:k8s-operator": ["autogroup:admin"], "tag:k8s": ["tag:k8s-operator"]}`
- Created new OAuth client scoped to both `tag:k8s` and `tag:k8s-operator`
- Re-sealed the OAuth secret with new credentials. Commit `d83b376`.

---

### 15. Tailscale Operator Scheduled on Worker (No DNS)

**Problem:** After operator restart, the new pod landed on `worker-2` — which has broken DNS. Operator logs: `dial tcp: lookup kubernetes.default.svc: i/o timeout`.
**Cause:** No nodeSelector on the operator pod. Scheduler placed it on worker-2.
**Fix:** The Tailscale Helm chart uses `operatorConfig.nodeSelector` (NOT top-level `nodeSelector`). First attempt with top-level `nodeSelector` was silently ignored.
- Wrong: `values.nodeSelector` (ignored by chart)
- Right: `values.operatorConfig.nodeSelector`
Commits `8802905` (wrong path), `5bb00c4` (correct path).

---

### 16. Tailscale Ingress Proxy Pod on Worker (No DNS)

**Problem:** Operator created proxy StatefulSet pod `ts-filesync-ingress-blx8r-0` but it landed on `worker-1` — also broken DNS. Same timeout errors.
**Cause:** The proxy pod is managed by the Tailscale operator (not by the Helm chart directly), so HelmRelease `nodeSelector` doesn't apply to it.
**Fix:** Created a `ProxyClass` CRD resource named `head-node` with `spec.statefulSet.pod.nodeSelector`, then annotated the Ingress with `tailscale.com/proxy-class: "head-node"`. The operator picked it up and rescheduled the proxy to `head`. Commit `95b819b`.

```yaml
# proxyclass.yaml
apiVersion: tailscale.com/v1alpha1
kind: ProxyClass
metadata:
  name: head-node
spec:
  statefulSet:
    pod:
      nodeSelector:
        node-role.kubernetes.io/control-plane: "true"
```

---

### 17. Tailscale Ingress HTTPS Not Working

**Problem:** Proxy pod running and connected to tailnet, but ports 80/443 refuse connections. Log: `"this node is configured as a proxy that exposes an HTTPS endpoint to tailnet... but it is not able to issue TLS certs"`.
**Cause:** HTTPS is not enabled on the tailnet. Tailscale Ingress only serves HTTPS and needs the tailnet's HTTPS feature enabled so it can auto-provision Let's Encrypt certs via Tailscale.
**Fix:** **PENDING** — Requires manual action: enable HTTPS in Tailscale admin console at `https://login.tailscale.com/admin/dns` (toggle "Enable HTTPS").
**Workaround:** App is fully functional via port-forward: `kubectl port-forward -n default svc/filesync-web 8080:80`.

---

### 18. MinIO HelmRelease Stuck in Failed State

**Problem:** MinIO HelmRelease shows `False` / `Helm upgrade failed: post-upgrade hooks failed: timeout waiting for Job`.
**Cause:** The MinIO chart runs a post-install/post-upgrade Job that creates buckets. The Job pod landed on `worker-2` (broken DNS), couldn't reach MinIO, and timed out. Even after adding nodeSelector to the HelmRelease values, the stale failure status persists.
**Fix attempted:** Deleted the stuck job (`kubectl delete job storage-minio-post-job -n storage`), then tried:
- `flux reconcile helmrelease minio`
- Force annotation: `kubectl annotate helmrelease minio -n flux-system reconcile.fluxcd.io/forceReconcile=$(date +%s) --overwrite`
- Suspend/resume
**Status:** **PENDING** — MinIO pod itself is running fine (1/1 on head). The HelmRelease status is stale from the old failure. May need `flux suspend`/`flux resume` or deleting the Helm release secret to force a clean install.

---

### 19. Tailscale Ingress Hostname Mismatch

**Problem:** Ingress annotation sets `tailscale.com/hostname: "filesync"` but the device appeared on tailnet as `default-filesync-ingress-ingress.tail280aca.ts.net`.
**Cause:** Likely because the proxy was initially created before the hostname annotation was fully processed, or the operator uses a different naming pattern for ingress proxies.
**Status:** Low priority — doesn't affect functionality. The device is reachable by its actual MagicDNS name. May self-resolve after HTTPS is enabled and the proxy is recreated.

---

## Current State (as of commit `95b819b`)

### What's Working
| Component | Status | Node |
|-----------|--------|------|
| Flux GitOps | Applied latest revision | - |
| filesync-api (2 replicas) | Running | head |
| filesync-web | Running | head |
| filesync-worker | Running | head |
| Postgres | Running | head |
| Redis | Running | head |
| MinIO (pod) | Running | head |
| Tailscale operator | Running | head |
| Tailscale proxy | Running, connected to tailnet | head |
| CI pipeline | Building + pushing images | GitHub Actions |
| Flux image automation | Updating image tags in Git | - |

### What's Broken / Pending
| Issue | Priority | Action Required |
|-------|----------|-----------------|
| Tailscale HTTPS not enabled | High | Manual: enable in Tailscale admin console |
| MinIO HelmRelease stuck failed | Medium | Suspend/resume or clean reinstall |
| Cross-node DNS broken | Medium | Investigate flannel over Tailscale (see TODO.md) |
| Tailscale hostname mismatch | Low | May self-resolve |
| Placeholder passwords everywhere | Low | Generate real passwords, re-seal |

### Key Commands
```bash
# Port-forward to access the app now (bypasses Tailscale ingress)
kubectl port-forward -n default svc/filesync-web 8080:80

# Check all pod status
kubectl get pods -A -o wide

# Check Flux status
flux get all -A

# Fix MinIO HelmRelease (try this)
flux suspend helmrelease minio -n flux-system
flux resume helmrelease minio -n flux-system

# Check Tailscale proxy
kubectl logs -n tailscale-system -l app.kubernetes.io/name=tailscale -f
```
