#!/bin/sh
set -e

PGHOST_ARG=""
PGPORT_ARG=""

if [ -n "${PGHOST:-}" ]; then
  PGHOST_ARG="--host=$PGHOST"
fi

if [ -n "${PGPORT:-}" ]; then
  PGPORT_ARG="--port=$PGPORT"
fi

if [ -n "${PGHOST:-}" ]; then
  echo "Waiting for PostgreSQL at ${PGHOST}:${PGPORT:-5432}..."
  until pg_isready $PGHOST_ARG $PGPORT_ARG --username "${POSTGRES_USER}" --dbname postgres >/dev/null 2>&1; do
    sleep 2
  done
fi

create_db() {
  db_name="$1"
  if [ -z "$db_name" ]; then
    return 0
  fi

  echo "Creating database: $db_name"
  psql -v ON_ERROR_STOP=1 $PGHOST_ARG $PGPORT_ARG --username "$POSTGRES_USER" --dbname postgres <<-EOSQL
    SELECT 'CREATE DATABASE "$db_name"'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$db_name')\gexec
EOSQL
}

create_db "$POSTGRES_DB"
create_db "$POSTGRES_SQL_BASE_DB"
create_db "$POSTGRES_PLUGIN_RUNNER_DB"
create_db "$POSTGRES_SANDBOX_DB"
