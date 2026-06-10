'use client';

// Step 2: file-level AI recognition workspace.

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  AlertCircle,
  Check,
  CheckCircle,
  Eye,
  FileSearch,
  ImageIcon,
  LoaderCircle,
  RefreshCcw,
  RotateCcw,
  Trash2,
  XCircle,
} from 'lucide-react';
import type {
  BatchIngestResultItem,
  DbTableColumn,
  DbTableRecord,
  FileIngestAttempt,
  FileIngestExtractionInfo,
  FileIngestExtractionMode,
  GetDbTableRecordsParams,
} from '@/services/types/db';
import { Type } from '@/services/types/db';
import { useCreateDbTableRecords } from '@/hooks/db/use-db-table-records';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { MarkdownImage } from '@/components/common/markdown-image';
import { Switch } from '@/components/ui/switch';
import { formatDateTimeLocalInput } from '@/utils/date-input';
import { useT } from '@/i18n';
import { useDbTablePrompt } from '@/hooks/db/use-db-table-prompt';
import { useIngestFileToTable } from '@/hooks/db/use-batch-ingest-file-to-table';
import type { FileItem } from '@/services/types/file';
import { FileIcon } from '@/components/ui/file-icon';
import { formatFileSize } from '@/utils/format';
import { cn } from '@/lib/utils';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useFileOriginalPreviewUrl } from '@/hooks/file/use-file-original-preview-url';
import {
  isTableIngestImageFile,
  normalizeIngestExtension,
} from '@/components/db/table-ingest/file-support';

export interface IngestStepTwoProps {
  selectedFiles: FileItem[];
  selectedModel: { provider: string; model: string };
  dbId: string;
  tableId: string;
  modelSupportsVision?: boolean;
  onRemoveFile?: (fileId: string) => void;
  onLeaveGuardChange?: (active: boolean, reason: 'processing' | 'unsaved' | null) => void;
}

type RecognitionStatus = 'idle' | 'queued' | 'recognizing' | 'success' | 'warning' | 'failed' | 'skipped';
type FileFilter = 'all' | 'needs_action' | 'failed' | 'ready';
type ContentTab = 'original' | 'text' | 'details';

interface RecognitionAttempt {
  id: string;
  mode: FileIngestExtractionMode;
  status: RecognitionStatus;
  content: string;
  records: DbTableRecord[];
  error?: string;
  warning?: string;
  extraction?: FileIngestExtractionInfo;
  createdAt: number;
}

interface FileRecognitionState {
  file: FileItem;
  status: RecognitionStatus;
  requestedMode: FileIngestExtractionMode;
  activeAttemptId?: string;
  attempts: RecognitionAttempt[];
  content: string;
  records: DbTableRecord[];
  values: DbTableRecord;
  error?: string;
  warning?: string;
  extraction?: FileIngestExtractionInfo;
  dirty: boolean;
  requestId?: string;
  requestStartedAt?: number;
}

const IngestLoadingBlock: React.FC<{
  title: string;
  desc: string;
  compact?: boolean;
}> = ({ title, desc, compact = false }) => (
  <div
    className={cn(
      'flex h-full min-h-[320px] items-center justify-center',
      compact ? 'min-h-[220px]' : 'min-h-[420px]'
    )}
  >
    <div
      className={cn(
        'w-full max-w-md rounded-lg border border-dashed border-primary/30 bg-primary/5 px-6 py-8 text-center',
        compact && 'px-4 py-6'
      )}
    >
      <div
        className={cn(
          'mx-auto flex items-center justify-center rounded-lg bg-primary/10 text-primary',
          compact ? 'h-11 w-11' : 'h-14 w-14'
        )}
      >
        <FileSearch className={compact ? 'h-5 w-5' : 'h-7 w-7'} />
      </div>
      <div className="mt-4 flex items-center justify-center gap-2 text-sm font-medium text-foreground">
        <LoaderCircle className="h-4 w-4 animate-spin text-primary" />
        <span>{title}</span>
      </div>
      <div className="mt-2 text-sm leading-6 text-muted-foreground">{desc}</div>
    </div>
  </div>
);

const NUMERIC_ONLY_PATTERN = /^-?\d+(?:\.\d+)?$/;

function isValidTimestampRecordValue(value: DbTableRecord[keyof DbTableRecord]): boolean {
  if (typeof value !== 'string') return false;
  const trimmed = value.trim();
  if (!trimmed || NUMERIC_ONLY_PATTERN.test(trimmed)) return false;
  return formatDateTimeLocalInput(trimmed) !== '';
}

function isPdfFile(file?: FileItem): boolean {
  if (!file) return false;
  return (
    normalizeIngestExtension(file.extension) === 'pdf' ||
    file.mime_type?.toLowerCase() === 'application/pdf'
  );
}

function createInitialFileState(file: FileItem): FileRecognitionState {
  return {
    file,
    status: 'idle',
    requestedMode: 'auto',
    attempts: [],
    content: '',
    records: [],
    values: {} as DbTableRecord,
    dirty: false,
  };
}

function toResultItem(result: {
  file_id?: string;
  file_name?: string;
  message: string;
  records: DbTableRecord[];
  content?: string;
  extraction?: FileIngestExtractionInfo;
  error?: string;
}): BatchIngestResultItem {
  return {
    file_id: result.file_id || '',
    file_name: result.file_name || '',
    message: result.message,
    records: result.records || [],
    content: result.content,
    extraction: result.extraction,
    error: result.error,
  };
}

const StepTwo: React.FC<IngestStepTwoProps> = ({
  selectedFiles,
  selectedModel,
  dbId,
  tableId,
  modelSupportsVision = false,
  onRemoveFile,
  onLeaveGuardChange,
}) => {
  const t = useT('dbs');
  const router = useRouter();

  const [activeFileId, setActiveFileId] = useState<string | undefined>(selectedFiles[0]?.id);
  const [columns, setColumns] = useState<DbTableColumn[]>([]);
  const [fileStates, setFileStates] = useState<Record<string, FileRecognitionState>>({});
  const [tsDrafts, setTsDrafts] = useState<Record<string, string>>({});
  const [filter, setFilter] = useState<FileFilter>('all');
  const [contentTab, setContentTab] = useState<ContentTab>('text');
  const [now, setNow] = useState(() => Date.now());

  const fileStatesRef = useRef(fileStates);
  useEffect(() => {
    fileStatesRef.current = fileStates;
  }, [fileStates]);

  const {
    prompt,
    isLoading: promptLoading,
    error: promptError,
    refetch: refetchPrompt,
  } = useDbTablePrompt(dbId, tableId, {
    staleTime: 60_000,
    gcTime: 600_000,
  });

  const { ingestFile } = useIngestFileToTable(dbId, tableId);

  const listParams: GetDbTableRecordsParams = { limit: 20, offset: 0, order: 'id DESC' };
  const { createRecords, isPending: saving } = useCreateDbTableRecords(dbId, tableId, listParams);

  const selectedFileIds = useMemo(() => selectedFiles.map(f => f.id), [selectedFiles]);
  const selectedFileIdsKey = useMemo(() => selectedFileIds.join('|'), [selectedFileIds]);
  const firstSelectedFileId = selectedFiles[0]?.id;
  const activeState = activeFileId ? fileStates[activeFileId] : undefined;
  const activeFile = activeState?.file || selectedFiles.find(file => file.id === activeFileId);
  const activeFileIsImage = Boolean(activeFile && isTableIngestImageFile(activeFile));
  const activeFileIsPDF = isPdfFile(activeFile);
  const activeFileCanUseVision = activeFileIsImage || activeFileIsPDF;
  const activePreviewableOriginal = activeFileIsImage || activeFileIsPDF;

  const {
    previewUrl: activeOriginalPreviewUrl,
    isLoading: activeOriginalPreviewLoading,
    error: activeOriginalPreviewError,
  } = useFileOriginalPreviewUrl(activeFile?.id, {
    enabled: activePreviewableOriginal && Boolean(activeFile?.id),
  });

  useEffect(() => {
    setContentTab(activePreviewableOriginal ? 'original' : 'text');
  }, [activeFileId, activePreviewableOriginal]);

  const promptUnavailable =
    Boolean(promptError) || (!promptLoading && !String(prompt || '').trim());
  const promptUnavailableMessage = promptError
    ? promptError
    : t('tableIngest.stepTwo.promptEmptyDesc');

  useEffect(() => {
    setFileStates(prev => {
      const next: Record<string, FileRecognitionState> = {};
      selectedFiles.forEach(file => {
        next[file.id] = prev[file.id] ? { ...prev[file.id], file } : createInitialFileState(file);
      });
      return next;
    });
    setActiveFileId(prev => (prev && selectedFileIds.includes(prev) ? prev : firstSelectedFileId));
  }, [firstSelectedFileId, selectedFileIds, selectedFileIdsKey, selectedFiles]);

  const hasRequiredValue = useCallback(
    (v: DbTableRecord[keyof DbTableRecord], type: Type): boolean => {
      if (v === null || typeof v === 'undefined') return false;
      switch (type) {
        case Type.Text:
          return typeof v !== 'string' ? true : v.trim().length > 0;
        case Type.Timestamp:
          return isValidTimestampRecordValue(v);
        case Type.Boolean:
          return typeof v === 'boolean';
        case Type.Integer:
        case Type.Numeric:
          return typeof v === 'number' && !Number.isNaN(v as number);
        default:
          if (Array.isArray(v as unknown)) return ((v as unknown[]) || []).length > 0;
          return true;
      }
    },
    []
  );

  const hasInvalidTimestampValue = useCallback((values: DbTableRecord): boolean => {
    return columns.some(col => {
      if (col.type !== Type.Timestamp) return false;
      const value = values[col.name] as DbTableRecord[keyof DbTableRecord];
      return (
        value !== null &&
        typeof value !== 'undefined' &&
        String(value).trim().length > 0 &&
        !isValidTimestampRecordValue(value)
      );
    });
  }, [columns]);

  const requiredFieldsComplete = useCallback(
    (values: DbTableRecord): boolean =>
      columns.every(col => {
        if (!col.is_required) return true;
        return hasRequiredValue(values[col.name] as DbTableRecord[keyof DbTableRecord], col.type);
      }),
    [columns, hasRequiredValue]
  );

  const getEffectiveStatus = useCallback(
    (state?: FileRecognitionState): RecognitionStatus => {
      if (!state) return 'idle';
      if (
        state.status === 'idle' ||
        state.status === 'queued' ||
        state.status === 'recognizing' ||
        state.status === 'failed' ||
        state.status === 'skipped'
      ) {
        return state.status;
      }
      if (columns.length === 0) return state.status;
      if (!requiredFieldsComplete(state.values) || hasInvalidTimestampValue(state.values)) {
        return 'warning';
      }
      return 'success';
    },
    [columns.length, hasInvalidTimestampValue, requiredFieldsComplete]
  );

  const applyResultToFile = useCallback(
    (
      fileId: string,
      requestId: string,
      mode: FileIngestExtractionMode,
      item: BatchIngestResultItem
    ) => {
      const content = item.content || '';
      const records = item.records || [];
      const firstRecord = (records[0] || {}) as DbTableRecord;
      const error =
        item.error || (records.length === 0 && !content ? item.message || t('tableIngest.stepTwo.fileErrorFallback') : undefined);
      const warning =
        !error && records.length === 0
          ? item.message || t('tableIngest.stepTwo.noRecordWarning')
          : undefined;
      const status: RecognitionStatus = error ? 'failed' : records.length > 0 ? 'success' : 'warning';
      const attempt: RecognitionAttempt = {
        id: requestId,
        mode,
        status,
        content,
        records,
        error,
        warning,
        extraction: item.extraction,
        createdAt: Date.now(),
      };

      setFileStates(prev => {
        const current = prev[fileId];
        if (!current || current.requestId !== requestId) return prev;
        return {
          ...prev,
          [fileId]: {
            ...current,
            status,
            requestedMode: mode,
            activeAttemptId: requestId,
            attempts: [...current.attempts, attempt],
            content,
            records,
            values: firstRecord,
            error,
            warning,
            extraction: item.extraction,
            dirty: false,
            requestId: undefined,
            requestStartedAt: undefined,
          },
        };
      });
    },
    [t]
  );

  const runRecognitionForFile = useCallback(
    async (fileId: string, mode: FileIngestExtractionMode, reset = false) => {
      if (promptUnavailable) return;
      const requestId = `${fileId}:${mode}:${Date.now()}:${Math.random().toString(36).slice(2)}`;
      setFileStates(prev => {
        const current = prev[fileId];
        if (!current) return prev;
        return {
          ...prev,
          [fileId]: {
            ...current,
            status: 'recognizing',
            requestedMode: mode,
            content: reset ? '' : current.content,
            records: reset ? [] : current.records,
            values: reset ? ({} as DbTableRecord) : current.values,
            error: undefined,
            warning: undefined,
            dirty: reset ? false : current.dirty,
            requestId,
            requestStartedAt: Date.now(),
          },
        };
      });

      try {
        const { columns: nextColumns, result } = await ingestFile({
          file_id: fileId,
          prompt: prompt || '',
          table_id: tableId,
          model: { provider: selectedModel.provider, name: selectedModel.model },
          extraction_mode: mode,
        });
        if (nextColumns.length > 0) {
          setColumns(nextColumns.filter(c => !c.is_system_field));
        }
        applyResultToFile(fileId, requestId, mode, toResultItem(result));
      } catch (err) {
        const message = (err as Error)?.message || t('tableIngest.stepTwo.fileErrorFallback');
        setFileStates(prev => {
          const current = prev[fileId];
          if (!current || current.requestId !== requestId) return prev;
          const attempt: RecognitionAttempt = {
            id: requestId,
            mode,
            status: 'failed',
            content: current.content,
            records: current.records,
            error: message,
            extraction: current.extraction,
            createdAt: Date.now(),
          };
          return {
            ...prev,
            [fileId]: {
              ...current,
              status: 'failed',
              error: message,
              warning: undefined,
              attempts: [...current.attempts, attempt],
              activeAttemptId: requestId,
              requestId: undefined,
              requestStartedAt: undefined,
            },
          };
        });
      }
    },
    [
      applyResultToFile,
      ingestFile,
      prompt,
      promptUnavailable,
      selectedModel.model,
      selectedModel.provider,
      tableId,
      t,
    ]
  );

  const hasOverwriteRisk = useCallback((fileIds: string[]) => {
    const states = fileStatesRef.current;
    return fileIds.some(id => {
      const state = states[id];
      return Boolean(state?.dirty || state?.content || state?.records.length || state?.attempts.length);
    });
  }, []);

  const confirmOverwrite = useCallback(
    (fileIds: string[], message: string) => {
      if (!hasOverwriteRisk(fileIds)) return true;
      return window.confirm(message);
    },
    [hasOverwriteRisk]
  );

  const runRecognitionForFiles = useCallback(
    async (fileIds: string[], mode: FileIngestExtractionMode, reset = false) => {
      const ids = fileIds.filter(id => Boolean(fileStatesRef.current[id]));
      if (ids.length === 0) return;
      setFileStates(prev => {
        const next = { ...prev };
        ids.forEach(id => {
          const current = next[id];
          if (!current) return;
          next[id] = { ...current, status: 'queued', requestedMode: mode };
        });
        return next;
      });

      let cursor = 0;
      const workerCount = Math.min(2, ids.length);
      await Promise.all(
        Array.from({ length: workerCount }, async () => {
          while (cursor < ids.length) {
            const fileId = ids[cursor];
            cursor += 1;
            await runRecognitionForFile(fileId, mode, reset);
          }
        })
      );
    },
    [runRecognitionForFile]
  );

  const initialRunKeyRef = useRef('');
  const initialRunKey = useMemo(
    () =>
      JSON.stringify({
        tableId,
        prompt: prompt || '',
        model: {
          provider: selectedModel.provider,
          name: selectedModel.model,
        },
        fileIds: selectedFileIdsKey,
      }),
    [prompt, selectedFileIdsKey, selectedModel.model, selectedModel.provider, tableId]
  );

  useEffect(() => {
    if (promptLoading || promptUnavailable || selectedFileIds.length === 0) return;
    if (initialRunKeyRef.current === initialRunKey) return;
    initialRunKeyRef.current = initialRunKey;
    setColumns([]);
    setTsDrafts({});
    void runRecognitionForFiles(selectedFileIds, 'auto', true);
  }, [
    initialRunKey,
    promptLoading,
    promptUnavailable,
    runRecognitionForFiles,
    selectedFileIds,
  ]);

  const updateValue = (col: DbTableColumn, v: DbTableRecord[keyof DbTableRecord]) => {
    if (!activeFileId) return;
    setFileStates(prev => {
      const current = prev[activeFileId];
      if (!current) return prev;
      return {
        ...prev,
        [activeFileId]: {
          ...current,
          values: { ...current.values, [col.name]: v },
          dirty: true,
        },
      };
    });
  };

  const retryCurrent = useCallback(
    (mode: FileIngestExtractionMode) => {
      if (!activeFileId) return;
      if (mode === 'vision') {
        if (!activeFileCanUseVision) {
          toast.error(t('tableIngest.stepTwo.visionUnsupportedFile'));
          return;
        }
        if (!modelSupportsVision) {
          toast.error(t('tableIngest.stepTwo.visionModelRequired'));
          return;
        }
      }
      if (!confirmOverwrite([activeFileId], t('tableIngest.stepTwo.confirmOverwriteCurrent'))) return;
      void runRecognitionForFile(activeFileId, mode, true);
    },
    [
      activeFileCanUseVision,
      activeFileId,
      confirmOverwrite,
      modelSupportsVision,
      runRecognitionForFile,
      t,
    ]
  );

  const retryFailedFiles = useCallback(() => {
    const failedIds = selectedFileIds.filter(id => getEffectiveStatus(fileStatesRef.current[id]) === 'failed');
    if (failedIds.length === 0) {
      toast.info(t('tableIngest.stepTwo.noFailedFiles'));
      return;
    }
    void runRecognitionForFiles(failedIds, 'auto', true);
  }, [getEffectiveStatus, runRecognitionForFiles, selectedFileIds, t]);

  const retryAllFiles = useCallback(() => {
    if (!confirmOverwrite(selectedFileIds, t('tableIngest.stepTwo.confirmOverwriteAll'))) return;
    setTsDrafts({});
    void runRecognitionForFiles(selectedFileIds, 'auto', true);
  }, [confirmOverwrite, runRecognitionForFiles, selectedFileIds, t]);

  const skipCurrentFile = useCallback(() => {
    if (!activeFileId) return;
    setFileStates(prev => {
      const current = prev[activeFileId];
      if (!current) return prev;
      return {
        ...prev,
        [activeFileId]: {
          ...current,
          status: 'skipped',
          error: undefined,
          warning: undefined,
          requestId: undefined,
          requestStartedAt: undefined,
        },
      };
    });
  }, [activeFileId]);

  const removeCurrentFile = useCallback(() => {
    if (!activeFileId || !onRemoveFile) return;
    onRemoveFile(activeFileId);
  }, [activeFileId, onRemoveFile]);

  const activeValues = useMemo(
    () => activeState?.values || ({} as DbTableRecord),
    [activeState?.values]
  );
  const activeContent = activeState?.content || '';
  const activeError = activeState?.error || '';
  const activeWarning = activeState?.warning || '';
  const activeExtraction = activeState?.extraction;
  const activeEffectiveStatus = getEffectiveStatus(activeState);
  const activeRecognizing = activeEffectiveStatus === 'queued' || activeEffectiveStatus === 'recognizing';
  const activeBackendAttempts = activeExtraction?.attempts || [];
  const activeElapsedMs =
    activeState?.requestStartedAt && activeRecognizing
      ? Math.max(0, now - activeState.requestStartedAt)
      : 0;

  const attemptMethodLabel = useCallback(
    (method?: string) => {
      switch (method) {
        case 'model_vision':
        case 'vision':
          return t('tableIngest.stepTwo.methodLabels.modelVision');
        case 'file_parse':
        case 'mineru':
        case 'hyper_parse_mineru':
          return t('tableIngest.stepTwo.methodLabels.fileParse');
        default:
          return method || t('tableIngest.stepTwo.parseDetails.empty');
      }
    },
    [t]
  );

  const extractionMethodLabel = useCallback(
    (extraction?: FileIngestExtractionInfo) => {
      if (!extraction) return '';
      if (
        extraction.actual_strategy === 'vision' ||
        extraction.source_type === 'image_original' ||
        extraction.source_type === 'pdf_rendered_pages'
      ) {
        return t('tableIngest.stepTwo.methodLabels.modelVision');
      }
      return t('tableIngest.stepTwo.methodLabels.fileParse');
    },
    [t]
  );

  const attemptResultLabel = useCallback(
    (attempt: FileIngestAttempt) => {
      if (attempt.status === 'failed') return t('tableIngest.stepTwo.attemptResults.error');
      switch (attempt.result) {
        case 'records':
          return t('tableIngest.stepTwo.attemptResults.records', {
            count: attempt.record_count || 0,
          });
        case 'no_records':
          return t('tableIngest.stepTwo.attemptResults.noRecords');
        case 'empty_content':
          return t('tableIngest.stepTwo.attemptResults.emptyContent');
        case 'content':
          return t('tableIngest.stepTwo.attemptResults.content');
        case 'error':
          return t('tableIngest.stepTwo.attemptResults.error');
        default:
          return attempt.result || t('tableIngest.stepTwo.parseDetails.empty');
      }
    },
    [t]
  );

  const attemptStatusLabel = useCallback(
    (status?: string) => {
      switch (status) {
        case 'completed':
          return t('tableIngest.stepTwo.attemptStatuses.completed');
        case 'failed':
          return t('tableIngest.stepTwo.attemptStatuses.failed');
        default:
          return status || t('tableIngest.stepTwo.parseDetails.empty');
      }
    },
    [t]
  );

  const formatAttemptDuration = useCallback(
    (durationMs?: number) => {
      const ms = Number(durationMs || 0);
      if (!ms) return '';
      if (ms >= 1000) {
        return t('tableIngest.stepTwo.parseDetails.durationSeconds', {
          seconds: Math.round(ms / 1000),
        });
      }
      return t('tableIngest.stepTwo.parseDetails.durationMs', { ms });
    },
    [t]
  );

  const activeLoadingCopy = useMemo(() => {
    if (activeEffectiveStatus === 'queued') {
      return {
        title: t('tableIngest.stepTwo.loadingStates.queuedTitle'),
        desc: t('tableIngest.stepTwo.loadingStates.queuedDesc'),
      };
    }
    if (activeState?.requestedMode === 'vision') {
      return {
        title: t('tableIngest.stepTwo.loadingStates.visionTitle'),
        desc:
          activeElapsedMs > 30_000
            ? t('tableIngest.stepTwo.loadingStates.longRunningDesc')
            : t('tableIngest.stepTwo.loadingStates.visionDesc'),
      };
    }
    if (activeElapsedMs > 30_000) {
      return {
        title: t('tableIngest.stepTwo.loadingStates.longRunningTitle'),
        desc: t('tableIngest.stepTwo.loadingStates.longRunningDesc'),
      };
    }
    if (activeElapsedMs > 10_000) {
      return {
        title: t('tableIngest.stepTwo.loadingStates.fileParseSlowTitle'),
        desc: t('tableIngest.stepTwo.loadingStates.fileParseSlowDesc'),
      };
    }
    return {
      title: t('tableIngest.stepTwo.loadingStates.fileParseTitle'),
      desc: t('tableIngest.stepTwo.loadingStates.fileParseDesc'),
    };
  }, [activeEffectiveStatus, activeElapsedMs, activeState?.requestedMode, t]);

  const activeFallbackReasonLabel =
    activeExtraction?.fallback_reason === 'mineru_zero_records'
      ? t('tableIngest.stepTwo.fallbackReasons.mineruZeroRecords')
      : activeExtraction?.fallback_reason;
  const activeExtractionMethodLabel = extractionMethodLabel(activeExtraction);
  const activeExtractionBaseLabel = activeExtractionMethodLabel
    ? activeFallbackReasonLabel && activeExtraction?.fallback_reason !== 'none'
      ? t('tableIngest.stepTwo.extractionFallback', {
          strategy: activeExtractionMethodLabel,
          reason: activeFallbackReasonLabel,
        })
      : t('tableIngest.stepTwo.extractionStrategy', {
          strategy: activeExtractionMethodLabel,
        })
    : '';
  const activeExtractionSourceLabel =
    activeExtraction?.source_type === 'image_original'
      ? t('tableIngest.stepTwo.sourceTypes.imageOriginal')
      : activeExtraction?.source_type === 'pdf_rendered_pages'
        ? t('tableIngest.stepTwo.sourceTypes.pdfRenderedPages')
        : activeExtraction?.source_type === 'mineru'
          ? t('tableIngest.stepTwo.sourceTypes.mineru')
          : activeExtraction?.source_type;
  const activeExtractionLabel = [activeExtractionBaseLabel, activeExtractionSourceLabel]
    .filter(Boolean)
    .join(' · ');
  const activeExtractionDebugTitle = [
    activeExtractionLabel,
    activeFileId ? `file_id: ${activeFileId}` : '',
    activeExtraction?.content_hash ? `content_hash: ${activeExtraction.content_hash}` : '',
  ]
    .filter(Boolean)
    .join('\n');
  const activeErrorHint =
    activeExtraction?.fallback_reason === 'mineru_zero_records'
      ? t('tableIngest.stepTwo.mineruZeroRecordsHint')
      : '';

  const highlightTerms = useMemo(() => {
    const set = new Set<string>();
    Object.entries(activeValues).forEach(([key, value]) => {
      if (key === 'id' || value === null || typeof value === 'undefined') return;
      if (Array.isArray(value)) {
        (value as Array<string | number>).forEach(v => {
          const s = String(v).trim();
          if (s.length > 1) set.add(s);
        });
      } else {
        const s = String(value).trim();
        if (s.length > 1) set.add(s);
      }
    });
    return Array.from(set).slice(0, 50);
  }, [activeValues]);

  const recognizedForColumn = (col: DbTableColumn): boolean => {
    const source = activeState?.records[0];
    const value = source?.[col.name];
    if (col.type === Type.Timestamp) {
      return isValidTimestampRecordValue(value as DbTableRecord[keyof DbTableRecord]);
    }
    return typeof value !== 'undefined' && value !== null && String(value).trim().length > 0;
  };

  const fieldReviewStats = useMemo(() => {
    return columns.reduce(
      (acc, col) => {
        const value = activeValues?.[col.name] as DbTableRecord[keyof DbTableRecord];
        const hasValue = hasRequiredValue(value, col.type);
        const invalidTimestamp =
          col.type === Type.Timestamp &&
          value !== null &&
          typeof value !== 'undefined' &&
          String(value).trim().length > 0 &&
          !isValidTimestampRecordValue(value);

        if (hasValue && !invalidTimestamp) acc.recognized += 1;
        if (col.is_required && !hasValue) acc.needs += 1;
        if (invalidTimestamp) acc.invalid += 1;
        return acc;
      },
      { recognized: 0, needs: 0, invalid: 0 }
    );
  }, [activeValues, columns, hasRequiredValue]);

  const visibleFiles = useMemo(() => {
    return selectedFiles.filter(file => {
      const status = getEffectiveStatus(fileStates[file.id]);
      switch (filter) {
        case 'needs_action':
          return status === 'warning' || status === 'failed';
        case 'failed':
          return status === 'failed';
        case 'ready':
          return status === 'success';
        case 'all':
        default:
          return true;
      }
    });
  }, [fileStates, filter, getEffectiveStatus, selectedFiles]);

  const stats = useMemo(() => {
    return selectedFiles.reduce(
      (acc, file) => {
        const status = getEffectiveStatus(fileStates[file.id]);
        if (status === 'queued' || status === 'recognizing') acc.processing += 1;
        if (status === 'success') acc.ready += 1;
        if (status === 'warning') acc.needs += 1;
        if (status === 'failed') acc.failed += 1;
        return acc;
      },
      { processing: 0, ready: 0, needs: 0, failed: 0 }
    );
  }, [fileStates, getEffectiveStatus, selectedFiles]);

  const anyFileValid = useMemo(
    () => selectedFiles.some(file => getEffectiveStatus(fileStates[file.id]) === 'success'),
    [fileStates, getEffectiveStatus, selectedFiles]
  );
  const anyFileFailed = stats.needs > 0 || stats.failed > 0;
  const hasProcessingFiles = stats.processing > 0;
  const hasUnsavedResults = useMemo(
    () =>
      selectedFiles.some(file => {
        const state = fileStates[file.id];
        if (!state) return false;
        if (state.dirty) return true;
        const status = getEffectiveStatus(state);
        return (
          status === 'success' ||
          status === 'warning' ||
          status === 'failed' ||
          Boolean(state.content) ||
          state.records.length > 0
        );
      }),
    [fileStates, getEffectiveStatus, selectedFiles]
  );
  const leaveGuardReason = hasProcessingFiles ? 'processing' : hasUnsavedResults ? 'unsaved' : null;

  useEffect(() => {
    if (!hasProcessingFiles) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [hasProcessingFiles]);

  useEffect(() => {
    onLeaveGuardChange?.(Boolean(leaveGuardReason), leaveGuardReason);
    return () => onLeaveGuardChange?.(false, null);
  }, [leaveGuardReason, onLeaveGuardChange]);

  useEffect(() => {
    if (!leaveGuardReason) return;
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [leaveGuardReason]);

  const onSave = async () => {
    const validFiles = selectedFiles.filter(file => getEffectiveStatus(fileStates[file.id]) === 'success');
    if (validFiles.length === 0) return;

    const buildPayloadForValues = (values: DbTableRecord): Omit<DbTableRecord, 'id'> => {
      const record = columns.reduce<DbTableRecord>((acc, col) => {
        const raw = values[col.name];
        let casted: DbTableRecord[keyof DbTableRecord] = raw as DbTableRecord[keyof DbTableRecord];
        switch (col.type) {
          case Type.Integer:
          case Type.Numeric:
            casted = raw === '' || raw === null || typeof raw === 'undefined' ? null : Number(raw);
            break;
          case Type.Boolean:
            casted = String(raw).toLowerCase() === 'true';
            break;
          case Type.Timestamp:
            casted =
              raw === '' || raw === null || typeof raw === 'undefined'
                ? null
                : (raw as DbTableRecord[keyof DbTableRecord]);
            break;
          case Type.Text:
          default:
            casted = (raw ?? '') as DbTableRecord[keyof DbTableRecord];
        }
        acc[col.name] = casted;
        return acc;
      }, {} as DbTableRecord);
      const { _id, ...rest } = record as DbTableRecord;
      return rest as Omit<DbTableRecord, 'id'>;
    };

    const payloads = validFiles.map(file =>
      buildPayloadForValues(fileStates[file.id]?.values || ({} as DbTableRecord))
    );

    try {
      await createRecords(payloads);
      onLeaveGuardChange?.(false, null);
      router.push(`/console/db/${dbId}/table/${tableId}`);
    } catch (_err) {
      // Error toast handled in hook
    }
  };

  const statusLabel = (status: RecognitionStatus): string => {
    switch (status) {
      case 'queued':
        return t('tableIngest.stepTwo.fileStatus.queued');
      case 'recognizing':
        return t('tableIngest.stepTwo.fileStatus.recognizing');
      case 'success':
        return t('tableIngest.stepTwo.fileStatus.success');
      case 'warning':
        return t('tableIngest.stepTwo.fileStatus.needsCompletion');
      case 'failed':
        return t('tableIngest.stepTwo.fileStatus.parseFailed');
      case 'skipped':
        return t('tableIngest.stepTwo.fileStatus.skipped');
      case 'idle':
      default:
        return t('tableIngest.stepTwo.fileStatus.pending');
    }
  };

  const statusClass = (status: RecognitionStatus) =>
    cn(
      status === 'success' && 'text-success',
      status === 'failed' && 'text-destructive',
      status === 'warning' && 'text-warning',
      (status === 'queued' || status === 'recognizing') && 'text-highlight'
    );

  return (
    <>
      <div className="mb-2 flex min-h-10 flex-wrap items-center gap-2 rounded-md border bg-background px-3 py-1.5 text-xs">
        <span className="mr-1 font-medium text-foreground">
          {t('tableIngest.stepTwo.workspaceTitle')}
        </span>
        <Badge variant="secondary">{t('tableIngest.stepTwo.stats.processing', { count: stats.processing })}</Badge>
        <Badge variant="secondary" className="text-success">
          {t('tableIngest.stepTwo.stats.ready', { count: stats.ready })}
        </Badge>
        <Badge variant="secondary" className="text-warning">
          {t('tableIngest.stepTwo.stats.needs', { count: stats.needs })}
        </Badge>
        <Badge variant="secondary" className="text-destructive">
          {t('tableIngest.stepTwo.stats.failed', { count: stats.failed })}
        </Badge>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <Button type="button" variant="outline" size="sm" onClick={retryFailedFiles} disabled={promptUnavailable || stats.failed === 0}>
            <RotateCcw className="h-4 w-4" />
            {t('tableIngest.stepTwo.retryFailedFiles')}
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={retryAllFiles} disabled={promptUnavailable || selectedFiles.length === 0}>
            <RefreshCcw className="h-4 w-4" />
            {t('tableIngest.stepTwo.reRecognizeAll')}
          </Button>
        </div>
      </div>

      {!promptUnavailable && hasProcessingFiles ? (
        <Alert className="mb-2 text-sm">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t('tableIngest.stepTwo.processingLeaveHint')}</AlertDescription>
        </Alert>
      ) : null}
      {!promptUnavailable && !hasProcessingFiles && hasUnsavedResults ? (
        <Alert className="mb-2 text-sm">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t('tableIngest.stepTwo.unsavedLeaveHint')}</AlertDescription>
        </Alert>
      ) : null}

      {!promptUnavailable && anyFileFailed && (
        <Alert className="mb-2 text-sm" variant={stats.failed > 0 ? 'destructive' : 'default'}>
          <AlertDescription>{t('tableIngest.stepTwo.validationAlert')}</AlertDescription>
        </Alert>
      )}
      {promptUnavailable ? (
        <Alert className="mb-2 text-sm" variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
              <div>
                <div className="font-medium">{t('tableIngest.stepTwo.promptLoadFailedTitle')}</div>
                <div className="mt-1 break-words">
                  {promptError
                    ? t('tableIngest.stepTwo.promptLoadFailedDesc', {
                        error: promptUnavailableMessage,
                      })
                    : promptUnavailableMessage}
                </div>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="shrink-0"
                disabled={promptLoading}
                onClick={() => void refetchPrompt()}
              >
                {t('tableIngest.stepTwo.retryPrompt')}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="grid h-0 min-h-0 grow grid-cols-[280px_minmax(620px,1fr)_420px] overflow-hidden rounded-md border bg-background">
        <div className="min-w-0 flex flex-col border-r overflow-hidden">
          <div className="px-3 py-2 border-b">
            <div className="flex items-center">
              <span className="font-medium text-sm">
                {t('tableIngest.stepTwo.leftPanelTitle')}
                <span className="ml-1">({selectedFiles.length})</span>
              </span>
            </div>
            {selectedFiles.length > 1 ? (
              <div className="mt-2 grid grid-cols-4 gap-1">
                {(['all', 'needs_action', 'failed', 'ready'] as FileFilter[]).map(item => (
                  <Button
                    key={item}
                    type="button"
                    variant={filter === item ? 'default' : 'outline'}
                    size="sm"
                    className="h-7 px-1 text-xs"
                    onClick={() => setFilter(item)}
                  >
                    {t(`tableIngest.stepTwo.filters.${item}`)}
                  </Button>
                ))}
              </div>
            ) : null}
          </div>
          <div className="p-2 overflow-auto space-y-2">
            {visibleFiles.map(file => {
              const state = fileStates[file.id];
              const status = getEffectiveStatus(state);
              const extractionSource = extractionMethodLabel(state?.extraction);
              return (
                <button
                  key={file.id}
                  className={cn(
                    'w-full flex items-center gap-3 rounded-md border px-3 py-2 text-left',
                    'hover:bg-highlight/5 hover:border-highlight/50',
                    activeFileId === file.id && 'bg-highlight/10 border-highlight text-highlight'
                  )}
                  onClick={() => setActiveFileId(file.id)}
                >
                  <FileIcon filename={file.name} className="shrink-0" />
                  <div className="min-w-0 grow">
                    <div className="truncate text-sm font-medium" title={file.name}>
                      {file.name}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-2 text-xs">
                      <span className="text-muted-foreground">{formatFileSize(file.size)}</span>
                      <span className={statusClass(status)}>{statusLabel(status)}</span>
                      {extractionSource ? (
                        <Badge variant="secondary" className="h-5 px-1.5 text-[11px]">
                          {extractionSource}
                        </Badge>
                      ) : null}
                    </div>
                  </div>
                  <div className="shrink-0">
                    {status === 'success' ? (
                      <CheckCircle className="w-4 h-4 text-success" />
                    ) : status === 'failed' ? (
                      <AlertCircle className="w-4 h-4 text-destructive" />
                    ) : status === 'warning' ? (
                      <AlertCircle className="w-4 h-4 text-warning" />
                    ) : status === 'queued' || status === 'recognizing' ? (
                      <LoaderCircle className="w-4 h-4 animate-spin text-highlight" />
                    ) : status === 'skipped' ? (
                      <XCircle className="w-4 h-4 text-muted-foreground" />
                    ) : null}
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        <div className="min-w-0 flex flex-col overflow-hidden">
          <Tabs
            value={contentTab}
            onValueChange={value => setContentTab(value as ContentTab)}
            className="flex h-full min-h-0 flex-col"
          >
            <div className="flex items-center gap-2 border-b px-3 py-2 text-sm font-medium">
              <span className="shrink-0">{t('tableIngest.stepTwo.previewPanelTitle')}</span>
              <TabsList className="h-8 rounded-md p-0.5">
                <TabsTrigger
                  value="original"
                  disabled={!activePreviewableOriginal}
                  className="h-6 rounded px-2 text-xs"
                >
                  {t('tableIngest.stepTwo.contentTabs.original')}
                </TabsTrigger>
                <TabsTrigger value="text" className="h-6 rounded px-2 text-xs">
                  {t('tableIngest.stepTwo.contentTabs.text')}
                </TabsTrigger>
                <TabsTrigger value="details" className="h-6 rounded px-2 text-xs">
                  {t('tableIngest.stepTwo.contentTabs.details')}
                </TabsTrigger>
              </TabsList>
              {activeExtractionLabel ? (
                <Badge
                  variant="secondary"
                  className="ml-auto max-w-[320px] truncate"
                  title={activeExtractionDebugTitle}
                >
                  {activeExtractionLabel}
                </Badge>
              ) : null}
            </div>

            <TabsContent value="original" className="m-0 min-h-0 flex-1 overflow-auto p-3">
              <div className="flex h-full min-h-[420px] items-center justify-center overflow-hidden rounded-md border bg-muted/20">
                {activeOriginalPreviewLoading ? (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <LoaderCircle className="h-4 w-4 animate-spin" />
                    <span>{t('tableIngest.stepTwo.loadingImagePreview')}</span>
                  </div>
                ) : activeOriginalPreviewUrl && activeFileIsImage ? (
                  <MarkdownImage
                    src={activeOriginalPreviewUrl}
                    alt={activeFile?.name || t('tableIngest.stepTwo.originalFilePreview')}
                    className="h-full w-full justify-center"
                    frameClassName="flex h-full min-h-[420px] w-full items-center justify-center border-0 bg-transparent"
                    imageClassName="max-h-full w-full object-contain"
                  />
                ) : activeOriginalPreviewUrl && activeFileIsPDF ? (
                  <iframe
                    title={activeFile?.name || t('tableIngest.stepTwo.originalFilePreview')}
                    src={activeOriginalPreviewUrl}
                    className="h-full min-h-[520px] w-full border-0 bg-background"
                  />
                ) : (
                  <div className="flex flex-col items-center gap-2 text-sm text-muted-foreground">
                    <ImageIcon className="h-5 w-5" />
                    <span>
                      {activeOriginalPreviewError ||
                        t('tableIngest.stepTwo.imagePreviewUnavailable')}
                    </span>
                  </div>
                )}
              </div>
            </TabsContent>

            <TabsContent value="text" className="m-0 min-h-0 flex-1 overflow-auto p-3">
              {promptUnavailable ? (
                <Alert variant="destructive" className="max-w-3xl">
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>
                    <div className="font-medium">
                      {t('tableIngest.stepTwo.promptLoadFailedTitle')}
                    </div>
                    <div className="mt-1 break-words">
                      {promptError
                        ? t('tableIngest.stepTwo.promptLoadFailedDesc', {
                            error: promptUnavailableMessage,
                          })
                        : promptUnavailableMessage}
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="mt-3"
                      disabled={promptLoading}
                      onClick={() => void refetchPrompt()}
                    >
                      {t('tableIngest.stepTwo.retryPrompt')}
                    </Button>
                  </AlertDescription>
                </Alert>
              ) : activeRecognizing && !activeContent ? (
                <IngestLoadingBlock
                  title={activeLoadingCopy.title}
                  desc={activeLoadingCopy.desc}
                />
              ) : (
                <div className="space-y-3">
                  {activeError ? (
                    <Alert variant="destructive" className="max-w-3xl">
                      <AlertCircle className="h-4 w-4" />
                      <AlertDescription>
                        <div className="font-medium">{t('tableIngest.stepTwo.fileErrorTitle')}</div>
                        <div className="mt-1 break-words">{activeError}</div>
                        {activeErrorHint ? (
                          <div className="mt-2 text-xs leading-5">{activeErrorHint}</div>
                        ) : null}
                        <div className="mt-3 flex flex-wrap gap-2">
                          <Button type="button" size="sm" variant="outline" onClick={() => retryCurrent('auto')}>
                            <RefreshCcw className="h-4 w-4" />
                            {t('tableIngest.stepTwo.retryCurrentFile')}
                          </Button>
                          {activeFileCanUseVision ? (
                            <Button type="button" size="sm" variant="outline" onClick={() => retryCurrent('vision')} disabled={!modelSupportsVision}>
                              <Eye className="h-4 w-4" />
                              {t('tableIngest.stepTwo.retryCurrentWithVision')}
                            </Button>
                          ) : null}
                        </div>
                      </AlertDescription>
                    </Alert>
                  ) : null}
                  {activeWarning ? (
                    <Alert className="max-w-3xl border-warning/40 bg-warning/10 text-warning">
                      <AlertCircle className="h-4 w-4" />
                      <AlertDescription>
                        <div className="font-medium">
                          {t('tableIngest.stepTwo.fileWarningTitle')}
                        </div>
                        <div className="mt-1 break-words">{activeWarning}</div>
                        {activeErrorHint ? (
                          <div className="mt-2 text-xs leading-5">{activeErrorHint}</div>
                        ) : null}
                      </AlertDescription>
                    </Alert>
                  ) : null}
                  {activeContent ? (
                    <MarkdownViewer content={activeContent} highlights={highlightTerms} />
                  ) : (
                    <div className="text-sm text-muted-foreground">
                      {t('tableIngest.stepTwo.noPreview')}
                    </div>
                  )}
                </div>
              )}
            </TabsContent>

            <TabsContent value="details" className="m-0 min-h-0 flex-1 overflow-auto p-3">
              <div className="space-y-3 text-sm">
                <div className="rounded-md border bg-muted/20 p-3">
                  <div className="font-medium">{t('tableIngest.stepTwo.parseDetails.title')}</div>
                  <div className="mt-3 grid gap-2 text-xs">
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-muted-foreground">
                        {t('tableIngest.stepTwo.parseDetails.strategy')}
                      </span>
                      <span className="truncate font-medium">
                        {activeExtractionMethodLabel || t('tableIngest.stepTwo.parseDetails.empty')}
                      </span>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-muted-foreground">
                        {t('tableIngest.stepTwo.parseDetails.sourceType')}
                      </span>
                      <span className="truncate font-medium">
                        {activeExtraction?.source_type || t('tableIngest.stepTwo.parseDetails.empty')}
                      </span>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-muted-foreground">
                        {t('tableIngest.stepTwo.parseDetails.fallbackReason')}
                      </span>
                      <span className="truncate font-medium">
                        {activeExtraction?.fallback_reason || t('tableIngest.stepTwo.parseDetails.empty')}
                      </span>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-muted-foreground">
                        {t('tableIngest.stepTwo.parseDetails.contentHash')}
                      </span>
                      <span className="max-w-[360px] truncate font-mono">
                        {activeExtraction?.content_hash || t('tableIngest.stepTwo.parseDetails.empty')}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="rounded-md border bg-background p-3">
                  <div className="font-medium">{t('tableIngest.stepTwo.parseDetails.attempts')}</div>
                  <div className="mt-3 space-y-2">
                    {activeBackendAttempts.length > 0 ? (
                      activeBackendAttempts.map((attempt, index) => (
                        <div key={`${attempt.method}:${index}`} className="flex items-center justify-between gap-3 rounded border px-2 py-1.5 text-xs">
                          <div className="min-w-0">
                            <div className="font-medium">
                              {attemptMethodLabel(attempt.method)}
                            </div>
                            <div className="truncate text-muted-foreground">
                              {[
                                attemptStatusLabel(attempt.status),
                                attemptResultLabel(attempt),
                                formatAttemptDuration(attempt.duration_ms),
                              ]
                                .filter(Boolean)
                                .join(' · ')}
                            </div>
                          </div>
                          <Badge variant="secondary" className={attempt.status === 'failed' ? 'text-destructive' : 'text-success'}>
                            {attemptResultLabel(attempt)}
                          </Badge>
                        </div>
                      ))
                    ) : (activeState?.attempts || []).length > 0 ? (
                      activeState?.attempts.map((attempt, index) => (
                        <div key={attempt.id} className="flex items-center justify-between gap-3 rounded border px-2 py-1.5 text-xs">
                          <div className="min-w-0">
                            <div className="font-medium">
                              {t('tableIngest.stepTwo.parseDetails.attemptIndex', {
                                index: index + 1,
                              })}
                            </div>
                            <div className="truncate text-muted-foreground">
                              {[
                                attemptMethodLabel(attempt.mode === 'vision' ? 'model_vision' : 'file_parse'),
                                statusLabel(attempt.status),
                              ]
                                .filter(Boolean)
                                .join(' · ')}
                            </div>
                          </div>
                          <Badge variant="secondary" className={statusClass(attempt.status)}>
                            {statusLabel(attempt.status)}
                          </Badge>
                        </div>
                      ))
                    ) : (
                      <div className="text-xs text-muted-foreground">
                        {t('tableIngest.stepTwo.parseDetails.noAttempts')}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </TabsContent>
          </Tabs>
        </div>

        <div className="min-w-0 flex flex-col border-l overflow-hidden">
          <div className="px-3 py-2 border-b text-sm font-medium">
            <div className="flex items-center gap-2">
              <span>{t('tableIngest.stepTwo.fieldsPanelTitle')}</span>
              {activeEffectiveStatus ? (
                <span className={cn('text-xs ml-auto', statusClass(activeEffectiveStatus))}>
                  {statusLabel(activeEffectiveStatus)}
                </span>
              ) : null}
            </div>
            <div className="mt-2 flex items-center justify-between gap-2">
              <div className="flex min-w-0 flex-wrap items-center gap-1.5 text-xs">
                <Badge variant="secondary" className="text-success">
                  {t('tableIngest.stepTwo.fieldStats.recognized', {
                    count: fieldReviewStats.recognized,
                  })}
                </Badge>
                <Badge variant="secondary" className="text-warning">
                  {t('tableIngest.stepTwo.fieldStats.needs', { count: fieldReviewStats.needs })}
                </Badge>
                <Badge variant="secondary" className="text-destructive">
                  {t('tableIngest.stepTwo.fieldStats.invalid', {
                    count: fieldReviewStats.invalid,
                  })}
                </Badge>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      isIcon
                      aria-label={t('tableIngest.stepTwo.retryCurrentFile')}
                      onClick={() => retryCurrent('auto')}
                      disabled={!activeFileId || promptUnavailable}
                    >
                      <RefreshCcw className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {t('tableIngest.stepTwo.retryCurrentFile')}
                  </TooltipContent>
                </Tooltip>
              {activeFileCanUseVision ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      isIcon
                      aria-label={t('tableIngest.stepTwo.retryCurrentWithVision')}
                      onClick={() => retryCurrent('vision')}
                      disabled={!activeFileId || promptUnavailable || !modelSupportsVision}
                    >
                      <Eye className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {modelSupportsVision
                      ? t('tableIngest.stepTwo.retryCurrentWithVision')
                      : t('tableIngest.stepTwo.visionModelRequired')}
                  </TooltipContent>
                </Tooltip>
              ) : null}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      isIcon
                      aria-label={t('tableIngest.stepTwo.skipCurrentFile')}
                      onClick={skipCurrentFile}
                      disabled={!activeFileId}
                    >
                      <XCircle className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {t('tableIngest.stepTwo.skipCurrentFile')}
                  </TooltipContent>
                </Tooltip>
              {onRemoveFile ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      isIcon
                      aria-label={t('tableIngest.stepTwo.removeCurrentFile')}
                      onClick={removeCurrentFile}
                      disabled={!activeFileId}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {t('tableIngest.stepTwo.removeCurrentFile')}
                  </TooltipContent>
                </Tooltip>
              ) : null}
              </div>
            </div>
          </div>
          <div className="p-3 space-y-3 overflow-auto h-0 grow">
            {promptUnavailable ? (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>
                  <div className="font-medium">
                    {t('tableIngest.stepTwo.promptLoadFailedTitle')}
                  </div>
                  <div className="mt-1 break-words">
                    {promptError
                      ? t('tableIngest.stepTwo.promptLoadFailedDesc', {
                          error: promptUnavailableMessage,
                        })
                      : promptUnavailableMessage}
                  </div>
                </AlertDescription>
              </Alert>
            ) : activeRecognizing && !activeContent ? (
              <IngestLoadingBlock
                title={activeLoadingCopy.title}
                desc={activeLoadingCopy.desc}
                compact
              />
            ) : activeError && columns.length === 0 ? (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>
                  <div className="font-medium">{t('tableIngest.stepTwo.fileErrorTitle')}</div>
                  <div className="mt-1 break-words">{activeError}</div>
                </AlertDescription>
              </Alert>
            ) : (
              <>
                {activeError ? (
                  <Alert variant="destructive">
                    <AlertCircle className="h-4 w-4" />
                    <AlertDescription>
                      <div className="font-medium">{t('tableIngest.stepTwo.fileErrorTitle')}</div>
                      <div className="mt-1 break-words">{activeError}</div>
                    </AlertDescription>
                  </Alert>
                ) : null}
                {activeWarning ? (
                  <Alert className="border-warning/40 bg-warning/10 text-warning">
                    <AlertCircle className="h-4 w-4" />
                    <AlertDescription>
                      <div className="font-medium">{t('tableIngest.stepTwo.fileWarningTitle')}</div>
                      <div className="mt-1 break-words">{activeWarning}</div>
                    </AlertDescription>
                  </Alert>
                ) : null}
                {columns.map(col => {
                  const recognized = recognizedForColumn(col);
                  const val = (activeValues?.[col.name] ?? '') as DbTableRecord[keyof DbTableRecord];
                  const cellKey = `${String(activeFileId || '')}:${col.name}`;
                  const invalidRequired =
                    col.is_required &&
                    !hasRequiredValue(val as DbTableRecord[keyof DbTableRecord], col.type);
                  const invalidTimestamp =
                    col.type === Type.Timestamp &&
                    val !== null &&
                    typeof val !== 'undefined' &&
                    String(val).trim().length > 0 &&
                    !isValidTimestampRecordValue(val as DbTableRecord[keyof DbTableRecord]);

                  return (
                    <div key={col.id} className="space-y-1">
                      <div className="flex items-center justify-between">
                        {col.description && col.description.trim() ? (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <label
                                className={`text-xs font-medium truncate ${invalidRequired || invalidTimestamp ? 'text-destructive' : ''}`}
                              >
                                {col.name}
                                {col.is_required ? (
                                  <span className="text-destructive ml-1" aria-hidden="true">
                                    *
                                  </span>
                                ) : null}
                              </label>
                            </TooltipTrigger>
                            <TooltipContent side="top" className="max-w-sm">
                              {col.description}
                            </TooltipContent>
                          </Tooltip>
                        ) : (
                          <label
                            className={`text-xs font-medium truncate ${invalidRequired || invalidTimestamp ? 'text-destructive' : ''}`}
                          >
                            {col.name}
                            {col.is_required ? (
                              <span className="text-destructive ml-1" aria-hidden="true">
                                *
                              </span>
                            ) : null}
                          </label>
                        )}
                        <span
                          className={`inline-flex items-center gap-1 text-xs ${recognized ? 'text-green-600' : 'text-muted-foreground'}`}
                        >
                          {recognized ? <Check className="h-3 w-3" /> : null}
                          {recognized
                            ? t('tableIngest.stepTwo.recognizedTag')
                            : t('tableIngest.stepTwo.notRecognized')}
                        </span>
                      </div>

                      {col.type === Type.Boolean ? (
                        <div className="flex items-center">
                          <Switch
                            checked={Boolean(val)}
                            onCheckedChange={checked => updateValue(col, !!checked)}
                            className={invalidRequired ? 'focus-visible:ring-destructive' : undefined}
                          />
                        </div>
                      ) : col.type === Type.Integer || col.type === Type.Numeric ? (
                        <Input
                          type="number"
                          value={typeof val === 'number' ? String(val) : ''}
                          onChange={e => {
                            const next = e.target.value;
                            const num = next === '' ? null : Number(next);
                            updateValue(col, num);
                          }}
                          placeholder={col?.description || ''}
                          className={
                            invalidRequired
                              ? 'border-destructive focus-visible:ring-destructive'
                              : undefined
                          }
                          aria-invalid={invalidRequired || undefined}
                        />
                      ) : col.type === Type.Timestamp ? (
                        <Input
                          type="datetime-local"
                          value={
                            tsDrafts[cellKey] !== undefined
                              ? tsDrafts[cellKey]
                              : typeof val === 'string' && isValidTimestampRecordValue(val)
                                ? formatDateTimeLocalInput(val)
                                : ''
                          }
                          onChange={e => {
                            const local = e.target.value;
                            setTsDrafts(prev => ({ ...prev, [cellKey]: local }));
                            if (local === '') {
                              updateValue(col, '');
                              return;
                            }
                            if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(local)) {
                              const iso = new Date(local).toISOString();
                              updateValue(col, iso);
                            }
                          }}
                          onBlur={e => {
                            const local = e.target.value;
                            if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(local)) {
                              const iso = new Date(local).toISOString();
                              updateValue(col, iso);
                              setTsDrafts(prev => {
                                const next = { ...prev };
                                delete next[cellKey];
                                return next;
                              });
                            }
                          }}
                          placeholder={col?.description || ''}
                          className={
                            invalidRequired || invalidTimestamp
                              ? 'border-destructive focus-visible:ring-destructive'
                              : undefined
                          }
                          aria-invalid={invalidRequired || invalidTimestamp || undefined}
                        />
                      ) : (
                        <Input
                          value={typeof val === 'string' ? val : ''}
                          onChange={e => updateValue(col, e.target.value)}
                          placeholder={col?.description || ''}
                          className={
                            invalidRequired
                              ? 'border-destructive focus-visible:ring-destructive'
                              : undefined
                          }
                          aria-invalid={invalidRequired || undefined}
                        />
                      )}
                      {invalidTimestamp ? (
                        <div className="text-xs text-destructive">
                          {t('tableIngest.stepTwo.timestampInvalidTag')}
                        </div>
                      ) : invalidRequired ? (
                        <div className="text-xs text-destructive">
                          {t('tableIngest.stepTwo.requiredEmptyTag')}
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </>
            )}
          </div>
          <div className="p-3 border-t">
            <Button
              className="w-full"
              onClick={onSave}
              disabled={saving || promptUnavailable || stats.processing > 0 || !anyFileValid}
            >
              {saving ? t('tableIngest.stepTwo.saving') : t('tableIngest.stepTwo.reviewAndSave')}
            </Button>
            <div className="mt-2 text-center text-xs text-muted-foreground">
              {t('tableIngest.stepTwo.saveSafetyHint')}
            </div>
          </div>
        </div>
      </div>
    </>
  );
};

export default StepTwo;
