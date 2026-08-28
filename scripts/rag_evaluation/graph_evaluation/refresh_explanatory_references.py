#!/usr/bin/env python3
"""Refresh explanatory English answers for prepared MultiHopRAG QA pairs."""

from __future__ import annotations

import argparse
import csv
import os
import tempfile
from pathlib import Path
from typing import Any

from prepare_multihop_rag import (
    DATASET_NAME,
    DEFAULT_OUTPUT_DIR,
    DEFAULT_SEED,
    QUESTION_CONFIG,
    SPLIT,
    build_explanatory_reference,
    import_datasets,
    select_questions,
)


def main() -> int:
    args = parse_args()
    qa_path = Path(args.qa_path).expanduser().resolve()
    existing_rows = read_qa_rows(qa_path)

    concatenate_datasets, load_dataset = import_datasets()
    load_kwargs: dict[str, Any] = {"split": SPLIT}
    if args.cache_dir:
        load_kwargs["cache_dir"] = str(Path(args.cache_dir).expanduser().resolve())

    print("Loading selected MultiHopRAG questions and gold evidence facts...")
    questions = load_dataset(DATASET_NAME, QUESTION_CONFIG, **load_kwargs)
    selected = select_questions(
        questions.add_column("source_id", list(range(len(questions)))),
        concatenate_datasets,
        args.seed,
    )
    refreshed_rows = build_rows(existing_rows, selected)

    print(f"Prepared {len(refreshed_rows)} explanatory English references")
    for index, row in enumerate(refreshed_rows[: args.preview_rows], start=1):
        print(f"\n--- Preview {index} ---")
        print(f"Question: {row['question']}")
        print(f"Reference: {row['reference']}")
        print(f"Enhanced answer:\n{row['enhanced_answer']}")

    if args.dry_run:
        print("Dry run completed; qa_pairs.csv was not changed.")
        return 0

    write_rows_atomically(qa_path, refreshed_rows)
    print(f"Updated QA file: {qa_path}")
    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Refresh enhanced English answers from MultiHopRAG gold evidence."
    )
    parser.add_argument(
        "--qa-path",
        default=str(DEFAULT_OUTPUT_DIR / "qa_pairs.csv"),
        help="Prepared QA CSV to update. Default: %(default)s",
    )
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED, help="Question-selection seed. Default: %(default)s")
    parser.add_argument("--cache-dir", default="", help="Optional Hugging Face datasets cache directory.")
    parser.add_argument("--preview-rows", type=int, default=2, help="Number of enhanced answers to preview. Default: %(default)s")
    parser.add_argument("--dry-run", action="store_true", help="Validate and preview without overwriting the CSV.")
    return parser.parse_args()


def read_qa_rows(path: Path) -> list[dict[str, str]]:
    if not path.exists():
        raise SystemExit(f"QA file does not exist: {path}")
    with path.open("r", encoding="utf-8-sig", newline="") as handle:
        rows = list(csv.DictReader(handle))
    if not rows:
        raise SystemExit(f"QA file contains no data rows: {path}")
    if not all(str(row.get("question") or "").strip() for row in rows):
        raise SystemExit(f"QA file has an empty question: {path}")
    return rows


def build_rows(existing_rows: list[dict[str, str]], selected_questions: Any) -> list[dict[str, str]]:
    if len(existing_rows) != len(selected_questions):
        raise SystemExit(
            f"QA file has {len(existing_rows)} rows, but seed-selected dataset has {len(selected_questions)} rows"
        )

    refreshed: list[dict[str, str]] = []
    for index, (existing, question) in enumerate(zip(existing_rows, selected_questions), start=1):
        expected_question = str(question.get("query") or "").strip()
        actual_question = str(existing.get("question") or "").strip()
        if actual_question != expected_question:
            raise SystemExit(
                f"question mismatch at row {index}; refusing to apply evidence to a different QA set"
            )
        reference = str(question.get("answer") or "").strip()
        refreshed.append(
            {
                "question": expected_question,
                "reference": reference,
                "enhanced_answer": build_explanatory_reference(dict(question)),
            }
        )
    return refreshed


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
