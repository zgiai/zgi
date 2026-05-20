// Hit Testing Component Types
// Using unified types from service layer

import type {
  HitTestingResult,
  HitTestingResponse,
  ExternalDatasetHitTestingResponse,
  InternalRetrievalConfig,
  HitTestingHistoryRecord,
  HitTestingHistoryResponse,
} from '@/services/types/dataset';

// Re-export service types for component use
export type {
  HitTestingResult as HitTestingResultItem,
  HitTestingResponse,
  ExternalDatasetHitTestingResponse,
  InternalRetrievalConfig as RetrievalConfig,
  HitTestingHistoryRecord as HitTestingRecord,
  HitTestingHistoryResponse as HitTestingRecordsResponse,
};

// Component-specific interfaces
export interface ExternalRetrievalSettings {
  top_k: number;
  score_threshold: number;
  score_threshold_enabled: boolean;
}

// Component Props Types
export interface QueryTextareaProps {
  query: string;
  onQueryChange: (query: string) => void;
  onSubmit: () => void;
  isLoading: boolean;
  isExternalDataSource: boolean;
  retrievalConfig: InternalRetrievalConfig;
  onConfigChange: () => void;
  maxLength?: number;
}

export interface ResultItemProps {
  result: HitTestingResult;
  index: number;
  isExternal?: boolean;
}

export interface ResultItemExternalProps {
  result: ExternalDatasetHitTestingResponse['records'][0];
  index: number;
}

export interface RecordsTableProps {
  records: HitTestingHistoryRecord[];
  isLoading: boolean;
  onLoadQuery: (record: HitTestingHistoryRecord) => void;
  currentPage: number;
  totalPages: number;
  total?: number;
  pageSize?: number;
  onLoadMore: () => void;
  hasMore: boolean;
  hasPreviousPage: boolean;
  isFetchingNextPage: boolean;
  onLoadPrevious: () => void;
}

export interface FloatRightContainerProps {
  children: React.ReactNode;
  isShow: boolean;
  onToggle: () => void;
  isMobile?: boolean;
}

export interface RetrievalConfigModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  config: InternalRetrievalConfig;
  onConfigChange: (config: InternalRetrievalConfig) => void;
  /** Save to dataset settings (persists to backend) */
  onSave: (config: InternalRetrievalConfig) => void;
  /** Save as test config only (local state, not persisted) */
  onSaveAsTest?: (config: InternalRetrievalConfig) => void;
  /** Whether graph search is enabled for this dataset */
  isGraphEnabled?: boolean;
}

export interface ExternalRetrievalConfigModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  settings: ExternalRetrievalSettings;
  onSettingsChange: (settings: ExternalRetrievalSettings) => void;
  onSave: (settings: ExternalRetrievalSettings) => void;
}

// Hook Types
export interface UseHitTestingState {
  // Query state
  query: string;
  setQuery: (query: string) => void;

  // Results state
  hitResult: HitTestingResponse | undefined;
  externalHitResult: ExternalDatasetHitTestingResponse | undefined;

  // History state
  recordsRes: HitTestingHistoryResponse | undefined;

  // UI state
  isShowRightPanel: boolean;
  setIsShowRightPanel: (show: boolean) => void;

  // Configuration state
  retrievalConfig: InternalRetrievalConfig;
  setRetrievalConfig: (config: InternalRetrievalConfig) => void;
  externalRetrievalSettings: ExternalRetrievalSettings;
  setExternalRetrievalSettings: (settings: ExternalRetrievalSettings) => void;

  // Loading states
  isSearching: boolean;
  isLoadingHistory: boolean;

  // Actions
  handleHitTesting: () => Promise<void>;
  handleExternalHitTesting: () => Promise<void>;
  loadHistoryRecords: (page?: number) => Promise<void>;
  loadQueryFromHistory: (record: HitTestingHistoryRecord) => void;
}

// Layout Types
export interface HitTestingLayoutProps {
  leftPanel: React.ReactNode;
  rightPanel: React.ReactNode;
  isShowRightPanel: boolean;
  onToggleRightPanel: () => void;
  isMobile?: boolean;
}

// Utility Types
export type HitTestingStatus = 'idle' | 'searching' | 'success' | 'error';

export interface HitTestingError {
  message: string;
  code?: string;
  details?: unknown;
}

export interface HitTestingMetrics {
  total_results: number;
  retrieval_time: number;
  query_length: number;
  avg_score: number;
  max_score: number;
  min_score: number;
}
