# ADR 0043: Prompt Framework Cleanup

- Status: Accepted
- Date: 2026-10-30

## Context

A captured Agent request showed the order the native loop sends:

1. the system prompt: the builder's prompt, the memory rules, the candidate skill directory and the native-loop rules;
2. the tools;
3. the history;
4. the current turn: memory, execution notes, the turn boundary, then the user's request.

Changing content already comes last, so the prefix is cached (ADR 0032). The content itself had grown:

- **The native-loop rules** were one paragraph of twenty lines.
  - Several banned protocols the loop no longer offers: `load_skill`, `call_skill_tool`, `submit_intermediate_answer` and `submit_final_answer`.
  - One told the model to ignore skill documents that still named those wrappers. Six built-in skills did.
  - The process-note rule set token counts, such as "60 to 180 estimated tokens" and "1920 in total", which a model cannot count.
- **Every tool-loop run got the planning tools.** `update_plan` and `submit_turn_state` cost about 6,000 characters, even for a run with no business tool to plan.
  - Their descriptions were written for the console AI chat: "this same AIChat turn", "server-owned phases", "hidden attestations".
  - `submit_turn_state` still called `submit_intermediate_answer` a compatibility shortcut.
- **The skill directory was JSON with runtime fields.** Each candidate took about 700 characters, most of them fields the model cannot use: `source`, `runtime_type`, `timeout_seconds`, `supported_callers`, `exposure`.
  - With more than ten candidates, only five were listed.
  - When some were left out, the directory was ranked for the question and could not be cached.
- **Small skills cost a round.** Multiplying two numbers took three model calls: activate the calculator, call it, answer. Asking the time was the same.
- **Agents did not know the date.** Only DigitalHuman runs carried the current time. Agents and the AI chat called the time skill and answered in UTC.
- **The execution notes contradicted the history.**
  - They said older turns were represented only by their final answers, but the history keeps their tool calls.
  - They repeated memory edits as the latest operation.
  - They still mentioned intermediate answers.

## Decision

1. **The native-loop rules are short sections.**
   - The sections are Skills, Tools, Progress notes, Answers and Safety, without the banned wrappers and without token counts.
   - The six built-in skills name their tools directly ("call `evaluate_expression`"). That reads correctly in the native loop and in the legacy loop, which still accepts `call_skill_tool` for old turns.
   - `update_plan` and `submit_turn_state` get descriptions without console terms.
2. **The planning tools come with business tools.**
   - A tool-loop run offers `update_plan` and `submit_turn_state` only when it can take a business action: it has a skill that is not resident, which includes connector operations and bound knowledge, databases and workflows.
   - A run with only memory, attachments or resident skills does not offer them.
   - The choice is made once from the run's configuration, so the tool list stays the same across rounds and turns.
3. **The skill directory is one line per skill.**
   - Each line reads `- skill_id: description Use when: when_to_use`, in skill ID order, under a short header.
   - Every candidate that fits the 8,000-character budget is listed, so the directory is usually complete and therefore cached in the system prompt.
4. **Resident skills need no activation.**
   - A resident skill's tools are in the tool list from the first round of every turn, at the front, in a fixed order. Its document is not injected: its tools must be usable from their descriptions.
   - It is not a candidate in the directory and cannot be activated.
   - Calculator and time are resident. Their tool descriptions carry the rules their documents held.
5. **Time is an inherent tool.**
   - Every tool-loop run of an Agent, the AI chat and a DigitalHuman gets `current_time` and `date_calculate`, whether or not the time skill was selected.
   - The time skill no longer appears in the Agent skill picker. It stays in the catalog for workflows and for configurations that already list it, which keep working.
   - An organization that disabled the skill does not get the tools.
   - A run that would be a direct chat stays one: it does not become a tool loop for the time tools.
6. **Every run knows the current time.**
   - The turn context gives the date, the time to the minute, the weekday and the timezone. It comes after the history (ADR 0032), so it changes no cached prefix.
   - The timezone is the run's user timezone: the account's own setting, or Asia/Shanghai when unknown, as for DigitalHuman runs.
   - A continuation within 15 minutes of its message's creation keeps that time, so its requests extend the first run's.
   - DigitalHuman runs keep their own wording, which adds the task-schedule rules.
7. **The execution notes describe what they are.**
   - The header drops the claim about older turns and the intermediate-answer rule.
   - Memory tool calls are left out of the operation and tool history; the memory files already show the result.

8. **`update_plan` describes fields, not server bookkeeping.**
   - Its field descriptions say what each field is for, in a few words. The explanations of projected bindings, attestations and ledgers are gone.
   - The schema still takes exactly one of outcomes, plan or phase_updates. The tool description no longer suggests combining them.
9. **A turn's context is stored with its request.**
   - The current turn's reminders travel inside its user message: the time, turn context, memory, execution notes and turn boundary. The transcript stores that message as sent.
   - The next turn therefore replays the previous turn's request unchanged, and the provider's cache covers it up to the first message that differs. Before, the previous request ended at its reminders, which the history did not keep.
   - A turn context section or a memory block the replayed history already holds verbatim is not sent again. Unchanged memory is shown once per conversation, not once per turn. After a compaction, or when memory changes, it is sent again, and a memory block says it replaces any earlier one.
   - DigitalHuman runs give their turn context as sections, so unchanged connector notes and memory are not repeated either.
   - Guidance the loop adds right after the question is stored with it as well: the instructions of skills active from the start, the script packages and the workflow bindings. Guidance the replayed history already holds verbatim is not sent again, so a skill used in an earlier turn does not resend its instructions. A turn that sends a directory ranked for its question is stored without that guidance, since the directory changes every turn.
   - The cache of a previous turn still stops at its first tool call. That call carried its reasoning within the turn (ADR 0033), and later turns leave reasoning out. Instructions added after a tool call are therefore not stored: storing them would not reach the cache.
10. **Resident tools and protocol tools are lean.**
    - The resident calculator offers only `evaluate_expression`. `calculate` and `percentage` stay in the skill for workflows and older loops.
    - `request_user_input` and `submit_turn_state` describe their fields in a few words.
    - The docx and pptx schemas describe only the fields whose names do not say enough. Fields such as `bold` or `italic` carry no description, so `generate_docx` went from 15.4K to 9.5K characters and `generate_pptx` from 6.5K to 4.2K.
11. **Recently used skills start active.**
    - A turn starts with the skills whose tools succeeded in the conversation's last three turns.
    - Their tools are in the tool list from the first round. Their instructions are stored with the question and then deduplicated (decision 9), instead of being resent after a tool call in every turn.
    - The stable skill directory in the system prompt lists every candidate, including those a turn starts with active. Otherwise a change in recently used skills changed the system prompt and lost the whole cache for that turn.

## Consequences

- The system prompt and tool list of an ordinary Agent shrink by roughly a third. Runs without business tools no longer see planning tools they cannot use.
- Calculator and time answers take one model call fewer. Questions about today need no tool call at all.
- A builder can no longer remove the time tools from an Agent by leaving the skill unselected. An organization can still disable the skill.
- Custom skills cannot be resident yet; only built-in skills on the server's list are.
- Prompts are compared with the runtime and memory evaluation suites before and after: pass rate, input tokens and model calls.
