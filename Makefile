# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: 
	cd operators/multiclusterobservability && make deploy

# UnDeploy controller from the configured Kubernetes cluster in ~/.kube/config
undeploy:
	cd operators/multiclusterobservability && make undeploy

# Build the docker image
docker-build:
	cd operators/multiclusterobservability && make docker-build

# Push the docker image
docker-push:
	cd operators/multiclusterobservability && make docker-push

.PHONY: unit-tests
unit-tests:
	cd loaders/dashboards; go test `go list ./... | grep -v test`
	cd operators/endpointmetrics; go test `go list ./... | grep -v test`
	cd operators/multiclusterobservability; go test `go list ./... | grep -v test`
	cd proxy; go test `go list ./... | grep -v test`
	cd collectors/metrics; go test `go list ./... | grep -v test`

.PHONY: e2e-tests

e2e-tests: test-e2e-setup
	@echo "Running E2E Tests.."
	@./cicd-scripts/run-e2e-tests.sh

test-e2e-setup:
	@echo "Seting up E2E Tests environment..."
ifdef COMPONENT_IMAGE_PIPELINE
	# override the image for the e2e test
	@./cicd-scripts/setup-e2e-tests.sh -a install -p $(COMPONENT_IMAGE_PIPELINE)
else
  ifdef COMPONENT_IMAGE_NAMES
	# override the image for the e2e test
	@./cicd-scripts/setup-e2e-tests.sh -a install -i $(COMPONENT_IMAGE_NAMES)
  else
	# fall back to the latest snapshot image from quay.io for the e2e test
	@./cicd-scripts/setup-e2e-tests.sh -a install
  endif
endif

test-e2e-clean:
	@echo "Clean E2E Tests environment..."
	@./cicd-scripts/setup-e2e-tests.sh -a uninstall

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle:
	cd operators/multiclusterobservability && make bundle
