# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437
ARG ALPINE_IMAGE=alpine@sha256:79ff19e9084a00eece421b2523fb93e22d730e2c0e525905de047e848e56d95f
ARG CADDY_VERSION=2.11.4
ARG CADDY_REVISION=e2eee6a7fce366321294c9c2a79f3146891dcbdf
ARG CADDY_CUSTOM_VERSION=v2.11.4-voxis.1+e2eee6a7
ARG CADDY_SOURCE_ARCHIVE=https://github.com/caddyserver/caddy/archive/e2eee6a7fce366321294c9c2a79f3146891dcbdf.tar.gz
ARG CADDY_SOURCE_SHA256=a593bd7077c76102ca76d19287a5e247d4e359dd67eddbc933f865afd3c131eb

FROM ${GO_IMAGE} AS build
ARG CADDY_SOURCE_ARCHIVE
ARG CADDY_SOURCE_SHA256
ARG CADDY_CUSTOM_VERSION
ENV GOTOOLCHAIN=local
WORKDIR /src
ADD --checksum=sha256:${CADDY_SOURCE_SHA256} ${CADDY_SOURCE_ARCHIVE} /tmp/caddy.tar.gz
RUN tar -xzf /tmp/caddy.tar.gz --strip-components=1 \
    && go get golang.org/x/crypto@v0.55.0 golang.org/x/net@v0.58.0 golang.org/x/text@v0.41.0 google.golang.org/grpc@v1.83.2 \
    && go mod verify \
    && go mod tidy \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X github.com/caddyserver/caddy/v2.CustomVersion=${CADDY_CUSTOM_VERSION}" -o /out/caddy ./cmd/caddy \
    && go list -m -json all > /out/modules.json \
    && cp LICENSE /out/LICENSE \
    && cp go.mod /out/go.mod \
    && cp go.sum /out/go.sum

FROM ${ALPINE_IMAGE}
ARG CADDY_VERSION
ARG CADDY_REVISION
ARG CADDY_SOURCE_ARCHIVE
ARG CADDY_SOURCE_SHA256
LABEL org.opencontainers.image.title="Voxis-OSS Caddy runtime" \
      org.opencontainers.image.description="Voxis-OSS web and proxy runtime" \
      org.opencontainers.image.version="${CADDY_VERSION}" \
      org.opencontainers.image.revision="${CADDY_REVISION}" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.source="${CADDY_SOURCE_ARCHIVE}" \
      org.voxis.runtime.caddy.source-sha256="${CADDY_SOURCE_SHA256}" \
      org.voxis.runtime.caddy.module-floors="golang.org/x/crypto=v0.55.0,golang.org/x/net=v0.58.0,golang.org/x/text=v0.41.0,google.golang.org/grpc=v1.83.2"

RUN apk add --no-cache ca-certificates 'libcrypto3>=3.5.8-r0' 'libssl3>=3.5.8-r0' \
    && install -d --mode=0755 /etc/caddy /usr/share/licenses/caddy /usr/share/doc/caddy /usr/share/src/caddy \
    && install -d -o 10001 -g 10001 -m 0700 /config /data
COPY --from=build /out/caddy /usr/bin/caddy
COPY --from=build /out/LICENSE /usr/share/licenses/caddy/LICENSE
COPY --from=build /out/modules.json /usr/share/doc/caddy/modules.json
COPY --from=build /out/go.mod /usr/share/doc/caddy/go.mod
COPY --from=build /out/go.sum /usr/share/doc/caddy/go.sum
COPY --from=build /tmp/caddy.tar.gz /usr/share/src/caddy/caddy-v${CADDY_VERSION}.tar.gz

ENV XDG_CONFIG_HOME=/config \
    XDG_DATA_HOME=/data
USER 10001:10001
EXPOSE 80 443 2019
CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
