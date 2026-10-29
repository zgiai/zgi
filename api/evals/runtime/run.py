"""Runs the Agent runtime scenario suite against a ZGI deployment and scores it.

The suite drives an Agent through its external API, one API end user per
scenario, and scores tool choice, multi-step tool use, questions back to the
user, multi-turn context, long tool arguments, and honesty about actions it
could not take. Each run also records model calls, tool calls, tokens and
latency, and, with --container, the share of input read from the provider
cache, so runs before and after a runtime change, or on different models, can
be compared. It uses only the Python standard library.

Credentials come from the environment (ZGI_E2E_EMAIL, ZGI_E2E_PASSWORD), or
from an env file given with --env-file; they are never printed.

Example:
    python run.py --workspace-id <id> --label baseline --repeat 3 --env-file ../../../.env.test.local
    python run.py --workspace-id <id> --label change --repeat 3 --baseline results/<dir>/results.json
"""

import argparse
import concurrent.futures
import datetime
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

HERE = os.path.dirname(os.path.abspath(__file__))

SKILLS = ["calculator", "time", "file-generator"]

SYSTEM_PROMPT = "You are a helpful assistant for a small business. Answer accurately and concisely."

# Names of runtime control functions and business tools that must never reach
# the user's screen.
INTERNAL_NAMES = [
    "activate_skills", "search_skills", "read_skill_reference", "request_user_input", "submit_turn_state",
    "update_plan", "skill_id", "evaluate_expression", "generate_file", "call_skill_tool", "load_skill",
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

    def login(self, email, password):
        data = self.console("POST", "/login", {"email": email, "password": password})
        self.console_token = data.get("access_token") or data["data"]["access_token"]

    def chat(self, user, query, conversation_id=None, pending_input=None):
        """Sends one turn and returns what the runtime reported about it.

        When the previous turn ended with the Agent asking the user a question,
        the conversation accepts only an answer to it, so the query is sent as
        the answer to every pending question instead.
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
            with urllib.request.urlopen(req, timeout=600) as resp:
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
        turn = parse_turn(events, conversation_id)
        turn.update({"query": query, "answered_question": bool(pending_input), "latency_s": round(time.time() - started, 2)})
        return turn


def parse_turn(events, conversation_id=None):
    """Collects the answer and runtime facts of one turn from its stream events."""
    streamed, usage, errors, tool_errors, pending, end = [], [], [], [], None, {}
    conversation, retracts, content_chars = conversation_id, 0, []
    for event in events:
        kind, data = event.get("event"), event.get("data") or {}
        conversation = data.get("conversation_id") or conversation
        if kind == "message":
            streamed.append(str(data.get("answer") or data.get("content") or data.get("text") or ""))
        elif kind == "message_retract":
            retracts += 1
            text = "".join(streamed)
            removed = str(data.get("content") or "")
            if removed and text.endswith(removed):
                streamed = [text[: -len(removed)]]
        elif kind == "token_usage_updated" and isinstance(data.get("usage"), dict):
            usage.append(data["usage"])
        elif kind in ("error", "message_error"):
            errors.append({"event": kind, "detail": str(data.get("message") or data.get("error") or data)[:300]})
        elif kind == "skill_call_error":
            # A failed tool call is sent back to the model, which may recover;
            # it is counted, not scored.
            tool_errors.append(str(data.get("error") or data.get("message") or data)[:300])
        elif kind == "skill_call_start":
            arguments = data.get("arguments") or data.get("arguments_summary") or {}
            if isinstance(arguments, dict) and isinstance(arguments.get("content_chars"), int):
                content_chars.append(arguments["content_chars"])
        elif kind == "user_input_requested":
            # Unnamed questions are answered as q1, q2, ... like the runtime expects.
            questions = data.get("questions") or []
            pending = {
                "conversation_id": data.get("conversation_id") or conversation,
                "message_id": data.get("message_id"),
                "request_id": data.get("request_id"),
                "question_ids": [str(q.get("id") or f"q{i + 1}") for i, q in enumerate(questions)] or ["q1"],
            }
        elif kind == "message_end":
            end = data
    metadata = end.get("metadata") or {}
    diagnostics = metadata.get("native_skill_diagnostics") or {}
    items = (metadata.get("presentation") or {}).get("items") or []
    texts = [item for item in items if item.get("kind") == "text" and str(item.get("content") or "").strip()]
    final = [str(item["content"]) for item in texts if item.get("content_phase") == "final"]
    answer = "\n".join(final) if final else "".join(streamed)
    visible = "\n".join([str(item["content"]) for item in texts] + [answer])
    return {
        "answer": answer,
        "visible_text": visible,
        "conversation_id": conversation,
        "pending_input": pending,
        "status": str(end.get("status") or ""),
        "errors": errors,
        "tool_errors": tool_errors,
        "retracts": retracts,
        "model_calls": int(metadata.get("model_invocation_count") or 0),
        "tool_calls": int(metadata.get("tool_call_count") or 0),
        "tool_names": sorted(set(metadata.get("tool_names") or [])),
        "activated_skills": sorted(set(diagnostics.get("activated_skill_ids") or [])),
        "files": int(metadata.get("generated_file_count") or 0),
        "tool_content_chars": max(content_chars) if content_chars else 0,
        "prompt_tokens": sum(u.get("prompt_tokens", 0) for u in usage),
        "completion_tokens": sum(u.get("completion_tokens", 0) for u in usage),
        "event_kinds": sorted({e.get("event") for e in events if e.get("event")}),
    }


def setup_agent(client, workspace_id, label, model_provider, model):
    name = f"runtime-eval-{label}-{datetime.datetime.now():%Y%m%d%H%M%S}"
    agent = client.console("POST", "/agents", {"name": name, "icon_type": "emoji", "icon": "🧪", "agent_type": "AGENT", "description": "Runtime evaluation agent", "workspace_id": workspace_id})
    agent_id = agent["id"]
    config = client.console("GET", f"/agents/{agent_id}/config")
    config["enabled_skill_ids"] = SKILLS
    config["system_prompt"] = SYSTEM_PROMPT
    if model:
        config["model"] = model
        config["model_provider"] = model_provider or config.get("model_provider")
    client.console("PUT", f"/agents/{agent_id}/config", config)
    client.console("POST", f"/agents/{agent_id}/publish", {})
    key = client.console("POST", f"/agents/{agent_id}/api-keys", {"name": "runtime-eval"})
    client.api_key = key.get("api_key") or key.get("key")
    config = client.console("GET", f"/agents/{agent_id}/config")
    return {"agent_id": agent_id, "name": name, "model_provider": config.get("model_provider"), "model": config.get("model")}


def contains(text, needle):
    return needle.lower() in text.lower()


def numbers_in(text):
    """Returns the numbers written in text, with thousands separators removed."""
    joined = re.sub(r"(?<=\d)[,，](?=\d{3}(?!\d))", "", text)
    return [float(value) for value in re.findall(r"-?\d+(?:\.\d+)?", joined)]


def cjk_share(text):
    cjk = len(re.findall(r"[一-鿿]", text))
    latin = len(re.findall(r"[A-Za-z]", text))
    return cjk / max(cjk + latin, 1), cjk


def asked_user(turn):
    return bool(turn["pending_input"]) or bool(re.search(r"[?？]", turn["answer"]))


def default_checks(turn, lang):
    checks = [
        ("completed", turn["status"] in ("completed", "waiting_question", "waiting_user_input") or bool(turn["pending_input"])),
        ("no errors", not turn["errors"]),
        ("answer is not empty", bool(turn["answer"].strip()) or bool(turn["pending_input"])),
    ]
    leaked = [name for name in INTERNAL_NAMES if name in turn["visible_text"]]
    checks.append(("no internal names" + (" (" + ", ".join(leaked) + ")" if leaked else ""), not leaked))
    share, cjk = cjk_share(turn["visible_text"])
    if lang == "zh" and turn["answer"].strip():
        checks.append((f"answers in Chinese ({share:.0%} CJK)", share >= 0.3))
    if lang == "en":
        checks.append((f"answers in English ({cjk} CJK chars)", cjk <= 2))
    return checks


def expectation_checks(expect, turn, conversation):
    """Scores one set of expectations. turn is the scored turn; conversation
    holds facts collected over every turn up to it."""
    answer, checks = turn["answer"], []
    for needle in expect.get("answer_all", []):
        checks.append(("answer has " + needle, contains(answer, needle)))
    if expect.get("answer_any"):
        checks.append(("answer has any of " + "/".join(expect["answer_any"]), any(contains(answer, n) for n in expect["answer_any"])))
    for needle in expect.get("answer_none", []):
        checks.append(("answer lacks " + needle, not contains(answer, needle)))
    if expect.get("answer_regex"):
        checks.append(("answer matches pattern", re.search(expect["answer_regex"], answer) is not None))
    if expect.get("answer_values"):
        tolerance = expect.get("tolerance", 0.005)
        found = numbers_in(answer)
        for value in expect["answer_values"]:
            checks.append((f"answer has {value:g}", any(abs(number - value) <= tolerance for number in found)))
    if expect.get("asked_user"):
        checks.append(("asked the user for missing details", asked_user(turn)))
    if expect.get("skills_none"):
        used = conversation["activated_skills"] + conversation["tool_names"]
        checks.append(("used no skill" + (" (" + ", ".join(used) + ")" if used else ""), not used))
    if expect.get("skills_any"):
        checks.append(("activated any of " + "/".join(expect["skills_any"]), any(s in conversation["activated_skills"] for s in expect["skills_any"])))
    if expect.get("tools_any"):
        checks.append(("called any of " + "/".join(expect["tools_any"]), any(t in conversation["tool_names"] for t in expect["tools_any"])))
    if "files_min" in expect:
        checks.append((f"generated at least {expect['files_min']} file(s) ({conversation['files']})", conversation["files"] >= expect["files_min"]))
    if "tool_content_min_chars" in expect:
        chars = conversation["tool_content_chars"]
        checks.append((f"tool content has at least {expect['tool_content_min_chars']} chars ({chars})", chars >= expect["tool_content_min_chars"]))
    if "model_calls_max" in expect:
        calls = conversation["model_calls"]
        checks.append((f"at most {expect['model_calls_max']} model call(s) ({calls})", 0 < calls <= expect["model_calls_max"]))
    return checks


def conversation_facts(turns):
    return {
        "activated_skills": sorted({s for t in turns for s in t["activated_skills"]}),
        "tool_names": sorted({n for t in turns for n in t["tool_names"]}),
        "files": sum(t["files"] for t in turns),
        "tool_content_chars": max([t["tool_content_chars"] for t in turns] or [0]),
        "model_calls": sum(t["model_calls"] for t in turns),
    }


def score(scenario, turns):
    lang, checks = scenario.get("lang"), []
    for index, turn in enumerate(turns):
        prefix = f"turn {index + 1}: " if len(turns) > 1 else ""
        checks += [(prefix + name, passed) for name, passed in default_checks(turn, lang)]
        expect = scenario.get("turn_expect", {}).get(str(index))
        if expect:
            facts = conversation_facts(turns[index:index + 1])
            checks += [(prefix + name, passed) for name, passed in expectation_checks(expect, turn, facts)]
    checks += expectation_checks(scenario.get("expect", {}), turns[-1], conversation_facts(turns))
    return checks


def run_scenario(client, scenario, run_index):
    user = f"runtime-{scenario['id']}-{run_index}-{uuid.uuid4().hex[:8]}"
    turns, conversation, pending = [], None, None
    for query in scenario["turns"]:
        turn = client.chat(user, query, conversation, pending)
        conversation, pending = turn["conversation_id"], turn["pending_input"]
        turns.append(turn)
    checks = score(scenario, turns)
    return {
        "scenario": scenario["id"],
        "category": scenario.get("category"),
        "run": run_index,
        "observe": bool(scenario.get("observe")),
        "user": user,
        "turns": turns,
        "checks": [{"check": name, "passed": passed} for name, passed in checks],
        "passed": all(passed for _, passed in checks),
    }


def provider_cache(container, db, agent_id):
    """Reads the provider-reported input and cache reads of the Agent's model calls from the deployment's database."""
    query = ("select count(*), coalesce(sum(prompt_tokens), 0), coalesce(sum(cache_read_tokens), 0) "
             f"from llm_usage_bills where app_id = '{agent_id}' and status = 'success'")
    out = subprocess.run(["docker", "exec", container, "psql", "-U", "postgres", "-d", db, "-AtF", "\t", "-c", query],
                         capture_output=True, text=True, check=True).stdout.strip()
    calls, uncached, cached = (int(value) for value in out.split("\t"))
    return {"model_calls": calls, "uncached_input_tokens": uncached, "cache_read_tokens": cached,
            "cache_read_share": round(cached / max(cached + uncached, 1), 3)}


def summarize(results):
    scored = [r for r in results if not r["observe"]]
    by_category = {}
    for result in scored:
        bucket = by_category.setdefault(result["category"], [0, 0])
        bucket[0] += 1 if result["passed"] else 0
        bucket[1] += 1
    turns = [t for r in results for t in r["turns"]]
    count = max(len(turns), 1)
    return {
        "scenarios_scored": len(scored),
        "scenarios_passed": sum(1 for r in scored if r["passed"]),
        "by_category": {k: {"passed": v[0], "total": v[1]} for k, v in sorted(by_category.items())},
        "checks_passed": sum(c["passed"] for r in scored for c in r["checks"]),
        "checks_total": sum(len(r["checks"]) for r in scored),
        "turns": len(turns),
        "avg_latency_s": round(sum(t["latency_s"] for t in turns) / count, 2),
        "avg_model_calls": round(sum(t["model_calls"] for t in turns) / count, 2),
        "avg_tool_calls": round(sum(t["tool_calls"] for t in turns) / count, 2),
        "tool_errors": sum(len(t.get("tool_errors", [])) for t in turns),
        "avg_prompt_tokens": round(sum(t["prompt_tokens"] for t in turns) / count),
        "avg_completion_tokens": round(sum(t["completion_tokens"] for t in turns) / count),
        "turn_errors": sum(len(t["errors"]) for t in turns),
    }


def pass_rates(results):
    rates = {}
    for result in results:
        bucket = rates.setdefault(result["scenario"], [0, 0])
        bucket[0] += 1 if result["passed"] else 0
        bucket[1] += 1
    return rates


def compare(baseline, summary, results):
    """Lists what changed against a baseline results.json."""
    lines = ["", "## Against the baseline", "", f"Baseline: `{baseline['meta']['label']}` ({baseline['meta']['agent']['model']}, {baseline['meta']['started_at']})", "",
             "| Metric | Baseline | This run |", "|---|---|---|"]
    before = baseline["summary"]
    for key in ("scenarios_passed", "checks_passed", "avg_latency_s", "avg_model_calls", "avg_tool_calls", "tool_errors", "avg_prompt_tokens", "avg_completion_tokens", "turn_errors"):
        lines.append(f"| {key} | {before.get(key)} | {summary.get(key)} |")
    if "provider_cache" in before and "provider_cache" in summary:
        lines.append(f"| cache_read_share | {before['provider_cache']['cache_read_share']} | {summary['provider_cache']['cache_read_share']} |")
    old, new = pass_rates(baseline["results"]), pass_rates(results)
    changed = [(s, old.get(s), new[s]) for s in sorted(new) if s in old and old[s][0] / old[s][1] != new[s][0] / new[s][1]]
    if changed:
        lines += ["", "| Scenario | Baseline | This run |", "|---|---|---|"]
        lines += [f"| {s} | {a[0]}/{a[1]} | {b[0]}/{b[1]} |" for s, a, b in changed]
    else:
        lines += ["", "No scenario changed its pass rate."]
    return lines


def write_report(path, meta, summary, results, baseline=None):
    lines = [f"# Runtime evaluation: {meta['label']}", "",
             f"- Started: {meta['started_at']}", f"- Agent: `{meta['agent']['name']}` ({meta['agent']['model_provider']}/{meta['agent']['model']})",
             f"- Skills: {', '.join(SKILLS)}; repeats: {meta['repeat']}", "",
             f"**Scenarios passed: {summary['scenarios_passed']}/{summary['scenarios_scored']}** "
             f"(checks {summary['checks_passed']}/{summary['checks_total']}); avg latency {summary['avg_latency_s']}s; "
             f"avg {summary['avg_model_calls']} model calls and {summary['avg_tool_calls']} tool calls per turn, {summary['tool_errors']} failed tool calls; "
             f"avg tokens {summary['avg_prompt_tokens']} in / {summary['avg_completion_tokens']} out"]
    cache = summary.get("provider_cache")
    if cache:
        lines.append(f"\nProvider cache: {cache['cache_read_tokens']} of {cache['cache_read_tokens'] + cache['uncached_input_tokens']} input tokens "
                     f"({cache['cache_read_share']:.0%}) over {cache['model_calls']} model calls.")
    lines += ["", "| Category | Passed |", "|---|---|"]
    lines += [f"| {k} | {v['passed']}/{v['total']} |" for k, v in summary["by_category"].items()]
    lines += ["", "| Scenario | Run | Result | Model calls | Failed checks | Failed tool calls |", "|---|---|---|---|---|---|"]
    for r in sorted(results, key=lambda x: (x["scenario"], x["run"])):
        tag = "observe" if r["observe"] else ("pass" if r["passed"] else "FAIL")
        failed = "; ".join(c["check"] for c in r["checks"] if not c["passed"])
        calls = "+".join(str(t["model_calls"]) for t in r["turns"])
        tool_errors = "; ".join(e.replace("|", "/") for t in r["turns"] for e in t.get("tool_errors", []))
        lines.append(f"| {r['scenario']} | {r['run']} | {tag} | {calls} | {failed} | {tool_errors} |")
    if baseline:
        lines += compare(baseline, summary, results)
    with open(path, "w", encoding="utf-8") as handle:
        handle.write("\n".join(lines) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--base-url", default="http://localhost:2679")
    parser.add_argument("--workspace-id", required=True)
    parser.add_argument("--label", required=True, help="Name of this configuration, used in output paths")
    parser.add_argument("--model", help="Model for the Agent; the workspace default when omitted")
    parser.add_argument("--model-provider", help="Provider of --model")
    parser.add_argument("--repeat", type=int, default=1)
    parser.add_argument("--concurrency", type=int, default=4)
    parser.add_argument("--only", nargs="*", help="Run only these scenario IDs")
    parser.add_argument("--category", help="Run only scenarios of this category")
    parser.add_argument("--baseline", help="A results.json from an earlier run to compare against")
    parser.add_argument("--container", help="Postgres container of the deployment, to read provider cache use")
    parser.add_argument("--db", default="zgi")
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
    if args.category:
        scenarios = [s for s in scenarios if s.get("category") == args.category]
    baseline = None
    if args.baseline:
        with open(args.baseline, encoding="utf-8") as handle:
            baseline = json.load(handle)

    client = Client(args.base_url)
    client.login(email, password)
    started = datetime.datetime.now()
    agent = setup_agent(client, args.workspace_id, args.label, args.model_provider, args.model)
    print(f"agent {agent['name']} ({agent['agent_id']}, {agent['model']}), {len(scenarios)} scenarios x {args.repeat}", flush=True)

    results = []
    jobs = [(s, run) for run in range(1, args.repeat + 1) for s in scenarios]
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        futures = {pool.submit(run_scenario, client, s, run): (s, run) for s, run in jobs}
        for future in concurrent.futures.as_completed(futures):
            scenario, run = futures[future]
            try:
                result = future.result()
            except Exception as err:  # keep the rest of the suite running
                result = {"scenario": scenario["id"], "category": scenario.get("category"), "run": run, "observe": bool(scenario.get("observe")),
                          "user": "", "turns": [], "checks": [{"check": "scenario ran: " + str(err)[:200], "passed": False}], "passed": False}
            results.append(result)
            failed = [c["check"] for c in result["checks"] if not c["passed"]]
            print(f"  {scenario['id']} #{run}: {'pass' if result['passed'] else 'fail: ' + '; '.join(failed)}", flush=True)

    summary = summarize(results)
    if args.container:
        summary["provider_cache"] = provider_cache(args.container, args.db, agent["agent_id"])
    out_dir = os.path.join(args.out, f"{started:%Y%m%d-%H%M%S}-{args.label}")
    os.makedirs(out_dir, exist_ok=True)
    meta = {"label": args.label, "repeat": args.repeat, "skills": SKILLS, "started_at": started.isoformat(timespec="seconds"),
            "finished_at": datetime.datetime.now().isoformat(timespec="seconds"), "agent": agent}
    with open(os.path.join(out_dir, "results.json"), "w", encoding="utf-8") as handle:
        json.dump({"meta": meta, "summary": summary, "results": results}, handle, ensure_ascii=False, indent=2)
    write_report(os.path.join(out_dir, "report.md"), meta, summary, results, baseline)
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    print("written to", out_dir)


if __name__ == "__main__":
    main()
