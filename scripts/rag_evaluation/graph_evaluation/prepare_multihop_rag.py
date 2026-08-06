#!/usr/bin/env python3
"""Prepare a focused MultiHop-RAG dataset for graph retrieval evaluation.

The script selects 50 temporal questions and 50 comparison questions with a
fixed random seed, then resolves and exports only the corpus articles referenced
by those questions' gold evidence annotations.
"""

from __future__ import annotations

import argparse
import csv
import re
import tempfile
from pathlib import Path
from typing import Any, Iterable
from urllib.parse import urlsplit, urlunsplit


DATASET_NAME = "yixuantt/MultiHopRAG"
QUESTION_CONFIG = "MultiHopRAG"
CORPUS_CONFIG = "corpus"
SPLIT = "train"
TEMPORAL_TYPE = "temporal_query"
COMPARISON_TYPE = "comparison_query"
QUESTIONS_PER_TYPE = 50
EXPECTED_QUESTION_COUNT = QUESTIONS_PER_TYPE * 2
DEFAULT_SEED = 42
SCRIPT_DIR = Path(__file__).resolve().parent
DEFAULT_OUTPUT_DIR = SCRIPT_DIR / "output"


def main() -> int:
    args = parse_args()
    output_dir = Path(args.output_dir).expanduser().resolve()
    ensure_output_target_is_available(output_dir)

    concatenate_datasets, load_dataset = import_datasets()

    load_kwargs: dict[str, Any] = {"split": SPLIT}
    if args.cache_dir:
        load_kwargs["cache_dir"] = str(Path(args.cache_dir).expanduser().resolve())

    print("Loading MultiHop-RAG questions...")
    questions = load_dataset(DATASET_NAME, QUESTION_CONFIG, **load_kwargs)
    questions = questions.add_column("source_id", list(range(len(questions))))

    print("Selecting 50 temporal and 50 comparison questions...")
    selected_questions = select_questions(questions, concatenate_datasets, args.seed)

    print("Loading the source corpus...")
    corpus = load_dataset(DATASET_NAME, CORPUS_CONFIG, **load_kwargs)
    corpus_rows = [dict(row) for row in corpus]

    selected_document_indices, unmatched = resolve_source_documents(
        selected_questions,
        corpus_rows,
    )
    if unmatched:
        details = "\n".join(
            f"- question source_id={item['source_id']}, "
            f"title={item['title']!r}, url={item['url']!r}"
            for item in unmatched[:10]
        )
        raise SystemExit(
            f"failed to match {len(unmatched)} evidence records to corpus articles:\n{details}"
        )

    output_dir.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(
        dir=output_dir.parent,
        prefix=".graph_evaluation_",
    ) as temporary_directory:
        staging_dir = Path(temporary_directory)
        write_qa_pairs(staging_dir / "qa_pairs.csv", selected_questions)
        write_documents(
            staging_dir / "documents",
            corpus_rows,
            selected_document_indices,
        )
        staging_dir.rename(output_dir)

    print(f"Prepared {len(selected_questions)} QA pairs")
    print(f"Prepared {len(selected_document_indices)} unique source documents")
    print(f"QA output: {output_dir / 'qa_pairs.csv'}")
    print(f"Document output: {output_dir / 'documents'}")
    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Select 100 MultiHop-RAG questions and export their referenced "
            "source articles as individual Markdown documents."
        )
    )
    parser.add_argument(
        "--output-dir",
        default=str(DEFAULT_OUTPUT_DIR),
        help="Output directory. It must not already contain files.",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=DEFAULT_SEED,
        help="Random seed used for reproducible sampling. Default: %(default)s",
    )
    parser.add_argument(
        "--cache-dir",
        default="",
        help="Optional Hugging Face datasets cache directory.",
    )
    return parser.parse_args()


def import_datasets() -> tuple[Any, Any]:
    try:
        from datasets import concatenate_datasets, load_dataset
    except ImportError as exc:
        raise SystemExit(
            "missing dependency 'datasets'; run "
            "'pip install -r scripts/rag_evaluation/requirements.txt' first"
        ) from exc
    return concatenate_datasets, load_dataset


def ensure_output_target_is_available(output_dir: Path) -> None:
    if output_dir.exists():
        if any(output_dir.iterdir()):
            raise SystemExit(
                f"output directory is not empty: {output_dir}\n"
                "Choose another --output-dir or move the existing artifacts first."
            )
        output_dir.rmdir()


def select_questions(questions: Any, concatenate_datasets: Any, seed: int) -> Any:
    temporal = questions.filter(
        lambda row: row["question_type"] == TEMPORAL_TYPE,
        desc="Filtering temporal questions",
    )
    comparison = questions.filter(
        lambda row: row["question_type"] == COMPARISON_TYPE,
        desc="Filtering comparison questions",
    )

    require_sample_capacity(temporal, TEMPORAL_TYPE)
    require_sample_capacity(comparison, COMPARISON_TYPE)

    selected = concatenate_datasets(
        [
            temporal.shuffle(seed=seed).select(range(QUESTIONS_PER_TYPE)),
            comparison.shuffle(seed=seed).select(range(QUESTIONS_PER_TYPE)),
        ]
    ).shuffle(seed=seed)

    if len(selected) != EXPECTED_QUESTION_COUNT:
        raise RuntimeError(
            f"selected {len(selected)} questions, expected {EXPECTED_QUESTION_COUNT}"
        )
    return selected


def require_sample_capacity(rows: Any, question_type: str) -> None:
    if len(rows) < QUESTIONS_PER_TYPE:
        raise SystemExit(
            f"dataset contains only {len(rows)} {question_type} rows; "
            f"at least {QUESTIONS_PER_TYPE} are required"
        )


def resolve_source_documents(
    selected_questions: Iterable[dict[str, Any]],
    corpus_rows: list[dict[str, Any]],
) -> tuple[list[int], list[dict[str, Any]]]:
    by_url: dict[str, int] = {}
    by_title: dict[str, int] = {}
    for index, document in enumerate(corpus_rows):
        url = normalize_url(document.get("url"))
        title = normalize_title(document.get("title"))
        if url:
            by_url.setdefault(url, index)
        if title:
            by_title.setdefault(title, index)

    selected_indices: set[int] = set()
    unmatched: list[dict[str, Any]] = []
    for question in selected_questions:
        source_id = question.get("source_id")
        evidence_list = question.get("evidence_list") or []
        for evidence in evidence_list:
            evidence = evidence or {}
            url = normalize_url(evidence.get("url"))
            title = normalize_title(evidence.get("title"))
            document_index = by_url.get(url) if url else None
            if document_index is None and title:
                document_index = by_title.get(title)
            if document_index is None:
                unmatched.append(
                    {
                        "source_id": source_id,
                        "title": evidence.get("title"),
                        "url": evidence.get("url"),
                    }
                )
                continue
            selected_indices.add(document_index)

    return sorted(selected_indices), unmatched


def normalize_url(value: Any) -> str:
    if not value:
        return ""
    parts = urlsplit(str(value).strip())
    return urlunsplit(
        (
            parts.scheme.lower(),
            parts.netloc.lower(),
            parts.path.rstrip("/"),
            "",
            "",
        )
    )


def normalize_title(value: Any) -> str:
    return " ".join(str(value or "").casefold().split())


def write_qa_pairs(path: Path, selected_questions: Iterable[dict[str, Any]]) -> None:
    with path.open("w", encoding="utf-8-sig", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=["question", "reference", "enhanced_answer"])
        writer.writeheader()
        for question in selected_questions:
            reference = clean_text(question.get("answer"))
            writer.writerow(
                {
                    "question": str(question.get("query") or "").strip(),
                    "reference": reference,
                    "enhanced_answer": build_explanatory_reference(question),
                }
            )


def build_explanatory_reference(question: dict[str, Any]) -> str:
    """Build an enhanced English answer from MultiHopRAG's gold evidence facts.

    The short benchmark answer is retained as the RAGAS ``reference``. Keeping the
    evidence in the evaluation-only QA file avoids leaking it into the Markdown
    source documents imported into the knowledge base.
    """

    original_answer = clean_text(question.get("answer"))
    if not original_answer:
        raise ValueError("question is missing its original answer")

    parts = [f"Answer: {original_answer.capitalize()}.", "Evidence:"]
    evidence_list = question.get("evidence_list") or []
    if not evidence_list:
        raise ValueError("question is missing gold evidence")

    for index, evidence in enumerate(evidence_list, start=1):
        evidence = evidence or {}
        source = clean_text(evidence.get("source")) or "Unknown source"
        title = clean_text(evidence.get("title")) or "untitled article"
        published_at = clean_text(evidence.get("published_at"))
        published = f" ({published_at[:10]})" if published_at else ""
        fact = clean_text(evidence.get("fact"))
        if not fact:
            raise ValueError(f"gold evidence {index} is missing its fact")
        parts.append(f"{index}. {source}, \"{title}\"{published}: {fact}")

    parts.append("Together, these source facts support the answer above.")
    return "\n".join(parts)


def write_documents(
    documents_dir: Path,
    corpus_rows: list[dict[str, Any]],
    selected_document_indices: list[int],
) -> None:
    documents_dir.mkdir(parents=True, exist_ok=False)
    for output_index, corpus_index in enumerate(selected_document_indices, start=1):
        article = corpus_rows[corpus_index]
        content = render_markdown_article(article, corpus_index)
        filename = documents_dir / f"document_{output_index:04d}.md"
        filename.write_text(content, encoding="utf-8")


def render_markdown_article(article: dict[str, Any], corpus_index: int) -> str:
    title = clean_text(article.get("title")) or f"Corpus document {corpus_index}"
    author = clean_text(article.get("author")) or "Unknown"
    source = clean_text(article.get("source")) or "Unknown"
    published_at = clean_text(article.get("published_at")) or "Unknown"
    category = clean_text(article.get("category")) or "Unknown"
    url = clean_text(article.get("url"))
    body = str(article.get("body") or "").strip()
    return (
        f"# {title}\n\n"
        f"- Source: {source}\n"
        f"- Author: {author}\n"
        f"- Published at: {published_at}\n"
        f"- Category: {category}\n"
        f"- URL: {url}\n\n"
        f"## Content\n\n{body}\n"
    )


def clean_text(value: Any) -> str:
    return re.sub(r"\s+", " ", str(value or "")).strip()


if __name__ == "__main__":
    raise SystemExit(main())
