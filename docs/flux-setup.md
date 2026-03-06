# Flux GitOps Setup

## Bootstrap

Install Flux and connect it to this repository:

```bash
flux bootstrap github \
  --owner=albertvo \
  --repository=Homelab \
  --branch=main \
  --path=deploy/k8s/overlays/homelab \
  --personal
```

## Image Automation

### ImageRepository

Tells Flux where to look for new container tags:

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageRepository
metadata:
  name: filesync-api
  namespace: flux-system
spec:
  image: ghcr.io/albertvo/homelab/api
  interval: 5m

---
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageRepository
metadata:
  name: filesync-worker
  namespace: flux-system
spec:
  image: ghcr.io/albertvo/homelab/worker
  interval: 5m
```

### ImagePolicy

Selects the latest SHA tag (filters out `latest`):

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImagePolicy
metadata:
  name: filesync-api
  namespace: flux-system
spec:
  imageRepositoryRef:
    name: filesync-api
  filterTags:
    pattern: '^[a-f0-9]{40}$'
  policy:
    alphabetical:
      order: asc

---
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImagePolicy
metadata:
  name: filesync-worker
  namespace: flux-system
spec:
  imageRepositoryRef:
    name: filesync-worker
  filterTags:
    pattern: '^[a-f0-9]{40}$'
  policy:
    alphabetical:
      order: asc
```

### ImageUpdateAutomation

Automatically commits tag updates back to the repo:

```yaml
apiVersion: image.toolkit.fluxcd.io/v1beta2
kind: ImageUpdateAutomation
metadata:
  name: filesync
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: flux-system
  git:
    checkout:
      ref:
        branch: main
    commit:
      author:
        name: fluxcdbot
        email: fluxcdbot@users.noreply.github.com
      messageTemplate: "chore: update images to {{range .Changed.Changes}}{{.NewValue}}{{end}}"
    push:
      branch: main
  update:
    path: deploy/k8s/overlays/homelab
    strategy: Setters
```
