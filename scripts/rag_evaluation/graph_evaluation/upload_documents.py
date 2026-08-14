#!/usr/bin/env python3
"""Upload generated evaluation documents to the ZGI file management API.

The web console is served on localhost:3000, while the local API configured by
this repository is normally served on localhost:2670. The script talks to the
API directly:

    POST http://localhost:2670/console/api/files/upload

Authentication uses the same Bearer access token as the web console. The token
can be supplied through ZGI_ACCESS_TOKEN, --token-file, or an interactive
prompt. It is never written to the output manifest or logs.
"""

from __future__ import annotations

import argparse
import getpass
import json
import mimetypes
import os
from pathlib import Path
import random
import sys
import time
import urllib.error
import urllib.request
import uuid


DEFAULT_DOCUMENT_DIR = Path(__file__).resolve().parent / "output" / "documents"
DEFAULT_API_URL = "http://localhost:2670"
UPLOAD_PATH = "/console/api/files/upload"
CAPABILITIES_PATH = "/console/api/account/capabilities"


class UploadError(RuntimeError):
    """An upload failed with an actionable API or network error."""


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Upload graph-evaluation documents to the ZGI file manager."
    )
    parser.add_argument(
        "--documents-dir",
        type=Path,
        default=DEFAULT_DOCUMENT_DIR,
        help=f"Directory containing documents (default: {DEFAULT_DOCUMENT_DIR})",
    )
    parser.add_argument(
        "--api-url",
        default=os.getenv("ZGI_API_URL", DEFAULT_API_URL),
        help=f"API base URL, without /console/api (default: {DEFAULT_API_URL})",
    )
    parser.add_argument(
        "--token",
        help="Bearer access token. Prefer ZGI_ACCESS_TOKEN or --token-file.",
    )
    parser.add_argument(
        "--token-file",
        type=Path,
        help="Read the Bearer access token from a local file.",
    )
    parser.add_argument(
        "--workspace-id",
        default=os.getenv("ZGI_WORKSPACE_ID"),
        help="Optional workspace UUID to attach the uploads to.",
    )
    parser.add_argument(
        "--folder-id",
        default=os.getenv("ZGI_FOLDER_ID"),
        help="Optional file-manager folder UUID.",
    )
    parser.add_argument(
        "--processing-mode",
        choices=("process_now", "store_only"),
        default="process_now",
        help="Whether uploaded files should be processed immediately (default: process_now).",
    )
    parser.add_argument(
        "--parse-provider",
        choices=("auto", "local", "mineru", "reducto", "vlm", "hyperparse_api"),
        default="auto",
        help="Parse provider passed to the file API (default: auto).",
    )
    parser.add_argument(
        "--offset",
        type=int,
        default=0,
        help="Skip this many sorted documents before uploading.",
    )
    parser.add_argument(
        "--limit",
        type=int,
        help="Upload at most this many documents.",
    )
    parser.add_argument(
        "--retries",
        type=int,
        default=2,
        help="Retries per file after transient failures (default: 2).",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=120.0,
        help="HTTP timeout in seconds per request (default: 120).",
    )
    parser.add_argument(
        "--delay",
        type=float,
        default=0.0,
        help="Delay in seconds between successful uploads (default: 0).",
    )
    parser.add_argument(
        "--manifest",
        type=Path,
        help="Optional JSON path for upload results. Tokens are never included.",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="List files and parameters without making API requests.",
    )
    parser.add_argument(
        "--fail-fast",
        action="store_true",
        help="Stop at the first failed upload instead of continuing.",
    )
    return parser.parse_args()


def load_token(args: argparse.Namespace) -> str:
    if args.token:
        token = args.token.strip()
    elif args.token_file:
        token = args.token_file.read_text(encoding="utf-8").strip()
    else:
        token = os.getenv("ZGI_ACCESS_TOKEN", "").strip()

    if not token and sys.stdin.isatty():
        token = getpass.getpass("ZGI access token: ").strip()
    if token.lower().startswith("bearer "):
        token = token[7:].strip()
    if not token:
        raise UploadError(
            "missing access token; set ZGI_ACCESS_TOKEN, use --token-file, "
            "or run interactively to enter it"
        )
    return token


def resolve_current_workspace_id(api_url: str, token: str, timeout: float) -> str:
    """Resolve the workspace selected by the current console login."""
    request = urllib.request.Request(
        url=f"{api_url.rstrip('/')}{CAPABILITIES_PATH}",
        method="GET",
        headers={
            "Accept": "application/json",
            "Authorization": f"Bearer {token}",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            payload = response_payload(response)
    except urllib.error.HTTPError as error:
        error_body = error.read().decode("utf-8", errors="replace")
        try:
            payload = json.loads(error_body)
        except json.JSONDecodeError:
            payload = error_body
        raise UploadError(
            f"cannot resolve current workspace (HTTP {error.code}): "
            f"{api_error_message(payload)}; pass --workspace-id explicitly"
        ) from error
    except (urllib.error.URLError, TimeoutError) as error:
        raise UploadError(f"cannot resolve current workspace: {error}") from error

    if isinstance(payload, dict) and "code" in payload:
        code = payload["code"]
        if code not in (0, "0", "success", "SUCCESS", None):
            raise UploadError(
                f"cannot resolve current workspace (API error {code}): "
                f"{api_error_message(payload)}; pass --workspace-id explicitly"
            )

    data: object = payload.get("data", payload) if isinstance(payload, dict) else payload
    if isinstance(data, dict):
        context = data.get("context")
        if isinstance(context, dict):
            workspace_id = context.get("current_workspace_id")
            if isinstance(workspace_id, str) and workspace_id.strip():
                return workspace_id.strip()
        workspace_id = data.get("current_workspace_id")
        if isinstance(workspace_id, str) and workspace_id.strip():
            return workspace_id.strip()

    raise UploadError(
        "the current login has no selected workspace; select a workspace in the "
        "console or pass --workspace-id explicitly"
    )


def collect_documents(directory: Path, offset: int, limit: int | None) -> list[Path]:
    if not directory.is_dir():
        raise UploadError(f"documents directory does not exist: {directory}")
    if offset < 0:
        raise UploadError("--offset must be non-negative")
    if limit is not None and limit <= 0:
        raise UploadError("--limit must be positive")

    documents = sorted(
        path
        for path in directory.iterdir()
        if path.is_file() and path.suffix.lower() == ".md" and not path.name.startswith(".")
    )
    selected = documents[offset:]
    if limit is not None:
        selected = selected[:limit]
    return selected


def multipart_body(
    file_path: Path,
    fields: dict[str, str],
) -> tuple[bytes, str]:
    boundary = f"----zgi-upload-{uuid.uuid4().hex}"
    body = bytearray()

    for name, value in fields.items():
        body.extend(f"--{boundary}\r\n".encode("utf-8"))
        body.extend(
            f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode("utf-8")
        )
        body.extend(value.encode("utf-8"))
        body.extend(b"\r\n")

    mime_type = mimetypes.guess_type(file_path.name)[0] or "text/markdown"
    body.extend(f"--{boundary}\r\n".encode("utf-8"))
    body.extend(
        (
            f'Content-Disposition: form-data; name="file"; '
            f'filename="{file_path.name}"\r\n'
            f"Content-Type: {mime_type}\r\n\r\n"
        ).encode("utf-8")
    )
    body.extend(file_path.read_bytes())
    body.extend(b"\r\n")
    body.extend(f"--{boundary}--\r\n".encode("utf-8"))
    return bytes(body), f"multipart/form-data; boundary={boundary}"


def response_payload(response: urllib.response.addinfourl) -> object:
    raw = response.read().decode("utf-8", errors="replace")
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return {"raw": raw}


def api_error_message(payload: object) -> str:
    if isinstance(payload, dict):
        message = payload.get("message") or payload.get("error")
        if message:
            return str(message)
        data = payload.get("data")
        if isinstance(data, dict) and data.get("message"):
            return str(data["message"])
    return str(payload)


def upload_one(
    file_path: Path,
    api_url: str,
    token: str,
    fields: dict[str, str],
    timeout: float,
) -> object:
    body, content_type = multipart_body(file_path, fields)
    request = urllib.request.Request(
        url=f"{api_url.rstrip('/')}{UPLOAD_PATH}",
        data=body,
        method="POST",
        headers={
            "Accept": "application/json",
            "Authorization": f"Bearer {token}",
            "Content-Type": content_type,
            "Content-Length": str(len(body)),
        },
    )

    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            payload = response_payload(response)
    except urllib.error.HTTPError as error:
        error_body = error.read().decode("utf-8", errors="replace")
        try:
            payload = json.loads(error_body)
        except json.JSONDecodeError:
            payload = error_body
        raise UploadError(f"HTTP {error.code}: {api_error_message(payload)}") from error
    except (urllib.error.URLError, TimeoutError) as error:
        raise UploadError(f"network error: {error}") from error

    if isinstance(payload, dict) and "code" in payload:
        code = payload["code"]
        if code not in (0, "0", "success", "SUCCESS", None):
            raise UploadError(f"API error {code}: {api_error_message(payload)}")
    return payload


def result_summary(payload: object) -> dict[str, object]:
    if not isinstance(payload, dict):
        return {"response": payload}
    data = payload.get("data", payload)
    if isinstance(data, dict):
        file_data = data.get("file", data)
        if isinstance(file_data, dict):
            return {
                key: file_data[key]
                for key in (
                    "id",
                    "name",
                    "size",
                    "processing_mode",
                    "processing_status",
                    "processing_request_id",
                    "asset_id",
                )
                if key in file_data
            }
    return {"response": payload}


def main() -> int:
    args = parse_args()
    if args.retries < 0:
        raise UploadError("--retries must be non-negative")
    if args.timeout <= 0 or args.delay < 0:
        raise UploadError("--timeout must be positive and --delay non-negative")

    documents = collect_documents(args.documents_dir, args.offset, args.limit)
    if not documents:
        raise UploadError("no .md documents selected")

    fields = {
        "processing_mode": args.processing_mode,
        "parse_provider": args.parse_provider,
    }
    if args.workspace_id:
        fields["workspace_id"] = args.workspace_id
    if args.folder_id:
        fields["folder_id"] = args.folder_id

    print(f"documents: {len(documents)}")
    print(f"api: {args.api_url.rstrip('/')}{UPLOAD_PATH}")
    print(f"processing_mode: {args.processing_mode}")
    if args.workspace_id:
        print(f"workspace_id: {args.workspace_id}")
    if args.folder_id:
        print(f"folder_id: {args.folder_id}")

    if args.dry_run:
        for index, path in enumerate(documents, start=args.offset + 1):
            print(f"[dry-run {index}/{args.offset + len(documents)}] {path.name}")
        return 0

    token = load_token(args)
    if not args.workspace_id:
        args.workspace_id = resolve_current_workspace_id(args.api_url, token, args.timeout)
        fields["workspace_id"] = args.workspace_id
        print(f"workspace_id: {args.workspace_id}")
    results: list[dict[str, object]] = []
    failures = 0
    total = len(documents)

    for index, file_path in enumerate(documents, start=1):
        last_error: str | None = None
        for attempt in range(args.retries + 1):
            try:
                payload = upload_one(
                    file_path=file_path,
                    api_url=args.api_url,
                    token=token,
                    fields=fields,
                    timeout=args.timeout,
                )
                summary = result_summary(payload)
                results.append({"file": file_path.name, "ok": True, **summary})
                print(f"[{index}/{total}] uploaded {file_path.name} {summary}")
                break
            except UploadError as error:
                last_error = str(error)
                if attempt < args.retries:
                    sleep_seconds = min(30.0, 2.0 ** attempt + random.random())
                    print(
                        f"[{index}/{total}] retrying {file_path.name} "
                        f"({attempt + 1}/{args.retries}): {last_error}",
                        file=sys.stderr,
                    )
                    time.sleep(sleep_seconds)
        else:
            failures += 1
            results.append({"file": file_path.name, "ok": False, "error": last_error})
            print(f"[{index}/{total}] failed {file_path.name}: {last_error}", file=sys.stderr)
            if args.fail_fast:
                break

        if args.delay and index < total:
            time.sleep(args.delay)

    if args.manifest:
        args.manifest.parent.mkdir(parents=True, exist_ok=True)
        args.manifest.write_text(
            json.dumps(
                {
                    "api_url": args.api_url,
                    "upload_path": UPLOAD_PATH,
                    "documents_dir": str(args.documents_dir.resolve()),
                    "processing_mode": args.processing_mode,
                    "parse_provider": args.parse_provider,
                    "uploaded": len([item for item in results if item["ok"]]),
                    "failed": failures,
                    "results": results,
                },
                ensure_ascii=False,
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
        print(f"manifest: {args.manifest}")

    print(f"completed: {len(results) - failures}, failed: {failures}")
    return 1 if failures else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except UploadError as error:
        print(f"error: {error}", file=sys.stderr)
        raise SystemExit(2)
