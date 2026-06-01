const modules = ["docx", "pptxgenjs", "pdf-lib"];
const missing = [];

for (const name of modules) {
  try {
    await import(name);
  } catch (error) {
    missing.push({ module: name, error: error.message });
  }
}

console.log(JSON.stringify({ profile: "skill-office", missing, ok: missing.length === 0 }));
process.exit(missing.length === 0 ? 0 : 1);
