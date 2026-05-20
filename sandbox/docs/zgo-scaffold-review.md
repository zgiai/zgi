# zgo Scaffold Review for zgi-sandbox

## Conclusion

`zgo` can be used as the engineering scaffold for `zgi-sandbox`, but it should not be used unchanged as the business foundation.

A more precise summary is:

- Reusable: HTTP bootstrap shell, Wire DI structure, routing conventions, configuration loading, logging and metrics wiring, and test layout
- Not safe to inherit directly: default business modules, deployment-platform capabilities, current CLI generators, and database-first startup assumptions

Recommended direction:

- Build `zgi-sandbox` on top of the `zgo` infrastructure skeleton
- Keep only the smallest set of sandbox-specific modules in the first runnable version
- Do not copy `deployment`, `platform`, `user`, or `apikey` business modules into the service

## Main Review Findings

### 1. `zgo` Is Not a Clean Empty Shell

The `zgo` README describes a minimal starter, but the actual application wiring still pulls in extra modules such as `deployment` and `platform`.

If used directly, those unrelated product capabilities would leak into `zgi-sandbox`.

For a sandbox service, that is noise and risk rather than leverage.

### 2. The Default Route Surface Is Too Broad

The `deployment` and `platform` modules register routes directly under `/v1` without the same tighter protection model used by security-sensitive APIs.

For a sandbox product, the default surface area should start narrow, not broad.

Even if authentication could be added later, the current default posture is not a secure minimum.

### 3. The Generators Are Out of Sync with the Current Layout

`make:model`, `make:service`, `make:handler`, and related generators still write to the old `app/` structure.

`make:module` targets `internal/modules/<name>`, but it still assumes missing wiring files and older route signatures.

That makes `zgo` better suited for manual trimming than for generator-driven service bootstrap.

### 4. Optional Database Support Is Incomplete

`database.NewDB` can return `nil` when the database is disabled, but the HTTP bootstrap still assumes the database checker should always be registered.

If `zgi-sandbox` inherits that startup path unchanged, the "lightweight first, storage later" plan becomes fragile.

For `zgi-sandbox`, V1 should be able to stay very light and avoid mandatory storage dependencies.

## How `zgo` Should Be Used

### Parts Worth Reusing

- `cmd/server` entrypoint structure
- `internal/bootstrap` startup layers
- `internal/contracts/module.go` conventions
- `internal/infra/router` route wrapper approach
- `internal/wiring` Wire organization
- Common packages such as `pkg/response`, `pkg/errors`, and `pkg/logger`
- `tests/unit`, `tests/integration`, and `tests/feature` layout

### Parts That Should Not Be Carried Forward

- The current default handler aggregation in `internal/app/app.go`
- The current default module injection in `internal/wiring/wire.go`
- `internal/modules/deployment`
- `internal/modules/platform`
- The current `make:*` generators
- Startup assumptions that treat the database as mandatory

## Recommended Delivery Shape

For `zgi-sandbox`, the recommended use of `zgo` is:

### Option A: Slim Fork

Copy the minimal engineering shell from `zgo` and remove all default business modules. Keep only:

- `bootstrap`
- `infra`
- `contracts`
- `wiring`
- `pkg`

Then create only sandbox-specific modules:

- `compat`
- `lifecycle`
- `exec`
- `policy`
- `observer`

This is the recommended option.

### Option B: Delete and Rename Inside `zgo`

This is technically possible, but it is not recommended.

Why:

- Business modules and scaffold concerns are currently mixed together
- It is easy to miss default business paths during cleanup
- It becomes harder to tell which code belongs to the scaffold and which belongs to the sandbox

## Design Impact on `zgi-sandbox`

Based on the review, `zgi-sandbox` should follow these engineering rules:

1. Reuse the `zgo` infrastructure style, not the `zgo` default module set.
2. Keep the first runnable version focused on one `compat` module and `/v1/sandbox/run`.
3. Add `lifecycle`, `exec`, `policy`, and `observer` in later stages.
4. Make database, Redis, and object storage optional integrations rather than mandatory V1 dependencies.

## Final Judgment

If the question is:

- Can `zgo` be used directly as the full `zgi-sandbox` base?

The answer is:

- No, that is not recommended.

If the question is:

- Can `zgo` be used as the engineering scaffold for `zgi-sandbox`?

The answer is:

- Yes, but only after slimming it down to the infrastructure layer.
