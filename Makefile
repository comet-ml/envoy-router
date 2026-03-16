IMAGE_REPO ?= ghcr.io/comet-ml/envoy-router
IMAGE_TAG  ?= latest
ENVOY_GATEWAY_VERSION ?= v1.7.1
CHART_NAMESPACE ?= envoy-router

.PHONY: build test lint tidy docker-build docker-push \
        envoy-gateway-install install upgrade uninstall

build:
	go build -o bin/manager ./cmd/main.go

test:
	go test ./... -v

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

docker-build:
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .

docker-push: docker-build
	docker push $(IMAGE_REPO):$(IMAGE_TAG)

## Install Envoy Gateway CRDs and controller (prerequisite)
envoy-gateway-install:
	helm install eg oci://docker.io/envoyproxy/gateway-helm \
		--version $(ENVOY_GATEWAY_VERSION) \
		-n envoy-gateway-system \
		--create-namespace

install:
	helm install envoy-router ./charts/envoy-router \
		--namespace $(CHART_NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMAGE_REPO) \
		--set image.tag=$(IMAGE_TAG)

upgrade:
	helm upgrade envoy-router ./charts/envoy-router \
		--namespace $(CHART_NAMESPACE) \
		--set image.repository=$(IMAGE_REPO) \
		--set image.tag=$(IMAGE_TAG)

uninstall:
	helm uninstall envoy-router -n $(CHART_NAMESPACE)
