# Runtime evaluation

A replayable scenario suite for the Agent runtime: the native agent loop, skill activation, tool calls, questions back to the user, and multi-turn conversations. It drives an Agent through the external API and scores the answers and what the runtime reports about each turn. Run it before and after a runtime change, or with different models, and compare.

## Files

- `scenarios.json`: the scenarios. Each one is a single conversation with one or more turns. A turn that follows a question from the Agent is sent as the answer to that question.
  - `expect` applies to the last turn and to facts collected over the whole conversation. `turn_expect` applies to single turns, keyed by index.
  - Answer checks: `answer_all`, `answer_any`, `answer_none`, `answer_regex`, `answer_values`, and `answer_values_none`. The value checks compare numbers with thousands separators removed, within `tolerance` (default 0.005).
  - Runtime checks:
    - `asked_user`: the turn asked a question, through `request_user_input` or in its text.
    - `skills_none`: no skill was activated and no business tool was called.
    - `skills_any` and `tools_any`: an activated skill, or a business tool that was called.
    - `files_min`: generated files.
    - `tool_content_min_chars`: the longest `content` argument a tool received. It catches long tool arguments that are cut off.
    - `model_calls_max`: model calls over the conversation.
  - `observe` marks scenarios that are reported but not scored.
  - `accept_language` is sent as the client's `Accept-Language` on every run of the scenario, for checks of the reply language rule (ADR 0048).
- `run.py`:
  - Creates an Agent with the calculator, time and file-generator skills, publishes it, and creates an API key.
  - Runs every scenario as its own API end user, and writes `results.json` and `report.md`.
  - Every turn also gets default checks:
    - it completes, with no runtime error, and the answer is not empty;
    - its visible text is in the scenario's language and names no internal function or tool, such as `activate_skills`. The language is judged on prose: code, URLs, link targets and short quotations are left out, and an answer with fewer than three letters or CJK characters left, such as a bare number or formula, is not judged.
  - A failed tool call is sent back to the model, which may recover. It is counted and listed in the report, not scored.
- `test_run.py`: tests for the scoring. Run them with `python -m unittest test_run.py`.

The report also records, per turn, the average model calls, tool calls, failed tool calls, tokens and latency. With `--container`, it reads the share of input tokens the provider served from its cache. That share comes from `llm_usage_bills` in the deployment's database, through `docker exec`.

## Running

Credentials come from `ZGI_E2E_EMAIL` and `ZGI_E2E_PASSWORD`, or from an env file such as the ignored `.env.test.local`.

```bash
python run.py --workspace-id <id> --label baseline --repeat 3 --container <postgres-container> --env-file ../../../.env.test.local
python run.py --workspace-id <id> --label change --repeat 3 --container <postgres-container> --baseline results/<dir>/results.json --env-file ../../../.env.test.local
```

`--model` and `--model-provider` select the Agent's model; otherwise it uses the workspace default. `--send-language` sends each scenario's language as `Accept-Language`, as a client that reports the user's system language; without it the runtime knows no interface language and the model follows the user's writing (ADR 0048). `--only` and `--category` select scenarios. With `--baseline`, the report adds a table of metrics and of scenarios whose pass rate changed.

Results go to `results/`, which is not committed. Keep the model fixed across runs you compare, and use `--repeat 3` or more: single runs of a model vary.

## Not covered

- **Context compaction:** the context window comes from the model catalog, and the API cannot lower it enough to reach the threshold. The Go tests in `contextmgr` and `skillloop` cover it.
- **Approval and resume:** the suite's skills need no approval. Governed tools need a connector or server, which a portable suite cannot assume.
- **The console sidebar:** the `intent` scenarios run through an Agent, so they do not reach the sidebar's turn strategy. After ADR 0047 the sidebar leaves the same decisions to the model.
- **DigitalHumans:** they run the same native loop. Their memory is covered by `../memory/dh.py`.
