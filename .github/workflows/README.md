# GitHub workflows

- `ci.yml` builds, lints, and tests the backend and frontend, runs the
  PostgreSQL integration tests against the locked PostgreSQL image, and checks
  the Compose files and the operator-script guards.
- `install-smoke.yml` installs the stack from scratch on a runner, with a small
  stub in place of the local model. It checks sign-in wiring, that the
  Keycloak admin plane is not public, and the web security headers.
- `security.yml` checks dependencies, scans every runtime image (API, web,
  Caddy, PostgreSQL, ClamAV, Keycloak, OpenBao, and the model runtime) for HIGH
  and CRITICAL vulnerabilities, and scans the repository history for secrets.

The workflows use no repository secrets. Speechmatics and the model are
replaced by local stubs. Live provider checks are manual operator runs with
synthetic audio; they do not run in GitHub Actions.
