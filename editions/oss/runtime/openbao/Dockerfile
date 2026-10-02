# syntax=docker/dockerfile:1.7

ARG OPENBAO_IMAGE=quay.io/openbao/openbao@sha256:a60afafda36337abe833c4a63894bf1095098f29abea4091e7e555a33dd52889
FROM ${OPENBAO_IMAGE}

LABEL org.opencontainers.image.title="Voxis-OSS OpenBao runtime" \
      org.opencontainers.image.description="OpenBao runtime with supported Alpine security updates" \
      org.opencontainers.image.licenses="MPL-2.0"

USER root
RUN apk upgrade --no-cache \
    && apk add --no-cache 'libcrypto3>=3.5.8-r0' 'libssl3>=3.5.8-r0'
USER openbao
