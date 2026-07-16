#!/usr/bin/env python3
"""Targeted unit tests for the decoupled Dify/ZGI evaluation workflow."""

from __future__ import annotations

import unittest
from pathlib import Path

import compare_rag_eval
import run_dify_eval
import run_ragas_eval


class EvaluationWorkflowTest(unittest.TestCase):
    def test_platform_output_paths_are_isolated(self) -> None:
        input_path = Path("/tmp/example.xlsx")

        dify_paths = run_ragas_eval.output_paths_for_input(input_path, "dify")
        zgi_paths = run_ragas_eval.output_paths_for_input(input_path, "zgi")

        self.assertTrue(dify_paths[0].name.endswith(".dify.ragas.dataset.json"))
        self.assertTrue(zgi_paths[0].name.endswith(".zgi.ragas.dataset.json"))
        self.assertEqual(dify_paths[0].parent, run_ragas_eval.MIDDLE_DIR)
        self.assertEqual(dify_paths[1].parent, run_ragas_eval.RESULT_DIR)
        self.assertEqual(zgi_paths[2].parent, run_ragas_eval.RESULT_DIR)
        self.assertEqual(len(set(dify_paths + zgi_paths)), 6)

    def test_dify_response_is_normalized_for_ragas(self) -> None:
        qa = run_ragas_eval.QAItem(question="退号流程", reference="费用原路退回")
        response = {
            "answer": "线上取消预约，现场退号录入，费用原路退回。",
            "message_id": "message-1",
            "conversation_id": "conversation-1",
            "metadata": {
                "retriever_resources": [
                    {"content": "门诊患者退号：费用原路退回", "score": 0.8},
                    {"content": "", "score": 0.2},
                ],
                "usage": {"total_tokens": 100},
            },
        }

        row = run_dify_eval.build_dify_row(1, qa, response, 1.5)

        self.assertEqual(row["sample_id"], 1)
        self.assertEqual(row["platform"], "dify")
        self.assertEqual(row["status"], "success")
        self.assertEqual(row["retrieved_contexts"], ["门诊患者退号：费用原路退回"])
        self.assertEqual(row["usage"]["total_tokens"], 100)

    def test_comparison_pairs_by_sample_id_and_computes_dify_minus_zgi(self) -> None:
        dify = {2: result_row(2, 0.8), 1: result_row(1, 0.7)}
        zgi = {1: result_row(1, 0.5), 2: result_row(2, 0.9)}

        rows = compare_rag_eval.build_comparison_rows(dify, zgi, tie_tolerance=0.01)

        self.assertEqual([row["sample_id"] for row in rows], [1, 2])
        self.assertAlmostEqual(rows[0]["faithfulness_delta"], 0.2)
        self.assertEqual(rows[0]["faithfulness_winner"], "dify")
        self.assertAlmostEqual(rows[1]["composite_score_delta"], -0.1)
        self.assertEqual(rows[1]["composite_score_winner"], "zgi")

    def test_summary_uses_paired_rows_only(self) -> None:
        dify = {1: result_row(1, 0.8), 2: result_row(2, 0.6)}
        zgi = {1: result_row(1, 0.5)}
        rows = compare_rag_eval.build_comparison_rows(dify, zgi, tie_tolerance=0.01)
        paired = [row for row in rows if row["pair_status"] == "paired"]

        summary = compare_rag_eval.summarize_metric(
            paired,
            "answer_correctness",
            tie_tolerance=0.01,
            bootstrap_samples=100,
        )

        self.assertEqual(summary["paired"], 1)
        self.assertAlmostEqual(summary["mean_delta"], 0.3)
        self.assertEqual(summary["dify_wins"], 1)


def result_row(sample_id: int, score: float) -> dict[str, object]:
    row: dict[str, object] = {
        "sample_id": sample_id,
        "platform": "test",
        "user_input": f"question-{sample_id}",
        "reference": f"reference-{sample_id}",
        "response": f"response-{sample_id}",
    }
    for metric in compare_rag_eval.METRICS:
        row[metric] = score
    return row


if __name__ == "__main__":
    unittest.main()
