import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const dialogSource = read('src/components/datasets/document/dataset-file-asset-dialog.tsx');
const zhSource = read('src/i18n/modules/datasets/zh-Hans.ts');
const enSource = read('src/i18n/modules/datasets/en-US.ts');
const normalizedDialogSource = dialogSource.replace(/\s+/g, ' ');
const normalizedZhSource = zhSource.replace(/\s+/g, ' ');
const normalizedEnSource = enSource.replace(/\s+/g, ' ');

const requiredDialogSnippets = [
  "import { ConfirmDialog } from '@/components/ui/confirm-dialog';",
  'const selectedReadyAssetIds = useMemo(',
  'const selectedEmbeddingGenerationAssetIds = useMemo(',
  'const [autoAddPendingAssetIds, setAutoAddPendingAssetIds] = useState<Set<string>>(',
  'const [isSelectionProcessing, setIsSelectionProcessing] = useState(false);',
  'selectedEmbeddingGenerationAssetIds.length > 0 ? (',
  'count: selectedEmbeddingGenerationAssetIds.length',
  'const handleAddSelected = useCallback(() => {',
  'setPartialAddConfirmOpen(true);',
  'const handleConfirmAddSelected = useCallback(async () => {',
  'setAutoAddPendingAssetIds(new Set(pendingAssetIds));',
  'await createRefsMutation.mutateAsync(readyAssetIds);',
  'const response = await generateEmbeddingsMutation.mutateAsync({',
  'const autoAddReadyCandidates = candidates.filter(',
  "title={t('documents.fileAssets.partialAddConfirmTitle')}",
  "description={t('documents.fileAssets.partialAddConfirmDescription'",
  "confirmText={t('documents.fileAssets.partialAddConfirmAction')}",
];

for (const snippet of requiredDialogSnippets) {
  if (!dialogSource.includes(snippet)) {
    throw new Error(`Missing dataset file asset selection snippet: ${snippet}`);
  }
}

const requiredNormalizedSnippets = [
  'const selectable = candidate.addable || candidate.requires_embedding_generation === true;',
  'checked={allVisibleSelectableSelected}',
  'disabled={visibleSelectableIds.length === 0}',
  'disabled={!selectable}',
  'toggleCandidate(candidate, checked === true)',
  'await createRefsMutation.mutateAsync( autoAddReadyCandidates.map(candidate => candidate.asset_id) );',
];

for (const snippet of requiredNormalizedSnippets) {
  if (!normalizedDialogSource.includes(snippet)) {
    throw new Error(`Missing normalized dataset file asset selection snippet: ${snippet}`);
  }
}

const removedDialogSnippets = [
  'embeddingGenerationNotice',
  'embeddingGenerationCandidateIds.length > 0 ?',
];

for (const snippet of removedDialogSnippets) {
  if (dialogSource.includes(snippet)) {
    throw new Error(`Old always-visible embedding notice/batch logic should be removed: ${snippet}`);
  }
}

const expectedZhCopy = [
  "partialAddConfirmTitle: '添加所选文件？'",
  "partialAddConfirmDescription: '已选择 {selected} 个文件，其中 {ready} 个可立即添加，{pending} 个需要先生成知识库向量。继续后，系统会自动生成向量并在完成后添加到当前知识库。'",
  "partialAddConfirmAction: '继续添加'",
];

for (const snippet of expectedZhCopy) {
  if (!normalizedZhSource.includes(snippet)) {
    throw new Error(`Missing zh-Hans partial add confirmation copy: ${snippet}`);
  }
}

const expectedEnCopy = [
  "partialAddConfirmTitle: 'Add selected files?'",
  "partialAddConfirmDescription: '{selected} files are selected. {ready} can be added now, and {pending} need dataset vectors first. Continuing will generate vectors automatically and add them to this knowledge base after they are ready.'",
  "partialAddConfirmAction: 'Continue Adding'",
];

for (const snippet of expectedEnCopy) {
  if (!normalizedEnSource.includes(snippet)) {
    throw new Error(`Missing en-US partial add confirmation copy: ${snippet}`);
  }
}

console.log('Dataset file asset selection checks passed.');
