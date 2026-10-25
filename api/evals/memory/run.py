"""Runs the memory scenario suite against a ZGI deployment and scores it.

The suite drives an Agent through its external API, one API end user per
scenario, so the same scenarios can be replayed against the current slot
memory and against later memory designs. It uses only the Python standard
library.

Credentials come from the environment (ZGI_E2E_EMAIL, ZGI_E2E_PASSWORD), or
from an env file given with --env-file; they are never printed.

Example:
    python run.py --workspace-id <id> --label explicit --env-file ../../../.env.test.local
    python run.py --workspace-id <id> --label auto --auto --wait-after-session 90
"""

import argparse
import concurrent.futures
import datetime
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

HERE = os.path.dirname(os.path.abspath(__file__))

# The general-assistant template, so results are comparable to what builders get today.
SLOTS = [
    {"key": "profile", "name": "Profile", "description": "Stable facts about the user: name, role, location, family, health constraints.", "max_chars": 2000},
    {"key": "preferences", "name": "Preferences", "description": "How the user likes answers and things they like or avoid.", "max_chars": 2000},
    {"key": "standing_instructions", "name": "Standing instructions", "description": "Instructions the user wants followed in every conversation.", "max_chars": 2000},
    {"key": "project_context", "name": "Project context", "description": "Ongoing work, goals, and plans the user is pursuing.", "max_chars": 2000},
]


def load_env_file(path):
    values = {}
    if not path:
        return values
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                values[key.strip()] = value.strip().strip('"').strip("'")
    return values


class Client:
    def __init__(self, base_url):
        self.base = base_url.rstrip("/")
        self.console_token = None
        self.api_key = None

    def _request(self, method, path, body=None, token=None, timeout=120):
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if token:
            req.add_header("Authorization", "Bearer " + token)
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                text = resp.read().decode()
                return resp.status, json.loads(text) if text else None
        except urllib.error.HTTPError as err:
            return err.code, err.read().decode()[:1000]

    def console(self, method, path, body=None):
        status, payload = self._request(method, "/console/api" + path, body, self.console_token)
        if status != 200 or not isinstance(payload, dict) or str(payload.get("code")) != "0":
            raise RuntimeError(f"{method} {path} failed: {status} {str(payload)[:300]}")
        return payload.get("data")

    def external(self, method, path, body=None):
        status, payload = self._request(method, "/api/v1" + path, body, self.api_key)
        if status != 200 or not isinstance(payload, dict) or str(payload.get("code")) != "0":
            raise RuntimeError(f"{method} {path} failed: {status} {str(payload)[:300]}")
        return payload.get("data")

    def login(self, email, password):
        data = self.console("POST", "/login", {"email": email, "password": password})
        self.console_token = data.get("access_token") or data["data"]["access_token"]

    def chat(self, user, query, conversation_id=None, pending_input=None):
        """Sends one turn and returns the answer, memory events, usage, and latency.

        When the previous turn ended with the Agent asking the user a question,
        the conversation accepts only an answer to it (a new chat turn is refused),
        so the query is sent as the answer to every pending question instead.
        """
        if pending_input:
            path = "/api/v1/agents/conversations/{}/messages/{}/user-input/{}/continue?user={}".format(
                pending_input["conversation_id"], pending_input["message_id"],
                urllib.parse.quote(pending_input["request_id"]), urllib.parse.quote(user))
            body = {"answers": {question_id: query for question_id in pending_input["question_ids"]}}
        else:
            path = "/api/v1/agents/chat"
            body = {"query": query, "response_mode": "streaming", "user": user}
            if conversation_id:
                body["conversation_id"] = conversation_id
        req = urllib.request.Request(self.base + path, data=json.dumps(body).encode(), method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("Authorization", "Bearer " + self.api_key)
        started = time.time()
        events = []
        try:
            with urllib.request.urlopen(req, timeout=300) as resp:
                for raw in resp:
                    line = raw.decode("utf-8", "replace").strip()
                    if line.startswith("data:"):
                        try:
                            events.append(json.loads(line[5:]))
                        except ValueError:
                            pass
        except urllib.error.HTTPError as err:
            with err:
                detail = err.read().decode("utf-8", "replace")[:300]
            raise RuntimeError(f"POST {path.split('?')[0]} failed: {err.code} {detail}") from err
        answer, conversation, usage, memory_events, errors, pending = [], conversation_id, [], [], [], None
        for event in events:
            kind, data = event.get("event"), event.get("data") or {}
            conversation = data.get("conversation_id") or conversation
            if kind == "message":
                answer.append(str(data.get("answer") or data.get("content") or data.get("text") or ""))
            elif kind == "token_usage_updated" and isinstance(data.get("usage"), dict):
                usage.append(data["usage"])
            elif kind == "memory_update":
                memory_events.append({k: data.get(k) for k in ("action", "key", "mutation_status", "reason", "error_code")})
            elif kind in ("error", "message_error"):
                errors.append(data)
            elif kind == "user_input_requested":
                # Unnamed questions are answered as q1, q2, ... like the runtime expects.
                questions = data.get("questions") or []
                pending = {
                    "conversation_id": data.get("conversation_id") or conversation,
                    "message_id": data.get("message_id"),
                    "request_id": data.get("request_id"),
                    "question_ids": [str(q.get("id") or f"q{i + 1}") for i, q in enumerate(questions)] or ["q1"],
                }
        return {
            "query": query,
            "answer": "".join(answer),
            "conversation_id": conversation,
            "answered_question": bool(pending_input),
            "pending_input": pending,
            "latency_s": round(time.time() - started, 2),
            "prompt_tokens": sum(u.get("prompt_tokens", 0) for u in usage),
            "completion_tokens": sum(u.get("completion_tokens", 0) for u in usage),
            "memory_events": memory_events,
            "errors": errors,
            "event_kinds": sorted({e.get("event") for e in events if e.get("event")}),
        }

    def memory(self, user):
        data = self.external("GET", "/agents/memory?user=" + urllib.parse.quote(user))
        values = data.get("values") or []
        return {v.get("key"): str(v.get("content") or v.get("value") or "") for v in values if (v.get("content") or v.get("value"))}

    def archive(self, user):
        data = self.external("GET", "/agents/memory/archive?user=" + urllib.parse.quote(user)) or {}
        return [str(entry.get("content") or "") for entry in data.get("entries") or []]

    def enable_auto_capture(self, user):
        query = "?user=" + urllib.parse.quote(user)
        settings = self.external("GET", "/agents/memory/settings" + query) or {}
        revision = settings.get("settings_revision") or (settings.get("settings") or {}).get("settings_revision") or 1
        self.external("PATCH", "/agents/memory/settings" + query, {"auto_capture_enabled": True, "expected_revision": revision})


def setup_agent(client, workspace_id, label, auto):
    name = f"memory-eval-{label}-{datetime.datetime.now():%Y%m%d%H%M%S}"
    agent = client.console("POST", "/agents", {"name": name, "icon_type": "emoji", "icon": "🧠", "agent_type": "AGENT", "description": "Memory evaluation agent", "workspace_id": workspace_id})
    agent_id = agent["id"]
    slots = [dict(slot, enabled=True, sort_order=index) for index, slot in enumerate(SLOTS)]
    client.console("PUT", f"/agents/{agent_id}/memory/config", {"enabled": True, "auto_extraction_enabled": auto, "slots": slots})
    client.console("POST", f"/agents/{agent_id}/publish", {})
    key = client.console("POST", f"/agents/{agent_id}/api-keys", {"name": "memory-eval"})
    client.api_key = key.get("api_key") or key.get("key")
    config = client.console("GET", f"/agents/{agent_id}/config")
    return {"agent_id": agent_id, "name": name, "model_provider": config.get("model_provider"), "model": config.get("model")}


FILLER_SENTENCES = [
    "历史备注：这条记录来自较早的对话，没有特别事项。",
    "以前提过的一般性偏好，后来没有再提起。",
    "Older note from a previous conversation with no follow-up. ",
]


def expand_filler(text):
    """Replaces {{filler:N}} with about N characters of neutral note text."""
    def fill(match):
        size, parts, index = int(match.group(1)), [], 0
        while sum(len(part) for part in parts) < size:
            parts.append(FILLER_SENTENCES[index % len(FILLER_SENTENCES)])
            index += 1
        return "".join(parts)[:size]
    return re.sub(r"\{\{filler:(\d+)\}\}", fill, text)


def contains(text, needle):
    return needle.lower() in text.lower()


def score(scenario, memory, answer, other_memory, archive=None):
    memory_text = "\n".join(memory.values())
    expect, checks = scenario.get("expect", {}), []
    if expect.get("archive_nonempty"):
        checks.append(("archive has entries", bool(archive)))
    if expect.get("archive_any"):
        archive_text = "\n".join(archive or [])
        checks.append(("archive has any of " + "/".join(expect["archive_any"]), any(contains(archive_text, n) for n in expect["archive_any"])))
    for needle in expect.get("memory_all", []):
        checks.append(("memory has " + needle, contains(memory_text, needle)))
    if expect.get("memory_any"):
        checks.append(("memory has any of " + "/".join(expect["memory_any"]), any(contains(memory_text, n) for n in expect["memory_any"])))
    for key, needles in expect.get("memory_in", {}).items():
        for needle in needles:
            checks.append((f"{key} has {needle}", contains(memory.get(key, ""), needle)))
    for key, needles in expect.get("memory_in_any", {}).items():
        checks.append((f"{key} has any of " + "/".join(needles), any(contains(memory.get(key, ""), n) for n in needles)))
    for needle, limit in expect.get("memory_max_occurrences", {}).items():
        count = memory_text.lower().count(needle.lower())
        checks.append((f"memory mentions {needle} at most {limit} times ({count})", count <= limit))
    for needle in expect.get("memory_none", []):
        checks.append(("memory lacks " + needle, not contains(memory_text, needle)))
    for needle in expect.get("answer_all", []):
        checks.append(("answer has " + needle, contains(answer, needle)))
    if expect.get("answer_any"):
        checks.append(("answer has any of " + "/".join(expect["answer_any"]), any(contains(answer, n) for n in expect["answer_any"])))
    for needle in expect.get("answer_none", []):
        checks.append(("answer lacks " + needle, not contains(answer, needle)))
    if expect.get("answer_regex"):
        checks.append(("answer matches pattern", re.search(expect["answer_regex"], answer) is not None))
    if "answer_max_chars" in expect:
        checks.append((f"answer has at most {expect['answer_max_chars']} chars ({len(answer)})", len(answer) <= expect["answer_max_chars"]))
    if "answer_max_sentences" in expect:
        sentences = len([part for part in re.split(r"[。！？!?\n]+|(?<=[a-z])\.\s", answer) if part.strip()])
        checks.append((f"answer has at most {expect['answer_max_sentences']} sentences ({sentences})", sentences <= expect["answer_max_sentences"]))
    if "answer_max_cjk" in expect:
        cjk = len(re.findall(r"[一-鿿]", answer))
        checks.append((f"answer has at most {expect['answer_max_cjk']} CJK chars ({cjk})", cjk <= expect["answer_max_cjk"]))
    if expect.get("other_memory_empty"):
        checks.append(("other user's memory is empty", not other_memory))
    return checks


def run_scenario(client, scenario, run_index, wait_after_session, auto):
    user = f"eval-{scenario['id']}-{run_index}-{uuid.uuid4().hex[:8]}"
    if auto:
        client.enable_auto_capture(user)
    for key, content in scenario.get("preload", {}).items():
        client.external("PUT", f"/agents/memory/{urllib.parse.quote(key)}?user={urllib.parse.quote(user)}", {"content": expand_filler(content)})
    turns = []
    for session in scenario["sessions"]:
        conversation, pending = None, None
        for query in session:
            turn = client.chat(user, query, conversation, pending)
            conversation, pending = turn["conversation_id"], turn["pending_input"]
            turns.append(turn)
        if wait_after_session:
            time.sleep(wait_after_session)
    memory = client.memory(user)
    expect = scenario.get("expect", {})
    archive = client.archive(user) if expect.get("archive_nonempty") or expect.get("archive_any") or expect.get("archive_observe") else None
    probe_user = user + "-other" if scenario.get("probe_as_other_user") else user
    if probe_user != user and auto:
        client.enable_auto_capture(probe_user)
    probe = client.chat(probe_user, scenario["probe"])
    other_memory = client.memory(probe_user) if probe_user != user else None
    checks = score(scenario, memory, probe["answer"], other_memory, archive)
    return {
        "scenario": scenario["id"],
        "category": scenario.get("category"),
        "difficulty": scenario.get("difficulty", "basic"),
        "run": run_index,
        "requires_auto": bool(scenario.get("requires_auto")),
        "observe": bool(scenario.get("observe")),
        "memory": memory,
        "archive": archive,
        "turns": turns,
        "probe": probe,
        "checks": [{"check": name, "passed": passed} for name, passed in checks],
        "passed": all(passed for _, passed in checks),
    }


def summarize(results, auto):
    scored = [r for r in results if not r["observe"] and (auto or not r["requires_auto"])]
    by_category, by_difficulty = {}, {}
    for result in scored:
        for table, name in ((by_category, result["category"]), (by_difficulty, result.get("difficulty", "basic"))):
            bucket = table.setdefault(name, [0, 0])
            bucket[0] += 1 if result["passed"] else 0
            bucket[1] += 1
    turns = [t for r in results for t in r["turns"] + [r["probe"]]]
    return {
        "scenarios_scored": len(scored),
        "scenarios_passed": sum(1 for r in scored if r["passed"]),
        "by_difficulty": {k: {"passed": v[0], "total": v[1]} for k, v in sorted(by_difficulty.items())},
        "by_category": {k: {"passed": v[0], "total": v[1]} for k, v in sorted(by_category.items())},
        "checks_passed": sum(c["passed"] for r in scored for c in r["checks"]),
        "checks_total": sum(len(r["checks"]) for r in scored),
        "turns": len(turns),
        "avg_latency_s": round(sum(t["latency_s"] for t in turns) / max(len(turns), 1), 2),
        "avg_prompt_tokens": round(sum(t["prompt_tokens"] for t in turns) / max(len(turns), 1)),
        "avg_completion_tokens": round(sum(t["completion_tokens"] for t in turns) / max(len(turns), 1)),
        "memory_events": sum(len(t["memory_events"]) for t in turns),
        "turn_errors": sum(len(t["errors"]) for t in turns),
    }


def write_report(path, meta, summary, results):
    lines = [f"# Memory evaluation: {meta['label']}", "",
             f"- Started: {meta['started_at']}", f"- Agent: `{meta['agent']['name']}` ({meta['agent']['model_provider']}/{meta['agent']['model']})",
             f"- Automatic capture: {'on' if meta['auto'] else 'off'}; wait after session: {meta['wait_after_session']}s; repeats: {meta['repeat']}", "",
             f"**Scenarios passed: {summary['scenarios_passed']}/{summary['scenarios_scored']}** "
             f"(checks {summary['checks_passed']}/{summary['checks_total']}); "
             f"avg latency {summary['avg_latency_s']}s; avg tokens {summary['avg_prompt_tokens']} in / {summary['avg_completion_tokens']} out", "",
             "| Difficulty | Passed |", "|---|---|"]
    lines += [f"| {k} | {v['passed']}/{v['total']} |" for k, v in summary["by_difficulty"].items()]
    lines += ["", "| Category | Passed |", "|---|---|"]
    lines += [f"| {k} | {v['passed']}/{v['total']} |" for k, v in summary["by_category"].items()]
    lines += ["", "| Scenario | Run | Result | Failed checks |", "|---|---|---|---|"]
    for r in sorted(results, key=lambda x: (x["scenario"], x["run"])):
        tag = "observe" if r["observe"] else ("n/a (needs auto)" if r["requires_auto"] and not meta["auto"] else ("pass" if r["passed"] else "FAIL"))
        failed = "; ".join(c["check"] for c in r["checks"] if not c["passed"])
        lines.append(f"| {r['scenario']} | {r['run']} | {tag} | {failed} |")
    with open(path, "w", encoding="utf-8") as handle:
        handle.write("\n".join(lines) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--base-url", default="http://localhost:2679")
    parser.add_argument("--workspace-id", required=True)
    parser.add_argument("--label", required=True, help="Name of this configuration, used in output paths")
    parser.add_argument("--auto", action="store_true", help="Enable automatic capture on the Agent and each relationship")
    parser.add_argument("--wait-after-session", type=float, default=0, help="Seconds to wait after each session for background extraction")
    parser.add_argument("--repeat", type=int, default=1)
    parser.add_argument("--concurrency", type=int, default=4)
    parser.add_argument("--only", nargs="*", help="Run only these scenario IDs")
    parser.add_argument("--difficulty", choices=["basic", "hard"], help="Run only scenarios of this difficulty")
    parser.add_argument("--env-file")
    parser.add_argument("--out", default=os.path.join(HERE, "results"))
    args = parser.parse_args()

    env = load_env_file(args.env_file)
    email = os.environ.get("ZGI_E2E_EMAIL") or env.get("ZGI_E2E_EMAIL")
    password = os.environ.get("ZGI_E2E_PASSWORD") or env.get("ZGI_E2E_PASSWORD")
    if not email or not password:
        sys.exit("ZGI_E2E_EMAIL and ZGI_E2E_PASSWORD are required")

    with open(os.path.join(HERE, "scenarios.json"), encoding="utf-8") as handle:
        scenarios = json.load(handle)["scenarios"]
    if args.only:
        scenarios = [s for s in scenarios if s["id"] in set(args.only)]
    if args.difficulty:
        scenarios = [s for s in scenarios if s.get("difficulty", "basic") == args.difficulty]

    client = Client(args.base_url)
    client.login(email, password)
    started = datetime.datetime.now()
    agent = setup_agent(client, args.workspace_id, args.label, args.auto)
    print(f"agent {agent['name']} ({agent['agent_id']}), {len(scenarios)} scenarios x {args.repeat}", flush=True)

    results = []
    jobs = [(s, run) for run in range(1, args.repeat + 1) for s in scenarios]
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        futures = {pool.submit(run_scenario, client, s, run, args.wait_after_session, args.auto): (s["id"], run) for s, run in jobs}
        for future in concurrent.futures.as_completed(futures):
            scenario_id, run = futures[future]
            try:
                result = future.result()
            except Exception as err:  # keep the rest of the suite running
                result = {"scenario": scenario_id, "category": "error", "difficulty": "error", "run": run, "requires_auto": False, "observe": False,
                          "memory": {}, "turns": [], "probe": {"answer": "", "latency_s": 0, "prompt_tokens": 0, "completion_tokens": 0, "memory_events": [], "errors": [str(err)]},
                          "checks": [{"check": "scenario ran: " + str(err)[:200], "passed": False}], "passed": False}
            results.append(result)
            print(f"  {scenario_id} #{run}: {'pass' if result['passed'] else 'fail'}", flush=True)

    out_dir = os.path.join(args.out, f"{started:%Y%m%d-%H%M%S}-{args.label}")
    os.makedirs(out_dir, exist_ok=True)
    meta = {"label": args.label, "auto": args.auto, "wait_after_session": args.wait_after_session, "repeat": args.repeat,
            "started_at": started.isoformat(timespec="seconds"), "finished_at": datetime.datetime.now().isoformat(timespec="seconds"), "agent": agent}
    summary = summarize(results, args.auto)
    with open(os.path.join(out_dir, "results.json"), "w", encoding="utf-8") as handle:
        json.dump({"meta": meta, "summary": summary, "results": results}, handle, ensure_ascii=False, indent=2)
    write_report(os.path.join(out_dir, "report.md"), meta, summary, results)
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    print("written to", out_dir)


if __name__ == "__main__":
    main()
