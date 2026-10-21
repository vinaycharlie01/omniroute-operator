# OmniRoute Operator

A Kubernetes Operator built with [Kubebuilder](https://kubebuilder.io/) to deploy and manage
[OmniRoute](https://github.com/diegosouzapw/OmniRoute) — a unified AI proxy/router — in
self-hosted Kubernetes environments.

The operator watches `OmniRoute` custom resources and reconciles Deployments, Services,
PersistentVolumeClaims, and Ingresses so you get a fully managed OmniRoute instance with a
single `kubectl apply`.

## Features

- Declarative OmniRoute lifecycle via a Kubernetes CRD (`gateway.omniroute.ai/v1alpha1`)
- Automatic Deployment, Service, PVC, and Ingress reconciliation
- Credentials injection from existing Secrets
- Configuration injection from existing ConfigMaps (envFrom)
- Auto-derived `NODE_OPTIONS` from memory limits (prevents OOM kills)
- Configurable health / readiness / startup probes (defaults to OmniRoute's `/api/monitoring/health`)
- Redis integration for rate limiting (URL or Secret-backed)
- Database encryption at rest (STORAGE_ENCRYPTION_KEY from Secret)
- Ingress resource management with TLS, className, and annotations
- Reverse proxy support via `basePath` (OMNIROUTE_BASE_PATH)
- Service annotations for cloud provider integrations (e.g., AWS NLB)
- Additional environment variables via `spec.env`
- Configurable persistence (SQLite data directory)
- Leader election for high-availability operator deployments
- Multi-arch container images (linux/amd64, linux/arm64)
- Helm chart for operator installation
- One-command local cluster via [k3d](https://k3d.io/) + [Nava](https://github.com/nirantaraai/nava) mage targets

## Prerequisites

| Tool | Version |
|------|---------|
| Kubernetes | >= 1.28 |
| Helm | >= 3.12 |
| kubectl | matching your cluster |

For local development and testing:

| Tool | Version |
|------|---------|
| [k3d](https://k3d.io/) | >= 5.6 |
| Docker | >= 24 |
| Go | 1.27 |
| [Task](https://taskfile.dev/) | v3 |

## Quick Start — Local Cluster with k3d

The operator ships with a `k3d.yaml` config and Task targets that handle
the entire local cluster lifecycle: create, bootstrap, install operator, and teardown.

### 1. Create the cluster and install the operator

```bash
go install github.com/go-task/task/v3/cmd/task@latest

task build
```

This single command:
- Creates a k3d cluster named `omniroute` with port `20128` forwarded to your host
- Bootstraps infrastructure (creates `omniroute-system` and `ai-platform` namespaces)
- Installs the operator Helm chart into `omniroute-system`

Verify everything is running:

```bash
task lint
kubectl get pods -n omniroute-system
```

### 2. Create the OmniRoute secrets

OmniRoute requires three secrets: `JWT_SECRET`, `API_KEY_SECRET`, and `INITIAL_PASSWORD`.

```bash
kubectl create secret generic omniroute-secrets \
  --namespace ai-platform \
  --from-literal=JWT_SECRET="$(openssl rand -base64 48)" \
  --from-literal=API_KEY_SECRET="$(openssl rand -hex 32)" \
  --from-literal=INITIAL_PASSWORD="changeme"
```

### 3. (Optional) Create a ConfigMap for environment configuration

All keys in the referenced ConfigMap are injected as environment variables via `envFrom`:

```bash
kubectl create configmap omniroute-config \
  --namespace ai-platform \
  --from-literal=PORT=20128 \
  --from-literal=APP_LOG_LEVEL=info \
  --from-literal=REQUIRE_API_KEY=false
```

### 4. Deploy an OmniRoute instance

```bash
kubectl apply -f config/samples/gateway_v1alpha1_omniroute.yaml
```

Or create a custom resource:

```bash
kubectl apply -f - <<'EOF'
apiVersion: gateway.omniroute.ai/v1alpha1
kind: OmniRoute
metadata:
  name: omniroute
  namespace: ai-platform
spec:
  image:
    repository: diegosouzapw/omniroute
    tag: "3.8.51"
  replicas: 1
  service:
    type: ClusterIP
    port: 20128
  resources:
    requests:
      cpu: 250m
      memory: 512Mi
    limits:
      cpu: "1"
      memory: 1Gi
  credentials:
    existingSecret: omniroute-secrets
  config:
    existingConfigMap: omniroute-config
  persistence:
    enabled: true
    size: 1Gi
EOF
```

Wait for the pod to be ready:

```bash
kubectl wait --for=condition=available deployment/omniroute \
  --namespace ai-platform --timeout=120s
```

### 5. Access the OmniRoute UI

**Option A — Port forward (works on any cluster)**

```bash
kubectl port-forward svc/omniroute 20128:20128 -n ai-platform
```

Open **http://localhost:20128** in your browser.

**Option B — Direct access via k3d port mapping (already configured)**

The `k3d.yaml` maps port `20128` from the cluster to your host. If the OmniRoute
Service is a `NodePort` targeting port 20128, you can access it directly at
**http://localhost:20128** without port-forwarding.

To use NodePort, set the service type in your CR:

```yaml
spec:
  service:
    type: NodePort
    port: 20128
```

**Option C — LoadBalancer (cloud clusters or MetalLB)**

```yaml
spec:
  service:
    type: LoadBalancer
    port: 20128
```

```bash
kubectl get svc omniroute -n ai-platform -w
# Wait for EXTERNAL-IP, then open http://<EXTERNAL-IP>:20128
```

**Option D — Ingress (production)**

```yaml
spec:
  ingress:
    enabled: true
    className: nginx
    host: omniroute.example.com
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod
    tls:
      secretName: omniroute-tls
```

### 6. Log in

Use the password set in `INITIAL_PASSWORD` (default: `changeme`).
The dashboard provides access to provider management, routing configuration, and monitoring.

### 7. Use the OmniRoute API

OmniRoute exposes an OpenAI-compatible API:

```bash
curl http://localhost:20128/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### 8. Teardown

```bash
kubectl delete omniroute omniroute -n ai-platform
# cleanup cluster if needed
```

## k3d Configuration

The cluster is configured in `k3d.yaml` (Nava k3d format):

```yaml
cluster:
  name: omniroute
  servers: 1
  agents: 0
  ports:
    - "20128:20128@server:0"    # OmniRoute UI + API
  k3sArgs:
    - "--disable=traefik@server:0"

infrastructure:
  kustomizePath: config/k3d/infrastructure

releases:
  - name: omniroute-operator
    chart: charts/omniroute-operator
    namespace: omniroute-system
    wait: true
    timeout: "120s"
```

You can customize ports, add agents for multi-node testing, or add extra Helm releases.

## Helm Installation (Production)

For production clusters, install the operator directly via Helm:

```bash
helm install omniroute-operator oci://ghcr.io/vinaycharlie01/helm-charts/omniroute-operator \
  --namespace omniroute-system --create-namespace
```

Or from the source chart:

```bash
helm install omniroute-operator charts/omniroute-operator \
  --namespace omniroute-system --create-namespace
```

### Helm Values

| Value | Default | Description |
|-------|---------|-------------|
| `replicaCount` | `1` | Operator pod replicas |
| `image.repository` | `ghcr.io/vinaycharlie01/omniroute-operator` | Operator image |
| `image.tag` | `""` (appVersion) | Operator image tag |
| `leaderElection.enabled` | `true` | Enable leader election |
| `resources.requests.cpu` | `10m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |

## CRD Reference

### Core

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.image.repository` | string | `diegosouzapw/omniroute` | OmniRoute container image |
| `spec.image.tag` | string | `3.8.51` | Image tag |
| `spec.image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `spec.replicas` | int | `1` | Pod count (max 1 — SQLite single-writer) |
| `spec.resources` | ResourceRequirements | — | CPU/memory requests and limits |
| `spec.env` | []EnvVar | — | Additional environment variables |

### Service

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.service.type` | string | `ClusterIP` | Service type: ClusterIP, NodePort, LoadBalancer |
| `spec.service.port` | int | `20128` | Service port |
| `spec.service.annotations` | map[string]string | — | Service annotations (e.g., for AWS NLB) |

### Configuration & Credentials

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.config.existingConfigMap` | string | — | ConfigMap with OmniRoute env vars (injected via envFrom) |
| `spec.credentials.existingSecret` | string | — | Secret with JWT_SECRET, API_KEY_SECRET, INITIAL_PASSWORD |

### Persistence

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.persistence.enabled` | bool | `true` | Create a PVC for the data directory |
| `spec.persistence.size` | string | `1Gi` | PVC storage size |
| `spec.persistence.storageClass` | string | cluster default | StorageClass name |
| `spec.persistence.accessMode` | string | `ReadWriteOnce` | PVC access mode |

### Health Probes

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.probes.livenessPath` | string | `/api/monitoring/health` | HTTP path for liveness probe |
| `spec.probes.readinessPath` | string | `/api/monitoring/health` | HTTP path for readiness probe |
| `spec.probes.startupPath` | string | `/api/monitoring/health` | HTTP path for startup probe |

The operator auto-configures all three probes with sensible defaults (initialDelaySeconds, periodSeconds, failureThreshold). All paths default to OmniRoute's health endpoint.

### Ingress

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.ingress.enabled` | bool | `false` | Create an Ingress resource |
| `spec.ingress.className` | string | — | IngressClass name (e.g., `nginx`) |
| `spec.ingress.host` | string | — | Hostname for the Ingress rule |
| `spec.ingress.tls.secretName` | string | — | TLS Secret name |
| `spec.ingress.annotations` | map[string]string | — | Ingress annotations |

### Redis (Rate Limiting)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.redis.url` | string | — | Redis connection URL (e.g., `redis://redis:6379`) |
| `spec.redis.existingSecret` | string | — | Secret containing a `REDIS_URL` key |
| `spec.redis.keyPrefix` | string | — | Redis key namespace prefix |

### Encryption (Database at Rest)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.encryption.existingSecret` | string | — | Secret containing `STORAGE_ENCRYPTION_KEY` |
| `spec.encryption.keyVersion` | string | — | Encryption key version (for key rotation) |

### Reverse Proxy

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.basePath` | string | — | Sets `OMNIROUTE_BASE_PATH` for reverse proxy deployments (e.g., `/omniroute`) |

### Auto-Derived Environment Variables

The operator automatically sets the following based on the CR spec:

| Variable | Source | Description |
|----------|--------|-------------|
| `NODE_OPTIONS` | `spec.resources.limits.memory` | `--max-old-space-size=<75% of memory limit in MB>` — prevents Node.js OOM kills |
| `OMNIROUTE_BASE_PATH` | `spec.basePath` | Reverse proxy base path |
| `REDIS_URL` | `spec.redis.url` or `spec.redis.existingSecret` | Redis connection URL |
| `REDIS_KEY_PREFIX` | `spec.redis.keyPrefix` | Redis key namespace prefix |
| `STORAGE_ENCRYPTION_KEY` | `spec.encryption.existingSecret` | Database encryption key (from Secret) |
| `STORAGE_ENCRYPTION_KEY_VERSION` | `spec.encryption.keyVersion` | Encryption key version |

## Development

### Taskfile Targets

```bash
go install github.com/go-task/task/v3/cmd/task@latest

# Build
task build            # Compile for current platform → bin/manager
task build-linux      # Cross-compile linux/amd64 + linux/arm64 → dist/

# Test
task test             # Unit tests
task race             # Tests with race detection
task coverage         # Tests with coverage → coverage.out
task bench            # Benchmarks
task integration      # Integration tests (requires Docker)

# Quality
task lint             # golangci-lint
task vet              # go vet
task govulncheck      # Vulnerability scanning

# Code generation
task generate         # Regenerate DeepCopy + CRD/RBAC manifests

# Helm
task helm-lint        # Lint the Helm chart
task helm-package     # Package the chart
task helm-push        # Package and push to OCI registry

# Cleanup
task clean            # Remove build artifacts
```

### Run the operator locally

```bash
# Install the CRD into your cluster
kubectl apply -f config/crd/bases/

# Run the operator outside the cluster (for debugging)
task build
./bin/manager --health-probe-bind-address=:8081
```

## CI/CD

The GitHub Actions pipeline (`.github/workflows/ci.yml`) runs on every push and PR:

| Job | Description |
|-----|-------------|
| **Lint** | golangci-lint v2.14.0 |
| **Unit Tests** | `task test` |
| **Race Tests** | `task race` |
| **Coverage** | `task coverage` + Codecov upload |
| **Security** | govulncheck + Trivy filesystem scan |
| **Generate** | Verify generated code is up to date |
| **Build** | `task build` + artifact upload |
| **Helm Lint** | `task helm-lint` |
| **Container** | Multi-arch build (amd64/arm64) via Buildx + QEMU, pushed to GHCR |
| **Helm Release** | OCI push to `ghcr.io/vinaycharlie01/helm-charts` (tag pushes only) |

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
