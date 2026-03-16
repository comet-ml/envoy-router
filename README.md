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
make envoy-gateway-install
# or manually:
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.2.0 -n envoy-gateway-system --create-namespace
```

## Install

```bash
make install
# or:
helm install envoy-router ./charts/envoy-router \
  --namespace envoy-router --create-namespace \
  --set image.repository=ghcr.io/comet-ml/envoy-router \
  --set image.tag=latest
```

This installs the operator, a `GatewayClass`, and a `Gateway` in the `envoy-router` namespace.

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
| `operator.watchNamespace` | `""` | Restrict to one namespace (empty = all) |
| `gateway.allowedRouteNamespaces` | `All` | Namespaces that can attach HTTPRoutes |
| `gateway.create` | `true` | Set `false` to bring your own Gateway |

## Development

```bash
make build          # build binary
make test           # run tests
make docker-build   # build image (IMAGE_REPO / IMAGE_TAG overridable)
make docker-push
make upgrade        # helm upgrade after changes
```
