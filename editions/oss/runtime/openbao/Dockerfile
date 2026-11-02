# syntax=docker/dockerfile:1.7

ARG OPENBAO_IMAGE=quay.io/openbao/openbao@sha256:09f1eaaee351c329d75a5a89654d6e1ad4792998650c3cbf0c3513a76d9d9b6e
FROM ${OPENBAO_IMAGE}

LABEL org.opencontainers.image.title="Voxis-OSS OpenBao runtime" \
      org.opencontainers.image.description="OpenBao runtime with supported Alpine security updates" \
      org.opencontainers.image.licenses="MPL-2.0"

USER root
RUN apk upgrade --no-cache \
    && apk add --no-cache 'libcrypto3>=3.5.8-r0' 'libssl3>=3.5.8-r0'
USER openbao
