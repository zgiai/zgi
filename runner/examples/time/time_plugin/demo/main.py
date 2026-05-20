"""
Simple plugin used to validate install -> run -> uninstall flow.

Behaviours:
1. Prints current timestamp and working directory to STDOUT.
2. Creates/updates ``artifacts/time_report.txt`` inside the workspace to
   verify write permissions.
3. Reaches out to a public URL when HTTP access is allowed; failures are
   logged but do not abort the plugin.
"""

from __future__ import annotations

import json
import os
import pathlib
import sys
import time
import urllib.error
import urllib.request


def collect_timestamp() -> str:
    return time.strftime("%Y-%m-%d %H:%M:%S %Z")


def write_artifact(workdir: pathlib.Path, message: str) -> None:
    artifacts = workdir / "artifacts"
    artifacts.mkdir(parents=True, exist_ok=True)
    target = artifacts / "time_report.txt"
    timestamp = collect_timestamp()
    target.write_text(f"[{timestamp}] {message}\n", encoding="utf-8")


def try_fetch(url: str) -> str:
    try:
        with urllib.request.urlopen(url, timeout=5) as resp:
            return f"{resp.status} {resp.reason}"
    except (urllib.error.URLError, urllib.error.HTTPError) as exc:
        return f"network-error: {exc}"


def main() -> None:
    workdir = pathlib.Path(os.environ.get("WORKDIR", os.getcwd()))
    timestamp = collect_timestamp()
    message = {
        "timestamp": timestamp,
        "workdir": str(workdir),
        "pid": os.getpid(),
    }
    write_artifact(pathlib.Path(os.getcwd()), json.dumps(message))
    net_status = try_fetch("https://example.com")
    message["network"] = net_status

    print(json.dumps(message))
    sys.stdout.flush()


if __name__ == "__main__":
    main()
