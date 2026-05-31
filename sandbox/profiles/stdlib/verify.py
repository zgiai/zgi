import json
import sys


def main() -> int:
    print(json.dumps({"profile": "stdlib", "python": sys.version_info.major == 3, "ok": True}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
