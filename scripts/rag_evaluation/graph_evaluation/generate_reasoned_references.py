#!/usr/bin/env python3
"""Generate enhanced English answers for the prepared MultiHopRAG QA set."""

from __future__ import annotations

import argparse
import csv
import os
import re
import tempfile
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Any

from prepare_multihop_rag import (
    DATASET_NAME,
    DEFAULT_OUTPUT_DIR,
    DEFAULT_SEED,
    QUESTION_CONFIG,
    SPLIT,
    clean_text,
    import_datasets,
    select_questions,
)


SCRIPT_DIR = Path(__file__).resolve().parent
EVAL_DIR = SCRIPT_DIR.parent
ENV_FILE = EVAL_DIR / ".env"
DEFAULT_BASE_URL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
DEFAULT_MODEL = "qwen3.7-max"
MAX_RETRIES = 3


def main() -> int:
    args = parse_args()
    config = resolve_model_config(args)
    qa_path = Path(args.qa_path).expanduser().resolve()
    existing_rows = read_qa_rows(qa_path)
    selected_questions = load_selected_questions(args)
    generation_inputs = build_generation_inputs(existing_rows, selected_questions)

    if args.limit:
        generation_inputs = generation_inputs[: args.limit]
    print(
        f"Generating {len(generation_inputs)} grounded English enhanced answers with "
        f"model={config['model']}, workers={args.workers}, thinking=false"
    )
    generated = generate_all(generation_inputs, config, args.workers)
    for item in generated:
        validate_reference(item["enhanced_answer"], item["reference"], item["evidence"])

    for item in generated[: args.preview_rows]:
        print(f"\n--- Preview row {item['index']} ---")
        print(f"Question: {item['question']}")
        print(f"Reference: {item['reference']}")
        print(f"Enhanced answer: {item['enhanced_answer']}")

    if args.dry_run:
        print("Dry run completed; qa_pairs.csv was not changed.")
        return 0
    if len(generated) != len(existing_rows):
        raise SystemExit("--limit can only be used together with --dry-run")

    output_rows = [
        {
            "question": item["question"],
            "reference": item["reference"],
            "enhanced_answer": item["enhanced_answer"],
        }
        for item in generated
    ]
    write_rows_atomically(qa_path, output_rows)
    print(f"Updated QA file: {qa_path}")
    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate grounded, reasoned enhanced answers from MultiHopRAG gold evidence."
    )
    parser.add_argument("--qa-path", default=str(DEFAULT_OUTPUT_DIR / "qa_pairs.csv"))
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED)
    parser.add_argument("--cache-dir", default="")
    parser.add_argument("--base-url", default="")
    parser.add_argument("--api-key", default="")
    parser.add_argument("--model", default="")
    parser.add_argument("--workers", type=int, default=5)
    parser.add_argument("--limit", type=int, default=0, help="Generate only the first N rows; requires --dry-run.")
    parser.add_argument("--preview-rows", type=int, default=2)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    if args.workers < 1 or args.workers > 16:
        raise SystemExit("--workers must be between 1 and 16")
    if args.limit < 0:
        raise SystemExit("--limit must be >= 0")
    if args.limit and not args.dry_run:
        raise SystemExit("--limit requires --dry-run so a partial enhanced-answer set is never written")
    return args


def load_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    if not path.exists():
        return values
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def resolve_model_config(args: argparse.Namespace) -> dict[str, str]:
    env = load_env(ENV_FILE)
    api_key = args.api_key or os.getenv("RAGAS_API_KEY") or env.get("RAGAS_API_KEY", "")
    base_url = args.base_url or os.getenv("RAGAS_BASE_URL") or env.get("RAGAS_BASE_URL", DEFAULT_BASE_URL)
    model = args.model or os.getenv("RAGAS_LLM_MODEL") or env.get("RAGAS_LLM_MODEL", DEFAULT_MODEL)
    if not api_key:
        raise SystemExit("RAGAS_API_KEY is required; configure scripts/rag_evaluation/.env or pass --api-key")
    if not base_url:
        raise SystemExit("RAGAS_BASE_URL is required; configure scripts/rag_evaluation/.env or pass --base-url")
    if not model:
        raise SystemExit("RAGAS_LLM_MODEL is required; configure scripts/rag_evaluation/.env or pass --model")
    return {"api_key": api_key, "base_url": base_url.rstrip("/"), "model": model}


def load_selected_questions(args: argparse.Namespace) -> Any:
    concatenate_datasets, load_dataset = import_datasets()
    load_kwargs: dict[str, Any] = {"split": SPLIT}
    if args.cache_dir:
        load_kwargs["cache_dir"] = str(Path(args.cache_dir).expanduser().resolve())
    questions = load_dataset(DATASET_NAME, QUESTION_CONFIG, **load_kwargs)
    return select_questions(
        questions.add_column("source_id", list(range(len(questions)))),
        concatenate_datasets,
        args.seed,
    )


def read_qa_rows(path: Path) -> list[dict[str, str]]:
    if not path.exists():
        raise SystemExit(f"QA file does not exist: {path}")
    with path.open("r", encoding="utf-8-sig", newline="") as handle:
        rows = list(csv.DictReader(handle))
    expected_columns = {"question", "reference", "enhanced_answer"}
    if not rows or not expected_columns.issubset(rows[0]):
        raise SystemExit("QA file must contain question, reference, and enhanced_answer columns")
    return rows


def build_generation_inputs(existing_rows: list[dict[str, str]], selected_questions: Any) -> list[dict[str, Any]]:
    if len(existing_rows) != len(selected_questions):
        raise SystemExit("QA file row count does not match the seed-selected MultiHopRAG set")
    out: list[dict[str, Any]] = []
    for index, (existing, question) in enumerate(zip(existing_rows, selected_questions), start=1):
        query = clean_text(question.get("query"))
        answer = clean_text(question.get("answer"))
        if clean_text(existing.get("question")) != query:
            raise SystemExit(f"question mismatch at row {index}; refusing to apply gold evidence to another QA set")
        if clean_text(existing.get("reference")) != answer:
            raise SystemExit(f"reference answer mismatch at row {index}; refusing to overwrite the QA file")
        evidence = [dict(item or {}) for item in question.get("evidence_list") or []]
        if not evidence:
            raise SystemExit(f"row {index} has no gold evidence")
        out.append({"index": index, "question": query, "reference": answer, "evidence": evidence})
    return out


def generate_all(items: list[dict[str, Any]], config: dict[str, str], workers: int) -> list[dict[str, Any]]:
    try:
        from openai import OpenAI
    except ImportError as exc:
        raise SystemExit("openai is required; run inside the rag-eval Conda environment") from exc

    client = OpenAI(api_key=config["api_key"], base_url=config["base_url"], timeout=120, max_retries=0)
    generated: dict[int, dict[str, Any]] = {}
    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = {
            executor.submit(generate_one, client, item, config["model"]): item["index"]
            for item in items
        }
        for completed, future in enumerate(as_completed(futures), start=1):
            item = future.result()
            generated[item["index"]] = item
            print(f"Generated {completed}/{len(items)} enhanced answers", flush=True)
    return [generated[index] for index in sorted(generated)]


def generate_one(client: Any, item: dict[str, Any], model: str) -> dict[str, Any]:
    prompt = build_prompt(item["question"], item["reference"], item["evidence"])
    last_error: Exception | None = None
    for attempt in range(1, MAX_RETRIES + 1):
        try:
            response = client.chat.completions.create(
                model=model,
                temperature=0,
                max_tokens=500,
                messages=[
                    {
                        "role": "system",
                        "content": (
                            "You produce concise, factual English enhanced answers for RAG evaluation. "
                            "Follow the supplied evidence exactly."
                        ),
                    },
                    {"role": "user", "content": prompt},
                ],
                extra_body={"enable_thinking": False},
            )
            content = response.choices[0].message.content if response.choices else ""
            enhanced_answer = normalize_reference(str(content or ""), item["reference"])
            validate_reference(enhanced_answer, item["reference"], item["evidence"])
            return {**item, "enhanced_answer": enhanced_answer}
        except Exception as exc:  # provider errors are retried with a bounded delay
            last_error = exc
            if attempt < MAX_RETRIES:
                time.sleep(float(attempt * 2))
    raise RuntimeError(f"row {item['index']} enhanced-answer generation failed after {MAX_RETRIES} attempts: {last_error}")


def build_prompt(question: str, reference: str, evidence: list[dict[str, Any]]) -> str:
    evidence_lines = []
    for index, item in enumerate(evidence, start=1):
        source = clean_text(item.get("source")) or "Unknown source"
        title = clean_text(item.get("title")) or "untitled article"
        fact = clean_text(item.get("fact"))
        evidence_lines.append(f"[E{index}] {source}, \"{title}\": {fact}")
    citations = ", ".join(f"[E{index}]" for index in range(1, len(evidence) + 1))
    return (
        "Write a canonical English answer to the question below for RAGAS evaluation.\n\n"
        f"Question: {question}\n"
        f"Gold answer label: {reference}\n"
        "Gold evidence:\n"
        + "\n".join(evidence_lines)
        + "\n\nRequirements:\n"
        + f"1. Start exactly with `Answer: {reference}.`\n"
        + "2. In 2 to 4 concise English sentences, explain how the evidence supports that answer.\n"
        + f"3. Use every evidence item and cite it with its tag: {citations}.\n"
        + "4. Do not add facts, assumptions, or qualifications not present in the evidence.\n"
        + "5. Return only the enhanced answer, without headings, analysis, or Markdown code fences."
    )


def normalize_reference(value: str, reference: str) -> str:
    enhanced_answer = re.sub(r"^```(?:text|markdown)?\s*|\s*```$", "", value.strip(), flags=re.IGNORECASE)
    expected_prefix = f"Answer: {reference}."
    if not enhanced_answer.startswith(expected_prefix):
        raise ValueError(f"enhanced answer must start with {expected_prefix!r}")
    return re.sub(r"\s+", " ", enhanced_answer).strip()


def validate_reference(enhanced_answer: str, reference: str, evidence: list[dict[str, Any]]) -> None:
    if len(enhanced_answer) < 100 or len(enhanced_answer) > 3_000:
        raise ValueError(f"enhanced answer length {len(enhanced_answer)} is outside the allowed range")
    if not enhanced_answer.startswith(f"Answer: {reference}."):
        raise ValueError("enhanced answer does not preserve the reference")
    missing = [f"[E{index}]" for index in range(1, len(evidence) + 1) if f"[E{index}]" not in enhanced_answer]
    if missing:
        raise ValueError(f"enhanced answer does not cite all evidence: {', '.join(missing)}")


def write_rows_atomically(path: Path, rows: list[dict[str, str]]) -> None:
    with tempfile.NamedTemporaryFile(
        mode="w",
        encoding="utf-8-sig",
        newline="",
        dir=path.parent,
        prefix=f".{path.name}.",
        suffix=".tmp",
        delete=False,
    ) as handle:
        temporary_path = Path(handle.name)
        writer = csv.DictWriter(handle, fieldnames=["question", "reference", "enhanced_answer"])
        writer.writeheader()
        writer.writerows(rows)
    try:
        os.replace(temporary_path, path)
    except OSError:
        temporary_path.unlink(missing_ok=True)
        raise


if __name__ == "__main__":
    raise SystemExit(main())
