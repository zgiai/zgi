import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, '..');

const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');

const hookSource = read('src/hooks/dataset/use-dataset-file-refs.ts');
const dialogSource = read('src/components/datasets/document/dataset-file-asset-dialog.tsx');
const datasetTypeSource = read('src/services/types/dataset.ts');

if (!datasetTypeSource.includes('accepted?: boolean')) {
  throw new Error('DatasetFileCandidateEmbeddingResult should support queued accepted responses.');
}

if (!hookSource.includes('refetchInterval?: number | false')) {
  throw new Error('useDatasetFileCandidates should accept a refetchInterval option for polling.');
}

if (!hookSource.includes('refetchInterval: options.refetchInterval ?? false')) {
  throw new Error('useDatasetFileCandidates should pass refetchInterval through to useQuery.');
}

if (!hookSource.includes('response.data?.accepted')) {
  throw new Error('Embedding generation mutation should treat accepted=true as a queued task.');
}

if (!dialogSource.includes('queuedEmbeddingAssetIds')) {
  throw new Error('File asset dialog should track queued embedding asset ids.');
}

if (!dialogSource.includes('hasQueuedEmbeddingGeneration ? 3000 : false')) {
  throw new Error('File asset dialog should poll candidates while embedding generation is queued.');
}

if (
  !/generateEmbeddingsMutation\.isPending\s*&&\s*activeEmbeddingAssetId === candidate\.asset_id/.test(
    dialogSource
  )
) {
  throw new Error('Row loading state should be gated by the mutation pending flag.');
}

if (!dialogSource.includes('response.data?.accepted')) {
  throw new Error('File asset dialog should keep queued state for accepted embedding tasks.');
}

console.log('Dataset file candidate embedding flow checks passed.');
