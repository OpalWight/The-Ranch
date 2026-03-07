# Jenkins CI/CD Setup

This project uses a self-hosted Jenkins instance for CI/CD, replacing the previous
GitHub Actions + FluxCD setup.

## Architecture

- **Jenkins** runs via Docker Compose on the `head` node alongside postgres, minio, and redis
- **CI**: Jenkins tests Go code, builds Docker images (api, worker, web), pushes to GHCR
- **CD**: Jenkins deploys to k3s via `kustomize` + `kubectl`, and manages Helm releases for infrastructure

## Prerequisites

Before starting Jenkins, prepare the following:

1. **GHCR Personal Access Token** — a GitHub PAT with `write:packages` scope
2. **kubeconfig** — copy from `/etc/rancher/k3s/k3s.yaml` on the control plane node.
   Keep the server as `127.0.0.1:6443` — Jenkins uses `network_mode: host` and
   reaches the k3s API directly via loopback.
3. **Base64-encode the kubeconfig**:
   ```bash
   cat /etc/rancher/k3s/k3s.yaml | base64 -w0
   ```

## Environment Variables

Create a `.env` file in the project root (gitignored):

```bash
JENKINS_ADMIN_USER=admin
JENKINS_ADMIN_PASSWORD=<your-password>
GHCR_USERNAME=<your-github-username>
GHCR_TOKEN=<your-github-pat>
KUBECONFIG_BASE64=<base64-encoded-kubeconfig>
REPO_URL=https://github.com/OpalWight/The-Ranch.git
```

## Starting Jenkins

```bash
# Build the custom Jenkins image (first time or after changing jenkins/Dockerfile)
make jenkins-build

# Start Jenkins
make jenkins-up

# View logs
make jenkins-logs
```

Jenkins UI will be available at `http://<head-node-ip>:8080` (uses host networking).

## Configuration

Jenkins is fully configured via **Configuration as Code (JCasC)**:

| File                  | Purpose                                              |
|-----------------------|------------------------------------------------------|
| `jenkins/Dockerfile`  | Custom image with Docker CLI, kubectl, kustomize, Helm |
| `jenkins/plugins.txt` | Jenkins plugins installed at image build time        |
| `jenkins/casc.yaml`   | JCasC — credentials, security, job definitions       |
| `Jenkinsfile`         | Pipeline definition — test, build, push, deploy      |

### Credentials (managed via JCasC)

| ID                | Type              | Purpose                    |
|-------------------|-------------------|----------------------------|
| `ghcr-credentials`| Username/Password | Push images to GHCR        |
| `kubeconfig`      | Secret File       | kubectl access to k3s      |

## Pipeline Stages

The `Jenkinsfile` defines these stages:

| Stage                       | Trigger      | Description                                           |
|-----------------------------|-------------|-------------------------------------------------------|
| **Test**                    | All branches | Starts postgres, runs migrations, `go test`, `go vet` |
| **Build Images** (parallel) | All branches | Builds api, worker, web Docker images simultaneously  |
| **Push Images**             | All branches | Pushes all images to GHCR with SHA + latest tags      |
| **Deploy App**              | `main` only  | Updates kustomize image tags, applies to k3s, waits for rollout |
| **Deploy Infrastructure**   | `main` only  | Runs `helm upgrade --install` for postgres, redis, minio, tailscale |

## Helm Releases

Infrastructure previously managed by FluxCD HelmReleases is now managed by Jenkins
via `helm upgrade --install`. Values files are in `helm/values/`:

| Chart                      | Namespace         | Values File              |
|----------------------------|-------------------|--------------------------|
| `bitnami/postgresql`       | `database`        | `helm/values/postgres.yaml` |
| `bitnami/redis`            | `cache`           | `helm/values/redis.yaml`    |
| `minio/minio`              | `storage`         | `helm/values/minio.yaml`    |
| `tailscale/tailscale-operator` | `tailscale-system` | (inline values in Jenkinsfile) |

## Differences from FluxCD

| Feature                  | FluxCD (old)                     | Jenkins (new)                      |
|--------------------------|----------------------------------|------------------------------------|
| Deployment model         | Pull-based (agent in cluster)    | Push-based (Jenkins applies)       |
| Drift detection          | Automatic reconciliation         | Manual — re-run pipeline to fix    |
| Image tag updates        | Auto-commit to Git               | Direct deploy, no Git commit       |
| Helm lifecycle           | HelmRelease CRD with rollback    | `helm upgrade --install` in pipeline |
| Credential location      | In-cluster secrets               | Jenkins credentials (JCasC)        |

## Uninstalling FluxCD from the Cluster

After verifying Jenkins deploys work correctly, remove FluxCD from the cluster:

```bash
# If flux CLI is installed:
flux uninstall

# Otherwise, manually:
kubectl delete namespace flux-system
kubectl delete crd -l app.kubernetes.io/part-of=flux
```
