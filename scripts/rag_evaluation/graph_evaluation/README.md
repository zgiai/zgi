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
  qa_pairs.csv       # Exactly 100 data rows: question, reference
  documents/         # One Markdown file per unique referenced article
    document_0001.md
    document_0002.md
    ...
```

Only source articles are exported. Questions, answers, and evidence facts are
not written into the document collection, preventing answer leakage during
knowledge-base ingestion.
