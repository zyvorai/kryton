# Copyright 2026 Kryton contributors
# SPDX-License-Identifier: Apache-2.0
.PHONY: run build test race vet fmt check vulncheck lint demo image deploy-remote harden-lab bootstrap-kubevirt setup-kubevirt setup-kubevirt-production run-kubevirt-production-remote build-golden enable-kubevirt-snapshots enable-rook-ceph

run:
	go run ./cmd/krytond

demo:
	KRYTON_PROVIDER=demo KRYTON_AUTH_MODE=disabled go run ./cmd/krytond

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/krytond ./cmd/krytond
	CGO_ENABLED=0 go build -trimpath -o bin/krytonctl ./cmd/krytonctl

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

lint:
	golangci-lint run ./...

check: fmt test vet build

image:
	docker build -t kryton:dev .

# Remote deploy (SSH + rsync). Example: make deploy-remote H=10.0.0.5 U=root
deploy-remote:
	@test -n "$(H)" || (echo "Usage: make deploy-remote H=<host> U=<user> [ARGS='--quick']"; exit 1)
	./scripts/deploy-remote.sh $(if $(U),$(U)@)$(H) $(ARGS)

harden-lab:
	@test -n "$(H)" || (echo "Usage: make harden-lab H=<host> U=<user>"; exit 1)
	ssh $(if $(U),$(U)@)$(H) 'bash -s' < ./scripts/harden-lab-services.sh

bootstrap-kubevirt:
	@if [ -z "$(IMAGE)" ] && [ -z "$(URL)" ]; then \
		echo "Usage: make bootstrap-kubevirt IMAGE=/path/to/win.qcow2 [ID=windows-11-enterprise]"; \
		echo "   or: make bootstrap-kubevirt URL=https://artifacts.example/win.qcow2 [ID=...]"; \
		exit 1; \
	fi
	@if [ -n "$(URL)" ]; then \
		KRYTON_IMAGE_URL="$(URL)" KRYTON_IMAGE_ID="$(or $(ID),windows-11-enterprise)" ./scripts/bootstrap-kubevirt-images.sh --http; \
	else \
		KRYTON_WINDOWS_IMAGE="$(IMAGE)" KRYTON_IMAGE_ID="$(or $(ID),windows-11-enterprise)" ./scripts/bootstrap-kubevirt-images.sh; \
	fi

build-golden:
	VERSION="$(or $(VERSION),11e)" FINALIZE="$(or $(FINALIZE),0)" ./scripts/build-golden-image.sh $(if $(filter 1,$(AUTO)),--auto,)

setup-kubevirt:
	@if [ -z "$(IMAGE)" ] && [ -z "$(URL)" ]; then \
		echo "Usage: make setup-kubevirt IMAGE=/path/to/win.qcow2 [ARGS='--helm']"; \
		echo "   or: make setup-kubevirt URL=https://artifacts.example/win.qcow2"; \
		exit 1; \
	fi
	@if [ -n "$(URL)" ]; then \
		KRYTON_IMAGE_URL="$(URL)" ./scripts/setup-kubevirt.sh --http $(ARGS); \
	else \
		KRYTON_WINDOWS_IMAGE="$(IMAGE)" ./scripts/setup-kubevirt.sh $(ARGS); \
	fi

setup-kubevirt-production:
	@./scripts/setup-kubevirt-production.sh \
		$(if $(filter 1,$(BUILD)),--build-golden,) \
		$(if $(filter 1,$(SKIP)),--skip-create,) \
		$(if $(filter 1,$(CUSTOMER)),--customer-helm,) \
		$(if $(IMAGE),--image $(IMAGE),) $(ARGS)

run-kubevirt-production-remote:
	@test -n "$(H)" || (echo "Usage: make run-kubevirt-production-remote H=<host> U=<user> [BUILD=1]"; exit 1)
	@./scripts/run-kubevirt-production-remote.sh $(H) $(U)

enable-kubevirt-snapshots:
	./scripts/enable-kubevirt-snapshots.sh $(ARGS)

enable-rook-ceph:
	./scripts/enable-rook-ceph.sh $(ARGS)
