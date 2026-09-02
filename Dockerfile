# Copyright 2026 Kryton contributors
# SPDX-License-Identifier: Apache-2.0
# Pinned by digest so builds are reproducible; bump via .github/dependabot.yml
# (docker ecosystem) or by re-resolving the tag's current digest.
FROM golang:1.27@sha256:192b74998e350966280a2cbffbb6c40064754f7ec005096aa64f04d7ece4467e AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/krytond ./cmd/krytond && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/krytonctl ./cmd/krytonctl

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/krytond /krytond
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/krytond"]
