#!/usr/bin/env python3
"""Compare completed Dify and ZGI Ragas results without calling either backend or Ragas."""

from __future__ import annotations

import argparse
import json
import math
import random
import statistics
from pathlib import Path
from typing import Any

import run_ragas_eval as shared


METRICS = [
    "faithfulness",
    "answer_relevancy",
    "context_precision",
    "context_recall",
    "answer_correctness",
]
COMPOSITE_METRIC = "composite_score"


def main() -> int:
    args = parse_args()
    input_path = shared.choose_input_path(args.input)
    if not input_path.exists():
        raise SystemExit(f"input file does not exist: {input_path}")

    dify_dataset_default, dify_result_default, _ = shared.output_paths_for_input(input_path, "dify")
    zgi_dataset_default, zgi_result_default, _ = shared.output_paths_for_input(input_path, "zgi")
    dify_result_path = resolve_path(args.dify_results, dify_result_default)
    zgi_result_path = resolve_path(args.zgi_results, zgi_result_default)

    dify_rows = load_result_rows(dify_result_path, "dify")
    zgi_rows = load_result_rows(zgi_result_path, "zgi")
    comparison_rows = build_comparison_rows(dify_rows, zgi_rows, args.tie_tolerance)
    paired_rows = [row for row in comparison_rows if row["pair_status"] == "paired"]
    if not paired_rows:
        raise SystemExit("Dify and ZGI results do not contain any comparable sample IDs")

    metric_summaries = {
        metric: summarize_metric(paired_rows, metric, args.tie_tolerance, args.bootstrap_samples)
        for metric in [*METRICS, COMPOSITE_METRIC]
    }

    dify_dataset_path = resolve_optional_path(args.dify_dataset, dify_dataset_default)
    zgi_dataset_path = resolve_optional_path(args.zgi_dataset, zgi_dataset_default)
    dataset_summaries = {
        "dify": summarize_dataset(dify_dataset_path),
        "zgi": summarize_dataset(zgi_dataset_path),
    }

    shared.RESULT_DIR.mkdir(parents=True, exist_ok=True)
    output_prefix = shared.RESULT_DIR / input_path.with_suffix("").name
    csv_path = output_prefix.with_name(output_prefix.name + ".comparison.csv")
    json_path = output_prefix.with_name(output_prefix.name + ".comparison.json")
    markdown_path = output_prefix.with_name(output_prefix.name + ".comparison.md")
    shared.write_csv(csv_path, comparison_rows)
    shared.write_json(
        json_path,
        {
            "dify_results": str(dify_result_path),
            "zgi_results": str(zgi_result_path),
            "dify_result_rows": len(dify_rows),
            "zgi_result_rows": len(zgi_rows),
            "paired_rows": len(paired_rows),
            "tie_tolerance": args.tie_tolerance,
            "metrics": metric_summaries,
            "datasets": dataset_summaries,
        },
    )
    markdown_path.write_text(
        render_markdown(
            input_path=input_path,
            dify_result_path=dify_result_path,
            zgi_result_path=zgi_result_path,
            comparison_rows=comparison_rows,
            metric_summaries=metric_summaries,
            dataset_summaries=dataset_summaries,
            tie_tolerance=args.tie_tolerance,
        ),
        encoding="utf-8",
    )
    print(f"saved comparison CSV: {csv_path}")
    print(f"saved comparison JSON: {json_path}")
    print(f"saved comparison report: {markdown_path}")
    return 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Compare existing Dify and ZGI Ragas result files.")
    parser.add_argument("--input", default="", help="QA input file used by both completed evaluations.")
    parser.add_argument("--dify-results", default="", help="Override the Dify Ragas result JSON path.")
    parser.add_argument("--zgi-results", default="", help="Override the ZGI Ragas result JSON path.")
    parser.add_argument("--dify-dataset", default="", help="Optional Dify dataset JSON path for operational statistics.")
    parser.add_argument("--zgi-dataset", default="", help="Optional ZGI dataset JSON path for operational statistics.")
    parser.add_argument("--tie-tolerance", type=float, default=0.01, help="Absolute score difference counted as a tie. Default: %(default)s")
    parser.add_argument("--bootstrap-samples", type=int, default=2000, help="Paired bootstrap samples for the mean-delta CI. Default: %(default)s")
    args = parser.parse_args()
    if args.tie_tolerance < 0:
        raise SystemExit("--tie-tolerance must be >= 0")
    if args.bootstrap_samples < 100:
        raise SystemExit("--bootstrap-samples must be >= 100")
    return args


def resolve_path(value: str, default: Path) -> Path:
    path = Path(value).expanduser().resolve() if value else default
    if not path.exists():
        raise SystemExit(f"required result file does not exist: {path}")
    return path


def resolve_optional_path(value: str, default: Path) -> Path | None:
    path = Path(value).expanduser().resolve() if value else default
    return path if path.exists() else None


def load_result_rows(path: Path, platform: str) -> dict[int, dict[str, Any]]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise SystemExit(f"result JSON is invalid: {path}") from exc
    if not isinstance(data, list):
        raise SystemExit(f"result JSON must contain a list: {path}")
    rows: dict[int, dict[str, Any]] = {}
    for fallback_id, row in enumerate(data, start=1):
        if not isinstance(row, dict):
            raise SystemExit(f"{platform} result row {fallback_id} is not an object: {path}")
        sample_id = parse_sample_id(row.get("sample_id"), fallback_id, platform)
        if sample_id in rows:
            raise SystemExit(f"duplicate {platform} sample_id {sample_id}: {path}")
        rows[sample_id] = row
    return rows


def parse_sample_id(value: Any, fallback: int, platform: str) -> int:
    if value is None or value == "":
        return fallback
    try:
        sample_id = int(value)
    except (TypeError, ValueError) as exc:
        raise SystemExit(f"invalid {platform} sample_id: {value}") from exc
    if sample_id < 1:
        raise SystemExit(f"invalid {platform} sample_id: {value}")
    return sample_id


def build_comparison_rows(
    dify_rows: dict[int, dict[str, Any]],
    zgi_rows: dict[int, dict[str, Any]],
    tie_tolerance: float,
) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for sample_id in sorted(set(dify_rows) | set(zgi_rows)):
        dify = dify_rows.get(sample_id)
        zgi = zgi_rows.get(sample_id)
        if dify and zgi:
            dify_question = str(dify.get("user_input") or "")
            zgi_question = str(zgi.get("user_input") or "")
            if dify_question != zgi_question:
                raise SystemExit(
                    f"sample_id {sample_id} question mismatch: Dify={dify_question!r}, ZGI={zgi_question!r}"
                )
            dify_reference = str(dify.get("reference") or "")
            zgi_reference = str(zgi.get("reference") or "")
            if dify_reference != zgi_reference:
                raise SystemExit(f"sample_id {sample_id} reference answer mismatch between Dify and ZGI results")
            pair_status = "paired"
        else:
            pair_status = "missing_zgi" if dify else "missing_dify"

        source = dify or zgi or {}
        row: dict[str, Any] = {
            "sample_id": sample_id,
            "pair_status": pair_status,
            "user_input": source.get("user_input", ""),
            "reference": source.get("reference", ""),
            "dify_response": dify.get("response", "") if dify else "",
            "zgi_response": zgi.get("response", "") if zgi else "",
        }
        dify_scores = metric_values(dify)
        zgi_scores = metric_values(zgi)
        for metric in METRICS:
            add_score_columns(row, metric, dify_scores.get(metric), zgi_scores.get(metric), tie_tolerance)
        dify_composite = composite_score(dify_scores)
        zgi_composite = composite_score(zgi_scores)
        add_score_columns(row, COMPOSITE_METRIC, dify_composite, zgi_composite, tie_tolerance)
        rows.append(row)
    return rows


def metric_values(row: dict[str, Any] | None) -> dict[str, float]:
    if not row:
        return {}
    values: dict[str, float] = {}
    for metric in METRICS:
        value = number_or_none(row.get(metric))
        if value is not None:
            values[metric] = value
    return values


def add_score_columns(
    row: dict[str, Any],
    metric: str,
    dify_value: float | None,
    zgi_value: float | None,
    tie_tolerance: float,
) -> None:
    row[f"dify_{metric}"] = "" if dify_value is None else dify_value
    row[f"zgi_{metric}"] = "" if zgi_value is None else zgi_value
    if dify_value is None or zgi_value is None:
        row[f"{metric}_delta"] = ""
        row[f"{metric}_winner"] = ""
        return
    delta = dify_value - zgi_value
    row[f"{metric}_delta"] = delta
    row[f"{metric}_winner"] = winner(delta, tie_tolerance)


def winner(delta: float, tolerance: float) -> str:
    if abs(delta) <= tolerance:
        return "tie"
    return "dify" if delta > 0 else "zgi"


def summarize_metric(
    paired_rows: list[dict[str, Any]],
    metric: str,
    tie_tolerance: float,
    bootstrap_samples: int,
) -> dict[str, Any]:
    triples = [
        (float(row[f"dify_{metric}"]), float(row[f"zgi_{metric}"]), float(row[f"{metric}_delta"]))
        for row in paired_rows
        if row[f"dify_{metric}"] != "" and row[f"zgi_{metric}"] != ""
    ]
    if not triples:
        return {"paired": 0}
    dify_values = [item[0] for item in triples]
    zgi_values = [item[1] for item in triples]
    deltas = [item[2] for item in triples]
    wins = {"dify": 0, "tie": 0, "zgi": 0}
    for delta in deltas:
        wins[winner(delta, tie_tolerance)] += 1
    ci_low, ci_high = bootstrap_mean_ci(deltas, bootstrap_samples)
    return {
        "paired": len(triples),
        "dify_mean": statistics.fmean(dify_values),
        "zgi_mean": statistics.fmean(zgi_values),
        "mean_delta": statistics.fmean(deltas),
        "median_delta": statistics.median(deltas),
        "ci95_low": ci_low,
        "ci95_high": ci_high,
        "dify_wins": wins["dify"],
        "ties": wins["tie"],
        "zgi_wins": wins["zgi"],
    }


def bootstrap_mean_ci(values: list[float], samples: int) -> tuple[float, float]:
    if len(values) == 1:
        return values[0], values[0]
    rng = random.Random(20260716)
    means = [statistics.fmean(rng.choice(values) for _ in values) for _ in range(samples)]
    means.sort()
    low_index = max(0, math.floor(samples * 0.025))
    high_index = min(samples - 1, math.ceil(samples * 0.975) - 1)
    return means[low_index], means[high_index]


def summarize_dataset(path: Path | None) -> dict[str, Any]:
    if path is None:
        return {"available": False}
    rows = shared.load_existing_dataset(path)
    latencies = [value for row in rows if (value := number_or_none(row.get("latency_seconds"))) is not None]
    context_counts = [len(row.get("retrieved_contexts") or []) for row in rows]
    successful = [row for row in rows if row.get("response") and not row.get("error")]
    return {
        "available": True,
        "path": str(path),
        "rows": len(rows),
        "successful": len(successful),
        "errors": len(rows) - len(successful),
        "empty_contexts": sum(1 for row in successful if not row.get("retrieved_contexts")),
        "mean_contexts": statistics.fmean(context_counts) if context_counts else None,
        "mean_latency_seconds": statistics.fmean(latencies) if latencies else None,
        "p50_latency_seconds": percentile(latencies, 0.50),
        "p95_latency_seconds": percentile(latencies, 0.95),
    }


def percentile(values: list[float], quantile: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    position = (len(ordered) - 1) * quantile
    lower = math.floor(position)
    upper = math.ceil(position)
    if lower == upper:
        return ordered[lower]
    fraction = position - lower
    return ordered[lower] * (1 - fraction) + ordered[upper] * fraction


def render_markdown(
    input_path: Path,
    dify_result_path: Path,
    zgi_result_path: Path,
    comparison_rows: list[dict[str, Any]],
    metric_summaries: dict[str, dict[str, Any]],
    dataset_summaries: dict[str, dict[str, Any]],
    tie_tolerance: float,
) -> str:
    paired = [row for row in comparison_rows if row["pair_status"] == "paired"]
    missing_dify = sum(1 for row in comparison_rows if row["pair_status"] == "missing_dify")
    missing_zgi = sum(1 for row in comparison_rows if row["pair_status"] == "missing_zgi")
    lines = [
        "# Dify 与 ZGI RAG 评测对比",
        "",
        f"- QA 文件：`{input_path}`",
        f"- Dify 结果：`{dify_result_path}`",
        f"- ZGI 结果：`{zgi_result_path}`",
        f"- 成功配对：{len(paired)}；缺少 Dify：{missing_dify}；缺少 ZGI：{missing_zgi}",
        f"- 差值方向：Dify - ZGI；绝对差值不超过 {tie_tolerance:.3f} 计为平局。",
        "",
        "## 总体结论",
        "",
        overall_conclusion(metric_summaries[COMPOSITE_METRIC]),
        "",
        "## 指标汇总",
        "",
        "| 指标 | Dify 均值 | ZGI 均值 | 平均差值 | 95% CI | Dify胜/平/ZGI胜 | 配对数 |",
        "|---|---:|---:|---:|---:|---:|---:|",
    ]
    labels = {
        "faithfulness": "忠实度",
        "answer_relevancy": "答案相关性",
        "context_precision": "上下文精确率",
        "context_recall": "上下文召回率",
        "answer_correctness": "答案正确性",
        COMPOSITE_METRIC: "综合分（五项平均）",
    }
    for metric in [*METRICS, COMPOSITE_METRIC]:
        summary = metric_summaries[metric]
        if not summary.get("paired"):
            lines.append(f"| {labels[metric]} | N/A | N/A | N/A | N/A | 0/0/0 | 0 |")
            continue
        lines.append(
            f"| {labels[metric]} | {summary['dify_mean']:.3f} | {summary['zgi_mean']:.3f} | "
            f"{summary['mean_delta']:+.3f} | [{summary['ci95_low']:+.3f}, {summary['ci95_high']:+.3f}] | "
            f"{summary['dify_wins']}/{summary['ties']}/{summary['zgi_wins']} | {summary['paired']} |"
        )

    lines.extend(["", "## 采集质量与耗时", ""])
    for platform in ("dify", "zgi"):
        summary = dataset_summaries[platform]
        name = platform.upper() if platform == "zgi" else "Dify"
        if not summary.get("available"):
            lines.append(f"- {name}：未找到 dataset JSON，仅生成 Ragas 指标对比。")
            continue
        latency = (
            f"平均/P50/P95 延迟={format_optional(summary['mean_latency_seconds'])}/"
            f"{format_optional(summary['p50_latency_seconds'])}/{format_optional(summary['p95_latency_seconds'])} 秒"
            if summary.get("mean_latency_seconds") is not None
            else "未记录逐题延迟"
        )
        lines.append(
            f"- {name}：{summary['successful']}/{summary['rows']} 成功，错误 {summary['errors']}，"
            f"空召回 {summary['empty_contexts']}，平均召回 {summary['mean_contexts']:.2f} 条，{latency}。"
        )

    scored = [row for row in paired if row[f"{COMPOSITE_METRIC}_delta"] != ""]
    dify_top = sorted(
        [row for row in scored if float(row[f"{COMPOSITE_METRIC}_delta"]) > tie_tolerance],
        key=lambda row: float(row[f"{COMPOSITE_METRIC}_delta"]),
        reverse=True,
    )[:10]
    zgi_top = sorted(
        [row for row in scored if float(row[f"{COMPOSITE_METRIC}_delta"]) < -tie_tolerance],
        key=lambda row: float(row[f"{COMPOSITE_METRIC}_delta"]),
    )[:10]
    lines.extend(["", "## Dify 优势最大的题目", ""])
    lines.extend(render_top_rows(dify_top))
    lines.extend(["", "## ZGI 优势最大的题目", ""])
    lines.extend(render_top_rows(zgi_top))
    lines.append("")
    return "\n".join(lines)


def render_top_rows(rows: list[dict[str, Any]]) -> list[str]:
    if not rows:
        return ["没有超过平局阈值的题目。"]
    lines = ["| 样本 | 综合差值 | 问题 |", "|---:|---:|---|"]
    for row in rows:
        question = str(row["user_input"]).replace("|", "\\|").replace("\n", " ")
        lines.append(f"| {row['sample_id']} | {float(row[f'{COMPOSITE_METRIC}_delta']):+.3f} | {question} |")
    return lines


def number_or_none(value: Any) -> float | None:
    if value is None or value == "":
        return None
    try:
        number = float(value)
    except (TypeError, ValueError):
        return None
    return number if math.isfinite(number) else None


def composite_score(values: dict[str, float]) -> float | None:
    if any(metric not in values for metric in METRICS):
        return None
    return statistics.fmean(values[metric] for metric in METRICS)


def overall_conclusion(summary: dict[str, Any]) -> str:
    if not summary.get("paired"):
        return "没有足够的配对综合分，无法形成总体结论。"
    delta = summary["mean_delta"]
    low = summary["ci95_low"]
    high = summary["ci95_high"]
    if summary["paired"] == 1:
        return (
            f"当前只有 1 个配对样本，只能作为流程冒烟结果，不能判断平台优劣。"
            f"该样本综合差值（Dify - ZGI）为 {delta:+.3f}。"
        )
    if low > 0:
        judgment = "Dify 的综合分稳定高于 ZGI"
    elif high < 0:
        judgment = "ZGI 的综合分稳定高于 Dify"
    else:
        judgment = "综合分差异的 95% 置信区间跨过 0，暂不能认为一方稳定优于另一方"
    return f"{judgment}。配对平均差值（Dify - ZGI）为 {delta:+.3f}，95% CI 为 [{low:+.3f}, {high:+.3f}]。"


def format_optional(value: Any) -> str:
    number = number_or_none(value)
    return "N/A" if number is None else f"{number:.3f}"


if __name__ == "__main__":
    raise SystemExit(main())
