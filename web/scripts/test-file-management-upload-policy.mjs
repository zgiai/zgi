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
if (!policySource.includes('FILE_MANAGEMENT_UPLOAD_MAX_SIZE_MB = 15')) {
  throw new Error('File management uploads should have a hard 15 MB per-file limit.');
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
if (!createLocalDialogSource.includes('uploadConfig?.upload_queue_limit ?? 200')) {
  throw new Error('File management upload queue should use the configured queue limit.');
}
if (!createLocalDialogSource.includes('Math.min(')) {
  throw new Error('File management upload size should be clamped to its hard limit.');
}
if (!createLocalDialogSource.includes('FILE_MANAGEMENT_UPLOAD_MAX_SIZE_MB')) {
  throw new Error('File management upload should use the fixed 15 MB size limit.');
}
if (!createLocalDialogSource.includes('concurrencyLimit={uploadConcurrency}')) {
  throw new Error('File management upload should use the configured upload pool size.');
}
if (!createLocalDialogSource.includes('allowFolderSelection')) {
  throw new Error('File management upload should allow selecting a local folder.');
}

const manualUploadSource = read('src/components/common/file-upload/manual-file-upload.tsx');
if (!manualUploadSource.includes('runUploadPool<UploadItem, UploadedFile | null>')) {
  throw new Error('Manual file upload should execute pending files through the upload pool.');
}
if (!manualUploadSource.includes("import { directoryOpen } from 'browser-fs-access'")) {
  throw new Error('Manual file upload should use the browser directory picker.');
}
if (!manualUploadSource.includes('const entries = await directoryOpen({')) {
  throw new Error('The folder button should open a directory instead of a regular file chooser.');
}
if (!manualUploadSource.includes('recursive: true')) {
  throw new Error('Directory selection should include files from nested folders.');
}

const zhUiSource = read('src/i18n/modules/ui/zh-Hans.ts');
if (!zhUiSource.includes("invalidExt: '不允许上传该类型文件'")) {
  throw new Error('Invalid extension copy should tell users that this file type is not allowed.');
}

console.log('File management upload policy checks passed.');
