import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const policySource = read('src/components/files/file-upload-policy.ts');
const match = policySource.match(/FILE_MANAGEMENT_UPLOAD_ACCEPT_EXT\s*=\s*\[([\s\S]*?)\]\s+as const/);

if (!match) {
  throw new Error('FILE_MANAGEMENT_UPLOAD_ACCEPT_EXT constant was not found.');
}

const actual = [...match[1].matchAll(/'([^']+)'/g)].map(item => item[1]);
const expected = [
  'pdf',
  'docx',
  'doc',
  'xlsx',
  'xls',
  'csv',
  'txt',
  'md',
  'markdown',
  'mdx',
  'png',
  'jpg',
  'jpeg',
  'pptx',
  'ppt',
];

if (JSON.stringify(actual) !== JSON.stringify(expected)) {
  throw new Error(
    `Unexpected file management upload extensions.\nActual: ${actual.join(',')}\nExpected: ${expected.join(',')}`
  );
}

const fileManagementSource = read('src/components/files/file-management-content.tsx');
if (!fileManagementSource.includes('FILE_MANAGEMENT_UPLOAD_ACCEPT_EXT')) {
  throw new Error('File management upload dialog does not use the fixed upload allowlist.');
}
if (!fileManagementSource.includes('acceptExt={uploadAcceptExt}')) {
  throw new Error('File management upload dialog should use uploadAcceptExt, not list-filter acceptExt.');
}

const createLocalDialogSource = read('src/components/files/create-local-file-dialog.tsx');
if (!createLocalDialogSource.includes('showAllowedTypesHint={false}')) {
  throw new Error('File management upload dialog should hide the allowed-types hint.');
}
if (!createLocalDialogSource.includes('useNativeAccept={false}')) {
  throw new Error('File management upload dialog should not set the native input accept attribute.');
}

const zhUiSource = read('src/i18n/modules/ui/zh-Hans.ts');
if (!zhUiSource.includes("invalidExt: '不允许上传该类型文件'")) {
  throw new Error('Invalid extension copy should tell users that this file type is not allowed.');
}

console.log('File management upload policy checks passed.');
