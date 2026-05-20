#!/bin/sh
set -e

run_server_command() {
  if [ "$(id -u)" -eq 0 ]; then
    if command -v su-exec >/dev/null 2>&1; then
      su-exec appuser "$@"
      return
    fi

    echo "Warning: su-exec not found, running as root" >&2
  fi

  "$@"
}

run_as_appuser() {
  run_server_command "$@"
}

# Fix mounted volume permissions before dropping privileges. Named or bind
# mounts can override the ownership baked into the image.
mkdir -p /app/storage/opendal /app/logs
if [ "$(id -u)" -eq 0 ]; then
  chown -R appuser:appuser /app/storage /app/logs /app/config /app/templates
fi

# Wait for database to be ready if enabled
if [ "${WAIT_FOR_DB}" = "true" ]; then
  echo "Waiting for database to be ready..."
  until run_as_appuser ./server db:check-connection; do
    sleep 2
  done
fi

# Run database migrations if enabled
if [ "${MIGRATION_ENABLED}" = "true" ]; then
  echo "Running database migrations..."
  run_as_appuser ./server migrate
fi

# Run seed data if enabled. The seed command is idempotent and will skip
# repeated bootstrap executions when the marker already exists.
if [ "${SEED_ENABLED}" = "true" ]; then
  echo "Running seed data..."
  run_as_appuser ./server seed
fi

# Start the server
echo "Starting server..."
if [ "$(id -u)" -eq 0 ] && command -v su-exec >/dev/null 2>&1; then
  exec su-exec appuser ./server start
fi

if [ "$(id -u)" -eq 0 ]; then
  echo "Warning: su-exec not found, running as root" >&2
fi

exec ./server start
