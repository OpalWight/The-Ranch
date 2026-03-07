# Jenkins-Based Infrastructure Plan for Selfhosted Webapps (with Cloudflare Tunnels)

This plan outlines how to build a scalable, Jenkins-driven deployment pipeline for your homelab, using Cloudflare Tunnels for secure ingress and KEDA for AWS-like autoscaling.

## 1. Jenkins-Centric CI/CD (The Push-Based Alternative to GitOps)
Instead of a pull-based GitOps model (ArgoCD), we will use Jenkins to "push" manifests to Kubernetes.

### Logistical Changes:
*   **Pipeline Control:** Jenkins becomes the source of truth for *when* a deployment happens. You gain fine-grained control over multi-stage pipelines (e.g., Test -> Security Scan -> Deploy).
*   **Credential Management:** Jenkins will need a `KubeConfig` or ServiceAccount token stored as a "Secret" to authenticate with the cluster.
*   **Manifest Management:** We will use the existing `deploy/k8s` structure. Jenkins will run `kubectl apply -k deploy/k8s/overlays/homelab` or `helm upgrade --install` commands.
*   **State Drift:** Unlike GitOps, Jenkins won't automatically revert manual `kubectl` changes in the cluster. You rely on the pipeline to enforce state.

## 2. Ingress via Cloudflare Tunnels (cloudflared)
Instead of opening ports or using the Tailscale operator for public access, we will use Cloudflare Tunnels.

### Implementation:
*   **Cloudflared Sidecar/Deployment:** Deploy `cloudflared` as a deployment in your cluster. It creates a secure, outbound-only connection to Cloudflare.
*   **No Public IP:** You don't need a static IP or port forwarding on your router.
*   **Cloudflare Zero Trust:** You can add Access Policies (SSO/Google/GitHub login) in front of your selfhosted apps with one click.
*   **Config:** We will use a Kubernetes `ConfigMap` to manage the tunnel's ingress rules, routing `app.yourdomain.com` to the internal K8s Service.

## 3. AWS-Like Scaling (KEDA + HPA)
To achieve "scale to zero" and rapid scaling like AWS Lambda or Fargate:

*   **KEDA (Kubernetes Event-driven Autoscaling):** We will install KEDA to scale your pods based on external events (e.g., Redis queue depth, Prometheus metrics, or even Cron schedules).
*   **Horizontal Pod Autoscaler (HPA):** For standard scaling (CPU/RAM), we will use HPA.
*   **Scaling to Zero:** KEDA can scale your **Worker** or **Backend** pods to 0 when there's no traffic/tasks, and spin them up instantly when a request arrives, saving power and resources.

## 4. Frontend & Backend Hosting Strategy
*   **Frontend (SvelteKit/Static):** 
    *   Deployed as a `Deployment` with a `Service`. 
    *   Cloudflare Tunnel routes `https://homelab.yourdomain.com` to this service.
    *   Can be cached at the Cloudflare Edge for global performance.
*   **Backend (Go API):** 
    *   Deployed as a `Deployment`.
    *   Scaled by KEDA based on HTTP request rate (via Prometheus) or Redis task count.
    *   Uses the existing Postgres/MinIO/Redis stack.

---

## Phase 1 Execution Steps (Jenkins & Cloudflare):

1.  **Jenkins Setup:**
    *   Configure a Jenkins agent with `kubectl` and `helm` installed.
    *   Add the Kubernetes cluster as a "Cloud" in Jenkins settings for dynamic agents.
2.  **Cloudflare Tunnel:**
    *   Create a Tunnel in the Cloudflare Dashboard.
    *   Deploy the `cloudflared` pod using a new manifest in `deploy/k8s/base/cloudflare-tunnel.yaml`.
3.  **Scaling Logic:**
    *   Define `ScaledObject` resources for the Backend and Worker to trigger scaling based on the Redis queue (already partially started in the current project).
4.  **The Pipeline:**
    *   Update the `Jenkinsfile` to include a `Deploy` stage that runs `kubectl apply` across the `deploy/k8s` overlays.
