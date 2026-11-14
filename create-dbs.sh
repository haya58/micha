#!/bin/sh
# Creates additional per-persona databases on the SAME postgres server.
# Set EXTRA_DBS="botdb_shiro botdb_kuro" on the db service.
set -e
: "${POSTGRES_USER:=bot}"

for db in $EXTRA_DBS; do
  [ -z "$db" ] && continue
  exists=$(psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
    -tAc "SELECT 1 FROM pg_database WHERE datname = '$db'")
  if [ "$exists" != "1" ]; then
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
      -c "CREATE DATABASE \"$db\""
  fi
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db" \
    -c "CREATE EXTENSION IF NOT EXISTS vector;"
done
