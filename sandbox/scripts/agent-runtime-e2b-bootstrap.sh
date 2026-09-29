#!/bin/sh
set -eu

token_file="${ZGI_RUNTIME_AGENT_TOKEN_FILE:-/run/zgi-agent/bootstrap-token}"
while [ ! -s "$token_file" ]; do
  sleep 0.05
done

exec /usr/local/bin/zgi-runtime-agent
