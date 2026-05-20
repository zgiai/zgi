import type { Dataset } from '@/services/types/dataset';

export interface OpenDatasetDialogPayload {
  mode: 'create' | 'edit';
  dataset?: Dataset;
  currentFolderId?: string;
}
