# syntax=docker/dockerfile:1.7

ARG NODE_IMAGE=node@sha256:6dac556d980b7f0e5498d08f08cee0ca67798b4ad6c23964a9214920e67758d0
ARG CADDY_IMAGE=voxis-oss-caddy:2.11.4-voxis.1-e2eee6a7

FROM ${NODE_IMAGE} AS build
WORKDIR /src
COPY package.json package-lock.json .npmrc ./
RUN npm ci
COPY . .

# The public browser bundle must stay on the same HTTPS origin that Caddy owns.
ARG VITE_API_URL=/api/v1
ARG VITE_KEYCLOAK_URL=/auth
ARG VITE_KEYCLOAK_REALM=voxis-oss
ARG VITE_KEYCLOAK_CLIENT_ID=voxis-oss-web
ENV VITE_API_URL=${VITE_API_URL} \
    VITE_KEYCLOAK_URL=${VITE_KEYCLOAK_URL} \
    VITE_KEYCLOAK_REALM=${VITE_KEYCLOAK_REALM} \
    VITE_KEYCLOAK_CLIENT_ID=${VITE_KEYCLOAK_CLIENT_ID}
RUN npm run build:strict

FROM ${CADDY_IMAGE}
USER 0
RUN printf '%s\n' \
    '{' \
    '  admin off' \
    '  auto_https off' \
    '}' \
    '' \
    ':8080 {' \
    '  root * /srv' \
    '  encode zstd gzip' \
    '  try_files {path} /index.html' \
    '  file_server' \
    '}' > /etc/caddy/Caddyfile
COPY --from=build --chown=10001:10001 /src/dist /srv
ENV HOME=/tmp \
    XDG_CONFIG_HOME=/tmp \
    XDG_DATA_HOME=/tmp
USER 10001:10001
EXPOSE 8080
CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
