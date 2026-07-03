# General Prompt Optimization

## Use When

Use when the user asks to improve, professionalize, rewrite, structure, or adapt an existing prompt but the target tool is not one of the specialized categories or the user wants a general prompt template.

## Optimization Rules

Preserve:

- Original intent.
- Target audience.
- Required output.
- Constraints.
- Tone and language preference.
- Provided facts, data, module names, or tool names.

Improve:

- Task objective.
- Context.
- Input requirements.
- Output format.
- Constraints and exclusions.
- Evaluation criteria.
- Examples, when useful and provided by the user.

## Prompt Shape

Use this structure when no other format is required:

- Role or task context.
- Goal.
- Input information.
- Requirements.
- Output format.
- Constraints.
- Quality criteria.

## Clarification Rules

Ask when the user asks for "more accurate" or "more professional" but does not specify target tool, use case, or expected output. If the user says not to ask, make conservative assumptions and mark them under `默认假设`.

## Boundary Rules

- Do not add facts that change the task.
- Do not convert the request into a different domain.
- Do not optimize prompts for harmful, illegal, deceptive, or infringing outputs.
