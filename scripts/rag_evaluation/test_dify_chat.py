#!/usr/bin/env python3
"""Send one question to a local Dify app and print its answer and retrieved contexts."""

from __future__ import annotations

import argparse
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


SCRIPT_DIR = Path(__file__).resolve().parent
ENV_FILE = SCRIPT_DIR / ".env"
DEFAULT_BASE_URL = "http://127.0.0.1:18000/v1"
DEFAULT_QUESTION = "退号流程"


def main() -> int:
    env_values = load_env_file(ENV_FILE)
    args = parse_args(env_values)

    api_key = args.api_key.strip()
    if not api_key:
        raise SystemExit(f"Dify API key is required. Set DIFY_API_KEY in {ENV_FILE}.")
    if api_key.startswith("dataset-"):
        raise SystemExit(
            "DIFY_API_KEY contains a knowledge-base API key (dataset-...). "
            "The /chat-messages endpoint requires the API key from the published Dify app's API Access page."
        )

    response_mode = args.response_mode.strip().lower()
    if response_mode != "blocking":
        raise SystemExit("This smoke-test script currently supports DIFY_RESPONSE_MODE=blocking only.")

    base_url = args.base_url.strip().rstrip("/")
    endpoint = f"{base_url}/chat-messages"
    payload = {
        "inputs": {},
        "query": args.question,
        "response_mode": response_mode,
        "conversation_id": "",
        "user": args.user,
    }

    print(f"Sending question to Dify: {args.question}", flush=True)
    started = time.perf_counter()
    data = post_json(endpoint, payload, api_key, args.timeout)
    elapsed = time.perf_counter() - started

    answer = data.get("answer")
    if not isinstance(answer, str) or not answer.strip():
        raise SystemExit("Dify response does not contain a non-empty answer.")

    print("\nAnswer:")
    print(answer)
    print(f"\nEnd-to-end latency: {elapsed:.3f}s")

    metadata = data.get("metadata")
    metadata = metadata if isinstance(metadata, dict) else {}
    resources = metadata.get("retriever_resources")
    resources = resources if isinstance(resources, list) else []
    print(f"\nRetrieved contexts: {len(resources)}")
    for index, resource in enumerate(resources, start=1):
        if not isinstance(resource, dict):
            continue
        position = resource.get("position", index)
        document_name = resource.get("document_name") or "unknown document"
        score = resource.get("score")
        score_text = f", score={score}" if score is not None else ""
        content = str(resource.get("content") or "").strip()
        print(f"\n[{position}] {document_name}{score_text}")
        print(content or "<empty content>")

    usage = metadata.get("usage")
    if isinstance(usage, dict):
        print("\nUsage:")
        for key in ("prompt_tokens", "completion_tokens", "total_tokens", "latency"):
            if key in usage:
                print(f"  {key}: {usage[key]}")
    return 0


def parse_args(env_values: dict[str, str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Send one question to Dify and print the answer and retrieval contexts.")
    parser.add_argument("question", nargs="?", default=DEFAULT_QUESTION, help="Question to send. Default: %(default)s")
    parser.add_argument("--base-url", default=env_value(env_values, "DIFY_BASE_URL", DEFAULT_BASE_URL))
    parser.add_argument("--api-key", default=env_value(env_values, "DIFY_API_KEY"), help=argparse.SUPPRESS)
    parser.add_argument("--user", default=f'{env_value(env_values, "DIFY_USER_PREFIX", "rag-eval")}-smoke-test')
    parser.add_argument("--response-mode", default=env_value(env_values, "DIFY_RESPONSE_MODE", "blocking"))
    parser.add_argument("--timeout", type=float, default=300.0)
    return parser.parse_args()


def load_env_file(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except FileNotFoundError:
        return values
    for line in lines:
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or "=" not in stripped:
            continue
        key, value = stripped.split("=", 1)
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key:
            values[key] = value
    return values


def env_value(env_values: dict[str, str], key: str, default: str = "") -> str:
    return os.getenv(key) or env_values.get(key, default)


def post_json(url: str, payload: dict[str, Any], api_key: str, timeout: float) -> dict[str, Any]:
    request = urllib.request.Request(
        url,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({})) if is_local_url(url) else urllib.request.build_opener()
    try:
        with opener.open(request, timeout=timeout) as response:
            raw = response.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"Dify returned HTTP {exc.code}: {detail}") from exc
    except urllib.error.URLError as exc:
        raise SystemExit(f"Cannot connect to Dify at {url}: {exc}") from exc

    try:
        data = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise SystemExit("Dify returned a response that is not valid JSON.") from exc
    if not isinstance(data, dict):
        raise SystemExit("Dify returned a JSON response with an unexpected shape.")
    return data


def is_local_url(url: str) -> bool:
    hostname = urllib.parse.urlparse(url).hostname or ""
    return hostname in {"localhost", "127.0.0.1", "::1"}


if __name__ == "__main__":
    raise SystemExit(main())
