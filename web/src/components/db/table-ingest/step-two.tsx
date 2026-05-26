'use client';

// Step 2: AI recognition & preview with type-specific editors
// English comments only as required. Strict types, no any.

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { AlertCircle, Check, CheckCircle, Database, FileSearch, ShieldCheck } from 'lucide-react';
import type { DbTableRecord, DbTableColumn, GetDbTableRecordsParams } from '@/services/types/db';
import { Type } from '@/services/types/db';
import { useCreateDbTableRecords } from '@/hooks/db/use-db-table-records';
import { Skeleton } from '@/components/ui/skeleton';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Switch } from '@/components/ui/switch';
import { formatDateTimeLocalInput } from '@/utils/date-input';
import { useT } from '@/i18n';
import { useDbTablePrompt } from '@/hooks/db/use-db-table-prompt';
import { useBatchIngestFileToTable } from '@/hooks/db/use-batch-ingest-file-to-table';
import { useFilePreview } from '@/hooks/file/use-file-preview';
import type { FileItem } from '@/services/types/file';
import { FileIcon } from '@/components/ui/file-icon';
import { formatFileSize } from '@/utils/format';
import { cn } from '@/lib/utils';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

export interface IngestStepTwoProps {
  selectedFiles: FileItem[];
  selectedModel: { provider: string; model: string };
  dbId: string;
  tableId: string;
  /** Nonce to force re-recognition when changed */
  reRecognitionNonce?: number;
}

const StepTwo: React.FC<IngestStepTwoProps> = ({
  selectedFiles,
  selectedModel,
  dbId,
  tableId,
  reRecognitionNonce = 0,
}) => {
  const t = useT('dbs');
  const router = useRouter();

  // Active file selection
  const [activeFileId, setActiveFileId] = useState<string | undefined>(selectedFiles[0]?.id);

  // Prompt for recognition
  const { prompt } = useDbTablePrompt(dbId, tableId, {
    staleTime: 60_000,
    gcTime: 600_000,
  });

  // Columns from ingest API (non-system). Do not request structure separately.
  const [columns, setColumns] = useState<DbTableColumn[]>([]);

  // Batch ingest mutation
  const { ingest, isPending: ingesting } = useBatchIngestFileToTable(dbId, tableId);

  // Ingest results mapping: file_id -> first record for preview/editing
  const [recognizedByFile, setRecognizedByFile] = useState<Record<string, DbTableRecord>>({});

  // Editable values per file_id
  const [valuesByFile, setValuesByFile] = useState<Record<string, DbTableRecord>>({});
  const [contentByFile, setContentByFile] = useState<Record<string, string>>({});

  // Drafts for timestamp inputs to allow partial edits before committing
  const [tsDrafts, setTsDrafts] = useState<Record<string, string>>({});

  // Create records (optimistic)
  const listParams: GetDbTableRecordsParams = { limit: 20, offset: 0, order: 'id DESC' };
  const { createRecords, isPending: saving } = useCreateDbTableRecords(dbId, tableId, listParams);

  // Track request key to ensure ingest runs only once per files+prompt+tableId
  const requestedKeyRef = useRef<string>('');
  const selectedFileIds = useMemo(() => selectedFiles.map(f => f.id), [selectedFiles]);
  const requestKey = useMemo(
    () =>
      JSON.stringify({
        tableId,
        prompt: prompt || '',
        reRecognitionNonce,
        model: {
          provider: selectedModel.provider,
          name: selectedModel.model,
        },
        fileIds: [...selectedFileIds].sort(),
      }),
    [
      tableId,
      prompt,
      reRecognitionNonce,
      selectedFileIds,
      selectedModel.provider,
      selectedModel.model,
    ]
  );
  useEffect(() => {
    setColumns([]);
    setRecognizedByFile({});
    setValuesByFile({});
    setContentByFile({});
    setTsDrafts({});
    setActiveFileId(selectedFiles[0]?.id);
  }, [reRecognitionNonce, selectedFiles]);

  // Run ingest when entering Step Two (exactly once per requestKey)
  useEffect(() => {
    const run = async () => {
      if (!tableId || selectedFileIds.length === 0 || !String(prompt || '').trim()) return;
      if (requestedKeyRef.current === requestKey) return; // already requested
      requestedKeyRef.current = requestKey;
      try {
        const res = await ingest({
          file_ids: selectedFileIds,
          prompt: prompt || '',
          table_id: tableId,
          model: { provider: selectedModel.provider, name: selectedModel.model },
        });

        // Map first record of each file into recognizedByFile
        const nextRecognized: Record<string, DbTableRecord> = {};
        const nextContent: Record<string, string> = {};
        Object.values(res.results || {}).forEach(item => {
          const first = (item.records || [])[0] || ({} as DbTableRecord);
          nextRecognized[item.file_id] = first;
          nextContent[item.file_id] = item.content || '';
        });
        setRecognizedByFile(nextRecognized);
        setContentByFile(nextContent);
        // Initialize editable values
        setValuesByFile(prev => {
          const copy = { ...prev };
          Object.keys(nextRecognized).forEach(fid => {
            copy[fid] = { ...(nextRecognized[fid] || ({} as DbTableRecord)) };
          });
          return copy;
        });

        // Use ingest-returned columns and exclude system fields
        setColumns((res.columns || []).filter(c => !c.is_system_field));
      } catch (_err) {
        // Toast handled inside hook
      }
    };
    run();
  }, [
    ingest,
    prompt,
    requestKey,
    selectedFileIds,
    selectedModel.model,
    selectedModel.provider,
    tableId,
  ]);

  // Active file preview content
  const { content: previewContent, isLoading: previewLoading } = useFilePreview(activeFileId, {
    enabled: !!activeFileId,
    staleTime: 60_000,
    gcTime: 300_000,
    refetchOnWindowFocus: false,
  });
  const displayedPreviewContent =
    (activeFileId && contentByFile[activeFileId]) || previewContent;

  // Build highlight terms from recognized values for active file
  const highlightTerms = useMemo(() => {
    const record = (activeFileId && valuesByFile[activeFileId]) || ({} as DbTableRecord);
    const set = new Set<string>();
    Object.entries(record).forEach(([key, value]) => {
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
  }, [activeFileId, valuesByFile]);

  const activeValues = activeFileId
    ? valuesByFile[activeFileId] || ({} as DbTableRecord)
    : ({} as DbTableRecord);

  const updateValue = (col: DbTableColumn, v: DbTableRecord[keyof DbTableRecord]) => {
    if (!activeFileId) return;
    setValuesByFile(prev => ({
      ...prev,
      [activeFileId]: { ...prev[activeFileId], [col.name]: v },
    }));
  };

  const recognizedForColumn = (fid: string | undefined, col: DbTableColumn): boolean => {
    if (!fid) return false;
    const r = recognizedByFile[fid];
    return (
      typeof r?.[col.name] !== 'undefined' &&
      r[col.name] !== null &&
      String(r[col.name]).trim().length > 0
    );
  };

  // Validation status tracking per file
  type ValidationStatus = 'normal' | 'success' | 'failed';
  const [validationStatusByFile, setValidationStatusByFile] = useState<
    Record<string, ValidationStatus>
  >({});

  // Check whether a required value is present based on type
  const hasRequiredValue = (v: DbTableRecord[keyof DbTableRecord], type: Type): boolean => {
    if (v === null || typeof v === 'undefined') return false;
    switch (type) {
      case Type.Text:
        return typeof v !== 'string' ? true : v.trim().length > 0;
      case Type.Timestamp:
        return typeof v !== 'string' ? true : v.trim().length > 0;
      case Type.Boolean:
        return typeof v === 'boolean';
      case Type.Integer:
      case Type.Numeric:
        return typeof v === 'number' && !Number.isNaN(v as number);
      default:
        // For unforeseen complex types, consider non-empty arrays as having value
        if (Array.isArray(v as unknown)) return ((v as unknown[]) || []).length > 0;
        return true;
    }
  };

  const computeFileStatus = useCallback(
    (fid: string | undefined): ValidationStatus => {
      if (!fid || columns.length === 0) return 'normal';
      // Only start validating after ingest returns for this file
      if (!recognizedByFile[fid]) return 'normal';
      const values = valuesByFile[fid] || ({} as DbTableRecord);
      const ok = columns.every(col => {
        if (!col.is_required) return true;
        return hasRequiredValue(values[col.name] as DbTableRecord[keyof DbTableRecord], col.type);
      });
      return ok ? 'success' : 'failed';
    },
    [columns, recognizedByFile, valuesByFile]
  );

  // At least one file is valid (used to enable save button)
  const anyFileValid = useMemo(
    () => selectedFiles.some(f => validationStatusByFile[f.id] === 'success'),
    [selectedFiles, validationStatusByFile]
  );

  // Show alerts only when there are actual failed validations
  const anyFileFailed = useMemo(
    () => selectedFiles.some(f => validationStatusByFile[f.id] === 'failed'),
    [selectedFiles, validationStatusByFile]
  );

  // Consider ingest completed once we received columns or any recognized record
  const ingestCompleted = useMemo(
    () => columns.length > 0 || Object.keys(recognizedByFile).length > 0,
    [columns, recognizedByFile]
  );

  // Recompute validation statuses when values or columns or ingest results change
  useEffect(() => {
    const next: Record<string, ValidationStatus> = {};
    selectedFiles.forEach(f => {
      next[f.id] = computeFileStatus(f.id);
    });
    setValidationStatusByFile(next);
  }, [computeFileStatus, selectedFiles]);

  const onSave = async () => {
    // Build payloads for all validated files
    const validFiles = selectedFiles.filter(f => validationStatusByFile[f.id] === 'success');
    if (validFiles.length === 0) return;

    const buildPayloadForValues = (values: DbTableRecord): Omit<DbTableRecord, 'id'> => {
      const record = columns.reduce<DbTableRecord>((acc, col) => {
        const raw = values[col.name];
        let casted: DbTableRecord[keyof DbTableRecord] = raw as DbTableRecord[keyof DbTableRecord];
        // Minimal casting based on column type
        switch (col.type) {
          case Type.Integer:
          case Type.Numeric:
            casted = raw === '' || raw === null || typeof raw === 'undefined' ? null : Number(raw);
            break;
          case Type.Boolean:
            casted = String(raw).toLowerCase() === 'true';
            break;
          case Type.Timestamp:
            // When timestamp value is empty, send null instead of empty string
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

    const payloads: Array<Omit<DbTableRecord, 'id'>> = validFiles.map(f => {
      const values = valuesByFile[f.id] || ({} as DbTableRecord);
      return buildPayloadForValues(values);
    });

    try {
      await createRecords(payloads);
      // Navigate to table detail page and open data tab on success
      router.push(`/console/db/${dbId}/table/${tableId}`);
    } catch (_err) {
      // Error toast handled in hook
    }
  };

  // Layout
  return (
    <>
      <div className="mb-3 grid gap-2 md:grid-cols-3">
        {[
          {
            icon: <FileSearch className="h-4 w-4" />,
            title: t('tableIngest.stepTwo.reviewSteps.recognizeTitle'),
            desc: t('tableIngest.stepTwo.reviewSteps.recognizeDesc'),
            active: ingesting && !ingestCompleted,
          },
          {
            icon: <ShieldCheck className="h-4 w-4" />,
            title: t('tableIngest.stepTwo.reviewSteps.reviewTitle'),
            desc: t('tableIngest.stepTwo.reviewSteps.reviewDesc'),
            active: ingestCompleted && !saving,
          },
          {
            icon: <Database className="h-4 w-4" />,
            title: t('tableIngest.stepTwo.reviewSteps.commitTitle'),
            desc: t('tableIngest.stepTwo.reviewSteps.commitDesc'),
            active: saving,
          },
        ].map(item => (
          <div
            key={item.title}
            className={cn(
              'rounded-md border px-3 py-2',
              item.active ? 'border-highlight bg-highlight/10' : 'bg-background'
            )}
          >
            <div className="flex items-center gap-2 text-sm font-medium">
              {item.icon}
              {item.title}
            </div>
            <div className="mt-1 text-xs text-muted-foreground">{item.desc}</div>
          </div>
        ))}
      </div>
      {!ingesting && anyFileFailed && (
        <Alert className="mb-2 text-sm" variant="destructive">
          <AlertDescription>{t('tableIngest.stepTwo.validationAlert')}</AlertDescription>
        </Alert>
      )}
      <div className="h-0 grow flex overflow-hidden rounded-md border">
        {/* Left: File list */}
        <div className="w-[280px] shrink-0 flex flex-col border-r overflow-hidden">
          <div className="px-3 py-2 border-b flex items-center">
            <span className="font-medium text-sm">
              {t('tableIngest.stepTwo.leftPanelTitle')}
              <span className="ml-1">({selectedFiles.length})</span>
            </span>
            {!ingesting && anyFileFailed && (
              <span className="text-destructive text-xs ml-auto">
                {t('tableIngest.stepTwo.leftFilesInvalidTip')}
              </span>
            )}
          </div>
          <div className="p-2 overflow-auto space-y-2">
            {selectedFiles.map(f => {
              const status: ValidationStatus = validationStatusByFile[f.id] || 'normal';
              return (
                <button
                  key={f.id}
                  className={cn(
                    'w-full flex items-center gap-3 rounded-md border px-3 py-2 text-left',
                    'hover:bg-highlight/5 hover:border-highlight/50',
                    activeFileId === f.id && 'bg-highlight/10 border-highlight text-highlight'
                  )}
                  onClick={() => setActiveFileId(f.id)}
                >
                  <FileIcon filename={f.name} className="shrink-0" />
                  <div className="min-w-0 grow">
                    <div className="truncate text-sm font-medium" title={f.name}>
                      {f.name}
                    </div>
                    <div className="text-xs flex items-center gap-2">
                      <span className="text-muted-foreground">{formatFileSize(f.size)}</span>
                    </div>
                  </div>
                  <div className="shrink-0">
                    {status === 'success' ? (
                      <CheckCircle className="w-4 h-4 text-success" />
                    ) : status === 'failed' ? (
                      <AlertCircle className="w-4 h-4 text-destructive" />
                    ) : null}
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        {/* Middle: Content preview */}
        <div className="grow flex flex-col overflow-hidden">
          <div className="px-3 py-2 border-b text-sm font-medium flex items-center gap-2">
            <span>{t('tableIngest.stepTwo.previewPanelTitle')}</span>
          </div>
          <div className="p-4 overflow-auto">
            {previewLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 12 }).map((_, i) => (
                  <Skeleton key={i} className="h-4 w-full" />
                ))}
              </div>
            ) : displayedPreviewContent ? (
              <MarkdownViewer content={displayedPreviewContent} highlights={highlightTerms} />
            ) : (
              <div className="text-sm text-muted-foreground">
                {t('tableIngest.stepTwo.noPreview')}
              </div>
            )}
          </div>
        </div>

        {/* Right: Field extraction */}
        <div className="w-[360px] shrink-0 flex flex-col border-l overflow-hidden">
          <div className="px-3 py-2 border-b text-sm font-medium flex items-center gap-2">
            <span>{t('tableIngest.stepTwo.fieldsPanelTitle')}</span>
            {activeFileId && validationStatusByFile[activeFileId] === 'failed' && (
              <span className="text-destructive text-xs ml-auto">
                {t('tableIngest.stepTwo.activeFileInvalidTip')}
              </span>
            )}
          </div>
          <div className="p-3 space-y-3 overflow-auto h-0 grow">
            {ingesting && !ingestCompleted ? (
              <div className="space-y-2">
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className="h-8 w-full" />
                ))}
              </div>
            ) : (
              columns.map(col => {
                const recognized = recognizedForColumn(activeFileId, col);
                const val = (activeValues?.[col.name] ?? '') as DbTableRecord[keyof DbTableRecord];
                const cellKey = `${String(activeFileId || '')}:${col.name}`;
                const invalidRequired =
                  col.is_required &&
                  !hasRequiredValue(val as DbTableRecord[keyof DbTableRecord], col.type);

                return (
                  <div key={col.id} className="space-y-1">
                    <div className="flex items-center justify-between">
                      {col.description && col.description.trim() ? (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <label
                              className={`text-xs font-medium truncate ${invalidRequired ? 'text-destructive' : ''}`}
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
                          className={`text-xs font-medium truncate ${invalidRequired ? 'text-destructive' : ''}`}
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

                    {/* Type-specific editor */}
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
                          const num = next === '' ? 0 : Number(next);
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
                            : typeof val === 'string' && val
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
                          // Commit only when datetime-local string is complete
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
                          invalidRequired
                            ? 'border-destructive focus-visible:ring-destructive'
                            : undefined
                        }
                        aria-invalid={invalidRequired || undefined}
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
                    {invalidRequired ? (
                      <div className="text-xs text-destructive">
                        {t('tableIngest.stepTwo.requiredEmptyTag')}
                      </div>
                    ) : null}
                  </div>
                );
              })
            )}
          </div>
          <div className="p-3 border-t">
            {
              // Enable save when at least one file validated; block only while ingesting first load
            }
            <Button
              className="w-full"
              onClick={onSave}
              disabled={saving || (ingesting && !ingestCompleted) || !anyFileValid}
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
