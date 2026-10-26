"""Runs the DigitalHuman memory scenarios (ADR 0014) and scores them.

It drives a fresh DigitalHuman through its external API, one API end user per
scenario, so the same scenarios can be replayed against the current
DigitalHuman memory tools and against the file view of ADR 0014. Scoring and
reports are shared with run.py.

The DigitalHuman external API does not expose memory, so each scenario's
memory is read from the local database (through `docker exec` into the
Postgres container) and grouped by where it lives:

    user   the user's own relationship memory      (/user/)
    notes  the DigitalHuman's notes on this person  (/user/notes/, instance memory)
    me     the DigitalHuman's own memory            (/me/, shared by all users)

`me` is scored only by scenarios that ask for it, because it is shared.

Example:
    python dh.py --workspace-id <id> --label dh-baseline --db mb_eval --env-file ../../../.env.test.local
"""

import argparse
import concurrent.futures
import datetime
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid

import run

HERE = os.path.dirname(os.path.abspath(__file__))
DIGITAL_HUMAN_SUBJECT_NAMESPACE = "external-digital-human-memory"


def api_end_user_subject(workspace_id, digital_human_id, user):
    """Mirrors usersubject.DigitalHumanAPIEndUserSubject."""
    seed = "\x00".join(["zgi", DIGITAL_HUMAN_SUBJECT_NAMESPACE, workspace_id, digital_human_id, user.strip()])
    return str(uuid.uuid5(uuid.NAMESPACE_OID, seed))


class DigitalHumanClient:
    def __init__(self, client, workspace_id, db, container):
        self.client, self.workspace_id, self.db, self.container = client, workspace_id, db, container
        self.digital_human_id, self.api_key = None, None

    def console(self, method, path, body=None):
        # DigitalHuman routes answer 201 on create, which run.Client.console rejects.
        status, payload = self.client._request(method, "/console/api" + path, body, self.client.console_token)
        if not 200 <= status < 300 or not isinstance(payload, dict) or str(payload.get("code")) != "0":
            raise RuntimeError(f"{method} {path} failed: {status} {str(payload)[:300]}")
        return payload.get("data")

    def setup(self, label, model, auto, me_preload):
        name = f"memory-eval-dh-{label}-{datetime.datetime.now():%Y%m%d%H%M%S}"
        created = self.console("POST", "/digital-humans", {
            "name": name, "autonomy_level": "supervised", "visibility": "workspace", "default_model": model, "rules": [],
            "persona": "你是一家软件公司的客户助理，帮客户解答问题、跟进需求。回答简洁。",
        })
        definition, draft = created["definition"], created["draft"]
        self.digital_human_id = definition["id"]
        self.console("POST", f"/digital-humans/{self.digital_human_id}/publish",
                     {"expected_revision": definition["revision"], "expected_draft_lock_version": draft["lock_version"]})
        self.console("PATCH", f"/digital-humans/{self.digital_human_id}/access", {"api_enabled": True, "webapp_enabled": False})
        self.api_key = self.console("POST", f"/digital-humans/{self.digital_human_id}/api-keys", {"name": "memory-eval"})["api_key"]
        state = self.memory_state()
        if auto and not state["auto_extraction_enabled"]:
            self.console("PATCH", f"/digital-humans/{self.digital_human_id}/memory/state",
                         {"expected_memory_epoch": state["memory_epoch"], "auto_extraction_enabled": True})
        for item in me_preload:
            self.propose_self(item["key"], item["content"], item.get("memory_type", "fact"), item.get("visibility", "everyone"))
        return {"digital_human_id": self.digital_human_id, "name": name, "model": model}

    def memory_state(self):
        return self.console("GET", f"/digital-humans/{self.digital_human_id}/memory?page=1&page_size=1")["state"]

    def propose_self(self, key, content, memory_type, visibility):
        state = self.memory_state()
        self.console("POST", f"/digital-humans/{self.digital_human_id}/memory/proposals", {
            "operation_id": str(uuid.uuid4()), "key": key, "action": "upsert", "memory_type": memory_type,
            "visibility": visibility,
            "content": content, "expected_revision": 0, "config_revision": state["config_revision"], "memory_epoch": state["memory_epoch"],
            "evidence": [{"type": "manual", "id": f"operator:{uuid.uuid4()}"}],
            "source_context": {"private_conversation": False, "promoted_by_authorized_actor": True},
            "admin_approved": True, "occurred_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        })

    def chat(self, user, query, conversation_id=None):
        body = {"request_id": str(uuid.uuid4()), "query": query, "user": user, "response_mode": "streaming"}
        if conversation_id:
            body["conversation_id"] = conversation_id
        req = urllib.request.Request(self.client.base + "/api/v1/digital-humans/chat", data=json.dumps(body).encode(), method="POST")
        req.add_header("Content-Type", "application/json")
        req.add_header("Authorization", "Bearer " + self.api_key)
        started, answer, events, errors, tools, usage, memory_events = time.time(), [], [], [], [], [], []
        conversation, status = conversation_id, ""
        try:
            with urllib.request.urlopen(req, timeout=300) as resp:
                for raw in resp:
                    line = raw.decode("utf-8", "replace").strip()
                    if not line.startswith("data:"):
                        continue
                    try:
                        envelope = json.loads(line[5:])
                    except ValueError:
                        continue
                    kind, data = envelope.get("event"), envelope.get("data") or {}
                    events.append(kind)
                    conversation = data.get("conversation_id") or conversation
                    if kind == "message":
                        answer.append(str(data.get("answer") or ""))
                    elif kind == "error":
                        errors.append(data)
                    elif kind and kind.startswith("memory_"):
                        memory_events.append({"event": kind, "key": data.get("key")})
                    elif kind == "token_usage_updated" and isinstance(data.get("usage"), dict):
                        usage.append(data["usage"])
                    elif kind == "run_end":
                        status = data.get("status") or status
                    elif kind and kind.startswith("skill_") and data.get("tool_name"):
                        tools.append(data.get("tool_name"))
        except urllib.error.HTTPError as err:
            with err:
                detail = err.read().decode("utf-8", "replace")[:300]
            raise RuntimeError(f"POST /digital-humans/chat failed: {err.code} {detail}") from err
        return {"query": query, "answer": "".join(answer), "conversation_id": conversation, "status": status,
                "latency_s": round(time.time() - started, 2), "errors": errors, "tools": sorted(set(tools)),
                "prompt_tokens": sum(u.get("prompt_tokens", 0) for u in usage),
                "completion_tokens": sum(u.get("completion_tokens", 0) for u in usage), "memory_events": memory_events,
                "event_kinds": sorted({e for e in events if e})}

    def sql(self, query):
        out = subprocess.run(["docker", "exec", self.container, "psql", "-U", "postgres", "-d", self.db, "-AtF", "\t", "-c", query],
                             capture_output=True, text=True, encoding="utf-8", check=True).stdout
        return [line.split("\t") for line in out.splitlines() if line]

    def memory(self, user):
        subject = api_end_user_subject(self.workspace_id, self.digital_human_id, user)
        dh = self.digital_human_id
        rows = self.sql(f"""
            SELECT CASE WHEN i.memory_key LIKE 'notes/%' THEN 'notes' ELSE 'user' END, i.memory_key, replace(i.content_payload, E'\\n', ' ')
              FROM user_memory_items i
              JOIN user_memory_relationships r ON r.id = i.relationship_id
             WHERE r.consumer_type = 'digital_human' AND r.consumer_id = '{dh}' AND r.subject_id = '{subject}'
            UNION ALL
            SELECT 'archive', a.source_key, replace(a.content_payload, E'\\n', ' ') FROM user_memory_archive_entries a
              JOIN user_memory_relationships r ON r.id = a.relationship_id
             WHERE r.consumer_type = 'digital_human' AND r.consumer_id = '{dh}' AND r.subject_id = '{subject}'
            UNION ALL
            SELECT 'me', m.memory_key, replace(m.content, E'\\n', ' ') FROM digital_human_memories m
             WHERE m.digital_human_id = '{dh}' AND m.status = 'active'""")
        grouped = {"user": [], "notes": [], "archive": [], "me": []}
        for partition, key, content in rows:
            grouped[partition].append(f"{key}: {content}")
        return {partition: "\n".join(lines) for partition, lines in grouped.items() if lines}


def run_scenario(dh, scenario, run_index, wait_after_session):
    user = f"eval-{scenario['id']}-{run_index}-{uuid.uuid4().hex[:8]}"
    # A scenario may need a longer wait than the run's, such as one that
    # waits for a note to expire and the archive sweep to move it.
    wait_after_session = max(wait_after_session or 0, scenario.get("min_wait_after_session", 0))
    turns = []
    for session in scenario["sessions"]:
        conversation = None
        for query in session:
            turn = dh.chat(user, query, conversation)
            conversation = turn["conversation_id"]
            turns.append(turn)
        if wait_after_session:
            time.sleep(wait_after_session)
    full_memory = dh.memory(user)
    scored_memory = {k: v for k, v in full_memory.items() if k != "me" or scenario.get("score_me")}
    if scenario.get("score_partitions"):
        scored_memory = {k: v for k, v in full_memory.items() if k in scenario["score_partitions"]}
    probe_user = user + "-other" if scenario.get("probe_as_other_user") else user
    probe = dh.chat(probe_user, scenario["probe"])
    other_memory = None
    if probe_user != user:
        other_memory = {k: v for k, v in dh.memory(probe_user).items() if k != "me"}
    checks = run.score(scenario, scored_memory, probe["answer"], other_memory)
    return {
        "scenario": scenario["id"], "category": scenario.get("category"), "difficulty": scenario.get("difficulty", "basic"),
        "run": run_index, "requires_auto": bool(scenario.get("requires_auto")), "observe": bool(scenario.get("observe")),
        "memory": full_memory, "turns": turns, "probe": probe,
        "checks": [{"check": name, "passed": passed} for name, passed in checks],
        "passed": all(passed for _, passed in checks),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--base-url", default="http://localhost:2679")
    parser.add_argument("--workspace-id", required=True)
    parser.add_argument("--label", required=True)
    parser.add_argument("--model", default="deepseek-v4-flash")
    parser.add_argument("--auto", action="store_true", help="Turn on the DigitalHuman's automatic memory extraction")
    parser.add_argument("--wait-after-session", type=float, default=0)
    parser.add_argument("--repeat", type=int, default=1)
    parser.add_argument("--concurrency", type=int, default=4)
    parser.add_argument("--only", nargs="*")
    parser.add_argument("--note-ttl", action="store_true", help="Include scenarios that need a deployment with a short note TTL")
    parser.add_argument("--db", default="zgi", help="Database the deployment uses, for reading memory")
    parser.add_argument("--container", default="zgi-postgres-1")
    parser.add_argument("--env-file")
    parser.add_argument("--out", default=os.path.join(HERE, "results"))
    args = parser.parse_args()

    env = run.load_env_file(args.env_file)
    email = os.environ.get("ZGI_E2E_EMAIL") or env.get("ZGI_E2E_EMAIL")
    password = os.environ.get("ZGI_E2E_PASSWORD") or env.get("ZGI_E2E_PASSWORD")
    if not email or not password:
        sys.exit("ZGI_E2E_EMAIL and ZGI_E2E_PASSWORD are required")
    with open(os.path.join(HERE, "dh_scenarios.json"), encoding="utf-8") as handle:
        suite = json.load(handle)
    scenarios = suite["scenarios"]
    if not args.note_ttl:
        scenarios = [s for s in scenarios if not s.get("requires_note_ttl")]
    if args.only:
        scenarios = [s for s in scenarios if s["id"] in set(args.only)]

    client = run.Client(args.base_url)
    client.login(email, password)
    dh = DigitalHumanClient(client, args.workspace_id, args.db, args.container)
    started = datetime.datetime.now()
    meta_dh = dh.setup(args.label, args.model, args.auto, suite.get("me_preload", []))
    print(f"digital human {meta_dh['name']} ({meta_dh['digital_human_id']}), {len(scenarios)} scenarios x {args.repeat}", flush=True)

    results = []
    jobs = [(s, r) for r in range(1, args.repeat + 1) for s in scenarios]
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        futures = {pool.submit(run_scenario, dh, s, r, args.wait_after_session): (s["id"], r) for s, r in jobs}
        for future in concurrent.futures.as_completed(futures):
            scenario_id, r = futures[future]
            try:
                result = future.result()
            except Exception as err:  # keep the rest of the suite running
                result = {"scenario": scenario_id, "category": "error", "difficulty": "error", "run": r, "requires_auto": False, "observe": False,
                          "memory": {}, "turns": [], "probe": {"answer": "", "latency_s": 0, "prompt_tokens": 0, "completion_tokens": 0, "memory_events": [], "errors": [str(err)]},
                          "checks": [{"check": "scenario ran: " + str(err)[:200], "passed": False}], "passed": False}
            results.append(result)
            print(f"  {scenario_id} #{r}: {'pass' if result['passed'] else 'fail'}", flush=True)

    out_dir = os.path.join(args.out, f"{started:%Y%m%d-%H%M%S}-{args.label}")
    os.makedirs(out_dir, exist_ok=True)
    meta = {"label": args.label, "auto": args.auto, "wait_after_session": args.wait_after_session, "repeat": args.repeat,
            "started_at": started.isoformat(timespec="seconds"), "finished_at": datetime.datetime.now().isoformat(timespec="seconds"),
            "agent": {"name": meta_dh["name"], "agent_id": meta_dh["digital_human_id"], "model_provider": "", "model": meta_dh["model"]}}
    # Keep the raw results even if summarizing fails.
    with open(os.path.join(out_dir, "results.json"), "w", encoding="utf-8") as handle:
        json.dump({"meta": meta, "results": results}, handle, ensure_ascii=False, indent=2)
    summary = run.summarize(results, args.auto)
    with open(os.path.join(out_dir, "results.json"), "w", encoding="utf-8") as handle:
        json.dump({"meta": meta, "summary": summary, "results": results}, handle, ensure_ascii=False, indent=2)
    run.write_report(os.path.join(out_dir, "report.md"), meta, summary, results)
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    print("written to", out_dir)


if __name__ == "__main__":
    main()
