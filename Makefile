.PHONY: run build test race vet fmt check demo image deploy-remote

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
