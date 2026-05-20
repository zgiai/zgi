# Public Content Parse Fixtures

These fixtures are safe to keep in the open-source repository and are intended
for smoke and regression checks of the shared parsing capability.

Rules:

1. Keep them small and redistributable.
2. Prefer plain-text and tiny public sample files for open-source CI.
3. Put private customer documents in a local, gitignored corpus outside this repo
   and reference them through manifest paths or environment variables.

Gitignored local directories reserved for larger or private corpora:

- `tests/contentparse/fixtures/hf/`
- `tests/contentparse/fixtures/private/`
- `tests/contentparse/downloads/`
- `tests/contentparse/generated/`
- `tests/contentparse/reports/`
