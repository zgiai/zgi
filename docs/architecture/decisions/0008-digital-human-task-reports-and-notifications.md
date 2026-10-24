# ADR 0008: DigitalHuman Task Reports and Notification Channels

- Status: Accepted
- Date: 2026-10-21
- Builds on: ADR 0005, ADR 0006, ADR 0007

## Context

A DigitalHuman task is work a requester hands to the DigitalHuman (ADR 0005). The task model is sound, but the path from "hand over work" to "get the result" is not:

- "Shared tasks" names an internal owner kind (`digital_human`), a visibility (`workspace`), and a page at once. Tasks requested in a conversation do not appear on the "shared" page, and managers see private tasks there once task visibility is on.
- A run's result is a text field on the run. Tasks created in the task center or the My tasks panel are always `task_only`, so their results exist only in the run history. The list shows only "Succeeded" or "Failed".
- Failures and expired proposals notify no one.
- Success criteria are only pasted into the run prompt. Nothing checks them, and there is no way to accept a result or ask for rework.
- The platform has no in-app notification or inbox. Email and SMS senders exist only for generic automation tasks.
- External API and WebApp end users can create tasks, but they have no way to receive or review results apart from a delivery into the original conversation.

## Decision

Users see three concepts: a **task** is work handed to the DigitalHuman, a **report** is what each run hands back, and a **notification** brings a report or a problem to the people who need it.

### Tasks

- A task shows its **requester**: the person or channel group that asked for it. A task created in the task center has the account that created it as its requester. The internal owner kind is unchanged and is not shown as a task type.
- A task offers two visibilities: **requester only** and **team**. Channel tasks stay visible to their conversation only.
  - The requester may share a task with the team or withdraw it.
  - A task center manager may narrow another requester's task but never widen it, because widening would publish someone else's request.
- Approval follows one rule: what the DigitalHuman proposes needs confirmation; what a person creates directly takes effect.
- Users see one combined status: awaiting confirmation, scheduled, running, **report to review**, needs attention (failed or blocked), paused, done, or canceled.
- The task page is named "Tasks" ("任务"). It has a Tasks tab and a Reports tab, with filters for "requested by me", "team", and "all". The "all" filter is for managers only.

### Reports

Every finished run produces a report, including a failed run:

| Part | Content |
|---|---|
| Outcome | Done, partly done, or not done, with a one-line reason |
| Result | The summary text |
| Files | Files the run produced |
| Success criteria | Each criterion marked met, not met, or unknown. The DigitalHuman assesses it, and a person confirms |
| Issues and next steps | What blocked the run and what needs a decision |
| Review state | New, then seen, then **accepted** or **rework requested** |

- At the end of a task run, the DigitalHuman submits the report through a dedicated report tool. A run that ends without one, or fails, gets a system-written failure report.
- Rework records the reviewer's feedback and runs the task again with it.
- Reports are visible to whoever can see the task.
- An end user's reports belong to their end-user instance (ADR 0006) and are exported and erased with it.

### Notification channels

A report is stored once. Notifications deliver it, and failures and pending approvals, through channels:

| Channel | Owner | Notes |
|---|---|---|
| In-app inbox | Built in for every account | Always on and cannot be removed. It is the fallback when nothing else is bound or a delivery fails |
| Source conversation | The task's origin | Added automatically when the task was created in a conversation |
| DigitalHuman channel group | The DigitalHuman's channel bindings | Only groups the DigitalHuman already serves |
| Email | The account | Verified before use, and sent through the existing email sender |
| Webhook | The DigitalHuman, overridable per API key | Signed and retried. Used by external API integrations |
| Personal IM | The account, through Connections (ADR 0007) | A direct message sent with the account's own Feishu or DingTalk Connection; see Stage 4 |

- Team channels are registered at workspace level by managers. Personal channels are bound by their owner.
- A private task can never notify a team channel.
- A task's **"report to"** setting lists its channels. The defaults depend on where the task came from:
  - created in a conversation: in-app inbox and that conversation;
  - created in a channel: in-app inbox and that group;
  - created by hand: in-app inbox and the requester's default channel.
- Only the requester changes the list. A manager may only remove entries.
- Approval cards show where reports will go.
- External channels receive the outcome line and a link by default. A team channel may be configured to carry the full report and its files.
- A failed delivery is retried. If it still fails, the report stays in the inbox, marked with the failure, and shows under "needs attention".
- Each user chooses which events (new report, failure, approval needed) go to which channel. By default every event goes in-app, and failures also go to a verified email.
- The in-app bell lives in the console header and is not limited to DigitalHuman pages. It is the first source of a general notification center, but this decision does not build one.

### External end users

- **API:** endpoints to list a user's reports (optionally unread only), read one, mark it read, accept it, or request rework. Files use the existing file endpoints with subject checks.
  - Webhook events: `report.created`, `task.failed`, and `task.approval_required`. They carry the `user`, the task and report identifiers, and the outcome line; full content is fetched through the API.
  - Integrations without a webhook poll for unread reports.
- **WebApp:** signed-in end users see reports, unread counts, and accept or rework in My tasks, and report cards in the conversation. Email for WebApp users is off unless the operator enables it. Anonymous sessions cannot create tasks, so they have no reports.
- End users never notify team channels. Their rework needs no operator review, because it only reruns their own task.

### Stage 4: personal IM, digests, and quiet hours

Decided 2026-10-23.

- **Personal IM channel.** A member binds a channel of type `im` to one of their own ready Connections of a Connector that can send a direct message, plus their own user ID in that tool: a Feishu `open_id` (`feishu.message.send`) or a DingTalk user ID (`dingtalk.messages.send_direct`). Connectors that can only reply inside a conversation (WeCom, WeChat) are not offered.
  - Like email, the channel is verified first: a six-digit code is sent through it, and the member enters it.
  - Sending uses the provider runtime directly with the Connection's credentials, outside model runs and their confirmations, because the member set it up for this purpose. The message carries the outcome line and a link, like other external channels.
  - Removing or disconnecting the Connection makes deliveries fail, so they stay in the inbox marked as failed.
- **Digests for recurring tasks.** A scheduled (cron) task's "report to" setting chooses each run, a daily digest, or a weekly digest. A digest only affects external channels (email, personal IM, webhooks, team channels): their report notifications are held and sent together as one message. The in-app inbox still gets every report, and failures and pending approvals are always sent at once.
  - A daily digest is sent at 09:00 and a weekly digest on Monday at 09:00, in the recipient account's time zone (UTC when unset). A digest for a team channel uses the task requester's time zone.
  - A digest lists each held report's task, outcome line, and time, with one link to the Reports tab.
- **Quiet hours.** Each account may set one daily window (start and end, in its time zone), for example 22:00 to 08:00. Deliveries to that account's own email and personal IM channels during the window are delayed to its end. The inbox, webhooks, and team channels are not affected.

### Delivery

1. Task wording and rules: the "Tasks" name and tabs, the two visibilities with narrow-only for managers, result summaries in the list, failure delivery to the requester, and "needs attention" covering the reader's own tasks.
2. Reports and the in-app inbox: the report record and tool, review states with accept and rework, the Reports tab, report cards in conversations and channels, the "report to" setting with in-app, source conversation, and channel groups, plus the API endpoints and WebApp views for end users.
3. Notification channels: binding and verification, per-user event settings, email, and the webhook.
4. Personal IM channels, digests for recurring tasks, and quiet hours.

## Consequences

- Every task result has one place to be found and reaches its requester, including failures.
- Success criteria become checkable, and people can accept or reject a result.
- Notification channels are pluggable: a new communication tool is a new channel provider, and the in-app inbox covers anyone without one.
- The existing `delivery_config` becomes a list of report destinations. Existing tasks convert without changing behavior: `source_conversation` becomes the source conversation or channel group, and `task_only` becomes in-app only.
- New tables hold reports, review states, notification channels, and notification deliveries. Report history replaces the N+1 run-history queries with one paged query.
- Defaults chosen here and open to adjustment: team channels at workspace level, the bell in the console header, external channels carrying the outcome line and a link, webhooks per DigitalHuman overridable per API key, and end-user rework without operator review.
