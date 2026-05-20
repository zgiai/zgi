# Hyperparse Runtime Source

This directory contains the runtime-relevant parser source imported for the
ZGI content parse capability.

Source provenance:
- source snapshot: Hyperparse SDK runtime subset
- source commit: 07e0997f76c48d79dbae1818b3ce247f56fd3d28
- import scope: parser runtime, provider adapters, OCR/VLM orchestration, and tests needed by zgi-api

Open-source hygiene:
- do not keep local machine paths, private artifact paths, generated debug output, or playground build artifacts here
- preserve third-party notices for embedded reference data, such as Unicode Adobe Symbol mapping data
- keep product-facing configuration and documentation under the ZGI/content-parse naming surface

Rules for this source subtree:
1. Keep only runtime-relevant source used by zgi-api.
2. Exclude CLI, playground UI, docs, output artifacts, and temporary debug files.
3. Keep imports local to this repository under `internal/capabilities/contentparse/engines/hyperparse/...`.
4. Business modules must still depend on `contentparse` contracts/capability rather than importing this mirror directly.
