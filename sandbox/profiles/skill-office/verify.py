import importlib
import json


MODULES = [
    "defusedxml",
    "lxml",
    "PIL",
    "openpyxl",
    "pandas",
    "pypdf",
    "pdfplumber",
    "reportlab",
    "pdf2image",
    "pytesseract",
    "yaml",
]


def main() -> int:
    missing = []
    for name in MODULES:
        try:
            importlib.import_module(name)
        except Exception as exc:
            missing.append({"module": name, "error": str(exc)})
    print(json.dumps({"profile": "skill-office", "missing": missing, "ok": not missing}))
    return 1 if missing else 0


if __name__ == "__main__":
    raise SystemExit(main())
