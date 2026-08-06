# Graph Retrieval Evaluation Data Preparation

This directory prepares a focused MultiHop-RAG dataset for comparing ZGI's
ordinary RAG retrieval with graph retrieval.

The preparation script uses a fixed random seed to select:

- 50 `temporal_query` questions;
- 50 `comparison_query` questions;
- every unique corpus article referenced by those questions' `evidence_list`.

## Setup

Install the existing RAG evaluation dependencies from the repository root:

```bash
python3 -m venv scripts/rag_evaluation/.venv
source scripts/rag_evaluation/.venv/bin/activate
pip install -r scripts/rag_evaluation/requirements.txt
```

## Run

```bash
python scripts/rag_evaluation/graph_evaluation/prepare_multihop_rag.py
```

The default random seed is `42`. To use a different seed or output directory:

```bash
python scripts/rag_evaluation/graph_evaluation/prepare_multihop_rag.py \
  --seed 42 \
  --output-dir scripts/rag_evaluation/graph_evaluation/output
```

The output directory must be empty. The script will not overwrite existing
evaluation artifacts.

## Outputs

```text
output/
  qa_pairs.csv       # Exactly 100 rows: question, original answer reference, enhanced English answer
  documents/         # One Markdown file per unique referenced article
    document_0001.md
    document_0002.md
    ...
```

Only source articles are exported. Questions, answers, and evidence facts are
not written into the document collection, preventing answer leakage during
knowledge-base ingestion.

`reference` is the raw benchmark answer (for example, Yes/No) and is the
column used directly for RAGAS evaluation. `enhanced_answer`, the final column,
is an English, evidence-backed version of that answer containing the gold facts
from every source article. Evidence facts are only present in `qa_pairs.csv`,
never in `documents/`.

To refresh `enhanced_answer` in an already-prepared QA file without regenerating
or re-uploading the document collection:

```bash
conda run -n rag-eval python scripts/rag_evaluation/graph_evaluation/refresh_explanatory_references.py
```

To replace the fact-list enhanced answers with concise, evidence-grounded
English answers that explicitly compare or order the evidence, use the
configured RAGAS LLM. This preserves `reference`, requires every gold-evidence
tag to be cited, and writes the CSV only after all 100 rows pass validation:

```bash
conda run -n rag-eval python scripts/rag_evaluation/graph_evaluation/generate_reasoned_references.py
```

## Upload documents to the local file manager

The generated Markdown files can be uploaded to the running ZGI file manager
through the API used by /console/files. The web UI is usually on port 3000,
while the API is configured on port 2670:

    export ZGI_ACCESS_TOKEN='your-current-console-access-token'
    python scripts/rag_evaluation/graph_evaluation/upload_documents.py

The script uploads all 147 Markdown files in output/documents and starts
processing them immediately. It automatically resolves the workspace selected
by the current console login and attaches that workspace to every upload. Use
--workspace-id to override it, --processing-mode store_only if the files should
be uploaded without starting parsing, or --folder-id to target a folder.

Useful options:

    python scripts/rag_evaluation/graph_evaluation/upload_documents.py \
      --processing-mode process_now \
      --workspace-id '<workspace-uuid>' \
      --manifest output/upload_manifest.json

The access token is only read locally and is never written to the manifest.
You can use --dry-run to inspect the selected files without authenticating or
making uploads.
