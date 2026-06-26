import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const iconSource = read('src/components/files/file-type-icon.tsx');
const fileListSource = read('src/components/files/file-list.tsx');

const requiredTypeSnippets = [
  "pdf: { Icon: FileText, color: 'text-rose-600' }",
  "doc: { Icon: FileText, color: 'text-blue-600' }",
  "txt: { Icon: FileText, color: 'text-slate-600' }",
  "xls: { Icon: FileSpreadsheet, color: 'text-emerald-600' }",
  "ppt: { Icon: FileType2, color: 'text-orange-600' }",
  "jpg: { Icon: FileImage, color: 'text-pink-600' }",
  "zip: { Icon: FileArchive, color: 'text-amber-600' }",
  "js: { Icon: FileCode, color: 'text-indigo-600' }",
];

for (const snippet of requiredTypeSnippets) {
  if (!iconSource.includes(snippet)) {
    throw new Error(`Missing visually distinct file type mapping: ${snippet}`);
  }
}

const fileTypeIconUses = fileListSource.match(/<FileTypeIcon[\s\S]*?\/>/g) ?? [];
const listRowIconUses = fileTypeIconUses.filter(snippet => snippet.includes('extension={file.extension}'));

if (listRowIconUses.length < 2) {
  throw new Error('Expected file list to render FileTypeIcon in both mobile card and desktop row views.');
}

for (const snippet of listRowIconUses) {
  if (!snippet.includes('filename={file.name}')) {
    throw new Error('File list FileTypeIcon usage should pass filename={file.name} as extension fallback.');
  }
}

console.log('File type icon visual distinction checks passed.');
