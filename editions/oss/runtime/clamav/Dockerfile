# syntax=docker/dockerfile:1.7

ARG CLAMAV_IMAGE=clamav/clamav@sha256:71fbb76b397cd84a90043caf1178a7f81bd0c131a031e7b0619afd721fbfad41
FROM ${CLAMAV_IMAGE}

LABEL org.opencontainers.image.title="Voxis-OSS ClamAV runtime" \
      org.opencontainers.image.description="ClamAV runtime with supported Alpine security updates" \
      org.opencontainers.image.licenses="GPL-2.0-or-later"

RUN apk upgrade --no-cache \
    && apk add --no-cache 'libcrypto3>=3.5.8-r0' 'libssl3>=3.5.8-r0'
