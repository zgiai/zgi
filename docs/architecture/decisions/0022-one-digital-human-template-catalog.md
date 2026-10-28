# ADR 0022: One Template Catalog for DigitalHumans

- Status: Accepted
- Date: 2026-10-27
- Builds on: ADR 0015 (memory instructions), ADR 0019, ADR 0021

## Context

DigitalHumans had two template systems that did not know about each other:

- **Creation templates.** Two templates were hard-coded in the web client: file generation and operations specialist. Each carried a persona, goals, rules, skills and defaults.
- **Persona templates.** The configuration page saved and applied workspace templates holding a name, persona, goals and rules.

Their problems:

- **Saved templates could not start a new DigitalHuman.** The creation page only listed the two hard-coded templates. The saved-template page nevertheless promised that saving one would make creating DigitalHumans faster.
- **The two kinds held different things.**
  - A creation template carried skills.
  - A saved template carried no skills and no memory instructions, so it lost what made a DigitalHuman able to work.
  - Neither set memory instructions. Memory instructions had templates of their own for five kinds of work that no creation template covered.
- **A saved template's name was also the DigitalHuman's name in conversation.** It could not be named for its purpose, and applying it renamed the DigitalHuman without asking.
- **Creation wrote text that did not hold up:**
  - Fixed Chinese headings in the persona whatever the language.
  - An instruction to the system inside the persona.
  - No safety rules for the blank ("custom") entry.
  - A memory policy with keys nothing reads (`enabled`, `ownership`). Its `enabled: false` looked like memory was off, although memory is always on (ADR 0019).
- **Missing capabilities were silent.**
  - The file template still claimed file generation when the workspace had no file skill.
  - The operations template was created and published without the server connection it depends on.

## Decision

1. **One catalog.**
   - `GET /digital-human-templates` lists the built-in templates in the caller's language followed by the workspace's saved templates.
   - Both the creation page and the configuration page use it. A saved template can now start a new DigitalHuman.
2. **A template holds what a DigitalHuman needs to work:**
   - title and description;
   - an optional name in conversation (saved templates only);
   - a responsibility;
   - persona, goals and rules;
   - memory instructions;
   - skills, each marked required or suggested;
   - whether it needs a connector connection.

   Connector connections themselves are never part of a template: they are credentials, chosen and verified per DigitalHuman.
3. **The built-in templates live in the API.**
   - The two existing templates stay.
   - Five are added, one per memory instructions template: customer support, sales follow-up, tutoring, e-commerce after-sales and internal helpdesk. Each carries that template's memory instructions.
   - Every built-in template's rules start with the shared safety rules. The blank entry gets the shared rules too, which the catalog returns alongside the templates.
4. **Saved templates keep their own title.**
   - The name in conversation is a separate field. Applying a template sets the name only when the template has one.
   - Saving captures the draft's memory instructions and the IDs of its bound skills.
   - Migration `202610271100000000` adds the new columns and copies each existing template's name into its name in conversation, so applying it behaves as before.
5. **Creation writes what the template says, and nothing else.**
   - The persona is the template's persona. A responsibility the user changed is added as a plain paragraph, with no fixed headings.
   - The description is the responsibility.
   - The memory policy holds only the memory instructions.
   - Creation is refused while a required skill is missing from the workspace or a needed connector connection is not selected.
   - The configuration page asks before applying a template over the current draft. Applying replaces the persona, goals, rules and memory instructions; skills are listed as suggestions to bind.

## Consequences

- Workspace templates become a real way to start DigitalHumans, and both pages show the same list.
- Template text is no longer shipped in the web client's translations. Built-in templates change with the API, in both languages.
- Memory policies written from now on carry only the keys the runtime reads. Existing drafts keep their old keys, which stay harmless.
- Creating from a template still publishes the first version right away; making creation stop at a reviewed draft is a separate decision.
