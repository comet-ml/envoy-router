# envoy-router

Kubernetes operator that dynamically routes traffic to 100+ pods without hitting the ALB 100-target-group limit.

## How it works

Instead of one ALB target group per pod, a single **Envoy Gateway** handles all routing internally. The operator watches labelled pods and manages Gateway API resources automatically:

```
ALB → Envoy Gateway (1 target group) → HTTPRoute /pp-<id> → pod
```

For each pod with `envoy-router/enabled: "true"`, the operator creates:
- A **Service** (selector-less, same name as the pod)
- An **Endpoints** pointing directly at the pod IP
- An **HTTPRoute** with path prefix `/<pod-name>`

Resources are cleaned up automatically when pods are deleted.

## Prerequisites

- Kubernetes cluster (EKS or any)
- Helm 3
- Envoy Gateway installed:

```bash
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.7.1 -n envoy-gateway-system --create-namespace
```

## Install

Run once per namespace. Each install is fully self-contained — operator + Gateway scoped to that namespace:

```bash
helm install envoy-router oci://ghcr.io/comet-ml/charts/envoy-router \
  --version 0.1.0 \
  --namespace ns-1 --create-namespace
```

The operator watches only its own namespace and attaches HTTPRoutes to the Gateway in the same namespace.

## Making a pod routable

Add the label to any pod:

```yaml
metadata:
  labels:
    envoy-router/enabled: "true"
```

The pod will be reachable at `https://your-domain.com/<pod-name>/` within seconds.

## Configuration

Key Helm values:

| Value | Default | Description |
|---|---|---|
| `operator.podPort` | `8080` | Port the pods listen on |
| `gateway.create` | `true` | Set `false` to skip Gateway creation |
| `gateway.className` | `eg` | GatewayClass name (Envoy Gateway installs `eg` by default) |
| `gateway.port` | `80` | Listener port on the Gateway |

## Multi-namespace setup

Each namespace is fully independent — just repeat the install:

```bash
helm install envoy-router oci://ghcr.io/comet-ml/charts/envoy-router \
  --version 0.1.0 --namespace ns-1 --create-namespace

helm install envoy-router oci://ghcr.io/comet-ml/charts/envoy-router \
  --version 0.1.0 --namespace ns-2 --create-namespace
```

```
ns-1:  ALB-1 → Gateway (ns-1) → HTTPRoutes for pp-* in ns-1
ns-2:  ALB-2 → Gateway (ns-2) → HTTPRoutes for pp-* in ns-2
```

**ALB configuration:** one rule per ALB — forward `/*` to the Envoy Gateway service in that namespace. No per-pod rules needed.

## Development

```bash
make build          # build binary
make test           # run tests
make docker-build   # build image (IMAGE_REPO / IMAGE_TAG overridable)
make docker-push
make upgrade        # helm upgrade after changes
```

## Releases

Docker images and Helm charts are published to GHCR on every push to `main` and on `v*` tags.

| Artifact | Location |
|---|---|
| Docker image | `ghcr.io/comet-ml/envoy-router` |
| Helm chart | `oci://ghcr.io/comet-ml/charts/envoy-router` |
