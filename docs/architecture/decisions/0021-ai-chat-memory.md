# ADR 0021: AI Chat Remembers Each Person

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0013, ADR 0017, ADR 0019, ADR 0020
- Completes: ADR 0013 phase 4 (AI chat becomes a memory consumer of its own; the account memory planner and its tables are retired)

## Context

Agents and DigitalHumans remember each person as files the model names and organizes (ADR 0017, ADR 0019). AI chat had no working memory:

- The chat runtime has no AI chat consumer in the relationship store. The access policy refuses the AI chat surface (`aichat_memory_disabled`).
- An older account memory is still wired in: the `memory` module, with `account_memory_*` tables, `/memory/me` routes, a per-turn planner (`usermemoryruntime`) and a memory switch in the chat input. The runtime forces it off on every turn, so it has done nothing for a long time.

## Decision

1. **AI chat is a memory consumer of its own** (`ai_chat`), with one memory per person.
   - The memory is the person's across organizations. It is personal, like an assistant that follows the person, not workspace data.
   - Its relationship has no organization or workspace: both are stored as the zero UUID, and its consumer is a fixed ID that stands for AI chat.
   - Only the person's own account can reach it. API end users and channels never do.
2. **It works like an Agent's memory.**
   - Files contract with the 10,000-character total.
   - Recalled files in the prompt.
   - `memory_view`, `memory_edit` and the archive search in the run.
   - The same in-turn guidance for explicit and automatic memory.
   - There are no builder instructions: AI chat has no builder. The general rules decide what is worth keeping.
3. **Where it applies.** The main AI chat and the console's contextual assistant share it, since both are the person's own assistant. External page chat and every Agent or DigitalHuman surface do not.
4. **Defaults and settings.**
   - Remembering and automatic memory both start on, under the deployment fuse `ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED`.
   - The person changes them, and sees, edits and undoes what is remembered, in "My memory", where AI chat appears as one assistant.
   - AI chat has a memory entry that opens that page. Replies show what was remembered, as with Agents.
5. **The account memory is retired without migration.**
   - Removed: the planner, the `memory` module's service and routes, the hidden `user-memory` skill, and the chat input's memory switch with its account memory manager.
   - The `account_memory_*` tables stay as retired tables, because up migrations never drop tables. Nothing reads or writes them.
   - Their data is not migrated. The feature has been off, so there is nothing current to carry over.

## Consequences

- One memory model and one management page for Agents, DigitalHumans and AI chat. ADR 0013's goal is met.
- A person's AI chat memory is visible from every organization they belong to. Organization administrators have no access to it.
- Leaving an organization does not remove it; deleting it is the person's choice in "My memory".
- The check constraint on `user_memory_relationships.consumer_type` gains `ai_chat`.
