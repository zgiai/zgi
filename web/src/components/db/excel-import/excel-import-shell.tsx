'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle,
  FileSpreadsheet,
  Loader2,
  ShieldAlert,
  Sparkles,
  Upload,
  X,
} from 'lucide-react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import { StepsProgressBar } from '@/components/common/steps-progress-bar';
import type { FileItem } from '@/services/types/file';
import {
  Type,
  type AnalyzeExcelImportData,
  type ConfirmExcelImportData,
  type ConfirmExcelImportRequest,
  type InferredExcelColumn,
  type RecognizeExcelImportData,
} from '@/services/types/db';
import {
  useAnalyzeExcelImport,
  useConfirmExcelImport,
  useExcelImportErrors,
  useRecognizeExcelImport,
} from '@/hooks/db/use-excel-import';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import {
  getDuplicateDbColumnNames,
  getTableNameValidationErrors,
  isInvalidDbColumnName,
  isReservedDbColumnName,
  type TableNameErrorCode,
} from '@/utils/validation';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';

type Step = 'file' | 'preview' | 'schema' | 'result';

interface ExcelImportShellProps {
  dbId: string;
}

const typeOptions = [Type.Text, Type.Integer, Type.Numeric, Type.Boolean, Type.Timestamp] as const;

function getExcelColumnKey(col: InferredExcelColumn, index: number) {
  return [col.source_column_index, col.source_column || 'source', index].join(':');
}

function getSampleValueKey(value: unknown, index: number) {
  return `${index}:${String(value)}`;
}

function applyRecognizedColumns(
  current: InferredExcelColumn[],
  recognized: InferredExcelColumn[]
): InferredExcelColumn[] {
  return current.map((col, index) => {
    const suggestion = recognized[index];
    if (!suggestion) return col;
    return {
      ...col,
      name: suggestion.name || col.name,
      display_name: suggestion.display_name || col.display_name,
      description: suggestion.description || col.description,
      type: suggestion.type || col.type,
    };
  });
}

export default function ExcelImportShell({ dbId }: ExcelImportShellProps) {
  const t = useT('dbs');
  const router = useRouter();
  const user = useCurrentUser();
  const { locale } = useLocale();
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canAnalyzeImport = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.importAnalyze);
  const canExecuteImport = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.importExecute);
  const canViewImportErrors = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.importErrorsView);
  const canOpenCreatedTableRecords = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.recordView,
    ...DATABASE_PERMISSION_ACTIONS.recordCreate,
    ...DATABASE_PERMISSION_ACTIONS.recordUpdate,
    ...DATABASE_PERMISSION_ACTIONS.recordDelete,
  ]);
  const canOpenCreatedTableSchema = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.schemaView,
    ...DATABASE_PERMISSION_ACTIONS.schemaManage,
  ]);
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');
  const { tables } = useDbTables(dbId, { enabled: canAnalyzeImport || canExecuteImport });
  const [step, setStep] = useState<Step>('file');
  const [fileDialogOpen, setFileDialogOpen] = useState(false);
  const [selectedFile, setSelectedFile] = useState<FileItem | null>(null);
  const [analysis, setAnalysis] = useState<AnalyzeExcelImportData | null>(null);
  const [columns, setColumns] = useState<InferredExcelColumn[]>([]);
  const [tableName, setTableName] = useState('');
  const [tableDescription, setTableDescription] = useState('');
  const [selectedSheetName, setSelectedSheetName] = useState<string | null>(null);
  const [analyzingSheetName, setAnalyzingSheetName] = useState<string | null>(null);
  const [selectedModel, setSelectedModel] = useState<ModelSelectorValue | null>(() => {
    if (!user?.id) return null;
    const saved = getLastSelectedAiModel(user.id, 'excelImport');
    return saved ? { provider: saved.provider, model: saved.model } : null;
  });
  const [recognitionDraft, setRecognitionDraft] = useState<RecognizeExcelImportData | null>(null);
  const [recognitionDialogOpen, setRecognitionDialogOpen] = useState(false);
  const [createdTableId, setCreatedTableId] = useState<string | null>(null);
  const [importResult, setImportResult] = useState<ConfirmExcelImportData | null>(null);

  const analyzeMutation = useAnalyzeExcelImport(dbId);
  const confirmMutation = useConfirmExcelImport(dbId, analysis?.job_id);
  const recognizeMutation = useRecognizeExcelImport(dbId, analysis?.job_id);
  const importErrorsQuery = useExcelImportErrors(
    dbId,
    importResult?.job_id,
    { limit: 20, offset: 0 },
    canViewImportErrors &&
      step === 'result' &&
      Boolean(importResult && importResult.failed_rows > 0)
  );
  const analyzeRequestSeq = useRef(0);

  useEffect(() => {
    if (!user?.id || selectedModel) return;
    const saved = getLastSelectedAiModel(user.id, 'excelImport');
    if (saved) {
      setSelectedModel({ provider: saved.provider, model: saved.model });
      return;
    }
    if (defaultModel) {
      setSelectedModel({ provider: defaultModel.provider, model: defaultModel.model });
    }
  }, [defaultModel, selectedModel, user?.id]);

  const enabledColumns = useMemo(() => columns.filter(col => col.enabled !== false), [columns]);
  const allEnabledColumnsRequired =
    enabledColumns.length > 0 && enabledColumns.every(col => col.is_required);
  const duplicateNames = useMemo(() => getDuplicateDbColumnNames(enabledColumns), [enabledColumns]);

  const invalidFieldNames = useMemo(
    () =>
      enabledColumns.filter(col => {
        const name = col.name.trim();
        return isInvalidDbColumnName(name) || isReservedDbColumnName(name);
      }),
    [enabledColumns]
  );
  const tableNameValue = tableName.trim();
  const isDuplicateTableName = useMemo(() => {
    const candidate = tableNameValue.toLowerCase();
    if (!candidate) return false;
    return tables.some(table => (table.name || table.table_name || '').toLowerCase() === candidate);
  }, [tableNameValue, tables]);
  const tableNameErrors: TableNameErrorCode[] = useMemo(() => {
    const errors = getTableNameValidationErrors(tableName);
    if (isDuplicateTableName) errors.push('duplicate');
    return errors;
  }, [isDuplicateTableName, tableName]);
  const tableNameErrorMessages = useMemo(
    () => tableNameErrors.map(code => t(`validation.tableName.${code}`)),
    [tableNameErrors, t]
  );
  const schemaValidationMessages = useMemo(() => {
    const messages: string[] = [...tableNameErrorMessages];
    if (duplicateNames.size > 0) messages.push(t('excelImport.schema.validation.duplicateFields'));
    if (invalidFieldNames.length > 0) {
      messages.push(t('excelImport.schema.validation.invalidFieldNames'));
    }
    return messages;
  }, [duplicateNames.size, invalidFieldNames.length, t, tableNameErrorMessages]);

  const canImport =
    Boolean(tableNameValue) &&
    tableNameErrors.length === 0 &&
    enabledColumns.length > 0 &&
    duplicateNames.size === 0 &&
    invalidFieldNames.length === 0;
  const canRecognize =
    Boolean(analysis) && Boolean(selectedModel?.model) && !recognizeMutation.isPending;
  const isAnalyzingWorkbook = analyzeMutation.isPending;
  const activeSheetName = selectedSheetName || analysis?.selection.sheet_name || null;
  const isPreviewSheetReady = Boolean(
    analysis && activeSheetName && activeSheetName === analysis.selection.sheet_name
  );

  const handleAnalyze = async (overrides?: { sheet_name?: string; header_row?: number }) => {
    if (!selectedFile || !canAnalyzeImport) return;
    const requestedSheetName = overrides?.sheet_name?.trim();
    if (requestedSheetName) {
      setSelectedSheetName(requestedSheetName);
    }
    if (requestedSheetName && requestedSheetName === analysis?.selection.sheet_name) return;

    const requestSeq = analyzeRequestSeq.current + 1;
    analyzeRequestSeq.current = requestSeq;
    setAnalyzingSheetName(requestedSheetName || null);
    try {
      const res = await analyzeMutation.mutateAsync({
        upload_file_id: selectedFile.id,
        sample_size: 500,
        ...overrides,
      });
      if (requestSeq !== analyzeRequestSeq.current) return;
      setAnalysis(res.data);
      setSelectedSheetName(res.data.selection.sheet_name);
      setColumns(res.data.columns.map(col => ({ ...col, enabled: col.enabled ?? true })));
      setRecognitionDraft(null);
      setRecognitionDialogOpen(false);
      if (!tableName) {
        const baseName = res.data.source.file_name.replace(/\.[^.]+$/, '').toLowerCase();
        const normalizedName =
          baseName.replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '') || 'imported_table';
        setTableName(/^[a-z]/.test(normalizedName) ? normalizedName : `table_${normalizedName}`);
      }
      setStep('preview');
    } finally {
      if (requestSeq === analyzeRequestSeq.current) {
        setAnalyzingSheetName(null);
      }
    }
  };

  const handleConfirm = async () => {
    if (!analysis || !canImport || !canExecuteImport) return;
    const payload: ConfirmExcelImportRequest = {
      table: {
        name: tableName.trim(),
        description: tableDescription.trim() || undefined,
      },
      selection: analysis.selection,
      columns,
      options: {
        error_policy: 'skip_invalid_rows',
        empty_row_policy: 'skip',
        batch_size: 500,
      },
    };
    const res = await confirmMutation.mutateAsync(payload);
    setCreatedTableId(res.data.table_id);
    setImportResult(res.data);
    setStep('result');
  };

  const recognizeCurrentColumns = async (): Promise<RecognizeExcelImportData | null> => {
    if (!analysis || !selectedModel || recognizeMutation.isPending) return null;
    const res = await recognizeMutation.mutateAsync({
      table: {
        name: tableName.trim(),
        description: tableDescription.trim(),
      },
      source: {
        file_name: analysis.source.file_name,
        sheet_name: analysis.selection.sheet_name,
      },
      columns,
      model: { provider: selectedModel.provider, name: selectedModel.model },
      operator_language: locale,
    });
    return res.data;
  };

  const handleEnterSchema = async () => {
    if (!isPreviewSheetReady || !selectedModel?.model || recognizeMutation.isPending) return;
    try {
      const recognized = await recognizeCurrentColumns();
      if (!recognized) return;
      setTableName(recognized.table.name);
      setTableDescription(recognized.table.description);
      setColumns(prev => applyRecognizedColumns(prev, recognized.columns));
      setRecognitionDraft(null);
      setRecognitionDialogOpen(false);
      setStep('schema');
    } catch {
      // The mutation displays the backend error and keeps the user on the preview step.
    }
  };

  const handleSmartRecognize = async () => {
    const recognized = await recognizeCurrentColumns();
    if (!recognized) return;
    setRecognitionDraft(recognized);
    setRecognitionDialogOpen(true);
  };

  const handleApplyRecognition = () => {
    if (!recognitionDraft) return;
    setTableName(recognitionDraft.table.name);
    setTableDescription(recognitionDraft.table.description);
    setColumns(prev => applyRecognizedColumns(prev, recognitionDraft.columns));
    setRecognitionDraft(null);
    setRecognitionDialogOpen(false);
  };

  const displayedFailedItems =
    importErrorsQuery.data?.data.data ?? importResult?.failed_items ?? [];
  const createdTableHref = createdTableId
    ? canOpenCreatedTableRecords
      ? `/console/db/${dbId}/table/${createdTableId}`
      : canOpenCreatedTableSchema
        ? `/console/db/${dbId}/table/${createdTableId}/structure`
        : null
    : null;

  if (isPermissionsLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!canAnalyzeImport && !canExecuteImport) {
    return (
      <div className="flex h-full w-full flex-col items-center justify-center p-6 text-center">
        <ShieldAlert className="mb-4 h-12 w-12 text-muted-foreground" />
        <h2 className="mb-2 text-xl font-semibold">{t('excelImport.permissions.title')}</h2>
        <p className="max-w-md text-sm text-muted-foreground">
          {t('excelImport.permissions.description')}
        </p>
        <Button
          className="mt-6"
          variant="outline"
          onClick={() => router.push(`/console/db/${dbId}`)}
        >
          {t('excelImport.actions.back')}
        </Button>
      </div>
    );
  }

  return (
    <div className="p-6 h-full flex flex-col overflow-hidden">
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Link
            href={`/console/db/${dbId}`}
            className="hover:bg-muted flex justify-center items-center w-9 h-9 rounded-md"
          >
            <ArrowLeft className="h-5 w-5" />
          </Link>
          <div>
            <h1 className="text-lg font-semibold">{t('excelImport.title')}</h1>
            <div className="text-xs text-muted-foreground">{t('excelImport.subtitle')}</div>
          </div>
        </div>
        <StepIndicator step={step} />
      </div>

      {step === 'file' && (
        <div className="h-0 grow flex flex-col justify-center">
          <div
            className="mx-auto w-full max-w-2xl rounded-md border-2 border-dashed p-10 text-center cursor-pointer hover:bg-muted/60"
            onClick={() => setFileDialogOpen(true)}
          >
            <FileSpreadsheet className="mx-auto h-10 w-10 text-highlight mb-4" />
            <div className="text-base font-medium">{t('excelImport.file.choose')}</div>
            <div className="text-sm text-muted-foreground mt-1">
              {t('excelImport.file.supported')}
            </div>
            {selectedFile && (
              <div className="mt-4 inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
                <FileSpreadsheet className="h-4 w-4 text-success" />
                <span>{selectedFile.name}</span>
                <button
                  onClick={event => {
                    event.stopPropagation();
                    setSelectedFile(null);
                  }}
                  className="text-muted-foreground hover:text-foreground"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            )}
          </div>
          <div className="mt-6 flex justify-center">
            <Button
              onClick={() => handleAnalyze()}
              disabled={!selectedFile || !canAnalyzeImport || analyzeMutation.isPending}
              className="gap-2"
            >
              {analyzeMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Upload className="h-4 w-4" />
              )}
              {t('excelImport.file.analyze')}
            </Button>
          </div>
        </div>
      )}

      {step === 'preview' && analysis && (
        <div className="h-0 grow grid grid-cols-[260px_1fr] gap-4 overflow-hidden">
          <div className="rounded-md border overflow-auto">
            <div className="px-3 py-2 border-b text-sm font-medium">
              {t('excelImport.preview.sheets')}
            </div>
            <div className="p-2 space-y-2">
              {analysis.source.sheets.map(sheet => {
                const isSelected = activeSheetName === sheet.name;
                const isPending = isAnalyzingWorkbook && analyzingSheetName === sheet.name;
                return (
                  <button
                    key={sheet.name}
                    disabled={isAnalyzingWorkbook}
                    className={cn(
                      'w-full rounded-md border px-3 py-2 text-left text-sm hover:bg-muted disabled:cursor-wait disabled:opacity-70',
                      isSelected && 'border-highlight bg-highlight/10',
                      isPending && 'border-highlight bg-highlight/10'
                    )}
                    onClick={() => handleAnalyze({ sheet_name: sheet.name })}
                  >
                    <div className="flex items-center gap-2">
                      {isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                      <div className="font-medium truncate">{sheet.name}</div>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {sheet.row_count} x {sheet.column_count}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>

          <div className="rounded-md border overflow-hidden flex flex-col">
            <div className="px-3 py-2 border-b flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2 text-sm font-medium">
                <span>{t('excelImport.preview.rows')}</span>
              </div>
              <div className="flex items-center gap-2">
                <ModelSelector
                  modelType="text-chat"
                  value={selectedModel ?? undefined}
                  onChange={value => {
                    setSelectedModel(value);
                    if (user?.id) {
                      saveLastSelectedAiModel(user.id, 'excelImport', {
                        provider: value.provider,
                        model: value.model,
                      });
                    }
                  }}
                  placeholder={t('modelSelector.placeholder')}
                  className="min-w-[220px]"
                />
                <Button
                  onClick={handleEnterSchema}
                  disabled={
                    !isPreviewSheetReady ||
                    isAnalyzingWorkbook ||
                    !selectedModel?.model ||
                    recognizeMutation.isPending
                  }
                >
                  {(isAnalyzingWorkbook || recognizeMutation.isPending) && (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  )}
                  {t('excelImport.actions.next')}
                </Button>
              </div>
            </div>
            <div className="overflow-auto">
              {!isPreviewSheetReady && (
                <div className="flex items-center gap-2 border-b px-3 py-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  {t('excelImport.preview.loadingSheet')}
                </div>
              )}
              <Table>
              <TableHeader>
                <TableRow>
                  {analysis.columns.map((col, index) => (
                      <TableHead key={getExcelColumnKey(col, index)}>{col.display_name}</TableHead>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {analysis.preview_rows.map(row => (
                    <TableRow key={row.row_index}>
                      {analysis.columns.map((col, index) => (
                        <TableCell key={getExcelColumnKey(col, index)}>
                          {String(row.values[col.name] ?? '')}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        </div>
      )}

      {step === 'schema' && analysis && (
        <div className="h-0 grow flex flex-col overflow-hidden gap-4">
          <div className="flex flex-wrap items-start justify-between gap-3 rounded-md border bg-muted/30 px-3 py-3 sm:items-end">
            <div>
              <div className="flex items-center gap-2 text-sm font-medium">
                <Sparkles className="h-4 w-4 text-highlight" />
                <span>{t('excelImport.schema.smartRecognizeTitle')}</span>
              </div>
              <div className="text-xs text-muted-foreground">
                {t('excelImport.schema.smartRecognizeDesc')}
              </div>
            </div>
            <div className="flex w-full flex-col gap-2 sm:min-w-[360px] sm:flex-row sm:items-center lg:w-auto">
              <ModelSelector
                modelType="text-chat"
                value={selectedModel ?? undefined}
                onChange={value => {
                  setSelectedModel(value);
                  if (user?.id) {
                    saveLastSelectedAiModel(user.id, 'excelImport', {
                      provider: value.provider,
                      model: value.model,
                    });
                  }
                }}
                placeholder={t('modelSelector.placeholder')}
                className="w-full min-w-[220px] flex-1"
              />
              <Button
                variant="outline"
                onClick={handleSmartRecognize}
                disabled={!canRecognize}
                className="w-full gap-2 whitespace-nowrap sm:w-auto"
              >
                {recognizeMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Sparkles className="h-4 w-4" />
                )}
                {t('excelImport.schema.smartRecognizeAction')}
              </Button>
            </div>
          </div>
          <div className="rounded-md border p-3">
            <div className="text-sm font-medium">{t('excelImport.schema.tableInfoTitle')}</div>
            <div className="mt-3 grid grid-cols-1 gap-3 lg:grid-cols-2">
              <div className="space-y-1.5">
                <div className="text-xs text-muted-foreground">
                  {t('excelImport.schema.tableName')}
                </div>
                <Input
                  value={tableName}
                  onChange={event => setTableName(event.target.value)}
                  placeholder={t('excelImport.schema.tableName')}
                  className={
                    tableNameErrors.length > 0
                      ? 'border-destructive focus-visible:ring-destructive'
                      : undefined
                  }
                  aria-invalid={tableNameErrors.length > 0}
                />
                {tableNameErrorMessages.length > 0 && (
                  <div className="space-y-1 text-[13px] text-destructive">
                    {tableNameErrorMessages.map((message, index) => (
                      <div key={index}>{message}</div>
                    ))}
                  </div>
                )}
              </div>
              <div className="space-y-1.5">
                <div className="text-xs text-muted-foreground">
                  {t('excelImport.schema.description')}
                </div>
                <Input
                  value={tableDescription}
                  onChange={event => setTableDescription(event.target.value)}
                  placeholder={t('excelImport.schema.description')}
                />
              </div>
            </div>
          </div>
          {schemaValidationMessages.length > 0 && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>
                <div className="space-y-1">
                  {schemaValidationMessages.map((message, index) => (
                    <div key={getSampleValueKey(message, index)}>{message}</div>
                  ))}
                </div>
              </AlertDescription>
            </Alert>
          )}
          <div className="rounded-md border overflow-auto">
            <Table containerClassName="overflow-visible">
              <TableHeader className="[&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-[var(--table-header)]">
                <TableRow>
                  <TableHead className="w-20">{t('excelImport.schema.enabled')}</TableHead>
                  <TableHead>{t('excelImport.schema.source')}</TableHead>
                  <TableHead>{t('excelImport.schema.name')}</TableHead>
                  <TableHead className="min-w-64">
                    {t('excelImport.schema.descriptionColumn')}
                  </TableHead>
                  <TableHead className="w-44">{t('excelImport.schema.type')}</TableHead>
                  <TableHead className="w-32">
                    <div className="flex items-center gap-2">
                      <span>{t('excelImport.schema.required')}</span>
                      <Switch
                        checked={allEnabledColumnsRequired}
                        disabled={enabledColumns.length === 0}
                        onCheckedChange={checked =>
                          setColumns(prev =>
                            prev.map(item =>
                              item.enabled === false ? item : { ...item, is_required: checked }
                            )
                          )
                        }
                        aria-label={t(
                          allEnabledColumnsRequired
                            ? 'excelImport.schema.clearAllRequired'
                            : 'excelImport.schema.setAllRequired'
                        )}
                      />
                    </div>
                  </TableHead>
                  <TableHead>{t('excelImport.schema.samples')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {columns.map((col, index) => {
                  const lowerName = col.name.trim().toLowerCase();
                  const isDuplicateName =
                    col.enabled !== false && lowerName.length > 0 && duplicateNames.has(lowerName);
                  const isInvalidName = col.enabled !== false && isInvalidDbColumnName(col.name);
                  const isReservedName = col.enabled !== false && isReservedDbColumnName(col.name);
                  const hasNameError = isDuplicateName || isInvalidName || isReservedName;
                  const nameErrorMessage = isDuplicateName
                    ? t('columns.duplicateNameTip')
                    : isReservedName
                      ? t('columns.reservedNameTip')
                      : t('columns.invalidNameTip');

                  return (
                    <TableRow key={getExcelColumnKey(col, index)}>
                      <TableCell>
                        <Switch
                          checked={col.enabled !== false}
                          onCheckedChange={checked =>
                            setColumns(prev =>
                              prev.map((item, i) =>
                                i === index ? { ...item, enabled: checked } : item
                              )
                            )
                          }
                        />
                      </TableCell>
                      <TableCell>{col.source_column}</TableCell>
                      <TableCell>
                        <div className="relative">
                          <Input
                            value={col.name}
                            className={cn(
                              hasNameError
                                ? 'border-destructive focus-visible:ring-destructive pr-8'
                                : undefined
                            )}
                            aria-invalid={hasNameError}
                            onChange={event =>
                              setColumns(prev =>
                                prev.map((item, i) =>
                                  i === index ? { ...item, name: event.target.value } : item
                                )
                              )
                            }
                          />
                          {hasNameError && (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <AlertCircle className="absolute right-2 top-1/2 h-4 w-4 -translate-y-1/2 text-destructive" />
                              </TooltipTrigger>
                              <TooltipContent side="top" className="max-w-[320px]">
                                <div className="text-xs">{nameErrorMessage}</div>
                              </TooltipContent>
                            </Tooltip>
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Input
                          value={col.description}
                          onChange={event =>
                            setColumns(prev =>
                              prev.map((item, i) =>
                                i === index ? { ...item, description: event.target.value } : item
                              )
                            )
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <Select
                          value={col.type}
                          onValueChange={value =>
                            setColumns(prev =>
                              prev.map((item, i) =>
                                i === index ? { ...item, type: value as Type } : item
                              )
                            )
                          }
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {typeOptions.map(option => (
                              <SelectItem key={option} value={option}>
                                {t(`biSearch.columnTypes.${option}`)}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </TableCell>
                      <TableCell>
                        <Switch
                          checked={col.is_required}
                          onCheckedChange={checked =>
                            setColumns(prev =>
                              prev.map((item, i) =>
                                i === index ? { ...item, is_required: checked } : item
                              )
                            )
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-1">
                          {col.sample_values.map((value, sampleIndex) => (
                            <Badge key={getSampleValueKey(value, sampleIndex)} variant="secondary">
                              {value}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
          <div className="flex justify-between">
            <Button variant="outline" onClick={() => setStep('preview')}>
              {t('excelImport.actions.previous')}
            </Button>
            <Button
              onClick={handleConfirm}
              disabled={!canImport || !canExecuteImport || confirmMutation.isPending}
            >
              {confirmMutation.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              {t('excelImport.schema.import')}
            </Button>
          </div>
        </div>
      )}

      {step === 'result' && (
        <div className="h-0 grow flex items-center justify-center">
          <div className="w-full max-w-3xl rounded-md border p-6 text-center">
            <CheckCircle className="mx-auto h-12 w-12 text-success mb-4" />
            <div className="text-lg font-semibold">{t('excelImport.result.title')}</div>
            <div className="text-sm text-muted-foreground mt-2">
              {t('excelImport.result.description')}
            </div>
            {importResult && (
              <div className="mt-5 grid grid-cols-3 gap-3 text-left">
                <ResultStat
                  label={t('excelImport.result.totalRows')}
                  value={importResult.total_rows}
                />
                <ResultStat
                  label={t('excelImport.result.importedRows')}
                  value={importResult.imported_rows}
                />
                <ResultStat
                  label={t('excelImport.result.failedRows')}
                  value={importResult.failed_rows}
                  tone={importResult.failed_rows > 0 ? 'warning' : 'default'}
                />
              </div>
            )}
            {importResult && importResult.failed_rows > 0 && (
              <div className="mt-5 rounded-md border border-warning/40 bg-warning/10 p-3 text-left">
                <div className="text-sm font-medium">
                  {t('excelImport.result.failedItemsTitle')}
                </div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {t('excelImport.result.failedItemsDescription', {
                    count: importResult.failed_rows,
                  })}
                </div>
                <div className="mt-3 max-h-44 overflow-auto rounded-md border bg-background">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-20">{t('excelImport.result.errorRow')}</TableHead>
                        <TableHead className="w-40">
                          {t('excelImport.result.errorColumn')}
                        </TableHead>
                        <TableHead>{t('excelImport.result.errorReason')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {displayedFailedItems.map((item, index) => (
                        <TableRow key={`${item.row_index}-${item.column_name ?? ''}-${index}`}>
                          <TableCell>{item.row_index}</TableCell>
                          <TableCell>{item.column_name || '-'}</TableCell>
                          <TableCell>{item.error_message || item.error_code}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </div>
            )}
            <div className="mt-6 flex justify-center gap-2">
              {createdTableHref && (
                <Button asChild>
                  <Link href={createdTableHref}>
                    {canOpenCreatedTableRecords
                      ? t('excelImport.result.openTable')
                      : t('actions.manageStructure')}
                  </Link>
                </Button>
              )}
              <Button variant="outline" onClick={() => router.push(`/console/db/${dbId}`)}>
                {t('excelImport.actions.back')}
              </Button>
            </div>
          </div>
        </div>
      )}

      <Dialog
        open={recognitionDialogOpen}
        onOpenChange={open => {
          setRecognitionDialogOpen(open);
          if (!open) setRecognitionDraft(null);
        }}
      >
        <DialogContent size="xl">
          <DialogHeader>
            <DialogTitle>{t('excelImport.schema.recognitionDialogTitle')}</DialogTitle>
            <DialogDescription>{t('excelImport.schema.recognitionDialogDesc')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4">
            {recognitionDraft && (
              <>
                <div className="rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-48">
                          {t('excelImport.schema.recognitionItem')}
                        </TableHead>
                        <TableHead>{t('excelImport.schema.recognitionCurrent')}</TableHead>
                        <TableHead>{t('excelImport.schema.recognitionSuggested')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      <TableRow>
                        <TableCell className="font-medium">
                          {t('excelImport.schema.recognitionTableName')}
                        </TableCell>
                        <TableCell className="break-words">
                          {tableName || t('excelImport.schema.recognitionEmpty')}
                        </TableCell>
                        <TableCell className="break-words">
                          {recognitionDraft.table.name || t('excelImport.schema.recognitionEmpty')}
                        </TableCell>
                      </TableRow>
                      <TableRow>
                        <TableCell className="font-medium">
                          {t('excelImport.schema.recognitionTableDescription')}
                        </TableCell>
                        <TableCell className="break-words">
                          {tableDescription || t('excelImport.schema.recognitionEmpty')}
                        </TableCell>
                        <TableCell className="break-words">
                          {recognitionDraft.table.description ||
                            t('excelImport.schema.recognitionEmpty')}
                        </TableCell>
                      </TableRow>
                    </TableBody>
                  </Table>
                </div>

                <div className="rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-56">
                          {t('excelImport.schema.recognitionSourceColumn')}
                        </TableHead>
                        <TableHead>{t('excelImport.schema.recognitionCurrent')}</TableHead>
                        <TableHead>{t('excelImport.schema.recognitionSuggested')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {columns.map((col, index) => {
                        const suggestion = recognitionDraft.columns[index];
                        return (
                          <TableRow key={getExcelColumnKey(col, index)}>
                            <TableCell className="break-words font-medium">
                              {col.source_column || col.display_name}
                            </TableCell>
                            <TableCell className="break-words">{col.name}</TableCell>
                            <TableCell className="break-words">
                              {suggestion?.name || t('excelImport.schema.recognitionEmpty')}
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </div>
              </>
            )}
          </DialogBody>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setRecognitionDraft(null);
                setRecognitionDialogOpen(false);
              }}
            >
              {t('excelImport.schema.cancelRecognition')}
            </Button>
            <Button onClick={handleApplyRecognition} disabled={!recognitionDraft}>
              {t('excelImport.schema.applyRecognition')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <FileSelectorDialog
        open={fileDialogOpen}
        onOpenChange={setFileDialogOpen}
        maxCount={1}
        initSelectedFiles={selectedFile ? [selectedFile] : []}
        acceptExt={['xlsx', 'xls', 'csv']}
        onConfirm={files => {
          setSelectedFile(files[0] ?? null);
          setSelectedSheetName(null);
          setFileDialogOpen(false);
        }}
      />
    </div>
  );
}

function ResultStat({
  label,
  value,
  tone = 'default',
}: {
  label: string;
  value: number;
  tone?: 'default' | 'warning';
}) {
  return (
    <div className="rounded-md border p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className={cn('mt-1 text-xl font-semibold', tone === 'warning' && 'text-warning')}>
        {value}
      </div>
    </div>
  );
}

function StepIndicator({ step }: { step: Step }) {
  const t = useT('dbs');
  const steps: Step[] = ['file', 'preview', 'schema', 'result'];
  const activeIndex = steps.indexOf(step);

  return (
    <StepsProgressBar
      currentStep={activeIndex + 1}
      totalSteps={steps.length}
      getStepInfo={stepNumber => ({
        title: t(`excelImport.steps.${steps[stepNumber - 1]}`),
      })}
      allowStepNavigation={false}
      className="w-[560px]"
    />
  );
}
