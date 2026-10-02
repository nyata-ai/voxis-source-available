# syntax=docker/dockerfile:1.7

# This image deliberately differs from the commercial distroless image. OSS
# production requires bubblewrap to isolate ffprobe, ffmpeg, and DeepFilterNet.
ARG GO_IMAGE=golang@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437
ARG ALPINE_IMAGE=alpine@sha256:79ff19e9084a00eece421b2523fb93e22d730e2c0e525905de047e848e56d95f
ARG FFMPEG_IMAGE=mwader/static-ffmpeg:9.0.1@sha256:54e55b0cb8f672870fc38ceb2e6c411855cb3b39c505f5f3b2505ee01ed5f2b7
ARG BWRAP_VERSION=0.12.0
ARG BWRAP_REVISION=2a76602a8c71f36c1527cf9fc3417d9149822e0c
ARG BWRAP_SOURCE_ARCHIVE=https://github.com/containers/bubblewrap/archive/refs/tags/v0.12.0.tar.gz
ARG BWRAP_SHA256=3f66d9e3f85396f6d7d7fbd2859f277e4fd3a9303341bfe2ff325251bda65fd4

FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w -X github.com/voxis/backend/internal/config.Version=${VERSION} -X github.com/voxis/backend/internal/config.Commit=${COMMIT} -X github.com/voxis/backend/internal/config.BuildTime=${BUILD_TIME}" \
    -o /out/voxis-api \
    ./cmd/server

FROM ${FFMPEG_IMAGE} AS fftools

FROM ${ALPINE_IMAGE} AS bwrap-build
ARG BWRAP_VERSION
ARG BWRAP_SOURCE_ARCHIVE
ARG BWRAP_SHA256
RUN apk add --no-cache build-base curl libcap-dev linux-headers meson ninja pkgconf
WORKDIR /src
# Bubblewrap 0.12 needs Linux's limits header to define PATH_MAX on musl.
RUN set -eux; \
    curl --fail --location --retry 3 --output bubblewrap.tar.gz "${BWRAP_SOURCE_ARCHIVE}"; \
    echo "${BWRAP_SHA256}  bubblewrap.tar.gz" | sha256sum -c -; \
    tar -xzf bubblewrap.tar.gz --strip-components=1; \
    CFLAGS="-include linux/limits.h" meson setup build -Dman=disabled -Dselinux=disabled -Dtests=false -Dbash_completion=disabled -Dzsh_completion=disabled --prefix=/usr/local; \
    meson compile -C build; \
    meson install -C build

FROM ${ALPINE_IMAGE} AS deepfilter
ARG DEEPFILTER_VERSION=0.5.6
ARG DEEPFILTER_SHA256=70775e251eee44c0f2451a1e833326cf8bcbbe304d3e7cd12851e6fce72ef7da
RUN apk add --no-cache ca-certificates curl
RUN set -eux; \
    asset="deep-filter-${DEEPFILTER_VERSION}-x86_64-unknown-linux-musl"; \
    curl --fail --location --retry 3 --output /usr/local/bin/deep-filter "https://github.com/Rikorose/DeepFilterNet/releases/download/v${DEEPFILTER_VERSION}/${asset}"; \
    echo "${DEEPFILTER_SHA256}  /usr/local/bin/deep-filter" | sha256sum -c -; \
    chmod 0755 /usr/local/bin/deep-filter

FROM ${ALPINE_IMAGE}
ARG BWRAP_VERSION
ARG BWRAP_REVISION
ARG BWRAP_SOURCE_ARCHIVE
ARG BWRAP_SHA256
LABEL org.opencontainers.image.title="Voxis-OSS API" \
      org.opencontainers.image.description="Voxis-OSS API with isolated local media tooling" \
      org.voxis.runtime.bubblewrap.license="LGPL-2.1-or-later" \
      org.voxis.runtime.bubblewrap.release="${BWRAP_VERSION}" \
      org.voxis.runtime.bubblewrap.revision="${BWRAP_REVISION}" \
      org.voxis.runtime.bubblewrap.source-archive="${BWRAP_SOURCE_ARCHIVE}" \
      org.voxis.runtime.bubblewrap.sha256="${BWRAP_SHA256}" \
      org.voxis.runtime.bubblewrap.cflags="-include linux/limits.h" \
      org.voxis.runtime.bubblewrap.meson-options="-Dman=disabled,-Dselinux=disabled,-Dtests=false,-Dbash_completion=disabled,-Dzsh_completion=disabled"

RUN apk add --no-cache 'libcrypto3>=3.5.8-r0' 'libssl3>=3.5.8-r0' ca-certificates curl libcap tzdata \
    && addgroup --system --gid 10001 voxis \
    && adduser --system --disabled-password --no-create-home --uid 10001 --ingroup voxis voxis \
    && install -d --owner=voxis --group=voxis --mode=0700 /var/lib/voxis /var/lib/voxis/scratch /tmp

COPY --from=fftools /ffmpeg /usr/local/bin/ffmpeg
COPY --from=fftools /ffprobe /usr/local/bin/ffprobe
COPY --from=fftools /doc /usr/share/doc/voxis-oss/media-tools/doc
COPY --from=fftools /versions.json /usr/share/doc/voxis-oss/media-tools/versions.json
COPY --from=bwrap-build /usr/local/bin/bwrap /usr/local/bin/bwrap
COPY --from=bwrap-build /src/COPYING /usr/share/licenses/bubblewrap/COPYING
COPY --from=deepfilter /usr/local/bin/deep-filter /usr/local/bin/deep-filter
COPY --from=build --chown=voxis:voxis /out/voxis-api /usr/local/bin/voxis-api

ENV PATH=/usr/local/bin:/usr/bin:/bin \
    TMPDIR=/var/lib/voxis/scratch
WORKDIR /var/lib/voxis
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/voxis-api"]
