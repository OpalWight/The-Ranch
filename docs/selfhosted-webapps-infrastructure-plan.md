# Infrastructure Plan for Selfhosted Webapps on Homelab

This document outlines the strategy for expanding the current Kubernetes-based homelab to support a variety of selfhosted web applications (e.g., Nextcloud, Jellyfin, Home Assistant, Pi-hole).

## 1. GitOps & Deployment Management
To manage multiple selfhosted applications declaratively and cleanly:
*   **Implement a GitOps Controller:** Introduce **ArgoCD** or **FluxCD** to the cluster. This allows you to manage the deployment of all selfhosted webapps via this Git repository.
*   **Directory Structure:** Create a dedicated directory (e.g., `deploy/apps/`) to house Helm values or Kustomize manifests for each third-party application.

## 2. Ingress & External Access
Currently, there is an `ingress.yaml` for the custom app. To scale this for multiple webapps securely:
*   **Cert-Manager:** Deploy `cert-manager` to automatically provision and renew TLS certificates (via Let's Encrypt) for all selfhosted services.
*   **Tailscale Integration:** Since Tailscale is already planned/used, deploy the **Tailscale Kubernetes Operator**. This allows you to expose services securely to your Tailnet without opening ports to the public internet. You can use Tailscale's MagicDNS for internal routing (e.g., `app-name.tailnet-name.ts.net`).
*   **External DNS:** (Optional) If using public domains, use `external-dns` to automatically sync Kubernetes Ingress resources to your DNS provider (e.g., Cloudflare).

## 3. Persistent Storage (CSI)
Selfhosted apps require reliable persistent data (e.g., databases, media files, config files).
*   **Block/File Storage:** Relying solely on local HostPath is risky. Implement a Storage Class using **Longhorn**, **TrueNAS (via democratic-csi)**, or an **NFS Provisioner** to ensure pods can mount persistent volumes (PVCs) regardless of which node they schedule on.
*   **Object Storage:** MinIO is already deployed and can be reused for applications that support S3-compatible backends (e.g., Nextcloud primary storage or backups).

## 4. Shared Infrastructure
Rather than deploying separate databases for every app, utilize shared services where appropriate to save resources:
*   **Postgres Cluster:** Use the existing Postgres instance (or deploy a robust operator like CloudNativePG) to host multiple databases for different webapps.
*   **Redis:** Share the Redis instance for caching and session management across apps that support it.

## 5. Observability & Monitoring
*   **Metrics:** Integrate new selfhosted apps into the existing Prometheus/Grafana stack using `ServiceMonitor` or `PodMonitor` CRDs.
*   **Logging:** Ensure Promtail/Loki is configured to capture logs from all namespaces where third-party apps are deployed.

## 6. Backup & Disaster Recovery
*   **Velero:** Deploy Velero to back up Kubernetes resources and persistent volumes (PVCs) to your MinIO instance or an off-site cloud bucket (like AWS S3 or Backblaze B2).
*   **Database Backups:** Schedule automated dumps of the Postgres database to S3/MinIO.

## Phase 1 Execution Steps:
1.  **Storage:** Set up a robust default StorageClass (e.g., Longhorn) to handle PVC requests from third-party Helm charts.
2.  **GitOps:** Install ArgoCD/Flux and bootstrap it to this repository.
3.  **Access:** Deploy cert-manager and configure the Tailscale operator for secure routing.
4.  **First App:** Deploy a lightweight test app (e.g., a static dashboard like Homepage or Dashy) using the GitOps workflow to validate routing, TLS, and storage.