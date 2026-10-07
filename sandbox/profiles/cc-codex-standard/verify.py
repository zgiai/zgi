import importlib

for module in ("numpy", "pandas", "PIL", "openpyxl", "pypdf", "reportlab", "lxml"):
    importlib.import_module(module)
print("python profile checks passed")
