# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`envoy-router` is a Kubernetes operator that solves the ALB 100-target-group limit for large pod fleets. It watches Pods labelled `envoy-router/enabled: "true"` and dynamically maintains:
- A selector-less **Service** (same name/namespace as the pod)
- An **Endpoints** object pointing directly at `pod.Status.PodIP`
- An **HTTPRoute** (Gateway API) mapping `/<pod-name>` → the Service

Traffic flows: `ALB → Envoy Gateway (single target group) → per-pod HTTPRoute → pod`

## Commands

```bash
# Build
make build              # outputs bin/manager
go build ./...

# Test
make test               # go test ./... -v

# Lint
make lint               # requires golangci-lint

# Dependencies
make tidy               # go mod tidy

# Docker
make docker-build       # IMAGE_REPO and IMAGE_TAG are overridable
make docker-push

# Deploy (run once — installs Envoy Gateway CRDs + controller)
make envoy-gateway-install

# Deploy operator + GatewayClass + Gateway
make install            # helm install into envoy-router namespace
make upgrade
make uninstall
```

## Architecture

```
cmd/main.go                   entrypoint; flag parsing, scheme setup, manager init
internal/controller/
  pod_controller.go           PodReconciler — the only controller
charts/envoy-router/          Helm chart (operator + GatewayClass + Gateway)
.github/workflows/
  docker.yml                  multi-arch image build → ghcr.io/comet-ml/envoy-router
  helm.yml                    Helm chart publish → oci://ghcr.io/comet-ml/charts
Dockerfile                    multi-stage: golang:1.24-alpine → distroless/static:nonroot
```

### Reconciler logic (`PodReconciler`)

Triggered on any change to a Pod where `envoy-router/enabled: "true"`:

1. **Pod has no IP yet** → return (re-triggered automatically when status updates)
2. **Pod being deleted** → finalizer cleanup: delete HTTPRoute → Endpoints → Service
3. **Pod running** → idempotently create/update Service + Endpoints + HTTPRoute

The Service has no selector; the operator manually controls Endpoints so no pod label requirements exist.

### Key configuration flags

| Flag | Default | Purpose |
|---|---|---|
| `--gateway-name` | `envoy-router` | Gateway resource to attach HTTPRoutes to |
| `--gateway-namespace` | `envoy-router` | Namespace of that Gateway |
| `--service-port` | `80` | Port on created Services |
| `--pod-port` | `8080` | Target port on pods |
| `--watch-namespace` | `""` (all) | Restrict pod watch to one namespace |

### Helm values of note

- `gateway.create` — set `false` to manage GatewayClass/Gateway yourself
- `gateway.allowedRouteNamespaces` — `All` by default (required when pods span namespaces)
- `operator.watchNamespace` — scope the operator to a single namespace

## CI/CD

Both workflows trigger on `main` push and `v*` tags.

**docker.yml** — builds `linux/amd64` and `linux/arm64` on native runners in parallel, merges into a multi-arch manifest, attests SBOM (via syft) and build provenance. Publishes to `ghcr.io/comet-ml/envoy-router`.

**helm.yml** — on `v*` tags, patches `Chart.yaml` version from the git tag, then packages and pushes to `oci://ghcr.io/comet-ml/charts`. On `main` push, publishes with the version already in `Chart.yaml`.

To release a new version: `git tag vX.Y.Z && git push --tags`

## Stack

- Go 1.23 / toolchain 1.24
- controller-runtime v0.20.x (k8s v0.32.x)
- sigs.k8s.io/gateway-api v1.2.0
- Envoy Gateway v1.2.0
