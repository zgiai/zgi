#!/usr/bin/env python3
"""Send one prompt to the Ragas judge LLM and print latency."""

from __future__ import annotations

import argparse
import os
import time
from pathlib import Path
from typing import Any


SCRIPT_DIR = Path(__file__).resolve().parent
ENV_FILE = SCRIPT_DIR / ".env"
DEFAULT_ALIYUN_BASE_URL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
DEFAULT_ALIYUN_LLM_MODEL = "qwen-plus"


def main() -> int:
    env_values = load_env_file(ENV_FILE)
    args = parse_args(env_values)

    prompt = args.prompt or input("Question: ").strip()
    if not prompt:
        raise SystemExit("question is required")

    provider = (args.provider or "auto").strip().lower()
    api_key = (args.api_key or "").strip()
    base_url = (args.base_url or "").strip().rstrip("/")
    model = (args.model or "").strip()
    enable_thinking = optional_bool(args.enable_thinking)

    if provider == "auto":
        if env_value(env_values, "ALIYUN_API_KEY") or env_value(env_values, "DASHSCOPE_API_KEY") or "dashscope.aliyuncs.com" in base_url:
            provider = "aliyun"
        else:
            provider = "openai"

    if provider == "aliyun":
        base_url = base_url or DEFAULT_ALIYUN_BASE_URL
        model = model or DEFAULT_ALIYUN_LLM_MODEL
        if enable_thinking is None:
            enable_thinking = False

    if not api_key:
        raise SystemExit("LLM API key is required. Set RAGAS_API_KEY, ALIYUN_API_KEY, DASHSCOPE_API_KEY, or OPENAI_API_KEY in .env.")
    if not model:
        raise SystemExit("LLM model is required. Set RAGAS_LLM_MODEL in .env or pass --model.")

    try:
        from openai import OpenAI
    except ImportError as exc:
        raise SystemExit("openai is required. Install it with: pip install openai") from exc

    client = OpenAI(api_key=api_key, base_url=base_url or None, timeout=args.timeout, max_retries=0)
    extra_body: dict[str, Any] | None = None
    if provider == "aliyun" and enable_thinking is not None:
        extra_body = {"enable_thinking": enable_thinking}

    print(f"calling LLM: provider={provider}, model={model}, enable_thinking={enable_thinking}", flush=True)
    started = time.perf_counter()
    response = client.chat.completions.create(
        model=model,
        messages=[
            {"role": "system", "content": "You are a concise assistant."},
            {"role": "user", "content": prompt},
        ],
        temperature=args.temperature,
        max_tokens=args.max_tokens,
        extra_body=extra_body,
    )
    elapsed = time.perf_counter() - started

    content = response.choices[0].message.content or ""
    print("\nResponse:")
    print(content)
    print(f"\nLatency: {elapsed:.3f}s")
    usage = getattr(response, "usage", None)
    if usage:
        print(f"Usage: prompt_tokens={usage.prompt_tokens}, completion_tokens={usage.completion_tokens}, total_tokens={usage.total_tokens}")
    return 0


def parse_args(env_values: dict[str, str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Measure one-call latency for the Ragas judge LLM.")
    parser.add_argument("prompt", nargs="?", help="Question to send. If omitted, prompt interactively.")
    parser.add_argument("--provider", default=env_value(env_values, "RAGAS_PROVIDER", "auto"), choices=["auto", "aliyun", "openai"])
    parser.add_argument("--api-key", default=first_env(env_values, "RAGAS_API_KEY", "ALIYUN_API_KEY", "DASHSCOPE_API_KEY", "OPENAI_API_KEY"))
    parser.add_argument("--base-url", default=first_env(env_values, "RAGAS_BASE_URL", "ALIYUN_BASE_URL", "DASHSCOPE_BASE_URL", "OPENAI_BASE_URL"))
    parser.add_argument("--model", default=first_env(env_values, "RAGAS_LLM_MODEL", "ALIYUN_LLM_MODEL", "DASHSCOPE_LLM_MODEL"))
    parser.add_argument("--enable-thinking", default=first_env(env_values, "RAGAS_ENABLE_THINKING", "ALIYUN_ENABLE_THINKING", "DASHSCOPE_ENABLE_THINKING"))
    parser.add_argument("--temperature", type=float, default=0.0)
    parser.add_argument("--max-tokens", type=int, default=1024)
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


def first_env(env_values: dict[str, str], *keys: str) -> str:
    for key in keys:
        value = env_value(env_values, key)
        if value:
            return value
    return ""


def optional_bool(value: str) -> bool | None:
    normalized = (value or "").strip().lower()
    if not normalized:
        return None
    if normalized in {"1", "true", "yes", "y", "on"}:
        return True
    if normalized in {"0", "false", "no", "n", "off"}:
        return False
    raise SystemExit(f"invalid boolean value: {value}; expected true or false")


if __name__ == "__main__":
    raise SystemExit(main())
