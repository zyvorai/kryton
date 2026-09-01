.PHONY: run build test race vet fmt check demo image deploy-remote bootstrap-kubevirt setup-kubevirt build-golden enable-kubevirt-snapshots enable-rook-ceph

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

check: fmt test vet build

image:
	docker build -t kryton:dev .

# Remote deploy (SSH + rsync). Example: make deploy-remote H=10.0.0.5 U=root
deploy-remote:
	@test -n "$(H)" || (echo "Usage: make deploy-remote H=<host> U=<user> [ARGS='--quick']"; exit 1)
	./scripts/deploy-remote.sh $(if $(U),$(U)@)$(H) $(ARGS)

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

enable-kubevirt-snapshots:
	./scripts/enable-kubevirt-snapshots.sh $(ARGS)

enable-rook-ceph:
	./scripts/enable-rook-ceph.sh $(ARGS)
