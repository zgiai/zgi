# Memory evaluation

A replayable scenario suite for Agent memory. It drives an Agent through the external API and scores what was remembered and whether later answers use it. The same scenarios are meant to run against the current slot memory and against the unified memory of ADR 0013, so results can be compared.

## Files

- `scenarios.json`: the scenarios. Each one has sessions (separate conversations), a probe question asked in a new conversation, and expectations:
  - `memory_all`, `memory_none`: checks on the saved memory;
  - `answer_all`, `answer_any`, `answer_none`, `answer_regex`, `answer_max_cjk`: checks on the probe answer;
  - `other_memory_empty`: checks isolation.
  `requires_auto` marks scenarios that only automatic capture is expected to pass; `observe` marks product decisions that are reported but not scored.
- `run.py`: creates an Agent with the general-assistant slots, publishes it, creates an API key, runs every scenario as its own API end user, and writes `results.json` and `report.md`.
- `seed.py`: writes memory in the shapes a migration must handle (limits, a 64-character key, a CJK slot name, a disabled slot, revisions, cleared and disabled relationships, a draft that differs from the published contract) and writes a manifest.

## Running

Credentials come from `ZGI_E2E_EMAIL` and `ZGI_E2E_PASSWORD`, or from an env file such as the ignored `.env.test.local`.

```bash
python run.py --workspace-id <id> --label explicit-only --env-file ../../../.env.test.local --repeat 3
python run.py --workspace-id <id> --label auto-capture --auto --wait-after-session 90 --repeat 3 --concurrency 6 --env-file ../../../.env.test.local
python seed.py --workspace-id <id> --label relationship --env-file ../../../.env.test.local
```

The deployment's memory switches decide which runtime is measured:

| Configuration | `ZGI_RELATIONSHIP_MEMORY_ENABLED` | `ZGI_RELATIONSHIP_MEMORY_AUTO_EXTRACTION_ENABLED` | `--auto` |
|---|---|---|---|
| Legacy slot store | `false` | `false` | no |
| Explicit writes only | `true` | `false` | no |
| Automatic capture | `true` | `true` | yes, with `--wait-after-session` long enough for background extraction (about 90s) |

Results go to `results/`, which is not committed. Keep the model fixed across runs you compare; the report records the Agent's model.
