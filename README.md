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

## Prerequisites

| Tool | Version |
|------|---------|
| Kubernetes | >= 1.28 |
| Helm | >= 3.12 |
| kubectl | matching your cluster |

For local development and testing:

| Tool | Version |
|------|---------|
| [Kind](https://kind.sigs.k8s.io/) | >= 0.20 |
| Docker | >= 24 |
| Go | 1.27 |
| [Mage](https://magefile.org/) | v1.17.2 |

## Quick Start with Kind

### 1. Create a Kind cluster

```bash
kind create cluster --name omniroute
```

Verify the cluster is running:

```bash
kubectl cluster-info --context kind-omniroute
```

### 2. Install the operator via Helm

```bash
helm install omniroute-operator oci://ghcr.io/vinaycharlie01/helm-charts/omniroute-operator \
  --namespace omniroute-system --create-namespace
```

Or install from source:

```bash
git clone https://github.com/vinaycharlie01/omniroute-operator.git
cd omniroute-operator
helm install omniroute-operator charts/omniroute-operator \
  --namespace omniroute-system --create-namespace
```

Verify the operator is running:

```bash
kubectl get pods -n omniroute-system
```

### 3. Create the OmniRoute secrets

OmniRoute requires three secrets: `JWT_SECRET`, `API_KEY_SECRET`, and `INITIAL_PASSWORD`.

```bash
kubectl create namespace ai-platform

kubectl create secret generic omniroute-secrets \
  --namespace ai-platform \
  --from-literal=JWT_SECRET="$(openssl rand -base64 48)" \
  --from-literal=API_KEY_SECRET="$(openssl rand -hex 32)" \
  --from-literal=INITIAL_PASSWORD="changeme"
```

### 4. Deploy an OmniRoute instance

Create an `OmniRoute` custom resource:

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

### 5. Access the OmniRoute UI

**Option A — Port forward (simplest)**

```bash
kubectl port-forward svc/omniroute 20128:20128 -n ai-platform
```

Open your browser at **http://localhost:20128**. Log in with the password you set in
`INITIAL_PASSWORD` (default: `changeme`).

**Option B — NodePort**

Change the service type in your CR:

```yaml
spec:
  service:
    type: NodePort
    port: 20128
```

Then find the assigned port:

```bash
kubectl get svc omniroute -n ai-platform -o jsonpath='{.spec.ports[0].nodePort}'
```

With Kind, the node IP is reachable via Docker. Get it and open the UI:

```bash
NODE_PORT=$(kubectl get svc omniroute -n ai-platform -o jsonpath='{.spec.ports[0].nodePort}')
echo "OmniRoute UI: http://localhost:${NODE_PORT}"
```

> **Note:** For Kind with NodePort, you need to create the cluster with port mappings. See
> [Kind extraPortMappings](https://kind.sigs.k8s.io/docs/user/configuration/#extra-port-mappings):
>
> ```yaml
> # kind-config.yaml
> kind: Cluster
> apiVersion: kind.x-k8s.io/v1alpha4
> nodes:
>   - role: control-plane
>     extraPortMappings:
>       - containerPort: 30000
>         hostPort: 20128
>         protocol: TCP
> ```
>
> ```bash
> kind create cluster --name omniroute --config kind-config.yaml
> ```
>
> Then set the NodePort to 30000 in a values override or directly in your CR.

**Option C — LoadBalancer (cloud or MetalLB)**

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

### 6. Use the OmniRoute API

Once accessible, OmniRoute exposes an OpenAI-compatible API:

```bash
# Via port-forward (step 5a)
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
kubectl delete secret omniroute-secrets -n ai-platform
kubectl delete namespace ai-platform
helm uninstall omniroute-operator -n omniroute-system
kind delete cluster --name omniroute
```

## CRD Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `spec.image.repository` | string | `diegosouzapw/omniroute` | Container image repository |
| `spec.image.tag` | string | `3.8.51` | Container image tag |
| `spec.image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `spec.replicas` | int | `1` | Number of pods (max 1 — SQLite single-writer) |
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

### Build

```bash
go install github.com/magefile/mage@v1.17.2

mage build            # Compile for current platform → bin/manager
mage buildLinux       # Cross-compile linux/amd64 + linux/arm64 → dist/
mage test             # Unit tests
mage lint             # golangci-lint
mage race             # Tests with race detection
mage coverage         # Tests with coverage profiling
mage generate         # Regenerate DeepCopy + CRD/RBAC manifests
mage helmLint         # Lint the Helm chart
mage clean            # Remove build artifacts
```

### Run locally against a Kind cluster

```bash
# Point at your Kind cluster
export KUBECONFIG="$(kind get kubeconfig-path --name=omniroute 2>/dev/null || echo ~/.kube/config)"

# Install the CRD
kubectl apply -f config/crd/bases/

# Run the operator outside the cluster
mage build
./bin/manager --health-probe-bind-address=:8081
```

### Run tests

```bash
mage test             # Unit tests
mage race             # Race detection
mage coverage         # Coverage report → coverage.out
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
