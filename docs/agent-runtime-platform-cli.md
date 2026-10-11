# Agent Runtime platform CLI

M6C extends the existing authenticated console execution environment. It does not
grant public API, WebApp or Runtime Workspace callers Shell or platform access.
The operator controls admission; Agent configuration remains an enabled flag.

## Delivery boundaries

1. Accept the pending process-note and execution-card changes through a new live
   console run. Reuse the previous backend installation, restoration and isolation
   evidence; do not rerun the complete standard-Skill matrix without a change that
   affects it. Investigate the known PID-exhaustion channel failure separately.
2. Introduce ephemeral Job-scoped grants and bounded idempotent platform requests.
3. Relay requests over the existing authenticated Worker control connection and a
   separate in-sandbox Unix socket. Do not add platform API hosts to package egress.
4. Provide file read/export, governed model calls and already bound tool calls.
   Reuse file ownership, model billing, quota, tool authorization and approvals.
5. Validate the new contract on native gVisor, real E2B and console downloads.

Kata, arbitrary networking, shared dependency caches and rootfs layers are outside
this iteration. The contract and scoped acceptance evidence are recorded separately
below; neither implies production certification or unrestricted Skill compatibility.

## Authorization and requests

Grants are bound to organization, subject, Agent, conversation, Run, Binding,
Binding generation and Job. The raw random grant is delivered once and is never
stored in a database, receipt, trace or checkpoint. Closing the Job or Run cancels
in-flight calls and invalidates the grant. Reconstructed compute receives a new
grant; an old session cannot be resumed by restoring its token.

Every request rechecks current authorization, including cached responses. The
same request ID with changed arguments is rejected; duplicates cannot invoke a
handler twice. A failed or indeterminate operation is not automatically replayed.
Receipts and grants are bounded and process-local; losing the owning process
fails closed rather than guessing whether a side effect should be repeated.

The initial authorization component uses at most 32 requests per Job, 64 KiB per
request, 32 KiB per response and a grant lifetime no longer than five minutes.
File bytes must use the bounded file data plane, not large inline JSON responses.
Public error projection uses the existing AppError catalog; diagnostic causes and
response bodies are not public audit summaries.

Pending approval/input is an explicit receipt, not an indefinitely blocked Shell call.
Approval continuation must bind to the existing governance interaction and retain
the operation identity. It must not invent a second approval system or bypass an
existing one. The parent run carries the existing governance, Connector or
workflow continuation payload. The Job is cancelled before further requests are
dispatched. Resuming the original script automatically is not promised: an
indeterminate side effect is never replayed. Console confirmation/rejection uses
the existing frozen-call continuation, not restoration of the original script.

The authenticated console CLI bridge has a typed, in-process interactive approval
capability. Ordinary parent Agent calls and external callers retain their existing
non-interactive behavior. Client JSON cannot set the capability, and it cannot
convert a missing or revoked persistent database/workflow/knowledge binding into
an approval prompt. Approved operations still use the existing frozen invocation
and one-shot grant, not a restored Shell grant.

The authenticated Agent draft transport exposes the existing confirmation UI;
public transports do not acquire a continuation endpoint. Current runtime admission
is checked again before approval. Rejection remains available after opt-in is
revoked so a pending decision can be closed without granting execution rights.
Generic non-file business results survive continuation summaries after identifier
sanitization, up to 32 KiB. File-operation projections remain unchanged. Historical
Shell approval traces are not new pending work: the original Shell ran until the
approval boundary and was stopped, while only the frozen business call resumes.

## Operator admission and CLI contract

Set `ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED=true` on both the API and its dedicated
Worker only after selecting a tested image/template. The default is false. Worker
capability negotiation requires platform protocol version 1; older templates fail
closed when the Worker requires this capability. The API retains ordinary Shell
behavior when the capability is absent. Agent configuration gains no new fields.
Runtime Agent startup receives `ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED` from the
Worker; E2B applies this through its authenticated bootstrap payload so a template
environment override cannot silently change admission.

The sandbox image contains `zgi`; `zgi --help` requires no grant. Inside an
admitted Job, examples are:

```sh
zgi file read --id input-1 --ref "$AUTHORIZED_INPUT_ID" --path inputs/source.csv
zgi file read --id reference-1 --ref "skill:$SKILL_ID:$PACKAGE_SHA256:references/guide.md" --path inputs/guide.md
zgi model call --id model-1 --prompt 'Summarize this permitted input.' --max-tokens 512
zgi tool call --id tool-1 --name "$EXPOSED_BUSINESS_FUNCTION" --arguments '{"value":1}'
zgi file export --id export-1 --path artifacts/result.json
zgi request status --id tool-1
```

References are current-turn admitted inputs or files in a currently loaded Skill
at its exact package digest. A reference cannot select another tenant, conversation,
Binding, host path or storage URL. Files use the existing transfer limit and
persistent ToolFile store. An export contributes a trusted artifact to the parent
Shell result and existing file event/download-card chain.

Model requests use the Agent's authorized provider/model and billing context, with
no conversation history, tools, provider settings or credentials exposed to the
script. Prompt size is bounded to 32 KiB, requested output to 1–4096 tokens, and
response text to 24 KiB. Provider usage is charged and counted toward the current
task budget, including unsuccessful/truncated requests with recorded usage.

Tool calls can use only currently exposed business functions of loaded Skills.
They reenter existing authorization, budgets, governance, tracing and approvals;
they cannot activate Skills, recurse into Shell or call arbitrary API endpoints.
CLI exit codes are 0 for completed, 1 for failed, 2 for invalid arguments, and 75
for required console continuation. A script must stop on 75, not invent a new ID
to bypass approval. Response JSON is model-facing, not a public audit log.

The control process runs as root while Jobs remain non-root. Socket directories
and bootstrap credentials are protected from Job writes. Plaintext Job grants
are redacted from stdout/stderr even across chunk boundaries; this is not a
guarantee against a script deliberately encoding or copying its own transient
grant. Such a copied grant expires with the owning Job and is not restorable.
No platform destination is added to the package-source allowlist.

## Current evidence

Go 1.26.2 Linux race tests and vet pass for the run-owned authorization component,
including scope mismatch, expiration, cancellation, live revocation, concurrent
duplicates, failed-call receipts and bounded request counts. The Runtime Agent
remote client now confirms a terminal result after a cancellation acknowledgement;
an unavailable, non-terminal or unsafe result invokes the existing quarantine
path. Its observation-failure regression tests and vet pass. An incremental native
gVisor PID-exhaustion test also passes: the severed channel yields exit -1 with
termination unconfirmed, real provider suspension leaves no cgroup processes, and
resuming restores an exact retained-file marker. The Worker quarantine persistence
transition remains separately covered by domain tests, not that provider test.

Linux race tests and vet also pass for the bounded relay and CLI. A new process
integration test uses an actual child process, separate Unix socket and Runtime
Agent authenticated HTTP control connection to relay a file-export request and
deliver its JSON receipt. Caller-supplied grants are stripped, legacy capability
defaults remain off and process completion removes the grant. This test does not
persist a real product file and is not native gVisor or E2B acceptance.

The Worker data plane now has owner-, generation- and live-Job-checked request and
response endpoints. Only non-sensitive admission metadata may enter persisted Job
environment; the platform grant namespace is rejected. Runtime Agent startup has
an opt-in platform socket. API Shell now has an optional relay using the existing
authenticated Worker gateway and live console permission/opt-in checks. Targeted
Linux race tests and vet cover model routing/billing budgets, real calculator
invocation through the existing engine, input/Skill file ownership and immutable
versions, Job generation fences, duplicate dispatch and approval-driven stopping.
The normal Token budget usage observer's typed-nil error was corrected with a
regression test. These are component and process tests, not real-backend evidence.

The real common-backend transport contract passes on native containerd/gVisor
and an E2B template derived from the accepted standard package runtime. It checks
non-root Jobs, inaccessible control credentials, actual CLI subprocesses, file
bytes, duplicate receipts, diagnostics, stopping and grant cleanup. Model/tool
responses in this transport test are stubs, not business acceptance.

Dedicated opt-in API/Worker deployments now pass a real console model/export
sample on both backends: one model request returns `M6C_OK`, a duplicate with the
same ID returns identical JSON, stderr is visible, and CLI-exported JSON downloads
through the existing file card. Billing records contain exactly one successful
short-prompt invocation per backend sample (134 and 74 tokens respectively);
suppressed invocation content has no persisted input/output body. Ordinary parent
Agent model calls and their failed routing attempts are separate records.

This acceptance found and corrected three integration gaps: Job Binding generation
was not persisted, submission activity changed the generation before grant binding,
and queue polling raced with process startup/terminal publication. The additive
`2026100801` migration persists generation for new Jobs; historical Jobs retain zero
and cannot gain platform authorization. Real PostgreSQL migration/repository tests
pass. Platform file transfers recheck the owning live Job without renewing the
Binding generation. Ordinary external transfers still fence a current grant.

A missing queue (404) while the authenticated owning Job remains queued/running
does not dispatch any action: polling waits within its existing timeout for process
startup or terminal status publication. Connection failures, ownership/generation
conflicts and resolution failures still fail closed. Claimed requests are never
requeued or silently replayed. Diagnostics contain stable phases/codes only, not
arguments, grants, upstream response bodies or transport URLs.

The native gVisor console sample additionally reads a current-turn synthetic
input, calls the loaded calculator for `21*2`, receives identical duplicate
receipts, and exports a downloaded JSON result containing `42`. The E2B console
sample also reads the exact loaded Skill package's `SKILL.md` (8,697 bytes) through
the CLI reference contract. Its 387-byte downloaded JSON confirms input content,
reference size, calculator output and duplicate equality. The ordinary calculator
catalog is unchanged; no runtime installation or new network destination is used.

Approval acceptance exposed the legacy Agent non-interactive policy and the missing
Agent console confirmation transport. The scoped console capability, route and UI
now pass race tests, vet and frontend checks, including external callers, forged
JSON flags, revoked runtime admission and missing resource bindings. Real gVisor
and E2B console samples each execute one Shell and one approved calculator call,
and report the actual value 42. An E2B rejection sample records one Shell, zero
calculator successes and a rejected decision. Disabling the test Agent's Linux
opt-in rejects confirmation while still allowing rejection; its original setting
is restored afterward. Temporary mandatory-approval calculator metadata is an
acceptance fixture, not a change to the ordinary system Skill catalog.

Final continuation-state samples also passed after correcting the model-facing
summary. The native gVisor sample recorded one Shell, one successful calculator
call and an approved decision; its final answer returned 42 and explicitly stated
that the original Shell was stopped, not still pending. The real E2B sample
recorded one Shell, zero calculator successes and a rejected decision; its final
answer distinguished that previously started Shell from the unexecuted business
call and reported no new pending operation. Neither sample replayed the Shell.
These assertions use both visible console answers and persisted invocation counts,
not model text alone. The temporary mandatory-approval API images were then
replaced with the normal catalog on both dedicated test services; both are healthy.

The related API and Sandbox package race tests and vet pass with Go 1.26.2.
Frontend transport/presentation/process-note tests, type-check and production
build pass; the full lint run has zero errors and 73 existing warnings. Previous
standard-Skill installation/isolation matrices are reused because this iteration
does not change their behavior. Admission remains limited to dedicated opt-in
acceptance deployments, not a general rollout or M8 production certification.

### Commit-boundary verification

The implementation was split into termination handling, Worker/CLI transport,
API authorization/model primitives, Agent service bridges, console approval,
chat presentation, and documentation. Each code group was checked from an
exported index tree without later working-tree changes. API Skill tests include
their tracked `docs/examples` fixtures. Runtime/API groups pass their targeted
race tests, vet and command builds; the approval and presentation trees pass
their frontend regressions and separate TypeScript checks. The final presentation
tree also passes the production build and full lint (zero errors, 73 existing
warnings). Changed Go sources pass gofmt verification on LF exports, avoiding
Windows archive line-ending conversion. Existing real gVisor, E2B and browser
acceptance above is reused; splitting commits did not rerun that external matrix.
