import importlib
import subprocess
import sys
from pathlib import Path

for module in ("numpy", "pandas", "PIL", "openpyxl", "pypdf", "reportlab", "lxml"):
    importlib.import_module(module)
subprocess.run(
    [sys.executable, str(Path(__file__).parent / "resources" / "verify_xml.py")],
    check=True,
)
print("python profile checks passed")
