# Voxis Source-Available frontend

The web app for Voxis Source-Available: a React 19 and TypeScript single-page
app built with Vite. In a normal installation you do not run it by hand.
Docker Compose builds it into the `web` image, and the Caddy proxy serves it on
the same origin as the API and Keycloak. See the
[installation guide](../infra/oss/README.md).

## Development

You need Node.js 22.

```bash
npm ci
npm run dev
```

The dev server listens on `http://127.0.0.1:5173` and forwards `/api/v1` and
`/mcp` to `VITE_DEV_PROXY_TARGET` (default `http://localhost:8080`). Copy
`.env.example` to `.env.local` to change the Keycloak settings.

## Checks

These are the checks CI runs:

```bash
npm run typecheck
npm run lint
npm test
npm run build:strict
```

Browser tests run against a complete installation; see
[`tests/e2e/README.md`](tests/e2e/README.md).

## User guide

The in-app guide lives in `src/pages/guide/`, with one Markdown file per
language under `content/`. Refresh its screenshots with
`npm run guide:screenshots`.
