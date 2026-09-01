FROM golang:1.23 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/krytond ./cmd/krytond && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/krytonctl ./cmd/krytonctl

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/krytond /krytond
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/krytond"]
