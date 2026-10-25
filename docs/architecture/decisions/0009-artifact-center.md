# ADR 0009: Artifact Center for Agent Deliverables

- Status: Accepted
- Date: 2026-10-21
- Builds on: ADR 0002, ADR 0004, ADR 0006, ADR 0007, ADR 0008

## Context

DigitalHumans and agents produce files and documents, but nothing keeps them as results that outlive a conversation:

- Generated files live in `tool_files`. Each one carries an execution identity (application, entry, conversation) and is listed only in the generating run's trace or message metadata (`generated_files`, `conversation_artifacts`). A file has no title, no version, no task, and no visibility of its own. A weekly report run every week produces unrelated files.
- Channel delivery attaches up to eight files of the final run, natively when the provider supports it and as signed links otherwise. Signed links are HMAC URLs valid for up to 30 days. They cannot be revoked and are not audited. Feishu sends images as files, and DingTalk sends links only.
- No model tool publishes or shares a result on purpose. Sharing happens only as a side effect of a run's reply.
- Task reports (ADR 0008) reserve a `files` column, but it is always empty.
- The organization has a file manager (`upload_files`) and a knowledge base (`datalibrary`). Neither models versions, provenance, task visibility, or revocable sharing.
- DigitalHumans already keep cross-session behavior in memory (ADR 0002, ADR 0004). Memory has classes (self and business), types (identity, experience, decision, fact, and others), business scopes (workspace, project, customer, campaign, channel), and a source-trust policy.
- External end users (ADR 0006) need the same results without ever seeing another user's or the team's.

## Decision

### Results, rules, and knowledge stay apart

Cross-session persistence covers three different things, kept in three places:

| Kind | What it is | Used by | Lives in |
|---|---|---|---|
| Artifact | A deliverable with versions, comments, and shares | People who read, review, and distribute it | The new artifact center |
| Rule | A constraint on how the DigitalHuman works, such as "write weekly reports in this structure" | The DigitalHuman in later runs | Memory, under the existing trust policy |
| Knowledge | Reference material | Retrieval | The knowledge base |

They are connected by explicit conversions, never merged:

- **Use as template:** an accepted artifact version creates a rule memory that references it. The rule passes the same source-trust policy as any other memory.
- **Add to knowledge base:** an artifact version is copied into the knowledge base as a document asset.
- **Save to files:** an artifact version is copied into the file manager.

### A platform capability with product adapters

The artifact center is a capability in `internal/capabilities/artifacts`. It sits beside `chatruntime`, `usersubject`, `memoryruntime`, and `shortlink`, which DigitalHumans and agents already share.

- **The capability owns:**
  - artifacts, versions, comments, and share links;
  - references to stored files;
  - the ownership and visibility model, and the read and write checks that enforce it.
- **Each product implements an authorization adapter.** DigitalHumans come first; agent applications can adopt the center later without new storage.
- **The DigitalHuman adapter owns:**
  - the model tools;
  - links to tasks, runs, and reports;
  - the mapping from end-user instances to owners;
  - channel distribution and notifications.
- **Surfaces:**
  - a console-wide artifact center;
  - an Artifacts page for each DigitalHuman;
  - "My artifacts" in the WebApp;
  - external API endpoints;
  - a public share page.

The capability is not a general cloud drive. It holds what agents deliver.

### Ownership and visibility

An artifact is scoped by:

- **Organization and workspace.**
- **Application:** its type (`digital_human` or `agent`) and identifier.
- **Owner:** an account, an API end user, the workspace, or the application itself.
- **Visibility:** owner only, team, or application public.
- **An optional business scope:** project, customer, or campaign, reusing the memory scope vocabulary.

Artifacts are stored by owner, not by conversation, so they persist across sessions: the same owner reaches them from any conversation.

These rules are enforced in the capability, not left to pages or tools:

- **End users are isolated.** A run for an external end user reads only that user's artifacts and the application's public artifacts. It never reads team artifacts or another user's.
- **Visibility follows the source task.** An artifact produced by a task starts with that task's visibility: requester only or team (ADR 0005, ADR 0008).
- **Operators can see end users' artifacts by default.** Members with management permission on the DigitalHuman may read them for support and audit. A per-DigitalHuman **privacy mode** in the external settings restricts them to the end user alone.
- **The application public tier is part of the model but ships later.** Public artifacts are deliverables offered for download, such as templates and manuals; knowledge stays in the knowledge base.
- **End users' artifacts follow their instance (ADR 0006).** They are exported and erased with it.

### Artifacts and versions

- **Types:** document (Markdown stored in the database), file, image (with a thumbnail), table (CSV or XLSX), and collection. Audio and video come later.
- **Versions are immutable.** Every revision creates a new version. A recurring task updates the same artifact rather than creating a new one.
- **Files are reused, not copied.** A file version references a persistent `tool_files` record. Identical content is not stored twice, because each version records a checksum.
- **Each version records where it came from:** the task, run, report, or conversation that produced it, or a manual upload.

### Explicit capture

Files a run produces stay in that run's record, as they do today. An artifact is created only when:

- the DigitalHuman calls the new `publish_artifact` tool, giving a title, a type, a source file or content, and an optional task;
- a task report references files (the report tool gains a `files` field, and ADR 0008's `files` column is used);
- a person saves a run or conversation file to the artifact center.

### Documents: written by the DigitalHuman, reviewed by people

DigitalHumans write and revise documents. People mostly read, comment, and send feedback; they rarely edit. So:

- **Markdown is the canonical format.** Models write it reliably, and the console already renders it (react-markdown with tables, math, and code highlighting).
- **Every block has an identifier.** Saving a version splits it into blocks (heading, paragraph, list item, table, image, code), each with an identifier. The DigitalHuman revises through block operations (replace, insert after, delete), so untouched blocks keep their identifiers.
- **Comments anchor like W3C Web Annotations.** Each comment stores a block identifier and a text-quote selector (the exact text plus a short prefix and suffix).
  - On a new version, a comment re-anchors within its block, then by fuzzy-matching the quote across the document.
  - A comment that no longer matches stays in the side panel, marked as changed, with its original quote.
- **A comment records:**
  - its anchor and body;
  - a kind: question, change request, or approval;
  - a state: open, resolved, or addressed in a given version;
  - an author: a member, an end user, or the DigitalHuman;
  - its replies.
- **The DigitalHuman closes the loop.** "Send to the DigitalHuman" hands it the open comments with their quotes. It revises the document into a new version and marks the comments it addressed. Reviewers then compare the two versions block by block. For an artifact produced by a task, this reuses report rework (ADR 0008).
- **New model tools:** read an artifact with its block identifiers, list comments, reply to a comment, and revise with block operations naming the comments addressed.
- **Editor choice:**
  - Reading and commenting use the existing Markdown renderer plus our own anchoring. Recogito's BSD-licensed text annotator may be used for range selection.
  - Occasional human edits open the document in the Tiptap editor the console already uses, or in the Markdown source, and save a new version.
  - We do not adopt a collaborative block editor such as BlockNote. It would make its own block format canonical and require Yjs.
  - We do not use Tiptap's commercial comments, which are tied to its cloud.
  - If tracked-change review becomes necessary, Plate's suggestion plugin is the candidate. It does not change the canonical format.
- **Images and PDFs** get region and page comments in a later phase (Annotorious for image regions). Office files remain view-only through the existing previews.

### Sharing

**Share links** replace bare signed URLs for artifacts:

- Each link records:
  - the artifact, pinned to one version or following the latest;
  - its audience: organization members, members of a given channel conversation, or anyone with the link;
  - an expiry and a revocation time;
  - whether downloads and comments are allowed;
  - who created it (a person or a DigitalHuman run);
  - an access count and the time of last access.
- Tokens are random. Only their hash is stored.
- Opening a link checks the token and then issues a short-lived signed URL for the content, so revocation takes effect at once.
- **Public links ("anyone with the link") are allowed.** An organization setting lets administrators turn them off; they are on by default.
- Anonymous visitors can only view. Commenting requires a signed-in member or end user and must be allowed on the link.

**Channel sharing:**

- The DigitalHuman shares on purpose with a `share_artifact` tool. It names the artifact, a target (the current conversation or a bound channel conversation), and a form (attachment, link, inline content, or card).
- Delivery degrades according to the channel's capabilities:
  - a native attachment when the provider supports it and the size fits;
  - a summary with a link for documents;
  - a native image message where supported (a new `NativeImages` provider capability; Feishu gains image upload);
  - a link otherwise.
- Sharing is an outward side effect governed by tool approval. The first share to a new target needs confirmation.

**Notifications** from ADR 0008 carry share links to the artifacts a report references. DigitalHuman webhooks gain an `artifact.published` event.

### Pages

- **Layout:** artifacts are browsed in three panes.
  - The left pane shows recent artifacts, artifacts by task, by type, and shared with me, plus tags.
  - The center pane is a grid or list with previews, searchable by title and document content.
  - The right pane shows the preview, version history, provenance links (task, report, run), and a share panel with links, revocation, channel shares, and access counts.
- **Documents open in a reading view.** Comments sit in the right margin, aligned with their highlights. A toolbar offers the version picker, a comparison with the previous version, "Send to the DigitalHuman (n open)", and Share.
- **Navigation:** the DigitalHuman sidebar becomes Tasks, Reports, and Artifacts. Reports show their artifacts as cards with preview, download, and share.

### Retention and quotas

- Artifacts and their files are persistent. Each workspace has a storage quota.
- External end users' artifacts are kept until erased with their instance. A DigitalHuman may set a retention period for them.
- Run files that were never captured keep the existing run-file lifecycle.

### Delivery

0. **Reports carry files:** the report tool accepts file references, and report details show file cards. This needs no new tables.
1. **Artifact core:** the capability with artifacts and versions, the ownership model and its checks, `publish_artifact`, automatic capture of report files, and the DigitalHuman Artifacts page with preview, versions, and provenance.
2. **Sharing:** revocable share links, the share panel, the public share page, `share_artifact` with approval, native Feishu images, and share links in notifications.
3. **Document review:** block identifiers and block-level comments, the DigitalHuman's read, comment, and revise tools, version comparison, the "send to the DigitalHuman" loop, then text-range anchoring.
4. **Breadth:**
   - the console-wide center and "My artifacts" in the WebApp;
   - external API endpoints and the `artifact.published` webhook;
   - conversion to rule memory, to knowledge, and to files;
   - image and PDF comments, occasional human editing, full-text search;
   - the application public tier, privacy mode, and quotas.

## Consequences

- Results outlive conversations and accumulate as versions instead of scattered files.
- Rules keep their trust policy in memory, and knowledge stays in the knowledge base. The artifact center connects to both only through explicit conversions.
- One ownership model, enforced in one place, keeps external end users isolated from each other and from the team. Agent applications can adopt the center through an adapter.
- Shared links become revocable and audited. Public links are possible but can be switched off per organization.
- Documents stay in a model-friendly format. Comments survive revisions, and review feedback flows back to the DigitalHuman as structured input.
- New tables hold artifacts, versions, comments, and share links. Files continue to live in `tool_files` and the configured storage.
- Defaults chosen here and open to adjustment:
  - the artifact center is a separate layer, not part of the file manager;
  - public links are allowed and can be switched off by administrators;
  - capture is explicit;
  - documents use Markdown with our own anchoring rather than a collaborative editor;
  - operators can see end users' artifacts unless privacy mode is on;
  - the application public tier ships later.
