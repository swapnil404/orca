#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is required" >&2
  exit 1
fi

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
migrations_dir="${root_dir}/server/migrations"

psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SQL

for migration in "${migrations_dir}"/*.sql; do
  version="$(basename "${migration}")"
  applied="$(psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -Atqc \
    "SELECT 1 FROM schema_migrations WHERE version = '${version}'")"
  if [[ "${applied}" == "1" ]]; then
    continue
  fi

  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -1 -f "${migration}" -c \
    "INSERT INTO schema_migrations (version) VALUES ('${version}')"
done
