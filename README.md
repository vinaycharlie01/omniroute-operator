# OmniRoute Operator

A Kubernetes Operator built with [Kubebuilder](https://kubebuilder.io/) to deploy and manage
[OmniRoute](https://github.com/diegosouzapw/OmniRoute) — a unified AI proxy/router — in
self-hosted Kubernetes environments.

The operator watches `OmniRoute` custom resources and reconciles Deployments, Services,
PersistentVolumeClaims, Ingresses, and Gateway API resources (`GatewayClass`, `Gateway`, `HTTPRoute`) so you get a fully managed OmniRoute instance with a single `kubectl apply`.

## Features

- Declarative OmniRoute lifecycle via a Kubernetes CRD (`gateway.omniroute.ai/v1alpha1`)
- Automatic Deployment, Service, PVC, Ingress, and Gateway API reconciliation
- Credentials injection from existing Secrets (JWT_SECRET, API_KEY_SECRET, INITIAL_PASSWORD)
- Configuration injection from existing ConfigMaps (envFrom)
- Auto-derived `NODE_OPTIONS` from memory limits (prevents OOM kills)
- Configurable health / readiness / startup probes with sensible defaults
- Redis integration for rate limiting (URL or Secret-backed)
- Database encryption at rest (`STORAGE_ENCRYPTION_KEY` from Secret)
- Ingress resource management with TLS, className, and annotations
- Kubernetes Gateway API support (`GatewayClass`, `Gateway`, `HTTPRoute`)
- Reverse proxy support via `basePath` (`OMNIROUTE_BASE_PATH`)
- Custom Pod DNS configuration (`dnsConfig` with public nameservers like 8.8.8.8, 1.1.1.1)
- Service annotations for cloud provider integrations (e.g., AWS NLB)
- Additional environment variables via `spec.env`
- Configurable persistence (SQLite data directory with ReadWriteOnce block storage)
- Leader election for high-availability operator deployments
- Multi-arch container images (linux/amd64, linux/arm64)
- Helm chart for operator installation
- One-command local cluster via [k3d](https://k3d.io/) + Task targets

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

The operator ships with a `Taskfile.yml` and targets that handle the entire local cluster lifecycle.

### 1. Create the cluster and install the operator

```bash
go install github.com/go-task/task/v3/cmd/task@latest
task build
```

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

### 3. Deploy an OmniRoute instance

```bash
kubectl apply -f config/samples/gateway_v1alpha1_omniroute.yaml
```

Or apply a custom resource directly:

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

### 4. Access the OmniRoute UI & API

```bash
kubectl port-forward svc/omniroute 20128:20128 -n ai-platform
```

Open **http://localhost:20128** in your browser and log in with `INITIAL_PASSWORD` (default: `changeme`).

---

## CRD Reference

### Core & Image
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.image.repository` | string | `diegosouzapw/omniroute` | OmniRoute container image |
| `spec.image.tag` | string | `3.8.51` | Image tag |
| `spec.image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `spec.replicas` | int | `1` | Pod count (must be 1 — SQLite single-writer) |
| `spec.resources` | ResourceRequirements | — | CPU/memory requests and limits |
| `spec.env` | []EnvVar | — | Additional environment variables |
| `spec.basePath` | string | — | Reverse proxy base path (`OMNIROUTE_BASE_PATH`) |

### Service & Persistence
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.service.type` | string | `ClusterIP` | Service type: ClusterIP, NodePort, LoadBalancer |
| `spec.service.port` | int | `20128` | Service port |
| `spec.service.annotations` | map[string]string | — | Service annotations (e.g., AWS NLB) |
| `spec.persistence.enabled` | bool | `true` | Create a PVC for the data directory |
| `spec.persistence.size` | string | `1Gi` | PVC storage size |
| `spec.persistence.storageClass` | string | cluster default | StorageClass name |
| `spec.persistence.accessMode` | string | `ReadWriteOnce` | PVC access mode (must be RWO) |

### Credentials & Configuration
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.credentials.existingSecret` | string | — | Secret with JWT_SECRET, API_KEY_SECRET, INITIAL_PASSWORD |
| `spec.credentials.initialPassword` | string | — | Initial password set directly in spec (managed secret) |
| `spec.config.existingConfigMap` | string | — | ConfigMap with environment variables (injected via envFrom) |

### Ingress & Gateway API
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.ingress.enabled` | bool | `false` | Create a standard Ingress resource |
| `spec.ingress.className` | string | — | IngressClass name (e.g., `nginx`) |
| `spec.ingress.host` | string | — | Hostname for Ingress rule |
| `spec.ingress.tls.secretName` | string | — | TLS Secret name |
| `spec.gateway.enabled` | bool | `false` | Create Kubernetes Gateway API resources |
| `spec.gateway.gatewayClassName` | string | `eg` | GatewayClass name (e.g., `eg` for Envoy Gateway) |
| `spec.gateway.gatewayName` | string | `eg` | Gateway resource name |
| `spec.gateway.hostname` | string | — | Hostname for HTTPRoute rule |

### Redis & Encryption
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.redis.url` | string | — | Redis connection URL (`redis://redis:6379`) |
| `spec.redis.existingSecret` | string | — | Secret containing `REDIS_URL` |
| `spec.redis.keyPrefix` | string | — | Redis key namespace prefix |
| `spec.encryption.existingSecret` | string | — | Secret containing `STORAGE_ENCRYPTION_KEY` |
| `spec.encryption.keyVersion` | string | — | Encryption key version for rotation |

---

## Development & Taskfile Targets

```bash
task build            # Compile manager binary
task test             # Run unit tests
task integration      # Run integration tests
task lint             # Run golangci-lint
task generate         # Regenerate CRDs, RBAC, and DeepCopy code
task helm-package     # Package Helm chart
```

## License

Licensed under the Apache License, Version 2.0.
