from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


ARTIFACT_PATH = Path("artifacts/result.json")
ALLOWED_MODES = {"upper", "lower", "title"}


def _read_payload() -> dict[str, Any]:
    payload = json.load(sys.stdin)
    if not isinstance(payload, dict):
        raise ValueError("stdin must contain a JSON object")
    return payload


def _safe_input_path(raw: Any) -> Path:
    path = Path(str(raw or ""))
    if not path.parts or path.is_absolute() or ".." in path.parts:
        raise ValueError("input file path is invalid")
    if path.parts[0] != "inputs":
        raise ValueError("input file must be under inputs/")
    return path


def _authorized_files(payload: dict[str, Any]) -> tuple[str | None, list[dict[str, Any]]]:
    input_files = payload.get("input_files")
    if input_files is None:
        return None, []
    if not isinstance(input_files, dict):
        raise ValueError("input_files must be an object")

    source_text = None
    source = input_files.get("source")
    if source is not None:
        if not isinstance(source, dict):
            raise ValueError("source input file metadata must be an object")
        source_text = _safe_input_path(source.get("path")).read_text(encoding="utf-8")

    attachments = input_files.get("attachments", [])
    if not isinstance(attachments, list) or any(not isinstance(item, dict) for item in attachments):
        raise ValueError("attachments input file metadata must be an array")
    for item in attachments:
        _safe_input_path(item.get("path"))
    return source_text, attachments


def _transform(text: str, mode: str) -> str:
    if mode == "upper":
        return text.upper()
    if mode == "lower":
        return text.lower()
    if mode == "title":
        return text.title()
    raise ValueError(f"unsupported mode: {mode}")


def _validate_payload(payload: dict[str, Any]) -> None:
    if payload.get("mode") not in ALLOWED_MODES:
        raise ValueError("mode must be upper, lower, or title")
    if not isinstance(payload.get("text"), str) or not payload["text"]:
        raise ValueError("text must be a non-empty string")
    if isinstance(payload.get("count"), bool) or not isinstance(payload.get("count"), int):
        raise ValueError("count must be an integer")
    if isinstance(payload.get("ratio"), bool) or not isinstance(payload.get("ratio"), (int, float)):
        raise ValueError("ratio must be a number")
    if not isinstance(payload.get("enabled"), bool):
        raise ValueError("enabled must be a boolean")
    if not isinstance(payload.get("tags"), list) or any(not isinstance(item, str) for item in payload["tags"]):
        raise ValueError("tags must be an array of strings")
    options = payload.get("options")
    if not isinstance(options, dict):
        raise ValueError("options must be an object")
    if not isinstance(options.get("prefix"), str):
        raise ValueError("options.prefix must be a string")
    if isinstance(options.get("repeat"), bool) or not isinstance(options.get("repeat"), int):
        raise ValueError("options.repeat must be an integer")
    if not isinstance(options.get("emit_diagnostic"), bool):
        raise ValueError("options.emit_diagnostic must be a boolean")
    if payload.get("note") is not None and not isinstance(payload.get("note"), str):
        raise ValueError("note must be a string or null")


def main() -> int:
    try:
        payload = _read_payload()
        _validate_payload(payload)
        mode = payload["mode"]
        source_text, attachments = _authorized_files(payload)
        combined = payload["text"]
        if source_text:
            combined = f"{combined}\n{source_text}"
        transformed = _transform(combined, mode) if payload["enabled"] else combined
        options = payload["options"]
        transformed = options["prefix"] + (transformed * options["repeat"])

        result = {
            "status": "success",
            "runtime": "python3",
            "mode": mode,
            "values": {
                "text": transformed,
                "count": payload["count"],
                "ratio": payload["ratio"],
                "enabled": payload["enabled"],
                "tags": payload["tags"],
                "options": options,
                "note": payload.get("note"),
            },
            "input_files": {
                "source_present": source_text is not None,
                "attachment_count": len(attachments),
            },
            "artifact": {
                "name": ARTIFACT_PATH.name,
                "content_type": "application/json",
            },
        }

        ARTIFACT_PATH.parent.mkdir(parents=True, exist_ok=True)
        ARTIFACT_PATH.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        if options["emit_diagnostic"]:
            print("template diagnostic: transformation completed", file=sys.stderr)
        print(json.dumps(result, ensure_ascii=False))
        return 0
    except (ValueError, OSError, UnicodeError, json.JSONDecodeError) as exc:
        print(f"script input error: {exc}", file=sys.stderr)
        return 2
