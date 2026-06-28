import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const dialogSource = read('src/components/files/create-local-file-dialog.tsx');

const requiredDialogSnippets = [
  "import { ConfirmDialog } from '@/components/ui/confirm-dialog';",
  'const [closeConfirmOpen, setCloseConfirmOpen] = useState(false);',
  'setCloseConfirmOpen(true);',
  "title={t('files.upload.cancelUploadConfirmTitle')}",
  "description={t('files.upload.cancelUploadConfirmDescription')}",
  "confirmText={t('files.upload.cancelUploadConfirmAction')}",
  "cancelText={t('common.cancel')}",
  'onConfirm={handleCancelUpload}',
];

for (const snippet of requiredDialogSnippets) {
  if (!dialogSource.includes(snippet)) {
    throw new Error(`Missing close confirmation behavior snippet: ${snippet}`);
  }
}

const removedDialogSnippets = [
  "import { Alert, AlertDescription } from '@/components/ui/alert';",
  'AlertCircle',
  'uploadInProgressCloseHint',
  'toast.info',
];

for (const snippet of removedDialogSnippets) {
  if (dialogSource.includes(snippet)) {
    throw new Error(`Upload dialog should no longer include: ${snippet}`);
  }
}

const zhSource = read('src/i18n/modules/files/zh-Hans.ts');
const enSource = read('src/i18n/modules/files/en-US.ts');
const normalizedEnSource = enSource.replace(/\s+/g, ' ');

const expectedZhCopy = [
  "cancelUploadConfirmTitle: '取消上传并关闭窗口？'",
  "cancelUploadConfirmDescription: '关闭后，尚未上传完成的文件将停止上传，已完成上传的文件会保留。'",
  "cancelUploadConfirmAction: '确认关闭'",
];

for (const snippet of expectedZhCopy) {
  if (!zhSource.includes(snippet)) {
    throw new Error(`Missing zh-Hans confirmation copy: ${snippet}`);
  }
}

const expectedEnCopy = [
  "cancelUploadConfirmTitle: 'Cancel upload and close this window?'",
  "cancelUploadConfirmDescription: 'Files that have not finished uploading will stop. Files already uploaded will be kept.'",
  "cancelUploadConfirmAction: 'Close Window'",
];

for (const snippet of expectedEnCopy) {
  if (!normalizedEnSource.includes(snippet)) {
    throw new Error(`Missing en-US confirmation copy: ${snippet}`);
  }
}

if (zhSource.includes('uploadInProgressCloseHint') || enSource.includes('uploadInProgressCloseHint')) {
  throw new Error('Old upload-in-progress close hint copy should be removed.');
}

console.log('File upload close confirmation checks passed.');
