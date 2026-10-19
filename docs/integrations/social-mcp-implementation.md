# Built-in social MCP implementation plan

Goal: implement the reviewed scope in `social-mcp-design.md` on `feat/builtin-social-mcp`.

- [x] Add behavioral contract tests for three new localized Connector definitions, strict operation inputs, effects and DigitalHuman surfaces. Run to establish failures, then implement definitions without changing existing contracts.
- [x] Add provider tests using a fake MCP transport and a real stdio fixture. Cover discovery, identity verification, allowed operations, output size, unsupported tools, secret redaction, configuration errors and write outcome uncertainty. Implement the native driver through `pkg/mcpclient`.
- [x] Build the Node bridge with per-process token input, schema-limited tool exposure, bounded HTTP, upstream library reuse and explicit unavailable LinkedIn messaging. Test API mappings and credential isolation without external requests.
- [x] Wire all providers into the container; add trusted deployment configuration for Node/script paths, localized catalog metadata and icons. Add targeted config/frontend checks.
- [x] Preserve upstream source/license notices, add deployment and operator instructions, and record capabilities and external access requirements.
- [x] Run targeted Go/Node/frontend checks, review implementation against the design and security/compatibility behavior, fix findings, then commit the development branch. Do not publish or merge automatically.

## Verified checks

- `go test -p 2 ./internal/modules/connectors/... ./config`: passed, including existing confirmation/credential/binding behavior.
- Social provider race tests and Go-to-real-Node stdio catalog compatibility: passed.
- `npm test` in `api/social-mcp`: 23 tests passed; production dependency audit reported no vulnerabilities.
- Frontend type check, targeted ESLint, icon behavior tests and formatting checks: passed.
- API container/service and routes package compilation, plus `go build ./cmd/server`: passed.
- Docker `social-mcp` stage build and loading the bridge inside that image: passed. The full API Docker image has not been built.
- Spec and code-quality reviews: completed; Facebook reply ownership and LinkedIn/Facebook statistics query corrections have regression coverage.

Real-account acceptance has not been run. Platform grants, application approval, real post delivery, private-message access and returned metrics still require operator-provided accounts. This is an explicit external acceptance step, not certified by fixture tests.
