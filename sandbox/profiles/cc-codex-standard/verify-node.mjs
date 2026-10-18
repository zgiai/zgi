for (const name of ["docx", "pptxgenjs", "pdf-lib", "sharp"]) {
  await import(name);
}
console.log("node profile checks passed");
