import { readFileSync } from 'node:fs';

const sidebarSource = readFileSync('src/components/files/file-sidebar.tsx', 'utf8');
const dialogSource = readFileSync('src/components/files/create-folder-dialog.tsx', 'utf8');
const zhSource = readFileSync('src/i18n/modules/files/zh-Hans.ts', 'utf8');
const enSource = readFileSync('src/i18n/modules/files/en-US.ts', 'utf8');

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

assert(
  !sidebarSource.includes('files.sidebar.folderLevelHint'),
  'Folder level hint should not be rendered in the file space sidebar.'
);

assert(
  dialogSource.includes("t('files.folder.levelHint')"),
  'Create folder dialog should render the folder level hint near the parent folder selector.'
);

assert(
  zhSource.includes("levelHint: '最多支持三级文件夹。已达到层级上限的文件夹不可作为父文件夹。'"),
  'Chinese folder level hint copy is missing or changed.'
);

assert(
  enSource.includes("levelHint:") &&
    enSource.includes('Folders support up to three levels.') &&
    enSource.includes('Folders at the limit cannot be selected as parent folders.'),
  'English folder level hint copy is missing or changed.'
);

console.log('Create folder level hint placement checks passed.');
