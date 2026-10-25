"""Seeds Agent memory in the shapes a migration must handle, through the public APIs.

Run it once per memory runtime (relationship store on or off) before taking a
baseline snapshot. It prints a manifest of what it created; the run writes the
same manifest to --out so a migration check can find every shape again.

Example:
    python seed.py --workspace-id <id> --label relationship --env-file ../../../.env.test.local
"""

import argparse
import datetime
import json
import os
import sys
import urllib.parse
import uuid

from run import Client, load_env_file

LONG_KEY = "k" + "_long" * 12 + "_ky"  # 64 characters, the key limit
SLOTS = [
    {"key": "profile", "name": "Profile", "description": "Stable facts about the user.", "max_chars": 2000, "enabled": True},
    {"key": "preferences", "name": "Preferences", "description": "How the user likes answers.", "max_chars": 2000, "enabled": True},
    {"key": "habits", "name": "习惯", "description": "用户的日常习惯。", "max_chars": 500, "enabled": True},
    {"key": LONG_KEY, "name": "Long key", "description": "A slot whose key uses the full 64 characters.", "max_chars": 1000, "enabled": True},
    {"key": "archived_notes", "name": "Archived notes", "description": "A disabled slot that keeps its values.", "max_chars": 1000, "enabled": False},
]


def put(client, user, key, content):
    path = f"/agents/memory/{urllib.parse.quote(key)}?user={urllib.parse.quote(user)}"
    return client.external("PUT", path, {"content": content})


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--base-url", default="http://localhost:2679")
    parser.add_argument("--workspace-id", required=True)
    parser.add_argument("--label", required=True)
    parser.add_argument("--env-file")
    parser.add_argument("--out", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "results"))
    args = parser.parse_args()

    env = load_env_file(args.env_file)
    email = os.environ.get("ZGI_E2E_EMAIL") or env.get("ZGI_E2E_EMAIL")
    password = os.environ.get("ZGI_E2E_PASSWORD") or env.get("ZGI_E2E_PASSWORD")
    if not email or not password:
        sys.exit("ZGI_E2E_EMAIL and ZGI_E2E_PASSWORD are required")

    client = Client(args.base_url)
    client.login(email, password)
    stamp = f"{datetime.datetime.now():%Y%m%d%H%M%S}"
    manifest = {"label": args.label, "created_at": stamp, "agents": [], "users": []}

    # An Agent with five slots: a CJK name, a 64-character key, a 500-character limit, and a disabled slot.
    agent = client.console("POST", "/agents", {"name": f"memory-seed-{args.label}-{stamp}", "icon_type": "emoji", "icon": "🧪",
                                               "agent_type": "AGENT", "description": "Memory migration shapes", "workspace_id": args.workspace_id})
    agent_id = agent["id"]
    slots = [dict(slot, sort_order=index) for index, slot in enumerate(SLOTS)]
    # Values can only be written to enabled slots, so the disabled slot starts enabled and is disabled after seeding.
    seeding_slots = [dict(slot, enabled=True) for slot in slots]
    client.console("PUT", f"/agents/{agent_id}/memory/config", {"enabled": True, "auto_extraction_enabled": True, "slots": seeding_slots})
    client.console("POST", f"/agents/{agent_id}/publish", {})
    client.api_key = client.console("POST", f"/agents/{agent_id}/api-keys", {"name": "memory-seed"}).get("api_key")
    manifest["agents"].append({"agent_id": agent_id, "web_app_id": agent.get("web_app_id"), "shape": "five slots incl. CJK name, 64-char key, disabled slot"})

    def user(shape):
        name = f"seed-{shape}-{uuid.uuid4().hex[:6]}"
        manifest["users"].append({"agent_id": agent_id, "user": name, "shape": shape})
        return name

    full = user("all-slots-at-limits")
    put(client, full, "profile", "住在杭州西湖区，" + "资料" * 990)  # 1,988 characters, near the 2,000 limit
    put(client, full, "preferences", "Prefers short answers. " * 80)
    put(client, full, "habits", "每天早上六点起床，晚上十点半睡觉。")
    put(client, full, LONG_KEY, "Value stored under the longest allowed key.")
    put(client, full, "archived_notes", "Written while the slot was enabled; the slot is disabled afterwards.")

    for side in ("a", "b"):  # two users of one key with the same slot, kept apart
        put(client, user(f"isolation-{side}"), "profile", f"Isolation user {side}.")

    revised = user("several-revisions")
    for city in ("北京", "上海", "深圳"):
        put(client, revised, "profile", f"住在{city}。")

    # Relationship settings exist only on the relationship store; on the legacy store these shapes are skipped.
    disabled = user("memory-disabled")
    put(client, disabled, "profile", "Saved before the user turned memory off.")
    try:
        settings = client.external("GET", "/agents/memory/settings?user=" + urllib.parse.quote(disabled)) or {}
        client.external("PATCH", "/agents/memory/settings?user=" + urllib.parse.quote(disabled),
                        {"memory_enabled": False, "expected_revision": settings.get("settings_revision") or 1})
    except RuntimeError as err:
        manifest["users"][-1]["skipped"] = str(err)[:200]

    auto = user("auto-capture-on")
    put(client, auto, "preferences", "Saved by a user who allows automatic capture.")
    try:
        client.enable_auto_capture(auto)
    except RuntimeError as err:
        manifest["users"][-1]["skipped"] = str(err)[:200]

    cleared = user("cleared-all")
    put(client, cleared, "profile", "Saved, then cleared by the user.")
    client.external("DELETE", "/agents/memory?user=" + urllib.parse.quote(cleared))

    single_deleted = user("one-value-deleted")
    put(client, single_deleted, "profile", "Kept.")
    put(client, single_deleted, "preferences", "Deleted afterwards.")
    client.external("DELETE", "/agents/memory/preferences?user=" + urllib.parse.quote(single_deleted))

    # Disable one slot and change a description without republishing: the published contract and the draft now differ.
    draft_slots = [dict(slot, enabled=slot["key"] != "archived_notes") for slot in slots]
    draft_slots[0]["description"] = "Draft-only description that is not published."
    client.console("PUT", f"/agents/{agent_id}/memory/config", {"enabled": True, "auto_extraction_enabled": True, "slots": draft_slots})
    manifest["agents"][0]["draft_differs_from_published"] = True

    out_dir = os.path.join(args.out, f"seed-{stamp}-{args.label}")
    os.makedirs(out_dir, exist_ok=True)
    with open(os.path.join(out_dir, "manifest.json"), "w", encoding="utf-8") as handle:
        json.dump(manifest, handle, ensure_ascii=False, indent=2)
    print(json.dumps(manifest, ensure_ascii=False, indent=2))
    print("written to", out_dir)


if __name__ == "__main__":
    main()
