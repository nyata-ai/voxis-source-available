# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437
ARG POSTGRES_IMAGE=postgres@sha256:cf78e76683b9ca8c5733cbbdce6c9262b45b6767934dd0a95e671f9a0fc20685
ARG GOSU_REVISION=6456aaa0f3c854d199d0f037f068eb97515b7513
ARG GOSU_SOURCE_ARCHIVE=https://github.com/tianon/gosu/archive/6456aaa0f3c854d199d0f037f068eb97515b7513.tar.gz
ARG GOSU_SOURCE_SHA256=33d7537d588ea49458b9509bcf4554bdf5ceacc66da71e5caa1058ea3b689c3b

FROM ${GO_IMAGE} AS gosu-build
ARG GOSU_SOURCE_ARCHIVE
ARG GOSU_SOURCE_SHA256
ENV GOTOOLCHAIN=local
WORKDIR /src/gosu
ADD --checksum=sha256:${GOSU_SOURCE_SHA256} ${GOSU_SOURCE_ARCHIVE} /tmp/gosu.tar.gz
RUN tar -xzf /tmp/gosu.tar.gz --strip-components=1 \
    && go mod download \
    && go mod verify \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/gosu . \
    && go version -m /out/gosu > /out/gosu-build-info.txt \
    && go list -m -json all > /out/gosu-modules.json \
    && cp LICENSE /out/LICENSE

FROM ${POSTGRES_IMAGE}
ARG GOSU_REVISION
ARG GOSU_SOURCE_ARCHIVE
ARG GOSU_SOURCE_SHA256
LABEL org.opencontainers.image.title="Voxis-OSS PostgreSQL runtime" \
      org.opencontainers.image.description="PostgreSQL 16 runtime with supported Alpine security updates" \
      org.opencontainers.image.licenses="PostgreSQL AND Apache-2.0" \
      org.voxis.runtime.gosu.revision="${GOSU_REVISION}" \
      org.voxis.runtime.gosu.source-archive="${GOSU_SOURCE_ARCHIVE}" \
      org.voxis.runtime.gosu.source-sha256="${GOSU_SOURCE_SHA256}"

COPY --from=gosu-build /out/gosu /usr/local/bin/gosu
COPY --from=gosu-build /out/LICENSE /usr/share/licenses/gosu/LICENSE
COPY --from=gosu-build /out/gosu-build-info.txt /usr/share/doc/gosu/build-info.txt
COPY --from=gosu-build /out/gosu-modules.json /usr/share/doc/gosu/modules.json
RUN set -eux; \
    apk upgrade --no-cache; \
    apk add --no-cache 'libcrypto3>=3.5.8-r0' 'libssl3>=3.5.8-r0' 'libuuid>=2.42.3-r1'; \
    gosu --version; \
    gosu nobody true; \
    postgres --version; \
    test "$(readlink /usr/local/bin/su-exec)" = gosu
