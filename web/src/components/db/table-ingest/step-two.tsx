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
  ChevronLeft,
  ChevronRight,
  FileSearch,
  LoaderCircle,
  RefreshCcw,
  RotateCcw,
  Trash2,
  XCircle,
} from 'lucide-react';
import type {
  DbTableColumn,
  DbTableRecord,
  FileIngestExtractionInfo,
  FileIngestFieldExtraction,
  FileIngestFieldMatch,
  GetDbTableRecordsParams,
} from '@/services/types/db';
import { Type } from '@/services/types/db';
import { useCreateDbTableRecords } from '@/hooks/db/use-db-table-records';
import { Switch } from '@/components/ui/switch';
import { formatDateTimeLocalInput } from '@/utils/date-input';
import { useT } from '@/i18n';
import { useDbTablePrompt } from '@/hooks/db/use-db-table-prompt';
import {
  useExtractTextToTableRecords,
  useParseFileForTableIngest,
} from '@/hooks/db/use-batch-ingest-file-to-table';
import type { FileItem } from '@/services/types/file';
import { fileManageService } from '@/services/file-manage.service';
import { FileIcon } from '@/components/ui/file-icon';
import { formatFileSize } from '@/utils/format';
import { cn } from '@/lib/utils';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { isOriginalPreviewSupported } from '@/utils/file-helpers';
import {
  FileEvidenceViewer,
  type FileEvidenceViewerLabels,
} from '@/components/content-parse/file-evidence-viewer';

export interface IngestStepTwoProps {
  selectedFiles: FileItem[];
  selectedModel: { provider: string; model: string };
  dbId: string;
  tableId: string;
  onRemoveFile?: (fileId: string) => void;
  onLeaveGuardChange?: (active: boolean, reason: 'processing' | 'unsaved' | null) => void;
}

type RecognitionStatus =
  | 'idle'
  | 'queued'
  | 'recognizing'
  | 'success'
  | 'warning'
  | 'failed'
  | 'skipped';
type FileFilter = 'all' | 'needs_action' | 'failed' | 'ready';
type ContentTab = 'original' | 'text';
type RecognitionMode = 'auto';
type TableIngestStage = 'parse' | 'recognition';

interface RecognitionAttempt {
  id: string;
  mode: RecognitionMode;
  status: RecognitionStatus;
  content: string;
  records: DbTableRecord[];
  error?: string;
  warning?: string;
  extraction?: FileIngestExtractionInfo;
  fieldExtraction?: FileIngestFieldExtraction;
  createdAt: number;
}

interface FileRecognitionState {
  file: FileItem;
  status: RecognitionStatus;
  requestedMode: RecognitionMode;
  activeStage?: TableIngestStage;
  activeAttemptId?: string;
  attempts: RecognitionAttempt[];
  parse: {
    status: RecognitionStatus;
    content: string;
    extraction?: FileIngestExtractionInfo;
    error?: string;
    requestId?: string;
    startedAt?: number;
  };
  recognition: {
    status: RecognitionStatus;
    fieldExtraction?: FileIngestFieldExtraction;
    error?: string;
    warning?: string;
    requestId?: string;
    startedAt?: number;
    sourceContentHash?: string;
  };
  content: string;
  records: DbTableRecord[];
  error?: string;
  warning?: string;
  extraction?: FileIngestExtractionInfo;
  fieldExtraction?: FileIngestFieldExtraction;
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

const IngestRecoveryCard: React.FC<{
  title: string;
  hint: string;
  detailsLabel: string;
  details?: string;
  retryLabel: string;
  onRetry?: () => void;
  compact?: boolean;
}> = ({ title, hint, detailsLabel, details, retryLabel, onRetry, compact = false }) => (
  <div
    role="alert"
    className={cn(
      'rounded-md border border-warning/35 bg-warning/5 text-sm',
      compact ? 'p-3' : 'p-4'
    )}
  >
    <div className="flex gap-3">
      <div
        className={cn(
          'mt-0.5 flex shrink-0 items-center justify-center rounded-md bg-warning/15 text-warning',
          compact ? 'h-7 w-7' : 'h-8 w-8'
        )}
      >
        <AlertCircle className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="font-medium text-foreground">{title}</div>
        <div className="mt-1 leading-6 text-muted-foreground">{hint}</div>
        {onRetry ? (
          <Button type="button" size="sm" variant="outline" className="mt-3" onClick={onRetry}>
            <RefreshCcw className="h-4 w-4" />
            {retryLabel}
          </Button>
        ) : null}
        {details ? (
          <details className="mt-3 text-xs text-muted-foreground">
            <summary className="cursor-pointer select-none">{detailsLabel}</summary>
            <div className="mt-2 whitespace-pre-wrap break-words rounded border bg-background/80 p-2 font-mono leading-5">
              {details}
            </div>
          </details>
        ) : null}
      </div>
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

function normalizeRecognizedValue(
  value: DbTableRecord[keyof DbTableRecord],
  type: Type
): DbTableRecord[keyof DbTableRecord] {
  if (value === null || typeof value === 'undefined') return null;
  switch (type) {
    case Type.Integer:
    case Type.Numeric: {
      if (typeof value === 'number') return Number.isNaN(value) ? null : value;
      const text = String(value).trim().replace(/,/g, '');
      if (!NUMERIC_ONLY_PATTERN.test(text)) return null;
      const num = Number(text);
      if (Number.isNaN(num)) return null;
      return type === Type.Integer && !Number.isInteger(num) ? null : num;
    }
    case Type.Boolean: {
      if (typeof value === 'boolean') return value;
      const text = String(value).trim().toLowerCase();
      if (['true', 'yes', '1', '是'].includes(text)) return true;
      if (['false', 'no', '0', '否'].includes(text)) return false;
      return null;
    }
    case Type.Timestamp: {
      const text = String(value).trim();
      return isValidTimestampRecordValue(text) ? text : null;
    }
    case Type.Text:
    default:
      return value;
  }
}

function normalizeRecognizedRecord(record: DbTableRecord, columns: DbTableColumn[]): DbTableRecord {
  return columns.reduce(
    (acc, col) => {
      acc[col.name] = normalizeRecognizedValue(
        record[col.name] as DbTableRecord[keyof DbTableRecord],
        col.type
      );
      return acc;
    },
    { ...record } as DbTableRecord
  );
}

function hasFieldMatchRawValue(match?: FileIngestFieldMatch): boolean {
  const raw = match?.raw_value ?? match?.value;
  return raw !== null && typeof raw !== 'undefined' && String(raw).trim().length > 0;
}

function formatFieldMatchValue(value: unknown): string {
  if (value === null || typeof value === 'undefined') return '';
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch (_err) {
      return String(value);
    }
  }
  return String(value);
}

function createInitialFileState(file: FileItem): FileRecognitionState {
  return {
    file,
    status: 'idle',
    requestedMode: 'auto',
    attempts: [],
    parse: {
      status: 'idle',
      content: '',
    },
    recognition: {
      status: 'idle',
    },
    content: '',
    records: [],
    dirty: false,
  };
}

const StepTwo: React.FC<IngestStepTwoProps> = ({
  selectedFiles,
  selectedModel,
  dbId,
  tableId,
  onRemoveFile,
  onLeaveGuardChange,
}) => {
  const t = useT('dbs');
  const router = useRouter();

  const [activeFileId, setActiveFileId] = useState<string | undefined>(selectedFiles[0]?.id);
  const [columns, setColumns] = useState<DbTableColumn[]>([]);
  const [fileStates, setFileStates] = useState<Record<string, FileRecognitionState>>({});
  const [activeRecordIndexes, setActiveRecordIndexes] = useState<Record<string, number>>({});
  const [tsDrafts, setTsDrafts] = useState<Record<string, string>>({});
  const [filter, setFilter] = useState<FileFilter>('all');
  const [contentTab, setContentTab] = useState<ContentTab>('text');
  const [now, setNow] = useState(() => Date.now());
  const [overwriteConfirm, setOverwriteConfirm] = useState<{
    message: string;
    onConfirm: () => void;
  } | null>(null);
  const [originalPreview, setOriginalPreview] = useState<{
    fileId?: string;
    url: string;
    loading: boolean;
    error: string | null;
  }>({ url: '', loading: false, error: null });

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

  const { parseFile } = useParseFileForTableIngest();
  const { extractRecords } = useExtractTextToTableRecords(dbId, tableId);

  const listParams: GetDbTableRecordsParams = { limit: 20, offset: 0, order: 'id DESC' };
  const { createRecords, isPending: saving } = useCreateDbTableRecords(dbId, tableId, listParams);

  const selectedFileIds = useMemo(() => selectedFiles.map(f => f.id), [selectedFiles]);
  const selectedFileIdsKey = useMemo(() => selectedFileIds.join('|'), [selectedFileIds]);
  const firstSelectedFileId = selectedFiles[0]?.id;
  const activeState = activeFileId ? fileStates[activeFileId] : undefined;
  const activeFile = activeState?.file || selectedFiles.find(file => file.id === activeFileId);
  const activePreviewableOriginal = Boolean(
    activeFile && isOriginalPreviewSupported(activeFile.extension, activeFile.mime_type)
  );
  const activeOriginalPreviewFile = activeFile
    ? {
        id: activeFile.id,
        name: activeFile.name,
        extension: activeFile.extension,
        mimeType: activeFile.mime_type,
        size: activeFile.size,
      }
    : null;

  const activeOriginalPreviewUrl =
    originalPreview.fileId === activeFile?.id ? originalPreview.url : '';
  const activeOriginalPreviewLoading =
    originalPreview.fileId === activeFile?.id ? originalPreview.loading : false;
  const activeOriginalPreviewError =
    originalPreview.fileId === activeFile?.id ? originalPreview.error : null;

  useEffect(() => {
    setContentTab(activePreviewableOriginal ? 'original' : 'text');
  }, [activeFileId, activePreviewableOriginal]);

  useEffect(() => {
    if (!activePreviewableOriginal || !activeFile?.id) {
      setOriginalPreview({ url: '', loading: false, error: null });
      return;
    }

    let cancelled = false;
    let objectUrl = '';

    setOriginalPreview({
      fileId: activeFile.id,
      url: '',
      loading: true,
      error: null,
    });

    fileManageService
      .downloadFile(activeFile.id)
      .then(blob => {
        objectUrl = URL.createObjectURL(blob);
        if (cancelled) {
          URL.revokeObjectURL(objectUrl);
          return;
        }
        setOriginalPreview({
          fileId: activeFile.id,
          url: objectUrl,
          loading: false,
          error: null,
        });
      })
      .catch(error => {
        if (cancelled) return;
        setOriginalPreview({
          fileId: activeFile.id,
          url: '',
          loading: false,
          error: error instanceof Error ? error.message : 'Failed to load original preview',
        });
      });

    return () => {
      cancelled = true;
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [activeFile?.id, activePreviewableOriginal]);

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

  const hasInvalidTimestampValue = useCallback(
    (values: DbTableRecord): boolean => {
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
    },
    [columns]
  );

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
      if (state.status === 'skipped') return 'skipped';
      if (state.parse.status === 'queued' || state.parse.status === 'recognizing') {
        return state.parse.status;
      }
      if (state.parse.status === 'failed') return 'failed';
      if (
        state.recognition.status === 'queued' ||
        state.recognition.status === 'recognizing' ||
        state.recognition.status === 'failed'
      ) {
        return state.recognition.status;
      }
      if (
        state.status === 'idle' ||
        state.status === 'queued' ||
        state.status === 'recognizing' ||
        state.status === 'failed'
      ) {
        return state.status;
      }
      if (columns.length === 0) return state.status;
      if (
        state.records.length === 0 ||
        state.records.some(
          record => !requiredFieldsComplete(record) || hasInvalidTimestampValue(record)
        )
      ) {
        return 'warning';
      }
      return 'success';
    },
    [columns.length, hasInvalidTimestampValue, requiredFieldsComplete]
  );

  const runRecognitionForFile = useCallback(
    async (fileId: string, content?: string, contentHash?: string, reset = false) => {
      if (promptUnavailable) return;
      const currentState = fileStatesRef.current[fileId];
      const sourceContent =
        typeof content === 'string'
          ? content
          : currentState?.parse.content || currentState?.content || '';
      const sourceContentHash =
        contentHash ||
        currentState?.parse.extraction?.content_hash ||
        currentState?.extraction?.content_hash ||
        '';
      if (!sourceContent.trim()) {
        const message = t('tableIngest.stepTwo.noParsedContentForRecognition');
        setFileStates(prev => {
          const current = prev[fileId];
          if (!current) return prev;
          return {
            ...prev,
            [fileId]: {
              ...current,
              status: 'failed',
              activeStage: 'recognition',
              error: message,
              warning: undefined,
              recognition: {
                ...current.recognition,
                status: 'failed',
                error: message,
                warning: undefined,
              },
            },
          };
        });
        return;
      }

      const requestId = `${fileId}:recognition:${Date.now()}:${Math.random().toString(36).slice(2)}`;
      setFileStates(prev => {
        const current = prev[fileId];
        if (!current) return prev;
        return {
          ...prev,
          [fileId]: {
            ...current,
            status: 'recognizing',
            requestedMode: 'auto',
            activeStage: 'recognition',
            records: reset ? [] : current.records,
            error: undefined,
            warning: undefined,
            dirty: reset ? false : current.dirty,
            recognition: {
              ...current.recognition,
              status: 'recognizing',
              error: undefined,
              warning: undefined,
              requestId,
              startedAt: Date.now(),
              sourceContentHash,
            },
          },
        };
      });

      try {
        const { columns: nextColumns, result } = await extractRecords({
          file_id: fileId,
          content: sourceContent,
          content_hash: sourceContentHash,
          prompt: prompt || '',
          table_id: tableId,
          model: { provider: selectedModel.provider, name: selectedModel.model },
        });
        if (nextColumns.length > 0) {
          setColumns(nextColumns.filter(c => !c.is_system_field));
        }
        const recognitionColumns =
          nextColumns.length > 0 ? nextColumns.filter(c => !c.is_system_field) : columns;
        const records = (result.records || []).map(record =>
          normalizeRecognizedRecord(record as DbTableRecord, recognitionColumns)
        );
        const error = result.error;
        const warning =
          !error && records.length === 0
            ? result.message || t('tableIngest.stepTwo.noRecordWarning')
            : undefined;
        const status: RecognitionStatus = error
          ? 'failed'
          : records.length > 0
            ? 'success'
            : 'warning';
        const responseHash = result.content_hash || sourceContentHash;

        setFileStates(prev => {
          const current = prev[fileId];
          if (!current || current.recognition.requestId !== requestId) return prev;
          const currentHash =
            current.parse.extraction?.content_hash || current.extraction?.content_hash || '';
          if (currentHash && responseHash && currentHash !== responseHash) return prev;

          const attempt: RecognitionAttempt = {
            id: requestId,
            mode: 'auto',
            status,
            content: current.content,
            records,
            error,
            warning,
            extraction: current.extraction,
            fieldExtraction: result.field_extraction,
            createdAt: Date.now(),
          };
          return {
            ...prev,
            [fileId]: {
              ...current,
              status,
              activeStage: 'recognition',
              activeAttemptId: requestId,
              attempts: [...current.attempts, attempt],
              records,
              error,
              warning,
              fieldExtraction: result.field_extraction,
              dirty: false,
              recognition: {
                ...current.recognition,
                status,
                fieldExtraction: result.field_extraction,
                error,
                warning,
                requestId: undefined,
                startedAt: undefined,
                sourceContentHash: responseHash,
              },
            },
          };
        });
      } catch (err) {
        const message = (err as Error)?.message || t('tableIngest.stepTwo.fileErrorFallback');
        setFileStates(prev => {
          const current = prev[fileId];
          if (!current || current.recognition.requestId !== requestId) return prev;
          const attempt: RecognitionAttempt = {
            id: requestId,
            mode: 'auto',
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
              activeStage: 'recognition',
              error: message,
              warning: undefined,
              attempts: [...current.attempts, attempt],
              activeAttemptId: requestId,
              recognition: {
                ...current.recognition,
                status: 'failed',
                error: message,
                warning: undefined,
                requestId: undefined,
                startedAt: undefined,
              },
            },
          };
        });
      }
    },
    [
      columns,
      extractRecords,
      prompt,
      promptUnavailable,
      selectedModel.model,
      selectedModel.provider,
      tableId,
      t,
    ]
  );

  const runParseForFile = useCallback(
    async (fileId: string, reset = false) => {
      const requestId = `${fileId}:parse:${Date.now()}:${Math.random().toString(36).slice(2)}`;
      setFileStates(prev => {
        const current = prev[fileId];
        if (!current) return prev;
        return {
          ...prev,
          [fileId]: {
            ...current,
            status: 'recognizing',
            activeStage: 'parse',
            content: reset ? '' : current.content,
            records: reset ? [] : current.records,
            error: undefined,
            warning: undefined,
            extraction: reset ? undefined : current.extraction,
            fieldExtraction: reset ? undefined : current.fieldExtraction,
            dirty: reset ? false : current.dirty,
            parse: {
              ...current.parse,
              status: 'recognizing',
              content: reset ? '' : current.parse.content,
              error: undefined,
              requestId,
              startedAt: Date.now(),
              extraction: reset ? undefined : current.parse.extraction,
            },
            recognition: {
              ...current.recognition,
              status: reset ? 'idle' : current.recognition.status,
              error: reset ? undefined : current.recognition.error,
              warning: reset ? undefined : current.recognition.warning,
              fieldExtraction: reset ? undefined : current.recognition.fieldExtraction,
              requestId: reset ? undefined : current.recognition.requestId,
              startedAt: reset ? undefined : current.recognition.startedAt,
              sourceContentHash: reset ? undefined : current.recognition.sourceContentHash,
            },
          },
        };
      });

      try {
        const result = await parseFile({
          file_id: fileId,
          table_id: tableId,
        });
        const content = result.content || '';
        const error =
          result.error ||
          (!content ? result.message || t('tableIngest.stepTwo.fileErrorFallback') : undefined);
        const status: RecognitionStatus = error ? 'failed' : 'success';
        setFileStates(prev => {
          const current = prev[fileId];
          if (!current || current.parse.requestId !== requestId) return prev;
          const attempt: RecognitionAttempt = {
            id: requestId,
            mode: 'auto',
            status,
            content,
            records: current.records,
            error,
            extraction: result.extraction,
            fieldExtraction: current.fieldExtraction,
            createdAt: Date.now(),
          };
          return {
            ...prev,
            [fileId]: {
              ...current,
              status,
              activeStage: 'parse',
              activeAttemptId: requestId,
              attempts: [...current.attempts, attempt],
              content,
              error,
              warning: undefined,
              extraction: result.extraction,
              parse: {
                status,
                content,
                extraction: result.extraction,
                error,
                requestId: undefined,
                startedAt: undefined,
              },
            },
          };
        });
        if (!error) {
          await runRecognitionForFile(fileId, content, result.extraction?.content_hash, reset);
        }
      } catch (err) {
        const message = (err as Error)?.message || t('tableIngest.stepTwo.fileErrorFallback');
        setFileStates(prev => {
          const current = prev[fileId];
          if (!current || current.parse.requestId !== requestId) return prev;
          const attempt: RecognitionAttempt = {
            id: requestId,
            mode: 'auto',
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
              activeStage: 'parse',
              error: message,
              warning: undefined,
              attempts: [...current.attempts, attempt],
              activeAttemptId: requestId,
              parse: {
                ...current.parse,
                status: 'failed',
                error: message,
                requestId: undefined,
                startedAt: undefined,
              },
            },
          };
        });
      }
    },
    [parseFile, runRecognitionForFile, tableId, t]
  );

  const hasOverwriteRisk = useCallback((fileIds: string[]) => {
    const states = fileStatesRef.current;
    return fileIds.some(id => {
      const state = states[id];
      return Boolean(
        state?.dirty || state?.content || state?.records.length || state?.attempts.length
      );
    });
  }, []);

  const runWithOverwriteConfirm = useCallback(
    (fileIds: string[], message: string, action: () => void) => {
      if (!hasOverwriteRisk(fileIds)) {
        action();
        return;
      }
      setOverwriteConfirm({ message, onConfirm: action });
    },
    [hasOverwriteRisk]
  );

  const runFullRecognitionForFiles = useCallback(
    async (fileIds: string[], reset = false) => {
      const ids = fileIds.filter(id => Boolean(fileStatesRef.current[id]));
      if (ids.length === 0) return;
      setFileStates(prev => {
        const next = { ...prev };
        ids.forEach(id => {
          const current = next[id];
          if (!current) return;
          next[id] = {
            ...current,
            status: 'queued',
            requestedMode: 'auto',
            activeStage: 'parse',
            parse: { ...current.parse, status: 'queued' },
          };
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
            await runParseForFile(fileId, reset);
          }
        })
      );
    },
    [runParseForFile]
  );

  const initialRunKeyRef = useRef('');
  const fileStatesReady = useMemo(
    () => selectedFileIds.length > 0 && selectedFileIds.every(id => Boolean(fileStates[id])),
    [fileStates, selectedFileIds]
  );
  const initialRunKey = useMemo(
    () =>
      JSON.stringify({
        tableId,
        fileIds: selectedFileIdsKey,
      }),
    [selectedFileIdsKey, tableId]
  );

  useEffect(() => {
    if (promptLoading || promptUnavailable || selectedFileIds.length === 0 || !fileStatesReady) {
      return;
    }
    if (initialRunKeyRef.current === initialRunKey) return;
    initialRunKeyRef.current = initialRunKey;
    setColumns([]);
    setTsDrafts({});
    void runFullRecognitionForFiles(selectedFileIds, true);
  }, [
    initialRunKey,
    fileStatesReady,
    promptLoading,
    promptUnavailable,
    runFullRecognitionForFiles,
    selectedFileIds,
  ]);

  const updateValue = (col: DbTableColumn, v: DbTableRecord[keyof DbTableRecord]) => {
    if (!activeFileId) return;
    setFileStates(prev => {
      const current = prev[activeFileId];
      if (!current) return prev;
      const recordIndex = Math.min(
        activeRecordIndexes[activeFileId] || 0,
        Math.max(current.records.length - 1, 0)
      );
      if (!current.records[recordIndex]) return prev;
      const records = current.records.map((record, index) =>
        index === recordIndex ? { ...record, [col.name]: v } : record
      );
      return {
        ...prev,
        [activeFileId]: {
          ...current,
          records,
          dirty: true,
        },
      };
    });
  };

  const retryParseCurrent = useCallback(() => {
    if (!activeFileId) return;
    runWithOverwriteConfirm(
      [activeFileId],
      t('tableIngest.stepTwo.confirmOverwriteParseCurrent'),
      () => {
        void runParseForFile(activeFileId, true);
      }
    );
  }, [activeFileId, runParseForFile, runWithOverwriteConfirm, t]);

  const retryRecognitionCurrent = useCallback(() => {
    if (!activeFileId) return;
    runWithOverwriteConfirm(
      [activeFileId],
      t('tableIngest.stepTwo.confirmOverwriteRecognitionCurrent'),
      () => {
        void runRecognitionForFile(activeFileId, undefined, undefined, true);
      }
    );
  }, [activeFileId, runRecognitionForFile, runWithOverwriteConfirm, t]);

  const reprocessCurrent = useCallback(() => {
    if (!activeFileId) return;
    runWithOverwriteConfirm(
      [activeFileId],
      t('tableIngest.stepTwo.confirmOverwriteCurrent'),
      () => {
        void runParseForFile(activeFileId, true);
      }
    );
  }, [activeFileId, runParseForFile, runWithOverwriteConfirm, t]);

  const retryParseFailedFiles = useCallback(() => {
    const failedIds = selectedFileIds.filter(
      id => fileStatesRef.current[id]?.parse.status === 'failed'
    );
    if (failedIds.length === 0) {
      toast.info(t('tableIngest.stepTwo.noParseFailedFiles'));
      return;
    }
    void runFullRecognitionForFiles(failedIds, true);
  }, [runFullRecognitionForFiles, selectedFileIds, t]);

  const retryRecognitionFailedFiles = useCallback(() => {
    const failedIds = selectedFileIds.filter(
      id =>
        fileStatesRef.current[id]?.recognition.status === 'failed' &&
        fileStatesRef.current[id]?.parse.status !== 'failed'
    );
    if (failedIds.length === 0) {
      toast.info(t('tableIngest.stepTwo.noRecognitionFailedFiles'));
      return;
    }
    failedIds.forEach(id => {
      void runRecognitionForFile(id, undefined, undefined, true);
    });
  }, [runRecognitionForFile, selectedFileIds, t]);

  const retryAllFiles = useCallback(() => {
    runWithOverwriteConfirm(selectedFileIds, t('tableIngest.stepTwo.confirmOverwriteAll'), () => {
      setTsDrafts({});
      void runFullRecognitionForFiles(selectedFileIds, true);
    });
  }, [runFullRecognitionForFiles, runWithOverwriteConfirm, selectedFileIds, t]);

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
          parse: {
            ...current.parse,
            error: undefined,
          },
          recognition: {
            ...current.recognition,
            error: undefined,
            warning: undefined,
          },
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

  const activeRecords = useMemo(() => activeState?.records || [], [activeState?.records]);
  const activeRecordIndex = activeFileId
    ? Math.min(activeRecordIndexes[activeFileId] || 0, Math.max(activeRecords.length - 1, 0))
    : 0;
  const activeValues = useMemo(
    () => activeRecords[activeRecordIndex] || ({} as DbTableRecord),
    [activeRecordIndex, activeRecords]
  );
  const activeContent = activeState?.parse.content || activeState?.content || '';
  const activeParseError = activeState?.parse.error || '';
  const activeRecognitionError = activeState?.recognition.error || '';
  const activeError = activeParseError || activeRecognitionError || activeState?.error || '';
  const activeWarning = activeState?.recognition.warning || activeState?.warning || '';
  const activeEffectiveStatus = getEffectiveStatus(activeState);
  const activeRecognizing =
    activeEffectiveStatus === 'queued' || activeEffectiveStatus === 'recognizing';
  const activeProcessingStage = activeState?.activeStage;
  const activeErrorStage: TableIngestStage | undefined = activeParseError
    ? 'parse'
    : activeRecognitionError
      ? 'recognition'
      : activeState?.activeStage;
  const activeElapsedMs = activeRecognizing
    ? Math.max(
        0,
        now -
          (activeProcessingStage === 'parse'
            ? activeState?.parse.startedAt || now
            : activeState?.recognition.startedAt || now)
      )
    : 0;

  const extractionMethodLabel = useCallback(
    (extraction?: FileIngestExtractionInfo) => {
      if (!extraction) return '';
      return t('tableIngest.stepTwo.methodLabels.fileParse');
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
    if (activeElapsedMs > 30_000) {
      return {
        title: t('tableIngest.stepTwo.loadingStates.longRunningTitle'),
        desc: t('tableIngest.stepTwo.loadingStates.longRunningDesc'),
      };
    }
    if (activeElapsedMs > 10_000) {
      return {
        title:
          activeProcessingStage === 'recognition'
            ? t('tableIngest.stepTwo.loadingStates.textRecognitionSlowTitle')
            : t('tableIngest.stepTwo.loadingStates.fileParseSlowTitle'),
        desc:
          activeProcessingStage === 'recognition'
            ? t('tableIngest.stepTwo.loadingStates.textRecognitionSlowDesc')
            : t('tableIngest.stepTwo.loadingStates.fileParseSlowDesc'),
      };
    }
    return {
      title:
        activeProcessingStage === 'recognition'
          ? t('tableIngest.stepTwo.loadingStates.textRecognitionTitle')
          : t('tableIngest.stepTwo.loadingStates.fileParseTitle'),
      desc:
        activeProcessingStage === 'recognition'
          ? t('tableIngest.stepTwo.loadingStates.textRecognitionDesc')
          : t('tableIngest.stepTwo.loadingStates.fileParseDesc'),
    };
  }, [activeEffectiveStatus, activeElapsedMs, activeProcessingStage, t]);

  const activeErrorHint =
    activeErrorStage === 'parse'
      ? t('tableIngest.stepTwo.parseErrorRetryHint')
      : t('tableIngest.stepTwo.recognitionErrorRetryHint');
  const suppressRequiredErrors = Boolean(activeError && !activeState?.dirty);

  const getPendingFieldCount = useCallback(
    (state?: FileRecognitionState): number => {
      if (!state) return 0;
      return state.records.reduce(
        (total, values) =>
          total +
          columns.reduce((count, col) => {
            const value = values[col.name] as DbTableRecord[keyof DbTableRecord];
            const hasValue = hasRequiredValue(value, col.type);
            const invalidTimestamp =
              col.type === Type.Timestamp &&
              value !== null &&
              typeof value !== 'undefined' &&
              String(value).trim().length > 0 &&
              !isValidTimestampRecordValue(value);
            return count + (col.is_required && !hasValue ? 1 : 0) + (invalidTimestamp ? 1 : 0);
          }, 0),
        0
      );
    },
    [columns, hasRequiredValue]
  );

  const statusLabel = useCallback(
    (status: RecognitionStatus): string => {
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
    },
    [t]
  );

  const statusClass = useCallback(
    (status: RecognitionStatus) =>
      cn(
        status === 'success' && 'text-success',
        status === 'failed' && 'text-destructive',
        status === 'warning' && 'text-warning',
        (status === 'queued' || status === 'recognizing') && 'text-highlight'
      ),
    []
  );

  const stageStatusLabel = useCallback(
    (state?: FileRecognitionState): string => {
      if (!state) return statusLabel('idle');
      if (state.status === 'skipped') return statusLabel('skipped');
      if (state.parse.status === 'queued' || state.parse.status === 'recognizing') {
        return t('tableIngest.stepTwo.stageStatus.fileParsing');
      }
      if (state.parse.status === 'failed') {
        return t('tableIngest.stepTwo.stageStatus.fileParseFailed');
      }
      if (state.recognition.status === 'queued' || state.recognition.status === 'recognizing') {
        return t('tableIngest.stepTwo.stageStatus.textRecognizing');
      }
      if (state.recognition.status === 'failed') {
        return t('tableIngest.stepTwo.stageStatus.textRecognitionFailed');
      }
      if (state.recognition.status === 'warning') {
        return t('tableIngest.stepTwo.stageStatus.needsCompletion');
      }
      if (getEffectiveStatus(state) === 'success') {
        return t('tableIngest.stepTwo.stageStatus.recognitionComplete');
      }
      if (state.parse.status === 'success') {
        return t('tableIngest.stepTwo.stageStatus.fileParsed');
      }
      return statusLabel(state.status);
    },
    [getEffectiveStatus, statusLabel, t]
  );

  const evidenceViewerLabels = useMemo<FileEvidenceViewerLabels>(
    () => ({
      title: t('tableIngest.stepTwo.previewPanelTitle'),
      tabs: {
        original: t('tableIngest.stepTwo.contentTabs.original'),
        text: t('tableIngest.stepTwo.contentTabs.text'),
      },
      originalPreviewAlt: activeFile?.name || t('tableIngest.stepTwo.originalFilePreview'),
      loadingOriginalPreview: t('tableIngest.stepTwo.loadingImagePreview'),
      originalPreviewUnavailable: t('tableIngest.stepTwo.imagePreviewUnavailable'),
      promptLoadFailedTitle: t('tableIngest.stepTwo.promptLoadFailedTitle'),
      retryPrompt: t('tableIngest.stepTwo.retryPrompt'),
      fileErrorTitle: t('tableIngest.stepTwo.fileErrorTitle'),
      fileWarningTitle: t('tableIngest.stepTwo.fileWarningTitle'),
      fileErrorDetails: t('tableIngest.stepTwo.fileErrorDetails'),
      retryFileParse:
        activeErrorStage === 'parse'
          ? t('tableIngest.stepTwo.retryFileParse')
          : t('tableIngest.stepTwo.retryTextRecognition'),
      noPreview: t('tableIngest.stepTwo.noPreview'),
    }),
    [activeErrorStage, activeFile?.name, t]
  );

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
    return hasRequiredValue(
      activeValues?.[col.name] as DbTableRecord[keyof DbTableRecord],
      col.type
    );
  };

  const fieldMatchForColumn = (col: DbTableColumn): FileIngestFieldMatch | undefined => {
    const fields = activeState?.fieldExtraction?.records?.[activeRecordIndex]?.fields || [];
    return fields.find(
      field =>
        field.column_id === col.id || field.column_id === col.name || field.column_name === col.name
    );
  };

  const fieldStatusForColumn = (col: DbTableColumn) => {
    const match = fieldMatchForColumn(col);
    const recognized = recognizedForColumn(col);
    if (recognized && match?.normalization_status === 'normalized') {
      return {
        status: 'normalized',
        label: t('tableIngest.stepTwo.fieldValueStatus.normalized'),
        className: 'text-highlight',
        match,
      };
    }
    if (recognized) {
      return {
        status: 'recognized',
        label: t('tableIngest.stepTwo.recognizedTag'),
        className: 'text-green-600',
        match,
      };
    }
    if (hasFieldMatchRawValue(match)) {
      return {
        status: 'needs_confirmation',
        label: t('tableIngest.stepTwo.fieldValueStatus.needsConfirmation'),
        className: 'text-warning',
        match,
      };
    }
    return {
      status: 'not_recognized',
      label: t('tableIngest.stepTwo.notRecognized'),
      className: 'text-muted-foreground',
      match,
    };
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
        if (fileStates[file.id]?.parse.status === 'failed') acc.parseFailed += 1;
        if (
          fileStates[file.id]?.recognition.status === 'failed' &&
          fileStates[file.id]?.parse.status !== 'failed'
        ) {
          acc.recognitionFailed += 1;
        }
        return acc;
      },
      { processing: 0, ready: 0, needs: 0, failed: 0, parseFailed: 0, recognitionFailed: 0 }
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
  const statusNotice = useMemo(() => {
    if (promptUnavailable) return null;
    if (hasProcessingFiles) {
      return {
        variant: 'default' as const,
        message: t('tableIngest.stepTwo.statusNotice.processing', {
          count: stats.processing,
        }),
      };
    }
    if (anyFileFailed) {
      return {
        variant: 'default' as const,
        message: t('tableIngest.stepTwo.statusNotice.needsAction', {
          count: stats.needs + stats.failed,
        }),
      };
    }
    if (hasUnsavedResults) {
      return {
        variant: 'default' as const,
        message: t('tableIngest.stepTwo.statusNotice.unsaved'),
      };
    }
    return null;
  }, [
    anyFileFailed,
    hasProcessingFiles,
    hasUnsavedResults,
    promptUnavailable,
    stats.failed,
    stats.needs,
    stats.processing,
    t,
  ]);

  useEffect(() => {
    if (!hasProcessingFiles) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [hasProcessingFiles]);

  useEffect(() => {
    onLeaveGuardChange?.(Boolean(leaveGuardReason), leaveGuardReason);
    return () => onLeaveGuardChange?.(false, null);
  }, [leaveGuardReason, onLeaveGuardChange]);

  const onSave = async () => {
    const validFiles = selectedFiles.filter(
      file => getEffectiveStatus(fileStates[file.id]) === 'success'
    );
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

    const payloads = validFiles.flatMap(file =>
      (fileStates[file.id]?.records || []).map(buildPayloadForValues)
    );

    try {
      await createRecords(payloads);
      onLeaveGuardChange?.(false, null);
      router.push(`/console/db/${dbId}/table/${tableId}`);
    } catch (_err) {
      // Error toast handled in hook
    }
  };

  return (
    <>
      <div className="mb-2 flex min-h-10 flex-wrap items-center gap-2 rounded-md border bg-background px-3 py-1.5 text-xs">
        <span className="mr-1 font-medium text-foreground">
          {t('tableIngest.stepTwo.workspaceTitle')}
        </span>
        <Badge variant="secondary">
          {t('tableIngest.stepTwo.stats.processing', { count: stats.processing })}
        </Badge>
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
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={retryParseFailedFiles}
            disabled={promptUnavailable || stats.parseFailed === 0}
          >
            <RotateCcw className="h-4 w-4" />
            {t('tableIngest.stepTwo.retryParseFailedFiles')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={retryRecognitionFailedFiles}
            disabled={promptUnavailable || stats.recognitionFailed === 0}
          >
            <RotateCcw className="h-4 w-4" />
            {t('tableIngest.stepTwo.retryRecognitionFailedFiles')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={retryAllFiles}
            disabled={promptUnavailable || selectedFiles.length === 0}
          >
            <RefreshCcw className="h-4 w-4" />
            {t('tableIngest.stepTwo.reRecognizeAll')}
          </Button>
        </div>
      </div>

      {statusNotice ? (
        <Alert
          className={cn(
            'mb-2 text-sm',
            anyFileFailed && !hasProcessingFiles && 'border-warning/35 bg-warning/5'
          )}
          variant={statusNotice.variant}
        >
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{statusNotice.message}</AlertDescription>
        </Alert>
      ) : null}
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

      <div className="grid h-0 min-h-0 grow grid-cols-[280px_minmax(620px,1fr)_420px] overflow-x-auto overflow-y-hidden rounded-md border bg-background">
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
              const fileStatusLabel =
                status === 'warning'
                  ? t('tableIngest.stepTwo.fileStatus.needsCompletionCount', {
                      count: getPendingFieldCount(state),
                    })
                  : stageStatusLabel(state);
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
                      <span className={statusClass(status)}>{fileStatusLabel}</span>
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
          <FileEvidenceViewer
            tab={contentTab}
            onTabChange={setContentTab}
            labels={evidenceViewerLabels}
            headerActions={
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={reprocessCurrent}
                disabled={!activeFileId || promptUnavailable}
              >
                <RotateCcw className="h-4 w-4" />
                {t('tableIngest.stepTwo.reRecognize')}
              </Button>
            }
            original={{
              enabled: activePreviewableOriginal,
              file: activeOriginalPreviewFile,
              url: activeOriginalPreviewUrl,
              loading: activeOriginalPreviewLoading,
              error: activeOriginalPreviewError || undefined,
            }}
            promptIssue={
              promptUnavailable
                ? {
                    message: promptError
                      ? t('tableIngest.stepTwo.promptLoadFailedDesc', {
                          error: promptUnavailableMessage,
                        })
                      : promptUnavailableMessage,
                    loading: promptLoading,
                    onRetry: () => void refetchPrompt(),
                  }
                : undefined
            }
            loading={
              activeRecognizing && !activeContent
                ? {
                    active: true,
                    title: activeLoadingCopy.title,
                    description: activeLoadingCopy.desc,
                    render: (
                      <IngestLoadingBlock
                        title={activeLoadingCopy.title}
                        desc={activeLoadingCopy.desc}
                      />
                    ),
                  }
                : undefined
            }
            error={
              activeError
                ? {
                    message: activeError,
                    hint: activeErrorHint,
                    onRetryFileParse:
                      activeErrorStage === 'parse' ? retryParseCurrent : retryRecognitionCurrent,
                  }
                : undefined
            }
            warning={
              activeWarning
                ? {
                    message: activeWarning,
                    hint: activeErrorHint,
                  }
                : undefined
            }
            content={activeContent}
            highlights={highlightTerms}
          />
        </div>

        <div className="min-w-0 flex flex-col border-l overflow-hidden">
          <div className="px-3 py-2 border-b text-sm font-medium">
            <div className="flex items-center gap-2">
              <span>{t('tableIngest.stepTwo.fieldsPanelTitle')}</span>
              {activeEffectiveStatus ? (
                <span className={cn('text-xs ml-auto', statusClass(activeEffectiveStatus))}>
                  {stageStatusLabel(activeState)}
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
                {activeRecords.length > 0 ? (
                  <div className="mr-1 flex items-center gap-1 text-xs text-muted-foreground">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      isIcon
                      aria-label={t('tableIngest.stepTwo.previousRecord')}
                      disabled={!activeFileId || activeRecordIndex === 0}
                      onClick={() => {
                        if (!activeFileId) return;
                        setActiveRecordIndexes(prev => ({
                          ...prev,
                          [activeFileId]: Math.max(activeRecordIndex - 1, 0),
                        }));
                      }}
                    >
                      <ChevronLeft className="h-4 w-4" />
                    </Button>
                    <span className="min-w-12 text-center tabular-nums">
                      {t('tableIngest.stepTwo.recordPosition', {
                        current: activeRecordIndex + 1,
                        total: activeRecords.length,
                      })}
                    </span>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      isIcon
                      aria-label={t('tableIngest.stepTwo.nextRecord')}
                      disabled={!activeFileId || activeRecordIndex >= activeRecords.length - 1}
                      onClick={() => {
                        if (!activeFileId) return;
                        setActiveRecordIndexes(prev => ({
                          ...prev,
                          [activeFileId]: Math.min(activeRecordIndex + 1, activeRecords.length - 1),
                        }));
                      }}
                    >
                      <ChevronRight className="h-4 w-4" />
                    </Button>
                  </div>
                ) : null}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      isIcon
                      aria-label={t('tableIngest.stepTwo.retryTextRecognition')}
                      onClick={retryRecognitionCurrent}
                      disabled={!activeFileId || promptUnavailable || !activeContent}
                    >
                      <RefreshCcw className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {t('tableIngest.stepTwo.retryTextRecognition')}
                  </TooltipContent>
                </Tooltip>
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
            ) : activeRecognizing ? (
              <IngestLoadingBlock
                title={activeLoadingCopy.title}
                desc={activeLoadingCopy.desc}
                compact
              />
            ) : activeError && columns.length === 0 ? (
              <IngestRecoveryCard
                title={t('tableIngest.stepTwo.fileErrorTitle')}
                hint={activeErrorHint}
                detailsLabel={t('tableIngest.stepTwo.fileErrorDetails')}
                details={activeError}
                retryLabel={
                  activeErrorStage === 'parse'
                    ? t('tableIngest.stepTwo.retryFileParse')
                    : t('tableIngest.stepTwo.retryTextRecognition')
                }
                onRetry={activeErrorStage === 'parse' ? retryParseCurrent : retryRecognitionCurrent}
                compact
              />
            ) : (
              <>
                {activeError ? (
                  <IngestRecoveryCard
                    title={t('tableIngest.stepTwo.fileErrorTitle')}
                    hint={activeErrorHint}
                    detailsLabel={t('tableIngest.stepTwo.fileErrorDetails')}
                    details={activeError}
                    retryLabel={
                      activeErrorStage === 'parse'
                        ? t('tableIngest.stepTwo.retryFileParse')
                        : t('tableIngest.stepTwo.retryTextRecognition')
                    }
                    onRetry={
                      activeErrorStage === 'parse' ? retryParseCurrent : retryRecognitionCurrent
                    }
                    compact
                  />
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
                  const fieldStatus = fieldStatusForColumn(col);
                  const recognized =
                    fieldStatus.status === 'recognized' || fieldStatus.status === 'normalized';
                  const val = (activeValues?.[col.name] ??
                    '') as DbTableRecord[keyof DbTableRecord];
                  const cellKey = `${String(activeFileId || '')}:${activeRecordIndex}:${col.name}`;
                  const rawValue = fieldStatus.match?.raw_value ?? fieldStatus.match?.value;
                  const normalizedValue =
                    fieldStatus.match?.normalized_value ?? fieldStatus.match?.value;
                  const invalidRequired =
                    !suppressRequiredErrors &&
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
                          className={`inline-flex items-center gap-1 text-xs ${fieldStatus.className}`}
                        >
                          {recognized ? <Check className="h-3 w-3" /> : null}
                          {fieldStatus.label}
                        </span>
                      </div>

                      {col.type === Type.Boolean ? (
                        <div className="flex items-center">
                          <Switch
                            checked={Boolean(val)}
                            onCheckedChange={checked => updateValue(col, !!checked)}
                            className={
                              invalidRequired ? 'focus-visible:ring-destructive' : undefined
                            }
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
                      {fieldStatus.status === 'normalized' &&
                      formatFieldMatchValue(rawValue) !== formatFieldMatchValue(normalizedValue) ? (
                        <div className="text-xs leading-5 text-muted-foreground">
                          {t('tableIngest.stepTwo.fieldValueStatus.normalizedHint', {
                            raw: formatFieldMatchValue(rawValue),
                            value: formatFieldMatchValue(normalizedValue),
                          })}
                        </div>
                      ) : fieldStatus.status === 'needs_confirmation' ? (
                        <div className="text-xs leading-5 text-warning">
                          {t('tableIngest.stepTwo.fieldValueStatus.candidateHint', {
                            raw: formatFieldMatchValue(rawValue),
                            type: col.type,
                          })}
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
      <ConfirmDialog
        open={Boolean(overwriteConfirm)}
        onOpenChange={open => {
          if (!open) setOverwriteConfirm(null);
        }}
        title={t('tableIngest.stepTwo.overwriteConfirmTitle')}
        description={overwriteConfirm?.message}
        cancelText={t('tableIngest.stepTwo.keepCurrentResult')}
        confirmText={t('tableIngest.stepTwo.overwriteConfirmAction')}
        variant="warning"
        onConfirm={() => {
          overwriteConfirm?.onConfirm();
          setOverwriteConfirm(null);
        }}
      />
    </>
  );
};

export default StepTwo;
