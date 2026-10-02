#!/usr/bin/env bash
set -euo pipefail

for value in "$VOXIS_APP_DB" "$VOXIS_APP_USER"; do
  [[ "$value" =~ ^[a-z_][a-z0-9_]*$ ]] || {
    echo 'VOXIS_APP_DB and VOXIS_APP_USER must be lowercase PostgreSQL identifiers.' >&2
    exit 1
  }
done
[[ -n ${VOXIS_APP_PASSWORD:-} ]] || {
  echo 'VOXIS_APP_PASSWORD is required.' >&2
  exit 1
}

psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=ON_ERROR_STOP=1 \
  --set=app_db="$VOXIS_APP_DB" --set=app_user="$VOXIS_APP_USER" \
  --set=app_password="$VOXIS_APP_PASSWORD" <<'SQL'
SELECT format(
  'CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD %L',
  :'app_user',
  :'app_password'
)
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user')
\gexec

SELECT format('CREATE DATABASE %I OWNER %I TEMPLATE template0', :'app_db', :'app_user')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = :'app_db')
\gexec

SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', :'app_db')
\gexec
SELECT format('GRANT CONNECT, TEMPORARY ON DATABASE %I TO %I', :'app_db', :'app_user')
\gexec
SQL
