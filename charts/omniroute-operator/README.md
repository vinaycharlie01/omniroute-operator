# OmniRoute Operator Helm Chart

A Helm chart to install the [OmniRoute Operator](https://github.com/vinaycharlie01/omniroute-operator) — a Kubernetes Operator that deploys and manages [OmniRoute](https://github.com/diegosouzapw/OmniRoute) AI gateway instances.

## Prerequisites

- Kubernetes >= 1.28
- Helm >= 3.12

## Installation

### From OCI Registry (recommended)

```bash
helm install omniroute-operator \
  oci://ghcr.io/vinaycharlie01/helm-charts/omniroute-operator \
  --namespace omniroute-system --create-namespace
```

### From Source

```bash
git clone https://github.com/vinaycharlie01/omniroute-operator.git
helm install omniroute-operator charts/omniroute-operator \
  --namespace omniroute-system --create-namespace
```

## What Gets Installed

- The `OmniRoute` Custom Resource Definition (CRD)
- The operator Deployment with RBAC (ClusterRole, ClusterRoleBinding, ServiceAccount)
- Leader election support for high-availability deployments

## Usage

Once the operator is running, create an OmniRoute instance:

```bash
# 1. Create the required secrets
kubectl create secret generic omniroute-secrets \
  --namespace ai-platform \
  --from-literal=JWT_SECRET="$(openssl rand -base64 48)" \
  --from-literal=API_KEY_SECRET="$(openssl rand -hex 32)" \
  --from-literal=INITIAL_PASSWORD="changeme"

# 2. Deploy an OmniRoute instance
kubectl apply -f - <<EOF
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

## Configuration

### Operator Values

| Parameter | Default | Description |
|-----------|---------|-------------|
| `replicaCount` | `1` | Operator pod replicas |
| `image.repository` | `ghcr.io/vinaycharlie01/omniroute-operator` | Operator image |
| `image.tag` | `0.1.1` | Operator image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `rbac.create` | `true` | Create RBAC resources |
| `leaderElection.enabled` | `true` | Enable leader election |
| `resources.requests.cpu` | `5m` | CPU request |
| `resources.requests.memory` | `32Mi` | Memory request |
| `resources.limits.cpu` | `200m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |
| `nodeSelector` | `{}` | Node selector |
| `tolerations` | `[]` | Tolerations |
| `affinity` | `{}` | Affinity rules |
| `podSecurityContext.runAsNonRoot` | `true` | Run as non-root |
| `metrics.enabled` | `false` | Enable metrics endpoint |
| `metrics.service.port` | `8443` | Metrics service port |
| `healthProbes.livenessPath` | `/healthz` | Liveness probe path |
| `healthProbes.readinessPath` | `/readyz` | Readiness probe path |
| `healthProbes.port` | `8081` | Health probe port |

### OmniRoute CRD Spec

The operator watches `OmniRoute` custom resources and reconciles these Kubernetes objects:

| Resource | Description |
|----------|-------------|
| Deployment | OmniRoute application pods |
| Service | ClusterIP / NodePort / LoadBalancer |
| PersistentVolumeClaim | SQLite data directory |
| Ingress | Optional ingress with TLS |

Key CRD fields:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.image.repository` | string | `diegosouzapw/omniroute` | OmniRoute container image |
| `spec.image.tag` | string | `3.8.51` | Image tag |
| `spec.replicas` | int | `1` | Pod count |
| `spec.service.type` | string | `ClusterIP` | Service type |
| `spec.service.port` | int | `20128` | Service port |
| `spec.service.annotations` | map | — | Service annotations |
| `spec.resources` | ResourceRequirements | — | CPU/memory requests and limits |
| `spec.credentials.existingSecret` | string | — | Secret with JWT_SECRET, API_KEY_SECRET, INITIAL_PASSWORD |
| `spec.config.existingConfigMap` | string | — | ConfigMap with env vars (injected via envFrom) |
| `spec.persistence.enabled` | bool | `true` | Create a PVC |
| `spec.persistence.size` | string | `1Gi` | PVC size |
| `spec.persistence.storageClass` | string | — | StorageClass name |
| `spec.probes.livenessPath` | string | `/api/monitoring/health` | Liveness probe path |
| `spec.probes.readinessPath` | string | `/api/monitoring/health` | Readiness probe path |
| `spec.probes.startupPath` | string | `/api/monitoring/health` | Startup probe path |
| `spec.ingress.enabled` | bool | `false` | Create an Ingress |
| `spec.ingress.className` | string | — | IngressClass name |
| `spec.ingress.host` | string | — | Ingress hostname |
| `spec.ingress.tls.secretName` | string | — | TLS secret name |
| `spec.ingress.annotations` | map | — | Ingress annotations |
| `spec.redis.url` | string | — | Redis URL for rate limiting |
| `spec.redis.existingSecret` | string | — | Secret with REDIS_URL key |
| `spec.redis.keyPrefix` | string | — | Redis key prefix |
| `spec.encryption.existingSecret` | string | — | Secret with STORAGE_ENCRYPTION_KEY |
| `spec.encryption.keyVersion` | string | — | Encryption key version |
| `spec.basePath` | string | — | Reverse proxy base path |
| `spec.env` | []EnvVar | — | Additional environment variables |

### Auto-Derived Environment Variables

The operator automatically sets:

| Variable | Source | Description |
|----------|--------|-------------|
| `NODE_OPTIONS` | `spec.resources.limits.memory` | `--max-old-space-size=<75% of limit>` |
| `OMNIROUTE_BASE_PATH` | `spec.basePath` | Reverse proxy base path |
| `REDIS_URL` | `spec.redis.url` or secret | Redis connection URL |
| `REDIS_KEY_PREFIX` | `spec.redis.keyPrefix` | Redis key namespace |
| `STORAGE_ENCRYPTION_KEY` | `spec.encryption.existingSecret` | Database encryption key |
| `STORAGE_ENCRYPTION_KEY_VERSION` | `spec.encryption.keyVersion` | Encryption key version |

## Uninstalling

```bash
# Remove OmniRoute instances first
kubectl delete omniroute --all -A

# Uninstall the operator
helm uninstall omniroute-operator -n omniroute-system
```

## License

Apache License 2.0
