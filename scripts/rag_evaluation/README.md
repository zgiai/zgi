# RAG Evaluation

This directory contains a local, paired evaluation workflow for Dify and ZGI knowledge-base RAG.

The workflow intentionally has three independent commands: evaluate Dify, evaluate ZGI, then compare the two saved result sets. No command runs all three stages automatically.

## Directory Layout

```text
scripts/rag_evaluation/
  input/                 # Put QA Excel/CSV files here
  middle/                # Platform-collected datasets and partial checkpoints
  result/                # Ragas results and Dify/ZGI comparison reports
  .env.example           # Example local config
  .env                   # Local config and cached token, ignored by git
  requirements.txt       # Python dependencies
  run_dify_eval.py       # Stage 1: collect Dify data and run Ragas
  run_zgi_eval.py        # Stage 2: collect ZGI data and run Ragas
  compare_rag_eval.py    # Stage 3: compare existing result files offline
  run_ragas_eval.py      # Shared implementation and legacy ZGI entry point
  test_dify_chat.py      # Optional local Dify answer/retrieval smoke test
  test_llm_latency.py    # Optional judge LLM latency test
```

## Input Format

Put the QA file under `input/`, or pass it with `--input`.

Supported formats:

- `.xlsx`
- `.xls`
- `.csv`

The first two columns are used:

| Column | Meaning |
|---|---|
| 1 | Question / user input |
| 2 | Reference / ground-truth answer |

The first row may be a header. Common headers such as `question`, `user_input`, `reference`, `answer`, `问题`, and `答案` are detected automatically.

## Setup

From this directory:

```bash
cd scripts/rag_evaluation
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env
```

Edit `.env`:

```env
DIFY_BASE_URL="http://127.0.0.1:18000/v1"
DIFY_API_KEY="app-your-published-app-api-key"
DIFY_USER_PREFIX="rag-eval"
DIFY_RESPONSE_MODE="blocking"

ZGI_BASE_URL="http://127.0.0.1:2670/console/api"
ZGI_EMAIL="your-login-email"
ZGI_KNOWLEDGE_BASE_NAME="your-knowledge-base-name"

ZGI_RAG_EVAL_TOP_K="10"
ZGI_RAG_EVAL_SCORE_THRESHOLD="0.35"

RAGAS_PROVIDER="auto"
RAGAS_BASE_URL=""
RAGAS_LLM_MODEL=""
RAGAS_EMBEDDING_MODEL=""
RAGAS_ENABLE_THINKING="false"
RAGAS_BATCH_SIZE="50"
RAGAS_MAX_WORKERS="8"
RAGAS_API_KEY="replace-with-your-api-key"
```

Do not commit `.env`; it can contain API keys and cached login tokens.

## Prerequisites

Before running the three stages:

1. Start the local Dify app API and the ZGI backend.
2. Import the same source documents into both target knowledge bases and wait for indexing to finish.
3. Use the published Dify app API key (`app-...`), not a knowledge-base key (`dataset-...`).
4. Make sure ZGI exposes `POST /console/api/rag-evaluation/batch`.
5. Make sure the Ragas judge model API key is valid and keep the same judge configuration for both runs.

## Three-Stage Comparison Workflow

All three commands must use the same QA input file. Each platform writes isolated files, so completing one stage never triggers or overwrites the other platform.

### Stage 1: Evaluate Dify

```bash
python run_dify_eval.py \
  --input input/rag-data_qa_pairs.xlsx \
  --recollect
```

This stage sends each question as a fresh Dify conversation, saves the answer, retrieval resources, usage, and client-observed latency, then runs the shared Ragas metrics. A partial dataset checkpoint is written during collection and removed after a complete dataset is saved.

Outputs:

```text
middle/rag-data_qa_pairs.dify.ragas.dataset.json
result/rag-data_qa_pairs.dify.ragas.results.json
result/rag-data_qa_pairs.dify.ragas.results.csv
```

### Stage 2: Evaluate ZGI

Run this separately after the Dify evaluation:

```bash
python run_zgi_eval.py \
  --input input/rag-data_qa_pairs.xlsx \
  --knowledge-base-name "rag评测" \
  --top-k 10 \
  --score-threshold 0.35 \
  --retrieval-mode hybrid \
  --recollect
```

Outputs:

```text
middle/rag-data_qa_pairs.zgi.ragas.dataset.json
result/rag-data_qa_pairs.zgi.ragas.results.json
result/rag-data_qa_pairs.zgi.ragas.results.csv
```

`run_ragas_eval.py` remains available as a legacy ZGI command, but the comparison workflow should use `run_zgi_eval.py` so result names include the `.zgi` platform suffix.

### Stage 3: Generate the Comparison

This command only reads files produced by stages 1 and 2. It does not call Dify, ZGI, or the Ragas judge:

```bash
python compare_rag_eval.py \
  --input input/rag-data_qa_pairs.xlsx
```

Outputs:

```text
result/rag-data_qa_pairs.comparison.csv
result/rag-data_qa_pairs.comparison.json
result/rag-data_qa_pairs.comparison.md
```

The report pairs rows by `sample_id`, validates that questions and references match, and reports per-metric means, Dify-minus-ZGI deltas, win/tie/loss counts, paired bootstrap confidence intervals, collection quality, latency when available, and the largest per-question differences.

### Quick Smoke Test

```bash
python run_dify_eval.py \
  --input input/rag-data_qa_pairs_lite.xlsx \
  --limit 5 \
  --ragas-limit 5 \
  --recollect

python run_zgi_eval.py \
  --input input/rag-data_qa_pairs_lite.xlsx \
  --limit 5 \
  --ragas-limit 5 \
  --top-k 10 \
  --score-threshold 0.35 \
  --recollect

python compare_rag_eval.py \
  --input input/rag-data_qa_pairs_lite.xlsx
```

## Retrieval Parameters

The backend retrieval parameters are passed to the RAG evaluation API:

| Parameter | Default source | Meaning |
|---|---|---|
| `--top-k` | `ZGI_RAG_EVAL_TOP_K` or prompt | Number of final retrieved contexts requested from backend |
| `--score-threshold` | `ZGI_RAG_EVAL_SCORE_THRESHOLD` or prompt | Backend confidence threshold |
| `--retrieval-mode` | `hybrid` | One of `hybrid`, `vector`, `graph` |
| `--model` | empty | Optional backend generation model |

If `top_k` and threshold are not passed:

- If saved values exist in `.env`, the script prompts whether to reuse them.
- Press Enter to reuse saved values.
- Enter `r` to reconfigure and save new values.

Valid ranges:

- `top_k`: 1 to 20
- `score_threshold`: 0 to 1

## Ragas Parameters

| Parameter | Env | Meaning |
|---|---|---|
| `--ragas-provider` | `RAGAS_PROVIDER` | `auto`, `aliyun`, or `openai` |
| `--ragas-api-key` | `RAGAS_API_KEY` | API key for judge model and embeddings |
| `--ragas-base-url` | `RAGAS_BASE_URL` | OpenAI-compatible API base URL |
| `--ragas-llm-model` | `RAGAS_LLM_MODEL` | Judge LLM model |
| `--ragas-embedding-model` | `RAGAS_EMBEDDING_MODEL` | Embedding model for Ragas metrics |
| `--ragas-enable-thinking` | `RAGAS_ENABLE_THINKING` | Provider-specific thinking mode flag, usually `false` |
| `--ragas-batch-size` | `RAGAS_BATCH_SIZE` | Rows per Ragas batch |
| `--ragas-max-workers` | `RAGAS_MAX_WORKERS` | Ragas concurrency |
| `--ragas-limit` | none | Limit rows sent to Ragas after backend collection |

## Output Files

For input file:

```text
input/rag-data_qa_pairs.xlsx
```

The three-stage workflow writes:

```text
middle/rag-data_qa_pairs.dify.ragas.dataset.json
result/rag-data_qa_pairs.dify.ragas.results.json
result/rag-data_qa_pairs.dify.ragas.results.csv
middle/rag-data_qa_pairs.zgi.ragas.dataset.json
result/rag-data_qa_pairs.zgi.ragas.results.json
result/rag-data_qa_pairs.zgi.ragas.results.csv
result/rag-data_qa_pairs.comparison.csv
result/rag-data_qa_pairs.comparison.json
result/rag-data_qa_pairs.comparison.md
```

File meanings:

| File | Meaning |
|---|---|
| `.<platform>.ragas.dataset.json` | Platform-collected dataset: sample ID, question, response, contexts, reference, and status |
| `.<platform>.ragas.results.json` | Platform Ragas result rows with stable sample IDs |
| `.<platform>.ragas.results.csv` | Platform Ragas metrics in CSV |
| `.comparison.csv` | Per-question paired scores, deltas, and winners |
| `.comparison.json` | Machine-readable aggregate comparison |
| `.comparison.md` | Human-readable analysis report |

If a platform dataset already exists, its evaluator asks whether to reuse it. Use `--reuse-dataset` or `--recollect` for non-interactive control.

## Metrics

The script evaluates these Ragas metrics:

- `faithfulness`
- `answer_relevancy`
- `context_precision`
- `context_recall`
- `answer_correctness`

The result CSV also includes:

- `user_input`
- `response`
- `reference`
- `retrieved_contexts`

## Latency Test

Use this to test whether the Ragas judge LLM is reachable and how long one call takes:

```bash
python test_llm_latency.py "请用一句话介绍南阳市第一人民医院"
```

It reads the same `.env` model settings.

## Dify Smoke Test

Set `DIFY_API_KEY` in `.env`, then send the default question `退号流程` to the local Dify app:

```bash
python test_dify_chat.py
```

Pass a different question as the positional argument when needed:

```bash
python test_dify_chat.py "门诊患者退号后的费用怎么退？"
```

The script prints the generated answer, end-to-end latency, retrieved knowledge contexts, and token usage returned by Dify.

## Common Issues

### Cannot connect to backend

Check `ZGI_BASE_URL`, and make sure the backend is running.

Default:

```env
ZGI_BASE_URL="http://127.0.0.1:2670/console/api"
```

### Login failed or token expired

The script caches `ZGI_ACCESS_TOKEN` in `.env`. If it expires, the script will ask you to login again. You can also remove `ZGI_ACCESS_TOKEN` and rerun.

### Knowledge base not found

Check `ZGI_KNOWLEDGE_BASE_NAME`. The script matches by knowledge-base name in the current workspace/organization.

### Ragas API key missing

Set `RAGAS_API_KEY` for the judge model provider:

```env
RAGAS_API_KEY="..."
```

### Ragas is slow

Try reducing:

```bash
--ragas-batch-size 10 --ragas-max-workers 4
```

Or run a small sample first:

```bash
python run_dify_eval.py --limit 10 --ragas-limit 10
```

### Existing dataset is stale

If you changed retrieval code or knowledge-base data, do not reuse the existing platform dataset. Pass `--recollect` to the corresponding platform evaluator.
