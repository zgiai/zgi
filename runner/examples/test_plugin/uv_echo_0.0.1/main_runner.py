#!/usr/bin/env python3
"""
UV echo plugin for exercising manager.Install dependency flow.
Uses requests so that pip/uv must fetch from network.
Also includes regex_extract tool for pattern matching.
"""
import json
import re
import sys
from datetime import datetime, timezone
from typing import Any

import requests


def log(message: str) -> None:
    """Write log to stderr (keeps stdout clean for protocol)."""
    sys.stderr.write(f"[uv-echo] {message}\n")
    sys.stderr.flush()


def send_message(msg_type: str, request_id: str = "", data: Any = None) -> None:
    msg = {
        "type": msg_type,
        "request_id": request_id,
        "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    }
    if data is not None:
        msg["data"] = data

    payload = json.dumps(msg, ensure_ascii=False)
    print(payload, flush=True)
    log(f"sent: {msg_type} request_id={request_id}")


def send_result(request_id: str, success: bool, data: Any = None, error: str | None = None) -> None:
    result = {"success": success}
    if data is not None:
        result["data"] = data
    if error:
        result["error"] = error
    send_message("result", request_id, result)


def echo_http(url: str, message: str) -> dict[str, Any]:
    if not url:
        return {"success": False, "error": "url is required"}
    try:
        resp = requests.get(url, timeout=5)
        return {
            "success": True,
            "data": {
                "status_code": resp.status_code,
                "message": message,
                "length": len(resp.content),
            },
        }
    except Exception as exc:  # noqa: BLE001
        return {"success": False, "error": str(exc)}


def regex_extract(content: str, expression: str) -> dict[str, Any]:
    """Execute regex extraction on content."""
    if not content:
        return {"success": False, "error": "content is required"}
    if not expression:
        return {"success": False, "error": "expression is required"}

    try:
        matches = re.findall(expression, content)
        return {
            "success": True,
            "data": {
                "matches": matches,
                "count": len(matches),
            },
        }
    except re.error as exc:
        return {"success": False, "error": f"invalid regex: {exc}"}
    except Exception as exc:  # noqa: BLE001
        return {"success": False, "error": str(exc)}


def handle_request(msg: dict[str, Any]) -> None:
    request_id = msg.get("request_id", "")
    data = msg.get("data", {}) or {}
    action = data.get("action", "")
    name = data.get("name", "")
    params = data.get("parameters", {}) or {}

    if action == "tool.invoke":
        if name in ("echo_http", "echo"):
            url = params.get("url", "https://httpbin.org/get")
            message = params.get("message", "hello-from-uv-echo")
            result = echo_http(url, message)
            send_result(request_id, result["success"], result.get("data"), result.get("error"))
        elif name in ("regex_extract", "extract"):
            content = params.get("content", "")
            expression = params.get("expression", params.get("pattern", ""))
            result = regex_extract(content, expression)
            send_result(request_id, result["success"], result.get("data"), result.get("error"))
        else:
            send_result(request_id, False, error=f"unknown tool: {name}")
    elif action == "list_tools":
        send_result(
            request_id,
            True,
            {
                "tools": [
                    {
                        "name": "echo_http",
                        "description": "GET a URL and echo a message",
                        "parameters": {
                            "url": {"type": "string", "required": False},
                            "message": {"type": "string", "required": False},
                        },
                    },
                    {
                        "name": "regex_extract",
                        "description": "Extract content using regular expression",
                        "parameters": {
                            "content": {"type": "string", "required": True},
                            "expression": {"type": "string", "required": True},
                        },
                    },
                ]
            },
        )
    else:
        send_result(request_id, False, error=f"unknown action: {action}")


def main() -> None:
    log("uv-echo plugin starting")
    send_message("ready")
    try:
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            try:
                msg = json.loads(line)
            except json.JSONDecodeError as exc:
                log(f"json decode error: {exc}")
                continue
            if msg.get("type") == "request":
                handle_request(msg)
            else:
                log(f"unsupported message type: {msg.get('type')}")
    except KeyboardInterrupt:
        log("shutdown requested")
    log("uv-echo plugin exiting")


if __name__ == "__main__":
    main()
