# Agent Runtime delivery acceptance

This is the scoped standard-Skill delivery acceptance dated 2026-10-08, not a
certification of arbitrary Skills, M6C, Kata/KVM or production readiness. Delivery
steps 1–4 are separate from the M0–M8 roadmap. Credentials, raw transcripts,
downloaded test files and operator-specific topology remain outside Git.

## Tested revisions

| Component | Tested identity |
|---|---|
| API/Web source | `64050cc5a7504d00a62b35b42452cd728639c3d6` |
| API image ID | `sha256:e25051e13ce3750187d88326de1960841715503bcc5d352d2866680a70279871` |
| Web image ID | `sha256:d2ca2790d23721faa40e750730ad3102463ecf49d630b61d4d71a3324b2e4ff2` |
| Acceptance-only fixture revision | `9fbcdbffedcda883a3156348a32ed3c180e76f09` |

These are locally built Docker image IDs, not published registry manifest digests.
API/Web were built using the repository Dockerfiles. The Web image retains its
existing Node 20 Alpine baseline; the Web quality gate used Node 22. Execution
environments provide Python 3.13 and Node 22 independently of the Web image.
Worker hashes, approved OCI digest, E2B template and retained resource identifiers
are pinned in the operator-owned, non-sensitive run manifest. Updating only this
record does not imply rebuilding or deploying product images from a later commit.

## Quality and real-backend results

| Gate | Actual result |
|---|---|
| API | Go 1.26.2 Linux: full tests, 327 packages with tests, and full vet passed |
| Sandbox | Go 1.26.2 Linux: full tests, 22 packages, and full vet passed; dedicated PostgreSQL ended with zero temporary test databases |
| Web | Node 22/pnpm 10.12.1: lint, type-check, three focused UI regressions and build passed; lint has 73 warnings and zero errors |
| Public delivery scripts/CI | Preflight/dispatch regression, ShellCheck and actionlint passed locally; hosted CI was not dispatched |
| Real E2B package contract | Public gate passed in 151.14 seconds, including native Python/Node, APT, network restriction, termination, compute-loss recovery and damaged-ledger rejection/repair |
| Real gVisor package recovery | Passed in 215.12 seconds with native compilation, guarded APT, denied destinations and retained-state recovery |
| API standard-Skill matrix | Real E2B: 148.55 seconds; real Linux Worker: 95.39 seconds; original helpers, exports, error cases, tenant isolation, cancellation/timeout and hash restoration passed |
| Native gVisor isolation | Checkpoint recovery, file/Job/PTY contract, CPU/PID and memory limits passed using the dedicated containerd address |
| Legacy Script Skills | Candidate Python/Node E2E and six backtest scenarios passed; Kest flows passed with an isolated PostgreSQL instance |
| Existing secure Linux runner | Seven tests and cancellation/timeout subcases passed through the existing bubblewrap/cgroup test body, not through gVisor |

The API matrix's in-memory export store is not browser-download evidence. The
separate console acceptance below supplies that evidence. Likewise Docker/runc
image checks are not substitutes for native gVisor isolation.

### Failures found and retested

- Full PostgreSQL coverage exposed an incorrect legacy authentication fixture and
  an administrator connection closed before cleanup. The fixtures now use the
  existing `X-API-Key` contract, reject Bearer authentication on that legacy route,
  and close the administrator after owned-database cleanup. Explicitly configured
  unavailable PostgreSQL fails the gate rather than skipping it. No product
  authentication was broadened.
- Initial Web dependency installation exhausted memory using a Windows-mounted
  store. An isolated Linux volume and bounded install/build concurrency passed;
  no dependencies or assertions were removed to make it pass.
- The real resource-limit fixture used the default containerd socket although
  the node's approved image was on a dedicated socket. `9fbcdbffe` makes this
  fixture honor the existing address environment variable. Real resource tests
  then passed. This is a fixture fix, not a product-binary change.
- The chained secure-runner command could not start Docker from a Go-only
  validation container. The same compiled tests and secure script runtime body
  passed using the host Docker CLI. The unavailable invocation itself is not
  recorded as a passing `make` command.

## Console and downloaded-content acceptance

Both dedicated consoles used the same two-row CSV (`A=10`, `B=20`) and original
XLSX, PPTX and GIF Skills without adding a manifest or rewriting Skill scripts.
The original XLSX ZIP was reimported through preview/confirm and identified as
session-script execution. Existing approved test Agent bindings were reused;
this acceptance did not expand access rights.

Each backend generated and actually downloaded seven files: XLSX, PPTX, PDF,
two page PNGs, GIF and checks JSON. Independent read-only verification confirmed:

- XLSX inputs 10/20, `SUM(B2:B3)` and cached result 30;
- two PPTX slides and a native chart containing A/B and 10/20;
- two PDF pages, valid page PNGs and the expected total;
- a 128×128 GIF with twelve distinct frames of 100 ms each;
- all six product hashes matching the downloaded checks JSON.

The E2B model diagnosed missing `imageio` and installed it through the approved
package source. Linux reused its already retained installation. Both models
encountered genuine script-call errors and corrected them; failures were not
removed from history. This does not claim the model never writes a command that
masks an inner failure with a successful final command.

After pause and restoring the candidate deployment, both original conversations
performed a separate expected-failure Job (exit 7 and visible stderr), imported
`imageio` without installation, verified all six unchanged hashes and exported
an additional restoration report. Both reports were actually downloaded. In
total this run downloaded fourteen original result files and two restoration
reports, in addition to the gate-off and rollback download checks.

Disabling the overall runtime environment rejected new execution-enabled Agent
requests before model execution on both backends. It did **not** silently convert
them into ordinary chat. Existing XLSX exports still downloaded with identical
hashes. After re-enabling, new Jobs and downloads passed. Legacy `run_script`
regression remains a separate compatibility path and gains no installation rights.

## Remaining limits

- Installation persistence is session-owned Python/Node installed-state recovery
  and fixed-version APT replay. Shared organization caches, retained upstream
  release archives and rootfs incremental layers are not implemented by this gate.
- Approved package destinations are network restrictions, not executable identity
  restrictions. This is not arbitrary internet access.
- Console Job duration can display less than 1 ms although wall-clock execution
  takes longer. Do not use that display as execution-time or cost accounting.
- The model-controlled checks JSON used either an object or an array on different
  runs. Verification read the actual shape; this is not output-Schema validation.
- Long original-Skill conversations consumed substantial tokens. Transcript and
  cost optimization remain productionization work, not a claim of this acceptance.
- Native PID exhaustion can close the Runtime Agent channel. The resource gate
  verifies the enforced limit and cleanup; it is not a high-availability guarantee.
- A private SSH test connection was used for the remote Linux Worker. It is not
  a permanent authenticated HTTPS gateway or a production connectivity solution.

See [the delivery checklist](agent-runtime-release-candidate.md) for the release
gate and [reproducible tests](agent-runtime-delivery-tests.md) for commands.
