"""Tests for the scoring in run.py.

Run with: python -m unittest test_run.py
"""

import json
import os
import unittest

import run


def ev(kind, **data):
    return {"event": kind, "data": data}


def end(status="completed", final=None, process=None, **metadata):
    items = []
    if process:
        items.append({"kind": "text", "content": process, "content_phase": "process"})
    if final:
        items.append({"kind": "text", "content": final, "content_phase": "final"})
    metadata["presentation"] = {"items": items}
    return ev("message_end", conversation_id="c1", status=status, metadata=metadata)


class ParseTurnTest(unittest.TestCase):
    def test_final_presentation_is_the_answer(self):
        turn = run.parse_turn([
            ev("message", conversation_id="c1", answer="Let me work that out. "),
            ev("message_retract", content="Let me work that out. "),
            ev("message", answer="It is 42."),
            end(final="It is 42.", process="Let me work that out.", model_invocation_count=2, tool_names=["evaluate_expression"],
                native_skill_diagnostics={"activated_skill_ids": ["calculator"]}),
        ])
        self.assertEqual(turn["answer"], "It is 42.")
        self.assertIn("Let me work that out.", turn["visible_text"])
        self.assertEqual((turn["model_calls"], turn["tool_names"], turn["activated_skills"], turn["retracts"]),
                         (2, ["evaluate_expression"], ["calculator"], 1))

    def test_streamed_text_without_presentation_drops_retracted_text(self):
        turn = run.parse_turn([
            ev("message", answer="note "),
            ev("message", answer="answer"),
            ev("message_retract", content="answer"),
            ev("message", answer="final"),
            ev("message_end", status="completed", metadata={}),
        ])
        self.assertEqual(turn["answer"], "note final")

    def test_tool_errors_are_counted_apart_from_turn_errors(self):
        turn = run.parse_turn([
            ev("skill_call_start", arguments={"content_chars": 7038}),
            ev("skill_call_error", error="power exponent absolute value must be <= 100"),
            end(final="done"),
        ])
        self.assertEqual(turn["errors"], [])
        self.assertEqual(turn["tool_errors"], ["power exponent absolute value must be <= 100"])
        self.assertEqual(turn["tool_content_chars"], 7038)

    def test_question_back_is_pending_input(self):
        turn = run.parse_turn([
            ev("user_input_requested", conversation_id="c1", message_id="m1", request_id="r1", questions=[{"id": "amount"}, {}]),
            end(status="waiting_question"),
        ])
        self.assertEqual(turn["pending_input"]["question_ids"], ["amount", "q2"])


def turn(answer, lang_text=None, **overrides):
    base = {"answer": answer, "visible_text": lang_text or answer, "pending_input": None, "status": "completed", "errors": [],
            "tool_errors": [], "model_calls": 1, "tool_calls": 0, "tool_names": [], "activated_skills": [], "files": 0,
            "tool_content_chars": 0}
    base.update(overrides)
    return base


class ScoreTest(unittest.TestCase):
    def failed(self, scenario, turns):
        return [name for name, passed in run.score(scenario, turns) if not passed]

    def test_numbers_ignore_thousands_separators(self):
        self.assertIn(4389.69, run.numbers_in("利息约 4,389.69 元"))
        self.assertIn(199800.0, run.numbers_in("共 199,800 元"))
        self.assertIn(1234.0, run.numbers_in("1234"))

    def test_answer_values_within_tolerance(self):
        scenario = {"lang": "zh", "turns": ["q"], "expect": {"answer_values": [55.77]}}
        self.assertEqual(self.failed(scenario, [turn("最终价格是 55.77 美元")]), [])
        self.assertEqual(self.failed(scenario, [turn("最终价格是 55.76 美元")]), ["answer has 55.77"])

    def test_answer_values_none_ignores_thousands_separators(self):
        scenario = {"lang": "zh", "turns": ["q"], "expect": {"answer_values_none": [1000950.29], "tolerance": 0.01}}
        self.assertEqual(self.failed(scenario, [turn("第一步结果是 7,006,652")]), [])
        self.assertEqual(self.failed(scenario, [turn("结果是 1,000,950.29")]), ["answer lacks 1.00095e+06"])

    def test_internal_names_and_language_are_checked_on_every_turn(self):
        scenario = {"lang": "en", "turns": ["a", "b"], "expect": {}}
        failed = self.failed(scenario, [turn("I used activate_skills for this."), turn("答案是四十二。")])
        self.assertEqual(failed, ["turn 1: no internal names (activate_skills)", "turn 2: answers in English (6 CJK chars)"])

    def test_language_is_judged_on_prose_only(self):
        zh = {"lang": "zh", "turns": ["q"], "expect": {}}
        # A bare number or formula has no language to judge.
        self.assertEqual(self.failed(zh, [turn("8760")]), [])
        self.assertEqual(self.failed(zh, [turn("2400 × 15% = 2400 × 0.15 = **360**")]), [])
        # Links, URLs and code are not counted against the answer's language.
        linked = "文件已生成：[下载 squares.csv](http://localhost:8679/console/api/files/tools/abc.csv?expires_at=1&sign=xyz)，表头是 `n,n^2,n^3`。"
        self.assertEqual(self.failed(zh, [turn(linked)]), [])
        # A short quotation of the source is not the answer's language.
        en = {"lang": "en", "turns": ["q"], "expect": {}}
        quoted = 'Tomorrow at 3 p.m. in Conference Room 2. ("二号会议室" could also be "Meeting Room 2".)'
        self.assertEqual(self.failed(en, [turn(quoted)]), [])
        # An answer written in the wrong language still fails.
        self.assertEqual(self.failed(zh, [turn("15% of 2400 is **360**.")]), ["answers in Chinese (0% CJK)"])

    def test_skill_expectations_use_the_whole_conversation(self):
        scenario = {"lang": "en", "turns": ["a", "b"], "expect": {"skills_any": ["calculator"]}}
        self.assertEqual(self.failed(scenario, [turn("ok", activated_skills=["calculator"]), turn("done")]), [])
        scenario["expect"] = {"skills_none": True}
        self.assertEqual(self.failed(scenario, [turn("ok"), turn("done", tool_names=["generate_file"])]), ["used no skill (generate_file)"])

    def test_turn_expectations_apply_to_their_turn(self):
        scenario = {"lang": "en", "turns": ["a", "b"], "turn_expect": {"0": {"asked_user": True}}, "expect": {"answer_values": [4490.45]}}
        asked = turn("", pending_input={"question_ids": ["q1"]}, status="waiting_question")
        self.assertEqual(self.failed(scenario, [asked, turn("The payment is 4,490.45.")]), [])
        self.assertEqual(self.failed(scenario, [turn("It is about 4,000."), turn("The payment is 4,490.45.")]),
                         ["turn 1: asked the user for missing details"])

    def test_scenarios_file_uses_known_expectations(self):
        known = {"answer_all", "answer_any", "answer_none", "answer_regex", "answer_values", "answer_values_none", "tolerance", "asked_user", "skills_none",
                 "skills_any", "tools_any", "files_min", "tool_content_min_chars", "model_calls_max"}
        with open(os.path.join(run.HERE, "scenarios.json"), encoding="utf-8") as handle:
            scenarios = json.load(handle)["scenarios"]
        self.assertEqual(len({s["id"] for s in scenarios}), len(scenarios))
        for scenario in scenarios:
            expectations = [scenario.get("expect", {})] + list(scenario.get("turn_expect", {}).values())
            for expect in expectations:
                self.assertLessEqual(set(expect), known, scenario["id"])
            for index in scenario.get("turn_expect", {}):
                self.assertLess(int(index), len(scenario["turns"]), scenario["id"])
            self.assertIn(scenario.get("lang"), ("zh", "en"), scenario["id"])
            self.assertLessEqual(set(scenario), {"id", "category", "lang", "turns", "turn_expect", "expect", "observe", "accept_language"}, scenario["id"])


if __name__ == "__main__":
    unittest.main()
