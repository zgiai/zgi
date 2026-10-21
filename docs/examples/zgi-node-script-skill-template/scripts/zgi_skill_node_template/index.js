"use strict";

const fs = require("fs");
const path = require("path");

const ARTIFACT_PATH = path.join("artifacts", "result.json");
const ALLOWED_MODES = new Set(["upper", "lower", "title"]);

function readStdin() {
  return new Promise((resolve, reject) => {
    let data = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => {
      data += chunk;
    });
    process.stdin.on("end", () => {
      try {
        const payload = JSON.parse(data || "{}");
        if (!payload || Array.isArray(payload) || typeof payload !== "object") {
          throw new Error("stdin must contain a JSON object");
        }
        resolve(payload);
      } catch (error) {
        reject(error);
      }
    });
    process.stdin.on("error", reject);
  });
}

function safeInputPath(raw) {
  if (typeof raw !== "string" || raw.length === 0 || path.isAbsolute(raw)) {
    throw new Error("input file path is invalid");
  }
  const normalized = raw.replaceAll("\\", "/");
  const parts = normalized.split("/");
  if (parts[0] !== "inputs" || parts.includes("..")) {
    throw new Error("input file must be under inputs/");
  }
  return normalized;
}

function validatePayload(payload) {
  if (!ALLOWED_MODES.has(payload.mode)) throw new Error("mode must be upper, lower, or title");
  if (typeof payload.text !== "string" || payload.text.length === 0) throw new Error("text must be a non-empty string");
  if (!Number.isInteger(payload.count)) throw new Error("count must be an integer");
  if (typeof payload.ratio !== "number" || !Number.isFinite(payload.ratio)) throw new Error("ratio must be a number");
  if (typeof payload.enabled !== "boolean") throw new Error("enabled must be a boolean");
  if (!Array.isArray(payload.tags) || payload.tags.some((item) => typeof item !== "string")) throw new Error("tags must be an array of strings");
  if (!payload.options || Array.isArray(payload.options) || typeof payload.options !== "object") throw new Error("options must be an object");
  if (typeof payload.options.prefix !== "string") throw new Error("options.prefix must be a string");
  if (!Number.isInteger(payload.options.repeat)) throw new Error("options.repeat must be an integer");
  if (typeof payload.options.emit_diagnostic !== "boolean") throw new Error("options.emit_diagnostic must be a boolean");
  if (payload.note !== null && payload.note !== undefined && typeof payload.note !== "string") throw new Error("note must be a string or null");
}

function authorizedFiles(payload) {
  const inputFiles = payload.input_files;
  if (inputFiles === undefined) return { sourceText: null, attachments: [] };
  if (!inputFiles || Array.isArray(inputFiles) || typeof inputFiles !== "object") throw new Error("input_files must be an object");

  let sourceText = null;
  if (inputFiles.source !== undefined) {
    if (!inputFiles.source || Array.isArray(inputFiles.source) || typeof inputFiles.source !== "object") {
      throw new Error("source input file metadata must be an object");
    }
    sourceText = fs.readFileSync(safeInputPath(inputFiles.source.path), "utf8");
  }
  const attachments = inputFiles.attachments || [];
  if (!Array.isArray(attachments) || attachments.some((item) => !item || Array.isArray(item) || typeof item !== "object")) {
    throw new Error("attachments input file metadata must be an array");
  }
  attachments.forEach((item) => safeInputPath(item.path));
  return { sourceText, attachments };
}

function transform(text, mode) {
  if (mode === "upper") return text.toUpperCase();
  if (mode === "lower") return text.toLowerCase();
  return text.replace(/\b\w/g, (character) => character.toUpperCase());
}

async function main() {
  try {
    const payload = await readStdin();
    validatePayload(payload);
    const { sourceText, attachments } = authorizedFiles(payload);
    const combined = sourceText === null ? payload.text : `${payload.text}\n${sourceText}`;
    const transformed = payload.enabled ? transform(combined, payload.mode) : combined;
    const text = payload.options.prefix + transformed.repeat(payload.options.repeat);
    const result = {
      status: "success",
      runtime: "nodejs",
      mode: payload.mode,
      values: {
        text,
        count: payload.count,
        ratio: payload.ratio,
        enabled: payload.enabled,
        tags: payload.tags,
        options: payload.options,
        note: payload.note ?? null,
      },
      input_files: {
        source_present: sourceText !== null,
        attachment_count: attachments.length,
      },
      artifact: {
        name: path.basename(ARTIFACT_PATH),
        content_type: "application/json",
      },
    };

    fs.mkdirSync(path.dirname(ARTIFACT_PATH), { recursive: true });
    fs.writeFileSync(ARTIFACT_PATH, `${JSON.stringify(result, null, 2)}\n`, "utf8");
    if (payload.options.emit_diagnostic) process.stderr.write("template diagnostic: transformation completed\n");
    process.stdout.write(`${JSON.stringify(result)}\n`);
    return 0;
  } catch (error) {
    process.stderr.write(`script input error: ${error.message}\n`);
    return 2;
  }
}

module.exports = { main };
