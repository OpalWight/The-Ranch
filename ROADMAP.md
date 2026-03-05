# Cloud-Native Homelab Implementation Roadmap

> A phased guide to building a production-grade, cloud-native file sync application on k3s — from first container to horizontally-scaled microservices.

---

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Technology Stack](#technology-stack)
- [Prerequisites](#prerequisites)
- [Phase 1: Containerization & The Basic Pod](#phase-1-containerization--the-basic-pod)
- [Phase 2: Internal Networking & Object Storage](#phase-2-internal-networking--object-storage)
- [Phase 3: Ingress & Real-Time Sync](#phase-3-ingress--real-time-sync)
- [Phase 4: True Orchestration & Scaling](#phase-4-true-orchestration--scaling)
- [Phase 5: Observability & Operations](#phase-5-observability--operations)
- [Phase 6: CI/CD & GitOps](#phase-6-cicd--gitops)
- [Appendix](#appendix)

---

## Architecture Overview

```
                          ┌─────────────────────────────────────────────────┐
                          │                  k3s Cluster                    │
                          │                                                 │
  Client ──► Traefik ─────┤──► Go API Pods (stateless, scratch containers) │
             Ingress      │       │           │           │                 │
             (TLS)        │       ▼           ▼           ▼                 │
                          │   PostgreSQL    MinIO       Redis              │
                          │   (Helm)       (Helm)      (Helm)             │
                          │                                                 │
                          │   Go Worker Pods ◄── Redis Streams (tasks)     │
                          └─────────────────────────────────────────────────┘
```

**Core Application:** A file synchronization service written in Go that allows clients to upload, store, retrieve, and receive real-time notifications about file changes.

**Design Principles:**
- Application pods are always stateless — all state lives in dedicated data stores
- Compute is decoupled from storage from day one
- Infrastructure is deployed via Helm charts with explicit resource constraints
- Every component is chosen for transferable, enterprise-relevant skills

---

## Technology Stack

| Layer              | Technology   | Purpose                                      |
|--------------------|-------------|----------------------------------------------|
| Compute            | Go 1.22+    | Application logic (API server + workers)     |
| Container Runtime  | scratch      | Minimal, secure container images             |
| Orchestration      | k3s          | Lightweight Kubernetes distribution          |
| Database           | PostgreSQL   | Structured metadata, user data, file records |
| Object Storage     | MinIO        | S3-compatible blob storage for file content  |
| Cache / Queues     | Redis        | Pub/Sub, SSE fan-out, task queuing           |
| Ingress            | Traefik      | TLS termination, routing, load balancing     |
| Observability      | Prometheus + Grafana | Metrics collection and dashboards    |
| Logging            | Loki + Promtail | Log aggregation                           |
| GitOps             | Flux or ArgoCD | Declarative cluster state management      |

---

## Prerequisites

### Hardware
- At least one machine capable of running k3s (2+ CPU cores, 4GB+ RAM recommended)
- For multi-node: 2-3 nodes (VMs, Raspberry Pis, or old laptops)
- Networked storage or sufficient local disk on each node

### Software
- k3s installed and running (`curl -sfL https://get.k3s.io | sh -`)
- `kubectl` configured to talk to your cluster
- `helm` v3 installed
- Go 1.22+ installed locally for development
- Docker or Podman for building container images
- A container registry (Docker Hub, GitHub Container Registry, or a local registry)

### Knowledge Assumptions
- Basic familiarity with Go syntax
- Understanding of HTTP APIs (REST)
- Basic Linux command-line proficiency
- Conceptual understanding of containers (what they are, why they exist)

---

## Phase 1: Containerization & The Basic Pod

**Goal:** Deploy a single Go HTTP server as a Pod in k3s, backed by PostgreSQL for metadata. Establish the foundational project structure, build pipeline, and Kubernetes deployment patterns.

**Duration estimate:** N/A — move at your own pace, focus on understanding each step.

### 1.1 Project Structure

```
Homelab/
├── cmd/
│   ├── api/                  # API server entrypoint
│   │   └── main.go
│   └── worker/               # Background worker entrypoint (Phase 4)
│       └── main.go
├── internal/
│   ├── config/               # Configuration loading (env vars)
│   │   └── config.go
│   ├── handler/              # HTTP handlers
│   │   ├── health.go         # Health/readiness probes
│   │   └── files.go          # File CRUD endpoints
│   ├── middleware/            # HTTP middleware (logging, auth)
│   │   └── logging.go
│   ├── model/                # Domain types and DB models
│   │   └── file.go
│   ├── repository/           # Database access layer
│   │   └── postgres.go
│   └── storage/              # Object storage abstraction (Phase 2)
│       └── minio.go
├── migrations/               # SQL migration files
│   ├── 001_create_files.up.sql
│   └── 001_create_files.down.sql
├── deploy/
│   ├── docker/
│   │   └── Dockerfile
│   └── k8s/
│       ├── base/             # Base Kustomize manifests
│       │   ├── kustomization.yaml
│       │   ├── deployment.yaml
│       │   ├── service.yaml
│       │   └── configmap.yaml
│       └── overlays/
│           └── homelab/      # Homelab-specific overrides
│               └── kustomization.yaml
├── helm/
│   └── values/
│       ├── postgres.yaml     # PostgreSQL Helm values
│       ├── redis.yaml        # Redis Helm values (Phase 3)
│       └── minio.yaml        # MinIO Helm values (Phase 2)
├── go.mod
├── go.sum
├── Makefile
└── ROADMAP.md
```

### 1.2 Go Application — API Server

**Deliverables:**
- A Go HTTP server using the standard library `net/http` (or a lightweight router like `chi`)
- Configuration loaded exclusively from environment variables (12-factor app)
- Health check endpoints: `GET /healthz` (liveness) and `GET /readyz` (readiness — checks DB connection)
- File metadata CRUD: `POST /api/v1/files`, `GET /api/v1/files`, `GET /api/v1/files/{id}`, `DELETE /api/v1/files/{id}`
- Structured JSON logging (using `slog` from the standard library)
- Graceful shutdown handling (`os.Signal`, context cancellation)

**1.2.1 Step-by-Step Scaffolding Guide:**

1.  **Initialize the Go Module:**
    ```bash
    go mod init github.com/YOUR_USERNAME/homelab
    ```
2.  **Define Configuration (internal/config/config.go):**
    Implement a `Load()` function that populates a `Config` struct from environment variables with sensible defaults.
3.  **Setup Structured Logging:**
    In `cmd/api/main.go`, initialize `slog` to output JSON to `os.Stdout`. This ensures logs are easily parsable by Loki later.
4.  **Implement Health Check Handlers (internal/handler/health.go):**
    - `Liveness`: Returns `200 OK` immediately.
    - `Readiness`: Pings the database and returns `200 OK` only if the connection is alive.
5.  **Create the HTTP Router:**
    Use `net/http.NewServeMux()` or `chi.NewRouter()` to define your API paths and link them to handlers.
6.  **Implement Graceful Shutdown:**
    Wrap the server start in a goroutine and use `signal.Notify` to listen for `SIGINT` or `SIGTERM`. Use `server.Shutdown(ctx)` to allow active requests to finish.

**Key implementation details:**

```go
// internal/config/config.go — All config from env vars, no config files
type Config struct {
    Port        string // SERVER_PORT, default "8080"
    DatabaseURL string // DATABASE_URL, required
    MinIOURL    string // MINIO_ENDPOINT (Phase 2)
    RedisURL    string // REDIS_URL (Phase 3)
}

func Load() *Config {
    return &Config{
        Port:        getEnv("SERVER_PORT", "8080"),
        DatabaseURL: getEnv("DATABASE_URL", ""),
    }
}
```

**Database schema (Phase 1):**
```sql
-- migrations/001_create_files.up.sql
CREATE TABLE IF NOT EXISTS files (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL,
    mime_type   TEXT NOT NULL DEFAULT 'application/octet-stream',
    checksum    TEXT NOT NULL,          -- SHA-256 of file content
    storage_key TEXT,                   -- MinIO object key (Phase 2, nullable initially)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_files_name ON files(name);
CREATE INDEX idx_files_created_at ON files(created_at);
```

**Migration tooling:** Use `golang-migrate/migrate` for versioned schema migrations. Migrations run as an init container in Kubernetes (see 1.5).

### 1.3 Dockerfile — Multi-Stage Scratch Build

```dockerfile
# deploy/docker/Dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /api ./cmd/api

# --- Final image ---
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /api /api

EXPOSE 8080
ENTRYPOINT ["/api"]
```

**Why scratch:**
- Zero CVEs from OS packages — nothing to scan, nothing to patch
- Image size under 15MB (compared to 300MB+ for a full OS base)
- Forces you to statically compile your Go binary (`CGO_ENABLED=0`)
- No shell access = reduced attack surface in production

**Caveats with scratch:**
- No shell for debugging — you cannot `kubectl exec` into the container
- No `curl` or `wget` for manual health checks from inside the pod
- If you need to debug, temporarily swap to `gcr.io/distroless/static` or `alpine`

### 1.4 PostgreSQL via Helm

```bash
# Add the Bitnami Helm repo
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Install PostgreSQL
helm install postgres bitnami/postgresql \
    -f helm/values/postgres.yaml \
    -n database --create-namespace
```

**helm/values/postgres.yaml:**
```yaml
auth:
  database: filesync
  username: filesync
  # In production, use an external secret. For homelab:
  existingSecret: ""
  password: "changeme-use-sealed-secrets-later"

primary:
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi
  persistence:
    enabled: true
    size: 5Gi
    # Use the default k3s local-path StorageClass
    storageClass: "local-path"

metrics:
  enabled: true  # Exposes Prometheus metrics (useful in Phase 5)
```

**Kubernetes Secret for DB credentials:**
```yaml
# deploy/k8s/base/secret-db.yaml
apiVersion: v1
kind: Secret
metadata:
  name: filesync-db-credentials
type: Opaque
stringData:
  DATABASE_URL: "postgres://filesync:changeme-use-sealed-secrets-later@postgres-postgresql.database.svc.cluster.local:5432/filesync?sslmode=disable"
```

> **Security note:** For a homelab this is acceptable. For anything shared or exposed, use SealedSecrets or an external secrets operator to encrypt secrets at rest in Git.

### 1.5 Kubernetes Manifests

**deploy/k8s/base/deployment.yaml:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: filesync-api
  labels:
    app: filesync
    component: api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: filesync
      component: api
  template:
    metadata:
      labels:
        app: filesync
        component: api
    spec:
      initContainers:
        - name: migrate
          image: migrate/migrate:latest
          args:
            - "-path=/migrations"
            - "-database=$(DATABASE_URL)"
            - "up"
          envFrom:
            - secretRef:
                name: filesync-db-credentials
          volumeMounts:
            - name: migrations
              mountPath: /migrations
      containers:
        - name: api
          image: ghcr.io/YOUR_USERNAME/filesync-api:latest
          ports:
            - containerPort: 8080
              name: http
          envFrom:
            - secretRef:
                name: filesync-db-credentials
            - configMapRef:
                name: filesync-config
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 5
          resources:
            requests:
              cpu: 50m
              memory: 32Mi
            limits:
              cpu: 200m
              memory: 128Mi
      volumes:
        - name: migrations
          configMap:
            name: filesync-migrations
```

**deploy/k8s/base/service.yaml:**
```yaml
apiVersion: v1
kind: Service
metadata:
  name: filesync-api
spec:
  selector:
    app: filesync
    component: api
  ports:
    - port: 80
      targetPort: http
      protocol: TCP
      name: http
  type: ClusterIP
```

### 1.6 Validation Checklist

- [ ] `docker build` succeeds and image is under 20MB
- [ ] Image pushed to container registry
- [ ] PostgreSQL pod is `Running` and accepting connections
- [ ] `kubectl apply -k deploy/k8s/overlays/homelab/` succeeds
- [ ] Init container runs migrations without error
- [ ] `kubectl port-forward svc/filesync-api 8080:80` allows local access
- [ ] `curl localhost:8080/healthz` returns `200 OK`
- [ ] `curl localhost:8080/readyz` returns `200 OK` (DB connected)
- [ ] `POST /api/v1/files` creates a record in PostgreSQL
- [ ] `GET /api/v1/files` returns the created record
- [ ] Pod restarts cleanly and reconnects to PostgreSQL without data loss

### 1.7 Concepts Learned

- Multi-stage Docker builds and scratch containers
- Kubernetes Deployments, Services, ConfigMaps, Secrets
- Init containers for database migrations
- Liveness vs. readiness probes (and why both matter)
- 12-factor app configuration via environment variables
- Resource requests and limits
- Internal DNS resolution (`service.namespace.svc.cluster.local`)

---

## Phase 2: Internal Networking & Object Storage

**Goal:** Add MinIO for S3-compatible file storage. The Go API stores file content in MinIO and metadata in PostgreSQL. Solidify understanding of inter-service communication inside the cluster.

### 2.1 MinIO via Helm

```bash
helm repo add minio https://charts.min.io/
helm repo update

helm install minio minio/minio \
    -f helm/values/minio.yaml \
    -n storage --create-namespace
```

**helm/values/minio.yaml:**
```yaml
mode: standalone     # Single node for homelab (not distributed)

resources:
  requests:
    cpu: 100m
    memory: 256Mi
  limits:
    cpu: 500m
    memory: 512Mi

persistence:
  enabled: true
  size: 20Gi
  storageClass: "local-path"

rootUser: minioadmin
rootPassword: "changeme-minio-secret"

buckets:
  - name: filesync
    policy: none       # Private — only accessible via API credentials
    purge: false

consoleIngress:
  enabled: false       # Enable later if you want the MinIO web console
```

### 2.2 Go Storage Layer

**internal/storage/minio.go:**

Implement a `Storage` interface so the application doesn't depend on MinIO directly:

```go
type Storage interface {
    Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
    Download(ctx context.Context, key string) (io.ReadCloser, error)
    Delete(ctx context.Context, key string) error
}
```

Use the official `minio/minio-go/v7` SDK, which is API-compatible with AWS S3. This means:
- Your code will work with AWS S3 in production with zero changes
- You learn the S3 API patterns (PutObject, GetObject, presigned URLs)
- Presigned URLs allow clients to upload/download directly to MinIO, bypassing the API server for large files

**Key endpoints to add/modify:**
- `POST /api/v1/files/upload` — Accepts multipart form data, streams to MinIO, stores metadata in Postgres
- `GET /api/v1/files/{id}/download` — Generates a presigned MinIO URL and redirects, or proxies the content
- `DELETE /api/v1/files/{id}` — Deletes from both MinIO and Postgres (within a transaction-like flow)

### 2.3 Upload Flow

```
Client                    API Pod                  MinIO               PostgreSQL
  │                         │                        │                     │
  │── POST /upload ────────►│                        │                     │
  │   (multipart body)      │                        │                     │
  │                         │── PutObject ──────────►│                     │
  │                         │   (stream, no buffer)  │                     │
  │                         │◄── OK ─────────────────│                     │
  │                         │                        │                     │
  │                         │── INSERT file record ──┼────────────────────►│
  │                         │   (name, size, key)    │                     │
  │                         │◄── OK ─────────────────┼─────────────────────│
  │                         │                        │                     │
  │◄── 201 Created ────────│                        │                     │
  │   {id, name, size}     │                        │                     │
```

**Important:** Stream the upload directly from the HTTP request body to MinIO. Do NOT buffer the entire file in memory — this would cause OOM kills on large files.

### 2.4 Internal DNS & Service Discovery

All inter-service communication uses Kubernetes internal DNS:

| Service    | Internal DNS Name                                  | Port  |
|------------|---------------------------------------------------|-------|
| PostgreSQL | `postgres-postgresql.database.svc.cluster.local`  | 5432  |
| MinIO API  | `minio.storage.svc.cluster.local`                 | 9000  |
| MinIO Console | `minio-console.storage.svc.cluster.local`     | 9001  |
| Redis      | `redis-master.cache.svc.cluster.local`            | 6379  |

Your Go application resolves these names automatically via the cluster DNS (CoreDNS in k3s).

### 2.5 ConfigMap Update

```yaml
# deploy/k8s/base/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: filesync-config
data:
  SERVER_PORT: "8080"
  MINIO_ENDPOINT: "minio.storage.svc.cluster.local:9000"
  MINIO_BUCKET: "filesync"
  MINIO_USE_SSL: "false"
```

MinIO credentials should go in a Secret, not the ConfigMap.

### 2.6 Validation Checklist

- [ ] MinIO pod is `Running` and the `filesync` bucket exists
- [ ] `POST /api/v1/files/upload` stores the file in MinIO and metadata in Postgres
- [ ] `GET /api/v1/files/{id}/download` returns the file content from MinIO
- [ ] `DELETE /api/v1/files/{id}` removes from both MinIO and Postgres
- [ ] Large file uploads (100MB+) succeed without API pod OOM
- [ ] Verify with `mc` CLI: `mc ls minio/filesync` shows uploaded objects

### 2.7 Concepts Learned

- S3-compatible object storage API (PutObject, GetObject, presigned URLs)
- Kubernetes internal DNS and service discovery
- Separating blob storage from relational metadata
- Streaming I/O patterns to avoid memory exhaustion
- Helm chart customization and namespace isolation

---

## Phase 3: Ingress & Real-Time Sync

**Goal:** Expose the API externally via Traefik with TLS. Add real-time file change notifications using Server-Sent Events (SSE) backed by Redis Pub/Sub.

### 3.1 Redis via Helm

```bash
helm install redis bitnami/redis \
    -f helm/values/redis.yaml \
    -n cache --create-namespace
```

**helm/values/redis.yaml:**
```yaml
architecture: standalone   # No need for sentinel in homelab

auth:
  enabled: true
  password: "changeme-redis-secret"

master:
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi
  persistence:
    enabled: true
    size: 1Gi
    storageClass: "local-path"

metrics:
  enabled: true
```

### 3.2 Server-Sent Events (SSE) vs. WebSockets

**Why SSE over WebSockets for this use case:**

| Aspect              | SSE                                | WebSocket                        |
|---------------------|------------------------------------|----------------------------------|
| Direction           | Server → Client (unidirectional)   | Bidirectional                    |
| Protocol            | Standard HTTP                      | Upgraded TCP connection          |
| Load balancing      | Works with standard HTTP LBs       | Requires sticky sessions         |
| Auto-reconnect      | Built into the browser API         | Must implement manually          |
| Traefik support     | Native, no config needed           | Needs annotation tweaks          |
| Use case fit        | Push notifications, sync updates   | Chat, gaming, collaborative edit |

For file sync notifications (server tells client "file X changed"), SSE is the simpler and more operationally correct choice.

### 3.3 SSE + Redis Pub/Sub Architecture

```
  Pod 1 (API)                Pod 2 (API)               Pod 3 (API)
     │                          │                          │
     │ User A connected         │ User B connected         │ Processes upload
     │ via SSE                  │ via SSE                  │
     │                          │                          │
     │                          │                     ┌────┴────┐
     │                          │                     │ PUBLISH  │
     │                          │                     │ "file:   │
     │                          │                     │  changed"│
     │                          │                     └────┬────┘
     │                          │                          │
     └──────────┬───────────────┴──────────────────────────┘
                │           Redis Pub/Sub
                │     (channel: "filesync:events")
                │
     ┌──────────┴──────────────────────────────────────────┐
     │                          │                          │
     ▼                          ▼                          ▼
  Pod 1 SUBSCRIBE            Pod 2 SUBSCRIBE            Pod 3 SUBSCRIBE
  → Push to User A           → Push to User B           (no SSE clients)
    via SSE                    via SSE
```

**How it works:**
1. Each API pod subscribes to the Redis `filesync:events` channel on startup
2. When any pod processes a file change, it publishes an event to Redis
3. All pods receive the event via their subscription
4. Each pod pushes the event to any SSE clients connected to that specific pod
5. Result: all clients receive the notification regardless of which pod they're connected to

### 3.4 SSE Handler Implementation

```go
// internal/handler/events.go
// GET /api/v1/events/stream
func (h *EventHandler) Stream(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Subscribe to Redis Pub/Sub
    sub := h.redis.Subscribe(r.Context(), "filesync:events")
    defer sub.Close()
    ch := sub.Channel()

    for {
        select {
        case msg := <-ch:
            fmt.Fprintf(w, "event: file_changed\ndata: %s\n\n", msg.Payload)
            flusher.Flush()
        case <-r.Context().Done():
            return
        }
    }
}
```

### 3.5 Traefik Ingress

k3s ships with Traefik pre-installed. You just need to create an IngressRoute:

**deploy/k8s/base/ingress.yaml:**
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: filesync-ingress
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
    # For SSE: increase timeouts so long-lived connections aren't dropped
    traefik.ingress.kubernetes.io/service.passhostheader: "true"
spec:
  tls:
    - hosts:
        - filesync.homelab.local
      secretName: filesync-tls
  rules:
    - host: filesync.homelab.local
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: filesync-api
                port:
                  name: http
```

**TLS options:**
- **Self-signed (simplest):** Generate certs with `mkcert` and store as a Kubernetes TLS Secret
- **Let's Encrypt (if externally accessible):** Use Traefik's built-in ACME resolver with a DNS challenge (Cloudflare, Route53)
- **cert-manager (recommended):** Install cert-manager via Helm, it automates certificate lifecycle

**Local DNS setup:**
- Add `filesync.homelab.local` to your `/etc/hosts` pointing to your k3s node IP
- Or run a local DNS server (CoreDNS, Pi-hole) that resolves `*.homelab.local`

### 3.6 Validation Checklist

- [ ] Redis pod is `Running` and accepting connections
- [ ] `curl -N https://filesync.homelab.local/api/v1/events/stream` opens a persistent SSE connection
- [ ] Uploading a file on one terminal triggers an SSE event on another terminal's stream
- [ ] TLS certificate is valid (browser shows lock icon or `curl` doesn't need `-k`)
- [ ] Multiple simultaneous SSE clients all receive events
- [ ] SSE clients automatically reconnect after a temporary network drop

### 3.7 Concepts Learned

- Server-Sent Events protocol and implementation
- Redis Pub/Sub for cross-pod communication
- Traefik Ingress configuration and TLS termination
- Long-lived HTTP connection management
- Fan-out notification patterns in distributed systems
- Why stateless pods + shared message bus solves the multi-pod notification problem

---

## Phase 4: True Orchestration & Scaling

**Goal:** Split the monolith into API and Worker deployments. Use Redis Streams for async task processing. Scale the API horizontally with HPA.

### 4.1 Monolith Split

Your application now becomes two separate binaries deployed as independent Kubernetes Deployments:

```
┌─────────────────────┐       ┌──────────────────────┐
│   filesync-api      │       │   filesync-worker     │
│   (cmd/api)         │       │   (cmd/worker)        │
├─────────────────────┤       ├──────────────────────┤
│ - HTTP endpoints    │       │ - No HTTP server      │
│ - SSE streaming     │       │ - Reads Redis Streams  │
│ - Accepts uploads   │       │ - Processes files      │
│ - Enqueues tasks    │──────►│ - Generates thumbnails │
│   to Redis Stream   │       │ - Computes checksums   │
│                     │       │ - Virus scanning       │
│ Scales: by request  │       │ Scales: by queue depth │
│   load (HPA on CPU) │       │   (HPA on queue len)  │
└─────────────────────┘       └──────────────────────┘
```

**Why split:**
- API pods handle synchronous HTTP traffic — they need to be fast and responsive
- Worker pods handle asynchronous background processing — they need CPU/memory but don't serve HTTP
- They scale independently: API scales on request rate, workers scale on queue depth
- A slow background task (e.g., virus scanning) doesn't block HTTP responses

### 4.2 Redis Streams for Task Queuing

Redis Streams are a durable, ordered log structure — think of them as a lightweight Kafka.

**Why Redis Streams over Redis Lists (LPUSH/BRPOP):**
- Consumer groups: multiple workers can process messages without duplication
- Acknowledgment: if a worker crashes, unacknowledged messages are re-delivered
- Message history: you can replay past messages for debugging
- Built-in backpressure via consumer group lag metrics

**Task flow:**
```
API Pod                    Redis Stream                     Worker Pod
  │                     ("filesync:tasks")                     │
  │── XADD ──────────────►│                                    │
  │  {type: "process",    │                                    │
  │   file_id: "uuid"}   │                                    │
  │                        │◄── XREADGROUP ────────────────────│
  │                        │    (consumer group: "workers")    │
  │                        │                                    │
  │                        │── message ───────────────────────►│
  │                        │                                    │── Process file
  │                        │                                    │── Update Postgres
  │                        │                                    │── PUBLISH event
  │                        │◄── XACK ──────────────────────────│
```

### 4.3 Worker Implementation

```go
// cmd/worker/main.go
// The worker runs an infinite loop:
// 1. XREADGROUP from the "filesync:tasks" stream
// 2. Process the task (download from MinIO, compute checksum, etc.)
// 3. Update the file record in PostgreSQL
// 4. PUBLISH a notification to Redis Pub/Sub
// 5. XACK the message to mark it processed
```

**Task types to implement:**
- `process_upload` — Verify checksum, extract metadata (EXIF, content type), store final record
- `generate_thumbnail` — For image files, create a thumbnail and store in MinIO
- `cleanup_orphans` — Periodic task to find MinIO objects with no Postgres record

### 4.4 Worker Deployment

```yaml
# deploy/k8s/base/worker-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: filesync-worker
  labels:
    app: filesync
    component: worker
spec:
  replicas: 1
  selector:
    matchLabels:
      app: filesync
      component: worker
  template:
    metadata:
      labels:
        app: filesync
        component: worker
    spec:
      containers:
        - name: worker
          image: ghcr.io/YOUR_USERNAME/filesync-worker:latest
          envFrom:
            - secretRef:
                name: filesync-db-credentials
            - secretRef:
                name: filesync-minio-credentials
            - secretRef:
                name: filesync-redis-credentials
            - configMapRef:
                name: filesync-config
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
            limits:
              cpu: 500m
              memory: 256Mi
          # Workers have no HTTP port — no liveness/readiness probes via HTTP
          # Use a startup probe with exec instead:
          livenessProbe:
            exec:
              command: ["/worker", "-healthcheck"]
            periodSeconds: 30
```

### 4.5 Horizontal Pod Autoscaler (HPA)

**API HPA — Scale on CPU utilization:**
```yaml
# deploy/k8s/base/hpa-api.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: filesync-api-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: filesync-api
  minReplicas: 2
  maxReplicas: 5
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
```

**Worker HPA — Scale on Redis Stream lag (custom metric):**

This requires the Prometheus adapter or KEDA. KEDA is the easier path:

```yaml
# deploy/k8s/base/keda-scaledobject.yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: filesync-worker-scaler
spec:
  scaleTargetRef:
    name: filesync-worker
  minReplicaCount: 1
  maxReplicaCount: 5
  triggers:
    - type: redis-streams
      metadata:
        address: redis-master.cache.svc.cluster.local:6379
        stream: filesync:tasks
        consumerGroup: workers
        lagCount: "10"   # Scale up when 10+ messages are pending
```

### 4.6 Validation Checklist

- [ ] API and Worker are separate Deployments with independent replica counts
- [ ] Uploading a file enqueues a task in Redis Streams
- [ ] Worker picks up the task and processes it
- [ ] If a worker crashes mid-processing, the task is retried by another worker
- [ ] HPA scales API pods up under load (`hey -n 1000 -c 50 https://filesync.homelab.local/healthz`)
- [ ] Worker replicas increase when the Redis Stream queue grows
- [ ] SSE events still work correctly with multiple API replicas

### 4.7 Concepts Learned

- Monolith to microservice decomposition (API vs. Worker pattern)
- Asynchronous task processing with durable message streams
- Consumer groups and at-least-once delivery guarantees
- Horizontal Pod Autoscaler (CPU-based and custom metrics)
- KEDA for event-driven autoscaling
- Independent scaling of synchronous and asynchronous workloads

---

## Phase 5: Observability & Operations

**Goal:** Add monitoring, alerting, and logging so you can understand what your cluster is doing and debug issues without guessing.

### 5.1 Monitoring Stack: Prometheus + Grafana

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install monitoring prometheus-community/kube-prometheus-stack \
    -n monitoring --create-namespace \
    --set prometheus.prometheusSpec.resources.requests.memory=256Mi \
    --set prometheus.prometheusSpec.resources.limits.memory=512Mi \
    --set grafana.resources.requests.memory=128Mi \
    --set grafana.resources.limits.memory=256Mi
```

### 5.2 Application Metrics

Instrument your Go application with Prometheus metrics using `prometheus/client_golang`:

**Metrics to expose:**
- `filesync_http_requests_total` — Counter, labeled by method, path, status code
- `filesync_http_request_duration_seconds` — Histogram of request latencies
- `filesync_upload_bytes_total` — Counter of total bytes uploaded
- `filesync_active_sse_connections` — Gauge of current SSE clients
- `filesync_worker_tasks_processed_total` — Counter, labeled by task type and result
- `filesync_worker_task_duration_seconds` — Histogram of task processing time

**Expose metrics endpoint:**
```go
// Add to your API server
mux.Handle("GET /metrics", promhttp.Handler())
```

**ServiceMonitor for Prometheus auto-discovery:**
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: filesync-api
  labels:
    release: monitoring
spec:
  selector:
    matchLabels:
      app: filesync
      component: api
  endpoints:
    - port: http
      path: /metrics
      interval: 15s
```

### 5.3 Logging Stack: Loki + Promtail

```bash
helm install loki grafana/loki-stack \
    -n monitoring \
    --set loki.resources.requests.memory=128Mi \
    --set loki.resources.limits.memory=256Mi
```

Your Go application already uses `slog` for structured JSON logging. Promtail automatically collects stdout/stderr from all pods and ships them to Loki. Grafana can query Loki alongside Prometheus.

### 5.4 Key Grafana Dashboards to Build

1. **API Overview** — Request rate, latency percentiles (p50, p95, p99), error rate (RED method)
2. **Upload Pipeline** — Upload rate, bytes/sec, MinIO latency, worker queue depth
3. **Infrastructure** — Pod CPU/memory usage, node capacity, PVC usage
4. **Redis** — Connected clients, commands/sec, memory usage, stream lag

### 5.5 Alerting Rules

```yaml
# Example PrometheusRule
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: filesync-alerts
spec:
  groups:
    - name: filesync
      rules:
        - alert: HighErrorRate
          expr: rate(filesync_http_requests_total{status=~"5.."}[5m]) > 0.05
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "High 5xx error rate on filesync API"

        - alert: WorkerQueueBacklog
          expr: filesync_redis_stream_lag > 100
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "Worker queue has >100 pending messages for 10m"

        - alert: PodCrashLooping
          expr: rate(kube_pod_container_status_restarts_total{namespace="default", pod=~"filesync.*"}[15m]) > 0
          for: 5m
          labels:
            severity: critical
```

### 5.6 Concepts Learned

- RED method (Rate, Errors, Duration) for service monitoring
- Prometheus metric types (counter, gauge, histogram)
- ServiceMonitor for automatic scrape target discovery
- Structured logging and log aggregation
- Building actionable Grafana dashboards
- Alert design: what to alert on, severity levels, avoiding alert fatigue

---

## Phase 6: CI/CD & GitOps

**Goal:** Automate the build-test-deploy pipeline. Every push to `main` builds a new image, runs tests, and automatically deploys to the cluster.

### 6.1 CI Pipeline (GitHub Actions)

```yaml
# .github/workflows/ci.yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_DB: filesync_test
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
        ports: ["5432:5432"]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - run: go test ./... -race -coverprofile=coverage.out
      - run: go vet ./...

  build:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v5
        with:
          context: .
          file: deploy/docker/Dockerfile
          push: true
          tags: |
            ghcr.io/${{ github.repository }}/filesync-api:${{ github.sha }}
            ghcr.io/${{ github.repository }}/filesync-api:latest
```

### 6.2 GitOps with Flux

Install Flux to watch your Git repository and automatically apply Kubernetes manifests:

```bash
flux bootstrap github \
    --owner=YOUR_USERNAME \
    --repository=Homelab \
    --path=deploy/k8s/overlays/homelab \
    --personal
```

**How it works:**
1. You push code → GitHub Actions builds and pushes a new container image
2. Flux detects the new image tag (via image automation)
3. Flux updates the Deployment manifest in Git with the new tag
4. Flux applies the updated manifest to the cluster
5. Kubernetes performs a rolling update — zero downtime

### 6.3 Validation Checklist

- [ ] Pushing to `main` triggers a CI build
- [ ] Tests run against a real PostgreSQL instance in CI
- [ ] Container image is pushed to GHCR with the commit SHA tag
- [ ] Flux detects the new image and updates the cluster
- [ ] Rolling update completes with zero downtime
- [ ] A failing test prevents the image from being built and deployed

### 6.4 Concepts Learned

- CI/CD pipeline design and GitHub Actions
- Container image tagging strategies (SHA, semver, latest)
- GitOps principles: Git as the single source of truth for cluster state
- Flux/ArgoCD for declarative, automated deployments
- Rolling updates and zero-downtime deployments

---

## Appendix

### A. Resource Budget for Homelab

Estimated resource usage for the full stack on a single node:

| Component          | CPU Request | Memory Request | Storage |
|--------------------|-------------|---------------|---------|
| filesync-api (x2)  | 100m        | 64Mi          | —       |
| filesync-worker (x1)| 100m       | 64Mi          | —       |
| PostgreSQL         | 100m        | 256Mi         | 5Gi     |
| MinIO              | 100m        | 256Mi         | 20Gi    |
| Redis              | 50m         | 64Mi          | 1Gi     |
| Traefik (k3s)      | 100m        | 128Mi         | —       |
| Prometheus+Grafana | 200m        | 384Mi         | 10Gi    |
| Loki+Promtail      | 100m        | 192Mi         | 5Gi     |
| **Total**          | **~850m**   | **~1.4Gi**    | **~41Gi** |

This fits comfortably on a machine with 2+ cores and 4GB RAM.

### B. Makefile

```makefile
.PHONY: build test run docker-build docker-push deploy

REGISTRY := ghcr.io/YOUR_USERNAME
IMAGE := filesync-api
TAG := $(shell git rev-parse --short HEAD)

build:
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker

test:
	go test ./... -race -v

run:
	go run ./cmd/api

docker-build:
	docker build -f deploy/docker/Dockerfile -t $(REGISTRY)/$(IMAGE):$(TAG) .

docker-push: docker-build
	docker push $(REGISTRY)/$(IMAGE):$(TAG)

deploy:
	kubectl apply -k deploy/k8s/overlays/homelab/

migrate:
	migrate -path migrations -database "$$DATABASE_URL" up
```

### C. Local Development with Docker Compose

For local development before deploying to k3s:

```yaml
# docker-compose.yaml
services:
  api:
    build:
      context: .
      dockerfile: deploy/docker/Dockerfile
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://filesync:dev@postgres:5432/filesync?sslmode=disable
      MINIO_ENDPOINT: minio:9000
      REDIS_URL: redis://redis:6379
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: filesync
      POSTGRES_USER: filesync
      POSTGRES_PASSWORD: dev
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U filesync"]
      interval: 5s

  minio:
    image: minio/minio
    command: server /data --console-address ":9001"
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
```

### D. Progression Order

For the clearest learning path, implement in this order:

1. **Phase 1.2** — Write the Go API server locally, test with `go run`
2. **Phase 1.3** — Containerize it with Docker
3. **Appendix C** — Run the full stack locally with Docker Compose
4. **Phase 1.4** — Deploy PostgreSQL to k3s via Helm
5. **Phase 1.5** — Deploy your API to k3s
6. **Phase 2** — Add MinIO, implement upload/download
7. **Phase 3.1** — Deploy Redis
8. **Phase 3.2–3.4** — Add SSE + Redis Pub/Sub
9. **Phase 3.5** — Configure Traefik Ingress with TLS
10. **Phase 4** — Split into API + Worker, add Redis Streams + HPA
11. **Phase 5** — Add monitoring and logging
12. **Phase 6** — Automate with CI/CD and GitOps
