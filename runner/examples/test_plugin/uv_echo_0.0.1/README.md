# UV Echo Plugin

This plugin covers the full dependency installation path for `manager.Install`, including the UV branch. It calls `requests` to fetch a URL during `tool.invoke`, so it requires network access for dependency downloads.

## Directory Layout

```text
test_plugin/uv_echo_0.0.1/
  ├─ main_runner.py     # Entry point, compatible with the runner protocol
  ├─ requirements.txt   # Includes requests to trigger pip/uv installation
  └─ manifest.yaml      # Helpful for inspecting metadata (the API/tests still provide manifest during installation)
```

## Packaging

```bash
cd test_plugin/uv_echo_0.0.1
zip -r ../uv_echo_0.0.1.zip .
```

## Installation and Running (Example)

1. Prepare the environment variables (the example uses a local Postgres/Redis setup):
   ```bash
   export EXECUTOR_PLUGIN_HOME=/path/to/zgi/runner/plugins
   export EXECUTOR_WORKSPACE_PATH=/path/to/zgi/runner/workspace
   export EXECUTOR_PACKAGE_CACHE_PATH=/path/to/zgi/runner/cache
   export EXECUTOR_HTTP_PORT=2665
   export EXECUTOR_API_KEY=test-api-key
   export EXECUTOR_ADMIN_API_KEYS=admin-key-123
   ```

2. Make sure UV is available (this will exercise the `installDependencies` UV branch):
   ```bash
   pip install --user uv
   export EXECUTOR_UV_PATH=$(python3 -c "import uv, pathlib; from uv._find_uv import find_uv_bin; print(find_uv_bin())")
   ```

3. Register and install it through the API:
   - Register the manifest (you can refer to the metadata in `main_runner`, or build the payload directly):
     ```bash
     curl -X POST http://127.0.0.1:2665/api/v1/plugins \
       -H "X-API-Key: admin-key-123" \
       -H "Content-Type: application/json" \
       -d '{
         "id": "uv-echo:0.0.1",
         "name": "uv-echo",
         "version": "0.0.1",
         "author": "vic",
         "runner": {"language": "python", "entrypoint": "main_runner"},
         "requirements": {"packages": []}
       }'
     ```
   - Upload the ZIP package to complete installation:
     ```bash
     curl -X POST http://127.0.0.1:2665/api/v1/plugins/uv-echo:0.0.1/install \
       -H "X-API-Key: admin-key-123" \
       -F "file=@test_plugin/uv_echo_0.0.1.zip"
     ```

4. Verify:
   ```bash
   curl -H "X-API-Key: test-api-key" http://127.0.0.1:2665/api/v1/plugins/installed
   ```
   You should see `uv-echo`, and the workspace should contain `.venv` (or a UV environment) and the `requests` dependency.

## Behavior

- `action=list_tools` returns the `echo_http` tool definition
- `action=tool.invoke` with `name=echo_http` (or `echo`) accepts optional parameters:
  - `url` (default: `https://httpbin.org/get`)
  - `message` (default: `hello-from-uv-echo`)
- The request uses `requests` to perform a GET and returns the status code, message, and response body length

## Coverage

- Includes `requirements.txt` so `installDependencies` creates a venv and installs packages through pip or UV
- When `EXECUTOR_UV_PATH` is set, the UV venv creation/install branch is used; otherwise the standard `python -m venv + pip` branch is used
- The network dependency (`requests`) ensures actual downloads happen, which is useful for isolation and installation log verification
