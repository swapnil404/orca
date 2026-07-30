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
    checksum TEXT,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT;
SQL

for migration in "${migrations_dir}"/*.sql; do
  version="$(basename "${migration}")"
  checksum="$(sha256sum "${migration}" | cut -d ' ' -f 1)"
  stored_checksum="$(psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -Atq \
    --set=version="${version}" <<'SQL'
SELECT COALESCE(checksum, '__missing__')
FROM schema_migrations
WHERE version = :'version';
SQL
)"
  if [[ "${stored_checksum}" == "__missing__" ]]; then
    psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 \
      --set=version="${version}" --set=checksum="${checksum}" <<'SQL'
UPDATE schema_migrations
SET checksum = :'checksum'
WHERE version = :'version';
SQL
    continue
  fi
  if [[ -n "${stored_checksum}" ]]; then
    if [[ "${stored_checksum}" != "${checksum}" ]]; then
      echo "migration checksum mismatch for ${version}: expected ${stored_checksum}, got ${checksum}" >&2
      exit 1
    fi
    continue
  fi

  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -1 \
    --set=version="${version}" --set=checksum="${checksum}" \
    -f "${migration}" -f - <<'SQL'
INSERT INTO schema_migrations (version, checksum)
VALUES (:'version', :'checksum');
SQL
done

psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 <<'SQL'
ALTER TABLE schema_migrations ALTER COLUMN checksum SET NOT NULL;
SQL
