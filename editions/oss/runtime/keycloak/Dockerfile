# syntax=docker/dockerfile:1.7

ARG KEYCLOAK_IMAGE=quay.io/keycloak/keycloak@sha256:6529db34610ce6328824218dce3bbe55c4ba4757bc5c8a09b5a4c746e28b6e9d
ARG TEMURIN_IMAGE=eclipse-temurin@sha256:db724162e4ff5f71fa0a2815026314d9e5e0e9a139f49be1d4717e6930e93341

FROM ${KEYCLOAK_IMAGE} AS keycloak
FROM ${TEMURIN_IMAGE}

LABEL org.opencontainers.image.title="Voxis-OSS Keycloak runtime" \
      org.opencontainers.image.description="Keycloak 26.7.5 distribution on the pinned Temurin 21 JRE runtime"

COPY --from=keycloak --chown=1000:0 /opt/keycloak /opt/keycloak
ENV LANG=en_US.UTF-8 \
    KC_RUN_IN_CONTAINER=true
USER 1000:0
ENTRYPOINT ["/opt/keycloak/bin/kc.sh"]
