# Agent Runtime release-candidate checklist

This checklist closes the current standard-Skill iteration in four delivery steps.
Its step numbers are not the M0–M8 roadmap stages. Completing it does not certify
M6C, Kata/KVM or full production readiness. The implementation baseline for this
iteration is `99a7fab79` on `px/agent-runtime-execution-environment`.

## Delivery gates

| Step | Deliverable | Required evidence | Current status |
|---|---|---|---|
| 1. Reconcile contracts | Current stage record and explicit limits | Match code, existing real-backend results and public setup/recovery contracts; identify deferred shared caching and rootfs layers | Reconciled; no new deployment claim |
| 2. Reproducible gates | Public test commands and CI | Go 1.26.2; isolated PostgreSQL with required extensions; API/Sandbox tests and vet; Web lint/type-check/build; independently opted-in real contracts | Local gates verified; CI definitions validated, not yet dispatched |
| 3. Final deployment | Final tested API/Web and approved Workers | Unmodified Skill import, model-driven installation, execution, content-checked downloads; pause/rebuild; cancellation/timeout/retry; disabled rollout and legacy behavior on both backends | Verified in dedicated test environments on 2026-10-08; see the acceptance record |
| 4. Release candidate | Version manifest and rollback runbook | Upgrade/rollback drills, low-concurrency repeated runs, resource reconciliation, retained-file inventory and recovery verification | Pending |

Run the narrow gate before the full suite. Record failures, their cause and the
successful retry against the corrected revision. Mock tests do not replace a real
gVisor node, real E2B allocation/recovery or browser downloads. Missing credentials
or unavailable infrastructure are blockers, not passing or silently skipped gates.

Reproducible commands and CI prerequisites are documented in
[runtime delivery tests](agent-runtime-delivery-tests.md).

The tested revisions, real results, failure/retry history and remaining rollout
limits are recorded in [delivery acceptance](agent-runtime-delivery-acceptance.md).

## Acceptance matrix

Use the same input and expected content on both local gVisor and E2B:

- Import a standard Skill without adding a manifest or rewriting its scripts.
- Read an uploaded two-row CSV (`A=10`, `B=20`), generate office/media outputs and
  check values, formula caches, page/frame counts and hashes after real download.
- Diagnose a missing package, install through an approved source, and rerun. Check
  stdout, stderr, terminal Job status and the result, not only model prose.
- Pause and restore; then destroy only disposable compute and reconstruct it from
  retained storage. Check file hashes and native Python/Node imports. Damaged
  required dependency artifacts must block execution; repair and retry must work.
- Cancel and time out commands with descendants. No delayed output file may appear
  after termination. Exercise failed installation followed by an explicit retry.
- Reject cross-tenant access, unauthorized package sources, global Node installs
  and unapproved network destinations. Keep Job execution non-root.
- Disable creation/Shell admission, confirm new conversations have no Shell, and
  verify old exported files still download. Restore the switch and check a new Job.
- Run legacy Python/Node `run_script` input/output/error/Artifact regressions.

Browser acceptance uses a dedicated local test account read from
`ZGI_E2E_EMAIL`/`ZGI_E2E_PASSWORD`. Keep local values in ignored configuration;
never embed them in tests. Only disposable test environments may be altered.

## Version manifest and rollback

Keep a non-sensitive, operator-owned acceptance record outside Git for each run.
It must identify the source commit; API/Web image digests; Worker binary hashes;
approved local OCI digest and E2B template; capability and configuration revisions;
test commands/results; downloaded-file hashes; and start/end resource inventories.
Do not include credentials, raw environment dumps, private source URLs or user data.

1. Preserve the previous API/Web images and stopped deployment definitions before
   upgrade. Pin Worker and Profile/template versions together; do not downgrade
   a retained ledger to a runtime lacking its required version.
2. Upgrade only the dedicated test services, run readiness and the matrix, then
   repeat a small bounded concurrent workload. Check owned Jobs, Bindings, provider
   compute, containerd tasks and cleanup queues for leaks or incomplete states.
3. Stop admission, perform a controlled rollback and verify health, legacy execution
   and previously exported downloads. Never revert a database destructively or
   delete a Workspace to make a recovery failure disappear.
4. Restore the candidate, repeat a new execution and download, and reconcile owned
   resources. Delete only disposable resources created by the test; list retained
   files, paused environments and rollback resources explicitly.

Use the [server deployment guide](agent-runtime-server-deployment.md) for Worker
install/doctor/rollback commands. A private test connection is not a production
public Worker gateway. Do not open public Worker ports to bypass missing secure
connectivity.

The final gate requires a clean feature worktree, independently reviewable commits,
passing required evidence and an explicit retained-resource list. It does not
authorize merge, push, PR creation or deployment to unrelated environments.
