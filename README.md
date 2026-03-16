# envoy-router

Kubernetes operator that dynamically routes traffic to 100+ pods without hitting the ALB 100-target-group limit.

## How it works

Instead of one ALB target group per pod, a single **Envoy Gateway** handles all routing internally. The operator watches labelled pods and manages Gateway API resources automatically:

```
ALB → Envoy Gateway (1 target group) → HTTPRoute /pp-<id> → pod
```

For each pod with `envoy-router/enabled: "true"`, the operator creates:
- A **Service** (selector-less, same name as the pod)
- An **EndpointSlice** pointing directly at the pod IP
- An **HTTPRoute** with path prefix `/<pod-name>`

Resources are cleaned up automatically when pods are deleted.

## Prerequisites

Envoy Gateway must be installed once per cluster:

```bash
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.7.1 -n envoy-gateway-system --create-namespace
```

Envoy Gateway proxy pods always run in `envoy-gateway-system` regardless of where the `Gateway` resource is defined — this is a design decision by the Envoy Gateway project.

## Install

One `helm install` per namespace. Each install is fully self-contained — operator + Gateway scoped to that namespace, namespace-scoped RBAC only:

```bash
helm install envoy-router oci://ghcr.io/comet-ml/charts/envoy-router \
  --version 0.1.2 --namespace ns-1 --create-namespace
```

For multiple namespaces, the `GatewayClass` only needs to be created once (it is cluster-scoped). Set `gateway.createClass=false` for subsequent installs:

```bash
helm install envoy-router oci://ghcr.io/comet-ml/charts/envoy-router \
  --version 0.1.2 --namespace ns-2 --create-namespace \
  --set gateway.createClass=false
```

```
ns-1:  ALB-1 → Gateway (ns-1) → HTTPRoutes for pp-* in ns-1
ns-2:  ALB-2 → Gateway (ns-2) → HTTPRoutes for pp-* in ns-2
```

Each ALB needs one rule: forward `/*` to the Envoy proxy Service for that namespace. No per-pod rules.

## Making a pod routable

Add the label to any pod:

```yaml
metadata:
  labels:
    envoy-router/enabled: "true"
```

The pod will be reachable at `https://your-domain.com/<pod-name>/` within seconds.

## Configuration

| Value | Default | Description |
|---|---|---|
| `operator.podPort` | `8080` | Port the pods listen on |
| `operator.servicePort` | `80` | Port exposed on created Services |
| `gateway.create` | `true` | Set `false` to skip Gateway creation |
| `gateway.createClass` | `true` | Create the GatewayClass (cluster-scoped — set `false` for 2nd+ namespace installs) |
| `gateway.className` | `envoy-router` | GatewayClass name |
| `gateway.port` | `80` | Listener port on the Gateway |
| `metrics.serviceMonitor.enabled` | `false` | Create a Prometheus ServiceMonitor |
| `metrics.serviceMonitor.namespace` | release namespace | Namespace to create the ServiceMonitor in |
| `metrics.serviceMonitor.interval` | `30s` | Prometheus scrape interval |
| `metrics.serviceMonitor.additionalLabels` | `{}` | Extra labels on the ServiceMonitor (e.g. `release: prometheus`) |

## Metrics

The operator exposes Prometheus metrics at `:8080/metrics`. A `Service` is always created; enable the `ServiceMonitor` for Prometheus discovery:

```bash
helm install envoy-router oci://ghcr.io/comet-ml/charts/envoy-router \
  --namespace ns-1 --create-namespace \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.serviceMonitor.additionalLabels.release=prometheus
```

| Metric | Type | Description |
|---|---|---|
| `envoy_router_managed_pods` | Gauge | Pods currently managed by the operator |
| `controller_runtime_reconcile_total` | Counter | Reconcile calls by result (success/error/requeue) |
| `controller_runtime_reconcile_time_seconds` | Histogram | Reconcile duration |

## Test chart

`charts/envoy-router-test` is a self-contained smoke-test chart that installs `envoy-router` as a dependency alongside test pods and an internal ALB Ingress:

```bash
# First namespace (creates GatewayClass)
helm upgrade --install --namespace envoy-router-test --create-namespace \
  envoy-router-test charts/envoy-router-test

# Additional namespaces
helm upgrade --install --namespace envoy-router-test-2 --create-namespace \
  --set ingress.host=test-pp-2.dev.comet.com \
  --set envoy-router.gateway.createClass=false \
  envoy-router-test-2 charts/envoy-router-test
```

## Development

```bash
make build          # build binary
make test           # run tests
make docker-build   # build image (IMAGE_REPO / IMAGE_TAG overridable)
make docker-push
make helm-test      # run helm-unittest
make upgrade        # helm upgrade after changes
```

## Releases

Docker images and Helm charts are published to GHCR on every `v*` tag.

| Artifact | Location |
|---|---|
| Docker image | `ghcr.io/comet-ml/envoy-router` |
| Helm chart | `oci://ghcr.io/comet-ml/charts/envoy-router` |
