# Integrations module

- Follow the package layout and dependency rules in `README.md`.
- Keep provider-specific code in
  `providers/<capability>/<provider>`; capability names describe the user
  function (`email`, `collaboration`, `developer`, `websearch`, or `social`).
- Do not add Go source files directly under `integrations/`; shared domain
  code belongs in `core`.
- Foundation packages (`audit`, `credentials`, `localization`, and `quota`)
  must not import `integrations/core`.
- Provider and infrastructure leaf packages may import `integrations/core`,
  but `core` must not import them. Provider packages must not import sibling
  providers. Preserve this direction to prevent Go import cycles.
- Keep provider clients, callback protocols, endpoints, and provider-specific
  authentication out of `core`.
- Never keep generated logs, credentials, uploads, or other runtime state in
  this source tree.
- Do not create generic `common`, `helpers`, or `utils` packages.
- Keep compatibility entry points when moving a stable root API.
- Run `gofmt` on changed Go files and
  `go test ./internal/modules/integrations/...` before handoff.
