'use client';

import { useMemo, useState } from 'react';
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
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import type { FileItem } from '@/services/types/file';
import {
  Type,
  type AnalyzeExcelImportData,
  type ConfirmExcelImportData,
  type ConfirmExcelImportRequest,
  type InferredExcelColumn,
} from '@/services/types/db';
import {
  useAnalyzeExcelImport,
  useConfirmExcelImport,
  useExcelImportErrors,
} from '@/hooks/db/use-excel-import';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

type Step = 'file' | 'preview' | 'schema' | 'result';

interface ExcelImportShellProps {
  dbId: string;
}

const typeOptions = [Type.Text, Type.Integer, Type.Numeric, Type.Boolean, Type.Timestamp] as const;
const fieldNamePattern = /^[a-z_][a-z0-9_]*$/;
const tableNamePattern = /^[a-z][a-z0-9_]*$/;
const reservedFieldNames = new Set(['id', 'uuid', 'created_time', 'updated_time']);
const genericFieldNamePattern = /^(column|field)_\d+$/;

function getExcelColumnKey(col: InferredExcelColumn, index: number) {
  return [col.source_column_index, col.source_column || 'source', col.name || 'field', index].join(
    ':'
  );
}

function getSampleValueKey(value: unknown, index: number) {
  return `${index}:${String(value)}`;
}

const sourcePhraseTranslations: Readonly<Record<string, string>> = {
  序号: 'number',
  员工关系: 'employee relation',
  用工形式: 'employment type',
  姓名: 'name',
  隶属部门: 'department',
  岗位: 'position',
  身份: 'identity',
  岗位类别: 'position category',
  职级: 'job level',
  入司日期: 'hire date',
  待转正日期: 'probation conversion date',
  联系方式: 'contact information',
  住院号: 'inpatient number',
  入院时间: 'admission time',
  上级医师查房记录: 'senior physician ward round record',
  标识: 'identifier',
  操作时间: 'operation time',
};

const sourceTokenTranslations: ReadonlyArray<readonly [string, string]> = [
  ['待转正', 'probation conversion'],
  ['员工关系', 'employee relation'],
  ['用工形式', 'employment type'],
  ['隶属部门', 'department'],
  ['岗位类别', 'position category'],
  ['上级医师', 'senior physician'],
  ['查房记录', 'ward round record'],
  ['联系方式', 'contact information'],
  ['入司', 'hire'],
  ['转正', 'conversion'],
  ['员工', 'employee'],
  ['用工', 'employment'],
  ['形式', 'type'],
  ['隶属', 'affiliated'],
  ['部门', 'department'],
  ['岗位', 'position'],
  ['身份', 'identity'],
  ['职级', 'job level'],
  ['类别', 'category'],
  ['序号', 'number'],
  ['联系', 'contact'],
  ['住院', 'inpatient'],
  ['入院', 'admission'],
  ['出院', 'discharge'],
  ['医师', 'physician'],
  ['医生', 'doctor'],
  ['护士', 'nurse'],
  ['患者', 'patient'],
  ['病人', 'patient'],
  ['查房', 'ward round'],
  ['记录', 'record'],
  ['时间', 'time'],
  ['日期', 'date'],
  ['编号', 'number'],
  ['号码', 'number'],
  ['标识', 'identifier'],
  ['操作', 'operation'],
  ['状态', 'status'],
  ['名称', 'name'],
  ['姓名', 'name'],
  ['类型', 'type'],
  ['备注', 'note'],
  ['描述', 'description'],
  ['科室', 'department'],
  ['医院', 'hospital'],
  ['病情', 'condition'],
  ['体重', 'weight'],
  ['血压', 'blood pressure'],
  ['脉搏', 'pulse'],
  ['呼吸', 'respiration'],
  ['主诉', 'chief complaint'],
  ['诊断', 'diagnosis'],
  ['治疗', 'treatment'],
  ['护理', 'nursing'],
  ['号', 'number'],
];

const sampleValueSemanticRules: ReadonlyArray<{
  test: (samples: readonly string[]) => boolean;
  phrase: string;
  type?: Type;
}> = [
  {
    test: samples => samples.some(value => /^1[3-9]\d{9}$/.test(value.replace(/\s+/g, ''))),
    phrase: 'phone number',
    type: Type.Text,
  },
  {
    test: samples => samples.some(value => /^\d{4}[-/.年]\d{1,2}/.test(value)),
    phrase: 'date',
    type: Type.Timestamp,
  },
  {
    test: samples => samples.some(value => value.includes('@')),
    phrase: 'email',
    type: Type.Text,
  },
];

function slugifyEnglish(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .replace(/_+/g, '_');
}

function translateSourceColumn(source: string): string {
  const trimmed = source.trim();
  if (!trimmed) return '';

  const exact = sourcePhraseTranslations[trimmed];
  if (exact) return exact;

  if (/^[\w\s-]+$/.test(trimmed)) return trimmed;

  const words: string[] = [];
  let index = 0;
  while (index < trimmed.length) {
    const match = sourceTokenTranslations.find(([token]) => trimmed.startsWith(token, index));
    if (match) {
      words.push(match[1]);
      index += match[0].length;
      continue;
    }
    index += 1;
  }

  return words.join(' ');
}

function inferFieldPhrase(col: InferredExcelColumn): string {
  const translatedSource = translateSourceColumn(col.source_column || col.display_name || '');
  if (translatedSource) return translatedSource;

  const sampleRule = sampleValueSemanticRules.find(rule =>
    rule.test((col.sample_values || []).map(value => String(value).trim()))
  );
  if (sampleRule) return sampleRule.phrase;

  return col.display_name || col.name;
}

function inferFieldType(col: InferredExcelColumn): Type {
  const sampleRule = sampleValueSemanticRules.find(rule =>
    rule.test((col.sample_values || []).map(value => String(value).trim()))
  );
  return sampleRule?.type || col.type;
}

function makeUniqueFieldName(base: string, used: Set<string>, fallbackIndex: number): string {
  const fallback = `field_${fallbackIndex + 1}`;
  const normalized = slugifyEnglish(base) || fallback;
  const safeBase = /^[a-z]/.test(normalized) ? normalized : `field_${normalized}`;
  let candidate = reservedFieldNames.has(safeBase) ? `${safeBase}_value` : safeBase;
  let suffix = 2;

  while (used.has(candidate) || reservedFieldNames.has(candidate)) {
    candidate = `${safeBase}_${suffix}`;
    suffix += 1;
  }

  used.add(candidate);
  return candidate;
}

function normalizeInferredColumns(incoming: InferredExcelColumn[]): InferredExcelColumn[] {
  const used = new Set<string>();

  return incoming.map((col, index) => {
    const originalName = col.name.trim();
    const translatedSource = inferFieldPhrase(col);
    const needsSemanticName =
      !fieldNamePattern.test(originalName) ||
      reservedFieldNames.has(originalName) ||
      genericFieldNamePattern.test(originalName);
    const semanticName = makeUniqueFieldName(
      needsSemanticName ? translatedSource : originalName,
      used,
      index
    );
    const englishDisplayName = translatedSource || semanticName;

    return {
      ...col,
      name: semanticName,
      display_name: englishDisplayName,
      description: `Imported from ${englishDisplayName}`,
      type: inferFieldType(col),
      enabled: col.enabled ?? true,
    };
  });
}

function smartRecognizeFieldNames(incoming: InferredExcelColumn[]): InferredExcelColumn[] {
  const used = new Set<string>();

  return incoming.map((col, index) => {
    if (col.enabled === false) return col;
    const phrase = inferFieldPhrase(col);
    const nextName = makeUniqueFieldName(phrase, used, index);
    const nextType = inferFieldType(col);
    return {
      ...col,
      name: nextName,
      display_name: phrase || nextName,
      description: `Imported from ${phrase || nextName}`,
      type: nextType,
    };
  });
}

export default function ExcelImportShell({ dbId }: ExcelImportShellProps) {
  const t = useT('dbs');
  const router = useRouter();
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canManage = hasPermission('database.manage');
  const [step, setStep] = useState<Step>('file');
  const [fileDialogOpen, setFileDialogOpen] = useState(false);
  const [selectedFile, setSelectedFile] = useState<FileItem | null>(null);
  const [analysis, setAnalysis] = useState<AnalyzeExcelImportData | null>(null);
  const [columns, setColumns] = useState<InferredExcelColumn[]>([]);
  const [tableName, setTableName] = useState('');
  const [tableDescription, setTableDescription] = useState('');
  const [createdTableId, setCreatedTableId] = useState<string | null>(null);
  const [importResult, setImportResult] = useState<ConfirmExcelImportData | null>(null);

  const analyzeMutation = useAnalyzeExcelImport(dbId);
  const confirmMutation = useConfirmExcelImport(dbId, analysis?.job_id);
  const importErrorsQuery = useExcelImportErrors(
    dbId,
    importResult?.job_id,
    { limit: 20, offset: 0 },
    step === 'result' && Boolean(importResult && importResult.failed_rows > 0)
  );

  const enabledColumns = useMemo(() => columns.filter(col => col.enabled !== false), [columns]);
  const duplicateNames = useMemo(() => {
    const seen = new Set<string>();
    const dup = new Set<string>();
    enabledColumns.forEach(col => {
      const name = col.name.trim();
      if (seen.has(name)) dup.add(name);
      seen.add(name);
    });
    return dup;
  }, [enabledColumns]);

  const invalidFieldNames = useMemo(
    () =>
      enabledColumns.filter(col => {
        const name = col.name.trim();
        return !fieldNamePattern.test(name) || reservedFieldNames.has(name);
      }),
    [enabledColumns]
  );
  const tableNameValue = tableName.trim();
  const tableNameInvalid = Boolean(tableNameValue) && !tableNamePattern.test(tableNameValue);
  const schemaValidationMessages = useMemo(() => {
    const messages: string[] = [];
    if (tableNameInvalid) messages.push(t('excelImport.schema.validation.invalidTableName'));
    if (duplicateNames.size > 0) messages.push(t('excelImport.schema.validation.duplicateFields'));
    if (invalidFieldNames.length > 0) {
      messages.push(t('excelImport.schema.validation.invalidFieldNames'));
    }
    return messages;
  }, [duplicateNames.size, invalidFieldNames.length, t, tableNameInvalid]);

  const canImport =
    Boolean(tableNameValue) &&
    !tableNameInvalid &&
    enabledColumns.length > 0 &&
    duplicateNames.size === 0 &&
    invalidFieldNames.length === 0;

  const handleAnalyze = async (overrides?: { sheet_name?: string; header_row?: number }) => {
    if (!selectedFile || !canManage) return;
    const res = await analyzeMutation.mutateAsync({
      upload_file_id: selectedFile.id,
      sample_size: 500,
      ...overrides,
    });
    setAnalysis(res.data);
    setColumns(normalizeInferredColumns(res.data.columns));
    if (!tableName) {
      const baseName = res.data.source.file_name.replace(/\.[^.]+$/, '').toLowerCase();
      const normalizedName =
        baseName.replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '') || 'imported_table';
      setTableName(/^[a-z]/.test(normalizedName) ? normalizedName : `table_${normalizedName}`);
    }
    setStep('preview');
  };

  const handleConfirm = async () => {
    if (!analysis || !canImport || !canManage) return;
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

  const handleSmartRecognizeFieldNames = () => {
    setColumns(prev => smartRecognizeFieldNames(prev));
  };

  const displayedFailedItems =
    importErrorsQuery.data?.data.data ?? importResult?.failed_items ?? [];

  if (isPermissionsLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!canManage) {
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
              disabled={!selectedFile || analyzeMutation.isPending}
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
              {analysis.source.sheets.map(sheet => (
                <button
                  key={sheet.name}
                  className={cn(
                    'w-full rounded-md border px-3 py-2 text-left text-sm hover:bg-muted',
                    analysis.selection.sheet_name === sheet.name &&
                      'border-highlight bg-highlight/10'
                  )}
                  onClick={() => handleAnalyze({ sheet_name: sheet.name })}
                >
                  <div className="font-medium truncate">{sheet.name}</div>
                  <div className="text-xs text-muted-foreground">
                    {sheet.row_count} x {sheet.column_count}
                  </div>
                </button>
              ))}
            </div>
          </div>

          <div className="rounded-md border overflow-hidden flex flex-col">
            <div className="px-3 py-2 border-b flex items-center justify-between">
              <div className="text-sm font-medium">{t('excelImport.preview.rows')}</div>
              <Button onClick={() => setStep('schema')}>{t('excelImport.actions.next')}</Button>
            </div>
            <div className="overflow-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-20">#</TableHead>
                    {analysis.columns.map((col, index) => (
                      <TableHead key={getExcelColumnKey(col, index)}>{col.display_name}</TableHead>
                    ))}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {analysis.preview_rows.map(row => (
                    <TableRow key={row.row_index}>
                      <TableCell>{row.row_index}</TableCell>
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
          <div className="grid grid-cols-2 gap-3">
            <Input
              value={tableName}
              onChange={event => setTableName(event.target.value)}
              placeholder={t('excelImport.schema.tableName')}
              className={tableNameInvalid ? 'border-destructive' : undefined}
            />
            <Input
              value={tableDescription}
              onChange={event => setTableDescription(event.target.value)}
              placeholder={t('excelImport.schema.description')}
            />
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2">
            <div>
              <div className="text-sm font-medium">{t('excelImport.schema.smartNamesTitle')}</div>
              <div className="text-xs text-muted-foreground">
                {t('excelImport.schema.smartNamesDesc')}
              </div>
            </div>
            <Button variant="outline" onClick={handleSmartRecognizeFieldNames} className="gap-2">
              <Sparkles className="h-4 w-4" />
              {t('excelImport.schema.smartNamesAction')}
            </Button>
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
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-20">{t('excelImport.schema.enabled')}</TableHead>
                  <TableHead>{t('excelImport.schema.source')}</TableHead>
                  <TableHead>{t('excelImport.schema.name')}</TableHead>
                  <TableHead className="w-44">{t('excelImport.schema.type')}</TableHead>
                  <TableHead className="w-24">{t('excelImport.schema.required')}</TableHead>
                  <TableHead>{t('excelImport.schema.samples')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {columns.map((col, index) => (
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
                      <Input
                        value={col.name}
                        className={cn(
                          (duplicateNames.has(col.name.trim()) ||
                            !fieldNamePattern.test(col.name.trim()) ||
                            reservedFieldNames.has(col.name.trim())) &&
                            col.enabled !== false
                            ? 'border-destructive'
                            : undefined
                        )}
                        onChange={event =>
                          setColumns(prev =>
                            prev.map((item, i) =>
                              i === index ? { ...item, name: event.target.value } : item
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
                ))}
              </TableBody>
            </Table>
          </div>
          <div className="flex justify-between">
            <Button variant="outline" onClick={() => setStep('preview')}>
              {t('excelImport.actions.previous')}
            </Button>
            <Button onClick={handleConfirm} disabled={!canImport || confirmMutation.isPending}>
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
              {createdTableId && (
                <Button onClick={() => router.push(`/console/db/${dbId}/table/${createdTableId}`)}>
                  {t('excelImport.result.openTable')}
                </Button>
              )}
              <Button variant="outline" onClick={() => router.push(`/console/db/${dbId}`)}>
                {t('excelImport.actions.back')}
              </Button>
            </div>
          </div>
        </div>
      )}

      <FileSelectorDialog
        open={fileDialogOpen}
        onOpenChange={setFileDialogOpen}
        maxCount={1}
        initSelectedFiles={selectedFile ? [selectedFile] : []}
        acceptExt={['xlsx', 'xls', 'csv']}
        onConfirm={files => {
          setSelectedFile(files[0] ?? null);
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
    <div className="flex items-center gap-2">
      {steps.map((item, index) => (
        <div
          key={item}
          className={cn(
            'h-7 rounded-md border px-2 text-xs flex items-center',
            index <= activeIndex
              ? 'border-highlight text-highlight bg-highlight/10'
              : 'text-muted-foreground'
          )}
        >
          {t(`excelImport.steps.${item}`)}
        </div>
      ))}
    </div>
  );
}
