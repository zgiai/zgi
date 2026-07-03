# RAG Evaluation

This directory contains a local evaluation workflow for ZGI knowledge-base RAG.

The script reads QA pairs from Excel/CSV, calls the backend RAG evaluation API to collect answers and retrieved contexts, then runs Ragas metrics and saves intermediate and final results.

## Directory Layout

```text
scripts/rag_evaluation/
  input/                 # Put QA Excel/CSV files here
  middle/                # Generated datasets, Ragas results, and analysis files
  .env.example           # Example local config
  .env                   # Local config and cached token, ignored by git
  requirements.txt       # Python dependencies
  run_ragas_eval.py      # Main evaluation script
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
ZGI_BASE_URL="http://127.0.0.1:2670/console/api"
ZGI_EMAIL="your-login-email"
ZGI_KNOWLEDGE_BASE_NAME="your-knowledge-base-name"

ZGI_RAG_EVAL_TOP_K="10"
ZGI_RAG_EVAL_SCORE_THRESHOLD="0.35"

RAGAS_PROVIDER="aliyun"
RAGAS_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode/v1"
RAGAS_LLM_MODEL="qwen-plus"
RAGAS_EMBEDDING_MODEL="text-embedding-v4"
RAGAS_ENABLE_THINKING="false"
RAGAS_BATCH_SIZE="50"
RAGAS_MAX_WORKERS="8"
RAGAS_API_KEY="replace-with-your-api-key"
```

Do not commit `.env`; it can contain API keys and cached login tokens.

## Prerequisites

Before running:

1. Start the ZGI backend.
2. Make sure the target knowledge base exists and has finished indexing.
3. Make sure the backend has the RAG evaluation route available: `POST /console/api/rag-evaluation/batch`.
4. Make sure the Ragas judge model API key is valid.

## Run Evaluation

Interactive mode:

```bash
python run_ragas_eval.py
```

The script will:

1. Ask you to select an input file from `input/`.
2. Reuse or recollect an existing Ragas dataset if one already exists.
3. Login to ZGI if there is no valid cached token.
4. Ask for knowledge-base name if not set in `.env`.
5. Ask for `top_k` and `score_threshold` on first use.
6. Save retrieval defaults to `.env`.
7. Collect backend answers and retrieved contexts.
8. Run Ragas.
9. Save outputs under `middle/`.

Non-interactive example:

```bash
python run_ragas_eval.py \
  --input input/rag-data_qa_pairs.xlsx \
  --knowledge-base-name "rag评测" \
  --top-k 10 \
  --score-threshold 0.35 \
  --retrieval-mode hybrid
```

Quick smoke test:

```bash
python run_ragas_eval.py \
  --input input/rag-data_qa_pairs_lite.xlsx \
  --limit 5 \
  --ragas-limit 5
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
| `--ragas-enable-thinking` | `RAGAS_ENABLE_THINKING` | DashScope thinking mode, usually `false` |
| `--ragas-batch-size` | `RAGAS_BATCH_SIZE` | Rows per Ragas batch |
| `--ragas-max-workers` | `RAGAS_MAX_WORKERS` | Ragas concurrency |
| `--ragas-limit` | none | Limit rows sent to Ragas after backend collection |

## Output Files

For input file:

```text
input/rag-data_qa_pairs.xlsx
```

The script writes:

```text
middle/rag-data_qa_pairs.ragas.dataset.json
middle/rag-data_qa_pairs.ragas.results.json
middle/rag-data_qa_pairs.ragas.results.csv
```

File meanings:

| File | Meaning |
|---|---|
| `.ragas.dataset.json` | Backend-collected dataset: question, response, retrieved contexts, reference, status |
| `.ragas.results.json` | Ragas result rows in JSON |
| `.ragas.results.csv` | Ragas metrics in CSV, easiest to analyze |

If `.ragas.dataset.json` already exists, the script asks whether to reuse it. Reusing skips backend retrieval and only reruns Ragas.

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

Set one of:

```env
RAGAS_API_KEY="..."
ALIYUN_API_KEY="..."
DASHSCOPE_API_KEY="..."
OPENAI_API_KEY="..."
```

### Ragas is slow

Try reducing:

```bash
--ragas-batch-size 10 --ragas-max-workers 4
```

Or run a small sample first:

```bash
python run_ragas_eval.py --limit 10 --ragas-limit 10
```

### Existing dataset is stale

If you changed retrieval code or knowledge-base data, do not reuse the existing `.ragas.dataset.json`. Press Enter when prompted to recollect backend data, or delete the old dataset file under `middle/`.

