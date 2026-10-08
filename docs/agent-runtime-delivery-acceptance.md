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
- The original console run's duration was not a Job measurement. The normal-use
  follow-up below adds measured Worker Job time; this is still not cost accounting.
- The model-controlled checks JSON used either an object or an array on different
  runs. Verification read the actual shape; this is not output-Schema validation.
- Read-only review of all four rendered test pages found a model-authored cell
  annotation on one E2B slide saying `A2 + A3`; the verified workbook uses
  `SUM(B2:B3)` and the correct cached value 30. The result and charts pass the
  runtime/content gates, but generated narrative still needs application-level
  review. This acceptance does not certify every sentence in model output.
- Long original-Skill conversations consumed substantial tokens. Transcript and
  cost optimization remain productionization work, not a claim of this acceptance.
- Native PID exhaustion can close the Runtime Agent channel. The resource gate
  verifies the enforced limit and cleanup; it is not a high-availability guarantee.
- A private SSH test connection was used for the remote Linux Worker. It is not
  a permanent authenticated HTTPS gateway or a production connectivity solution.

## Step 4: rollback, bounded load and final reconciliation

The dedicated four-service API/Web deployment passed this actual sequence:

1. Upgrade to the candidate and verify all four services healthy.
2. Disable runtime admission; verify rejected new execution requests and retained
   downloads on both backends; restore admission.
3. Roll back all four services to their preserved previous images without
   destructively reverting PostgreSQL or deleting storage. Reload original
   conversations and download the existing XLSX results with identical hashes.
4. Restore the candidate, verify all four services healthy, and run the new
   same-conversation Jobs and restoration-report downloads described above.
5. Checkpoint retained test Bindings, stop the owned E2B test Worker and close the
   SSH test connection. Native Worker rollback and `doctor` passed; the original
   Worker and shared containerd process IDs were unchanged. Both XLSX exports
   still downloaded with identical hashes after the execution connections stopped.

Both real backends also passed cancellation of an actual native Python compiler,
an installation timeout, process-tree and Job-temporary-directory cleanup, native
installation retry, and two concurrent installations each for pip, npm and APT.
Installed imports and system commands passed after checkpoint restoration. A
separate root-APT cancellation/retry passed on each backend. This is a small
bounded concurrency/repetition check, not a throughput benchmark or production
load certification. The native node has approximately 1.7 GiB memory and was
tested sequentially across larger contracts.

Final inventories distinguish retained resources from leaks:

| Resource | Final verified state |
|---|---|
| Dedicated Linux database | Four retained suspended Bindings with checkpoints; fifteen deleted test Bindings; zero queued/running Jobs; no pending/running cleanup tasks |
| Native runtime namespace | No running containerd tasks; previous Worker and shared containerd preserved; dedicated deployment rollback/health check passed |
| Dedicated E2B database | Three suspended Bindings with checkpoints; eleven deleted test Bindings; zero queued/running Jobs |
| E2B provider, approved template | Two paused Sandboxes, both mapped to retained UI Bindings; no running or unmatched test Sandbox; the third suspended Binding retains its checkpoint without compute |
| E2B cleanup queue | Sixteen completed tasks; one future allocation-attempt reconciliation associated with a retained Binding, not a due cleanup failure |
| Local services/storage | Four healthy candidate API/Web services; four stopped previous-version backups; existing application storage, test checkpoints and build caches retained |
| Disposable validation infrastructure | Owned PostgreSQL/anonymous volume/network and temporary admission-disabled API containers removed; no application database or user result removed |

To verify cleanup of the three deleted E2B test allocations without waiting the
normal grace period, only their dedicated-database reconciliation due times were
advanced. The real Worker completed all three. The retained Binding's future task
was left unchanged. Paused compute and checkpoints are deliberate retention and
may still incur provider storage charges; they are not described as zero resources.

### Operational handoff

Use [the server deployment guide](agent-runtime-server-deployment.md) and
[the public delivery gates](agent-runtime-delivery-tests.md) for a repeat run.
Keep source revision, image IDs/registry digests, Worker binaries and approved
Profile/template references together in the operator manifest. Before replacing
services, verify ownership, private listeners, readiness, storage mounts and the
previous image. Do not retag an old image and call it a new build.

If an upgrade fails, stop new admission, checkpoint only quiescent owned Bindings,
restore the preserved API/Web deployment and the approved Worker configuration,
then run health checks, legacy regression and retained-download checks. Preserve
storage and compatible migrations. Do not downgrade an installed-state format to
an incompatible runtime. Re-enable only after repeating the relevant real gate.

This run leaves the candidate services available for history/download inspection,
but new remote execution requires restarting the dedicated E2B Worker or
re-establishing the approved private Linux test connection. No permanent gateway
or public Worker port was created. No branch merge, push or PR was performed.

See [the delivery checklist](agent-runtime-release-candidate.md) for the release
gate and [reproducible tests](agent-runtime-delivery-tests.md) for commands.

## Normal-use delivery follow-up (2026-10-08)

This follow-up prioritizes ordinary setup and original script execution; it does
not add another failure-injection, load or disaster-recovery round.

- The original DOCX/PDF browser acceptance from October 7–8 also passed on both
  backends: original ZIP import/preview, binding, uploaded CSV, original DOCX
  validation/LibreOffice and PDF image helpers, real downloads and retained-file
  recovery. Downloaded DOCX XML, PDF text and PNGs were independently checked for
  the test marker and total 30. This records existing evidence, not a new browser
  run or a claim that every model-generated document is correct.
- The new narrow document gate passed against the real Linux gVisor Worker in
  36.34 seconds and real E2B in 65.68 seconds. Each produced DOCX, two PDFs and two
  PNGs through the API's export path and checked measured Job duration. These five
  files per backend use the integration test's in-memory export store; this run
  is not additional browser-download evidence.
- The E2B run used the new Worker container and public Compose example on a private
  existing Docker network, with UID 65532, retained state volume and no published
  port. Its disposable runtime database, container and volume were removed after
  acceptance. The Linux run reused the approved native Worker/template rather
  than upgrading its service or shared containerd.
- Native Worker export and container builds passed from the new shared Dockerfile.
  The standard execution image built successfully and passed its Python, Node,
  offline XML catalog and native-module Profile checks. The approved remote
  execution images/E2B templates were not replaced by this build.
- Shell results now carry elapsed time from Worker start/finish timestamps. UI
  tests prefer that measurement, retain a genuinely measured zero and hide old
  unmeasured zero traces; missing/invalid timestamps do not invent a duration.
- Focused API tests/vet, UI presentation tests, type-check and lint passed. Lint
  retains 73 existing warnings and no errors. No full historical matrix was rerun.

The release workflow now builds Worker/runtime images and a checksummed native
Worker archive with public setup examples. Local workflow/static validation is
not a published release or a dispatched hosted CI run. Start with
[setup and use](agent-runtime-quickstart.md); deployment configuration and keys
remain operator-owned and are not bundled.
