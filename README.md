# OmniRoute Operator

A Kubernetes Operator built with [Kubebuilder](https://kubebuilder.io/) to deploy and manage
[OmniRoute](https://github.com/diegosouzapw/OmniRoute) — a unified AI proxy/router — in
self-hosted Kubernetes environments.

The operator watches `OmniRoute` custom resources and reconciles Deployments, Services,
and PersistentVolumeClaims so you get a fully managed OmniRoute instance with a single
`kubectl apply`.

## Features

- Declarative OmniRoute lifecycle via a Kubernetes CRD (`gateway.omniroute.ai/v1alpha1`)
- Automatic Deployment, Service, and PVC reconciliation
- Credentials injection from existing Secrets
- Configurable persistence (SQLite data directory)
- Health / readiness / startup probes
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
| [Mage](https://magefile.org/) | v1.17.2 |

## Quick Start — Local Cluster with k3d

The operator ships with a `k3d.yaml` config and Nava-powered mage targets that handle
the entire local cluster lifecycle: create, bootstrap, install operator, and teardown.

### 1. Create the cluster and install the operator

```bash
go install github.com/magefile/mage@v1.17.2

mage k3d:up
```

This single command:
- Creates a k3d cluster named `omniroute` with port `20128` forwarded to your host
- Bootstraps infrastructure (creates `omniroute-system` and `ai-platform` namespaces)
- Installs the operator Helm chart into `omniroute-system`

Verify everything is running:

```bash
mage k3d:status
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

### 3. Deploy an OmniRoute instance

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

### 4. Access the OmniRoute UI

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

### 5. Log in

Use the password set in `INITIAL_PASSWORD` (default: `changeme`).
The dashboard provides access to provider management, routing configuration, and monitoring.

### 6. Use the OmniRoute API

OmniRoute exposes an OpenAI-compatible API:

```bash
curl http://localhost:20128/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### 7. Teardown

```bash
kubectl delete omniroute omniroute -n ai-platform
mage k3d:down
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

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.image.repository` | string | `diegosouzapw/omniroute` | OmniRoute container image |
| `spec.image.tag` | string | `3.8.51` | Image tag |
| `spec.image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `spec.replicas` | int | `1` | Pod count (max 1 — SQLite single-writer) |
| `spec.service.type` | string | `ClusterIP` | Service type: ClusterIP, NodePort, LoadBalancer |
| `spec.service.port` | int | `20128` | Service port |
| `spec.resources` | ResourceRequirements | — | CPU/memory requests and limits |
| `spec.credentials.existingSecret` | string | — | Secret with JWT_SECRET, API_KEY_SECRET, INITIAL_PASSWORD |
| `spec.config.existingConfigMap` | string | — | ConfigMap with OmniRoute env vars |
| `spec.persistence.enabled` | bool | `true` | Create a PVC for the data directory |
| `spec.persistence.size` | string | `1Gi` | PVC storage size |
| `spec.persistence.storageClass` | string | cluster default | StorageClass name |
| `spec.persistence.accessMode` | string | `ReadWriteOnce` | PVC access mode |
| `spec.env` | []EnvVar | — | Additional environment variables |

## Development

### Mage Targets

```bash
go install github.com/magefile/mage@v1.17.2

# Build
mage build            # Compile for current platform → bin/manager
mage buildLinux       # Cross-compile linux/amd64 + linux/arm64 → dist/

# Test
mage test             # Unit tests
mage race             # Tests with race detection
mage coverage         # Tests with coverage → coverage.out
mage bench            # Benchmarks

# Quality
mage lint             # golangci-lint
mage vet              # go vet
mage govulncheck      # Vulnerability scanning

# Code generation
mage generate         # Regenerate DeepCopy + CRD/RBAC manifests

# Helm
mage helmLint         # Lint the Helm chart
mage helmPackage      # Package the chart
mage helmPush         # Package and push to OCI registry

# k3d local cluster
mage k3d:up           # Create cluster + bootstrap + install operator
mage k3d:down         # Tear down the cluster
mage k3d:create       # Create just the cluster
mage k3d:delete       # Delete just the cluster
mage k3d:bootstrap    # Apply infrastructure manifests
mage k3d:status       # Show cluster state

# Cleanup
mage clean            # Remove build artifacts
```

### Run the operator locally

```bash
# Install the CRD into your cluster
kubectl apply -f config/crd/bases/

# Run the operator outside the cluster (for debugging)
mage build
./bin/manager --health-probe-bind-address=:8081
```

## CI/CD

The GitHub Actions pipeline (`.github/workflows/ci.yml`) runs on every push and PR:

| Job | Description |
|-----|-------------|
| **Lint** | golangci-lint v2.14.0 |
| **Unit Tests** | `mage test` |
| **Race Tests** | `mage race` |
| **Coverage** | `mage coverage` + Codecov upload |
| **Security** | govulncheck + Trivy filesystem scan |
| **Generate** | Verify generated code is up to date |
| **Build** | `mage build` + artifact upload |
| **Helm Lint** | `mage helmLint` |
| **Container** | Multi-arch build (amd64/arm64) via Buildx + QEMU, pushed to GHCR |
| **Helm Release** | OCI push to `ghcr.io/vinaycharlie01/helm-charts` (tag pushes only) |

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
