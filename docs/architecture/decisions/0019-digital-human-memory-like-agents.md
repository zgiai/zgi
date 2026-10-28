# ADR 0019: DigitalHuman Memory Works Like Agent Memory

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0014, ADR 0015, ADR 0017
- Amends: ADR 0014 (the deployment switch and the legacy tools), ADR 0015 (the person's own memory keeps no categories)

## Context

After ADR 0017, an Agent keeps each person's memory as files the model names and organizes, guided by memory instructions, with one total limit per person. DigitalHumans still differed in three ways:

- **A deployment switch.** The file view of ADR 0014 was off unless `ZGI_DIGITAL_HUMAN_MEMORY_FILES_ENABLED=true`. Without it, a run used the legacy tools:
  - `retrieve_digital_human_memory` to search memory;
  - `manage_digital_human_memory` to change the DigitalHuman's own memory;
  - `mutate_user_relationship_memory` to change the person's memory.

  Every evaluation since ADR 0014 measured the file view as clearly better. The DigitalHuman suite before phase 3 scored 86/87 explicit with files against 60/87 with the legacy tools.
- **Categories for the person's memory.** A person's memory with a DigitalHuman (`/user/`) was a list of items, each in one of four categories (`profile`, `preference`, `constraint`, `relationship`), at most 30 items, rendered up to 4,000 characters. An Agent's is free files bounded by 10,000 characters in total.
- **A default that kept automatic memory off.** A person's own switch for automatic memory started off, so automatic memory never ran unless they found and turned it on. The Agent default already changed in `202610261600000000`.

## Decision

1. **Memory is always files.**
   - The deployment switch is removed. Every DigitalHuman run gets:
     - the recalled files and the index;
     - `memory_view` and `memory_search`;
     - `memory_edit` for the directories the run may write;
     - `conversation_search`.
   - The three legacy tools and their guidance are removed.
2. **The person's memory is the Agent's file model.**
   - `/user/` files use the `files` contract of ADR 0017: any valid name (`agentmemory.ValidMemoryKey`), no categories, and 10,000 characters in total per person.
   - A full memory still keeps a new file in the person's archive.
   - Existing items keep their keys and content; their stored category is no longer used.
   - The DigitalHuman's notes on the person (`notes/` keys, ADR 0015) are not the person's memory. They keep their tags and their own rules, and do not count toward the person's total.
   - All of the person's files are shown in the run, not only those that fit in 4,000 characters.
3. **Automatic memory follows the consumer.**
   - A new relationship, with an Agent or a DigitalHuman, starts with the person's switch on.
   - Migration `202610261700000000` turns it on once for DigitalHuman relationships whose settings were never changed.
   - Whether automatic memory runs is still decided by the DigitalHuman's own setting and the deployment fuse `ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED`.
   - The operator has the Agent's two switches in the published memory policy: `user_memory_enabled` turns remembering each person off, and `user_auto_capture_enabled` turns its automatic part off. Both are on when missing.
4. **Profile pages show both consumers the same way:** topics of facts named by their files.

Not in this decision: background consolidation of `/user/` after a conversation, as Agents have. A DigitalHuman remembers about the person only within the conversation, explicitly or, when allowed, proactively. Adding consolidation is a separate step with its own token cost.

## Consequences

- One memory model and one set of tools for Agents and DigitalHumans.
- Deployments that never set the switch move from the legacy tools to the file view on upgrade. Nothing is migrated for it: the file view reads the same tables.
- `ZGI_DIGITAL_HUMAN_MEMORY_FILES_ENABLED` has no effect and is removed from the example environments.
- A person with more than 10,000 characters of DigitalHuman memory keeps it; new writes are refused until something is removed or archived, as with an Agent.
