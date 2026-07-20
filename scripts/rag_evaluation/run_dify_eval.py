#!/usr/bin/env python3
"""Collect Dify answers and contexts, then run the shared Ragas metrics."""

from __future__ import annotations

import argparse
import time
import urllib.error
from pathlib import Path
from typing import Any

import run_ragas_eval as shared


DEFAULT_DIFY_BASE_URL = "http://127.0.0.1:18000/v1"
DEFAULT_MAX_RETRIES = 2


def main() -> int:
    shared.ENV_VALUES = shared.load_env_file(shared.ENV_FILE)
    args = parse_args()
    input_path = shared.choose_input_path(args.input)
    if not input_path.exists():
        raise SystemExit(f"input file does not exist: {input_path}")

    dataset_path, result_json_path, result_csv_path = shared.output_paths_for_input(input_path, "dify")
    dataset_rows: list[dict[str, Any]] | None = None
    if dataset_path.exists():
        if args.reuse_dataset:
            reuse = True
        elif args.recollect:
            reuse = False
        else:
            answer = input(
                f"Found existing Dify dataset: {dataset_path}\n"
                "Use it directly for evaluation? Enter y/yes to reuse, or press Enter to recollect Dify data: "
            ).strip().lower()
            reuse = answer in {"y", "yes"}
        if reuse:
            dataset_rows = shared.load_existing_dataset(dataset_path)
            print(f"loaded existing Dify dataset: {dataset_path} ({len(dataset_rows)} rows)")

    if dataset_rows is None:
        api_key = args.api_key.strip()
        if not api_key:
            raise SystemExit(f"Dify app API key is required. Set DIFY_API_KEY in {shared.ENV_FILE}.")
        if api_key.startswith("dataset-"):
            raise SystemExit("DIFY_API_KEY must be the published app API key, not a dataset API key.")

        qa_items = shared.read_qa_items(input_path, args.limit)
        if not qa_items:
            raise SystemExit("no QA rows found in input file")

        base_url = args.base_url.strip().rstrip("/")
        shared.ENV_VALUES["DIFY_BASE_URL"] = base_url
        shared.ENV_VALUES["DIFY_USER_PREFIX"] = args.user_prefix
        shared.ENV_VALUES["DIFY_RESPONSE_MODE"] = args.response_mode
        shared.write_env_file(shared.ENV_FILE, shared.ENV_VALUES)

        partial_path = dataset_path.with_name(dataset_path.name.replace(".dataset.json", ".dataset.partial.json"))
        dataset_rows = collect_dify_rows(
            qa_items=qa_items,
            endpoint=f"{base_url}/chat-messages",
            api_key=api_key,
            user_prefix=args.user_prefix,
            response_mode=args.response_mode,
            max_retries=args.max_retries,
            partial_path=partial_path,
        )
        shared.write_json(dataset_path, dataset_rows)
        partial_path.unlink(missing_ok=True)
        print(f"saved Dify Ragas dataset: {dataset_path}")

    ragas_model_config = shared.build_ragas_model_config(args)
    shared.remember_ragas_model_config(ragas_model_config)
    shared.ENV_VALUES["RAGAS_BATCH_SIZE"] = str(args.ragas_batch_size)
    shared.write_env_file(shared.ENV_FILE, shared.ENV_VALUES)
    results = shared.run_ragas(dataset_rows, ragas_model_config, args.ragas_batch_size, args.ragas_limit)
    shared.write_ragas_outputs(results, result_json_path, result_csv_path)
    print(f"saved Dify Ragas result JSON: {result_json_path}")
    print(f"saved Dify Ragas result CSV: {result_csv_path}")
    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run Dify RAG evaluation and shared Ragas metrics for an input QA file.")
    parser.add_argument("--input", default="", help="Optional input file path. If omitted, choose from scripts/rag_evaluation/input.")
    parser.add_argument("--limit", type=int, default=shared.DEFAULT_LIMIT, help="Number of QA rows to evaluate. 0 means all rows.")
    parser.add_argument("--base-url", default=shared.env_value("DIFY_BASE_URL", DEFAULT_DIFY_BASE_URL))
    parser.add_argument("--api-key", default=shared.env_value("DIFY_API_KEY"), help=argparse.SUPPRESS)
    parser.add_argument("--user-prefix", default=shared.env_value("DIFY_USER_PREFIX", "rag-eval"))
    parser.add_argument("--response-mode", default=shared.env_value("DIFY_RESPONSE_MODE", "blocking"), choices=["blocking"])
    parser.add_argument("--max-retries", type=int, default=DEFAULT_MAX_RETRIES)
    parser.add_argument("--ragas-batch-size", type=int, default=shared.int_env_value("RAGAS_BATCH_SIZE", shared.DEFAULT_RAGAS_BATCH_SIZE))
    parser.add_argument("--ragas-limit", type=int, default=0)
    parser.add_argument("--ragas-provider", default=shared.env_value("RAGAS_PROVIDER", "auto"), choices=["auto", "aliyun", "openai"])
    parser.add_argument("--ragas-api-key", default=shared.ragas_env_value("RAGAS_API_KEY", "ALIYUN_API_KEY", "DASHSCOPE_API_KEY", "OPENAI_API_KEY"))
    parser.add_argument("--ragas-base-url", default=shared.ragas_env_value("RAGAS_BASE_URL", "ALIYUN_BASE_URL", "DASHSCOPE_BASE_URL", "OPENAI_BASE_URL"))
    parser.add_argument("--ragas-llm-model", default=shared.ragas_env_value("RAGAS_LLM_MODEL", "ALIYUN_LLM_MODEL", "DASHSCOPE_LLM_MODEL"))
    parser.add_argument("--ragas-embedding-model", default=shared.ragas_env_value("RAGAS_EMBEDDING_MODEL", "ALIYUN_EMBEDDING_MODEL", "DASHSCOPE_EMBEDDING_MODEL"))
    parser.add_argument("--ragas-enable-thinking", default=shared.ragas_env_value("RAGAS_ENABLE_THINKING", "ALIYUN_ENABLE_THINKING", "DASHSCOPE_ENABLE_THINKING"))
    parser.add_argument("--ragas-max-workers", type=int, default=shared.int_env_value("RAGAS_MAX_WORKERS", shared.DEFAULT_RAGAS_MAX_WORKERS))
    dataset_group = parser.add_mutually_exclusive_group()
    dataset_group.add_argument("--reuse-dataset", action="store_true")
    dataset_group.add_argument("--recollect", action="store_true")
    args = parser.parse_args()
    if args.max_retries < 0:
        raise SystemExit("--max-retries must be >= 0")
    return args


def collect_dify_rows(
    qa_items: list[shared.QAItem],
    endpoint: str,
    api_key: str,
    user_prefix: str,
    response_mode: str,
    max_retries: int,
    partial_path: Path,
) -> list[dict[str, Any]]:
    total = len(qa_items)
    run_id = int(time.time())
    rows: list[dict[str, Any]] = []
    print(f"collecting Dify RAG data: {total} questions, response_mode={response_mode}", flush=True)
    for sample_id, qa in enumerate(qa_items, start=1):
        print(f"Dify question {sample_id}/{total} started", flush=True)
        payload = {
            "inputs": {},
            "query": qa.question,
            "response_mode": response_mode,
            "conversation_id": "",
            "user": f"{user_prefix}-{run_id}-{sample_id}",
            "auto_generate_name": False,
        }
        started = time.perf_counter()
        try:
            data = post_with_retries(endpoint, payload, api_key, max_retries)
            elapsed = time.perf_counter() - started
            row = build_dify_row(sample_id, qa, data, elapsed)
        except shared.HTTPStatusError as exc:
            if exc.status in {401, 403}:
                raise SystemExit(f"Dify authentication failed with HTTP {exc.status}; check DIFY_API_KEY.") from exc
            elapsed = time.perf_counter() - started
            row = error_row(sample_id, qa, elapsed, f"HTTP {exc.status}: {exc.body}")
        except urllib.error.URLError as exc:
            elapsed = time.perf_counter() - started
            row = error_row(sample_id, qa, elapsed, f"connection error: {exc.reason}")
        rows.append(row)
        shared.write_json(partial_path, rows)
        print(
            f"Dify question {sample_id}/{total} finished: status={row['status']}, "
            f"contexts={len(row['retrieved_contexts'])}, latency={row['latency_seconds']:.3f}s",
            flush=True,
        )
    print(f"Dify RAG data collection finished: {len(rows)}/{total}", flush=True)
    return rows


def post_with_retries(url: str, payload: dict[str, Any], api_key: str, max_retries: int) -> dict[str, Any]:
    for attempt in range(max_retries + 1):
        try:
            return shared.post_json(url, payload, token=api_key)
        except shared.HTTPStatusError as exc:
            retryable = exc.status == 429 or exc.status >= 500
            if not retryable or attempt >= max_retries:
                raise
        except urllib.error.URLError:
            if attempt >= max_retries:
                raise
        delay = min(2**attempt, 5)
        print(f"Dify request failed transiently; retrying in {delay}s ({attempt + 1}/{max_retries})", flush=True)
        time.sleep(delay)
    raise RuntimeError("unreachable retry state")


def build_dify_row(sample_id: int, qa: shared.QAItem, data: dict[str, Any], elapsed: float) -> dict[str, Any]:
    metadata = data.get("metadata")
    metadata = metadata if isinstance(metadata, dict) else {}
    resources = metadata.get("retriever_resources")
    resources = resources if isinstance(resources, list) else []
    contexts = [
        str(resource.get("content") or "")
        for resource in resources
        if isinstance(resource, dict) and str(resource.get("content") or "").strip()
    ]
    answer = str(data.get("answer") or "")
    error = "" if answer.strip() else "Dify response does not contain a non-empty answer"
    return {
        "sample_id": sample_id,
        "platform": "dify",
        "user_input": qa.question,
        "response": answer,
        "retrieved_contexts": contexts,
        "reference": qa.reference,
        "status": "success" if not error else "error",
        "error": error,
        "latency_seconds": elapsed,
        "retrieval_resources": resources,
        "usage": metadata.get("usage") if isinstance(metadata.get("usage"), dict) else {},
        "message_id": str(data.get("message_id") or data.get("id") or ""),
        "conversation_id": str(data.get("conversation_id") or ""),
    }


def error_row(sample_id: int, qa: shared.QAItem, elapsed: float, error: str) -> dict[str, Any]:
    return {
        "sample_id": sample_id,
        "platform": "dify",
        "user_input": qa.question,
        "response": "",
        "retrieved_contexts": [],
        "reference": qa.reference,
        "status": "error",
        "error": error,
        "latency_seconds": elapsed,
        "retrieval_resources": [],
        "usage": {},
        "message_id": "",
        "conversation_id": "",
    }


if __name__ == "__main__":
    raise SystemExit(main())
