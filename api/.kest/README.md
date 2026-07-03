# API skill-script E2E coverage

The API skill-script runtime currently has one reliable end-to-end test path:

```bash
make test-api-skill-script-e2e
```

That command starts a temporary `zgi-sandbox` process, waits for `/health`, and
runs the API runtime test that loads a script skill, uploads the skill package as
an archive, executes `scripts/run.py`, collects `artifacts/`, and deletes the
sandbox.

To target an existing sandbox instead of starting a temporary one:

```bash
ZGI_SANDBOX_E2E_ENDPOINT=http://127.0.0.1:2660 make test-api-skill-script-e2e
```

If that sandbox requires authentication, also pass:

```bash
ZGI_SANDBOX_E2E_API_KEY=local-key ZGI_SANDBOX_E2E_ENDPOINT=http://127.0.0.1:2660 make test-api-skill-script-e2e
```

## Why this is not a Kest flow yet

Kest is a good fit for black-box HTTP contracts. The sandbox contract is covered
under `sandbox/.kest`, including lifecycle, archive upload, command execution,
artifact download, and rejection cases.

The API `run_script` path is not exposed as a standalone HTTP endpoint. It is an
internal skill runtime tool call used after skill resolution and chat/tool
orchestration. A full public API Kest flow would need a bootstrapped API server,
database state, authentication, organization membership, admin permissions for
custom skill import, and deterministic LLM/tool routing. Adding a test-only HTTP
endpoint just for Kest would weaken the public API surface.

Recommended next step for Kest is richer support for authenticated multi-step
API setup and deterministic service mocks. Once the API has a stable public or
internal test harness for skill runs, add Kest flows here for:

- custom skill import preview and confirm
- skill preference enablement
- chat request that deterministically triggers `run_script`
- artifact response verification
