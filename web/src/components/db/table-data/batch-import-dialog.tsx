'use client';

import type { FC } from 'react';
import React, { useCallback, useMemo, useState } from 'react';
import { AlertCircle, AlertTriangle, ArrowLeft, Check, CheckCircle2, Download, FileSpreadsheet, Loader, Search } from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import { useDownloadDbTableTemplate, useExistingTableExcelImport } from '@/hooks/db/use-db-table-import';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useT } from '@/i18n';
import { dbService } from '@/services';
import type { ExistingTableExcelImportDraftRequest, ExistingTableExcelImportMapping, ExistingTableExcelImportPreviewData, ConfirmExcelImportData, ExcelImportSheet } from '@/services/types/db';
import type { FileItem } from '@/services/types/file';
import { getErrorMessage } from '@/utils/error-notifications';

export interface BatchImportDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dbId: string;
  tableId: string;
  onSuccess?: () => void;
}

type Step = 'file' | 'mapping' | 'preview' | 'result';

const BatchImportDialog: FC<BatchImportDialogProps> = ({ open, onOpenChange, dbId, tableId, onSuccess }) => {
  const t = useT('dbs.batchImport');
  const { analyze, preview, confirm } = useExistingTableExcelImport(dbId, tableId);
  const { downloadTemplate, isDownloading } = useDownloadDbTableTemplate(dbId, tableId);
  const defaultModel = useDefaultModelByUseCase('text-chat');
  const [step, setStep] = useState<Step>('file');
  const [selectedFile, setSelectedFile] = useState<FileItem | null>(null);
  const [fileSelectorOpen, setFileSelectorOpen] = useState(false);
  const [mappings, setMappings] = useState<ExistingTableExcelImportMapping[]>([]);
  const [previewData, setPreviewData] = useState<ExistingTableExcelImportPreviewData | null>(null);
  const [draft, setDraft] = useState<ExistingTableExcelImportDraftRequest | null>(null);
  const [rowChanges, setRowChanges] = useState<Record<number, Record<string, string>>>({});
  const [skippedRows, setSkippedRows] = useState<number[]>([]);
  const [result, setResult] = useState<ConfirmExcelImportData | null>(null);
  const [isEnriching, setIsEnriching] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [previewQuery, setPreviewQuery] = useState('');
  const [availableSheets, setAvailableSheets] = useState<ExcelImportSheet[]>([]);
  const [selectedSheetName, setSelectedSheetName] = useState('');

  const analysis = analyze.data?.data;
  const targetColumns = analysis?.target_columns ?? [];
  const targetColumnLabel = (column: (typeof targetColumns)[number]) => {
    const businessLabel = column.source_column_name?.trim() || column.display_name?.trim() || column.name;
    return businessLabel === column.name ? column.name : `${businessLabel} - ${column.name}`;
  };
  const usedTargetIds = useMemo(
    () => new Set(mappings.filter(item => item.action === 'map').map(item => item.target_column_id)),
    [mappings]
  );
  const mappingReady = mappings.length > 0 && mappings.every(item => {
    if (!item.confirmed) return false;
    if (item.action === 'map') return Boolean(item.source_column && item.target_column_id);
    if (item.action === 'fixed') return Boolean(item.target_column_id && item.fixed_value?.trim());
    return !item.target_required;
  }) && targetColumns.every(column => mappings.some(item => item.target_column_id === column.id));
  const pendingMappingCount = mappings.filter(item => {
    if (!item.confirmed) return true;
    if (item.action === 'map') return !item.source_column || !item.target_column_id;
    if (item.action === 'fixed') return !item.target_column_id || !item.fixed_value?.trim();
    return item.target_required;
  }).length;

  const reset = useCallback(() => {
    analyze.reset(); preview.reset(); confirm.reset();
    setStep('file'); setSelectedFile(null); setMappings([]); setPreviewData(null);
    setDraft(null); setRowChanges({}); setSkippedRows([]); setResult(null);
    setPreviewQuery(''); setAvailableSheets([]); setSelectedSheetName('');
  }, [analyze, preview, confirm]);

  const handleOpenChange = useCallback((next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  }, [onOpenChange, reset]);

  const inspectFile = async (file: FileItem) => {
    try {
      const response = await analyze.mutateAsync({ upload_file_id: file.id });
      const sheets = response.data.source.source.sheets.filter(sheet => !sheet.hidden);
      setAvailableSheets(sheets);
      setSelectedSheetName(sheets.length === 1 ? sheets[0].name : '');
    } catch (error) {
      toast.error(getErrorMessage(error));
    }
  };

  const handleFileConfirm = (files: FileItem[]) => {
    const file = files[0];
    if (!file) return;
    if (!['xlsx', 'xls'].includes(file.extension?.toLowerCase() ?? '')) {
      toast.error(t('invalidFileType'));
      return;
    }
    setSelectedFile(file);
    setAvailableSheets([]);
    setSelectedSheetName('');
    void inspectFile(file);
  };

  const enterMapping = (data: NonNullable<typeof analysis>) => {
    setMappings(data.mappings);
    setStep('mapping');
    void enrichWithModel(data);
  };

  const handleAnalyze = async () => {
    if (!selectedFile || !selectedSheetName) return;
    if (analysis?.source.selection.sheet_name === selectedSheetName) {
      enterMapping(analysis);
      return;
    }
    try {
      const response = await analyze.mutateAsync({
        upload_file_id: selectedFile.id,
        sheet_name: selectedSheetName,
      });
      enterMapping(response.data);
    } catch (error) { toast.error(getErrorMessage(error)); }
  };

  const enrichWithModel = async (data: NonNullable<typeof analysis>) => {
    if (!defaultModel.value) return;
    setIsEnriching(true);
    try {
      const response = await dbService.recognizeExcelImport(dbId, data.job_id, {
        table: { name: 'existing_table_import', description: '' },
        source: { file_name: data.source.source.file_name, sheet_name: data.source.selection.sheet_name },
        columns: data.source.columns,
        model: { provider: defaultModel.value.provider, name: defaultModel.value.model },
        operator_language: 'zh-CN',
      });
      const normalize = (value: string) => value.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '');
      setMappings(current => {
        const occupied = new Set(current.filter(item => item.action === 'map').map(item => item.target_column_id));
        const enriched = current.map(item => {
          if (item.confirmed || item.status === 'exact' || !item.source_column) return item;
          const recognized = response.data.columns.find(column => column.source_column_index === item.source_column_index);
          if (!recognized) return item;
          const names = [recognized.name, recognized.display_name].map(normalize);
          const target = data.target_columns.find(column => !occupied.has(column.id) && names.some(name => name && [column.name, column.display_name ?? ''].map(normalize).includes(name)));
          if (!target) return item;
          occupied.add(target.id);
          return { ...item, target_column_id: target.id, target_column_name: target.name, target_display_name: target.display_name ?? target.name, target_type: target.type, target_required: target.is_required, confidence: Math.max(item.confidence, 75), status: 'possible' as const, reason: t('modelSuggested'), action: 'map' as const };
        });
        return enriched.filter(item => item.source_column || !occupied.has(item.target_column_id));
      });
    } catch {
      // Model enrichment is optional; local matching remains fully usable.
    } finally { setIsEnriching(false); }
  };

  const updateMapping = (index: number, patch: Partial<ExistingTableExcelImportMapping>) => {
    setMappings(current => current.map((item, itemIndex) => itemIndex === index ? { ...item, ...patch } : item));
  };

  const fallbackForTarget = (targetId: string): ExistingTableExcelImportMapping | null => {
    const target = targetColumns.find(column => column.id === targetId);
    if (!target) return null;
    return {
      target_column_id: target.id,
      target_column_name: target.name,
      target_display_name: target.display_name ?? target.name,
      target_type: target.type,
      target_required: target.is_required,
      confidence: 0,
      status: 'unmatched',
      reason: t('targetMissing'),
      action: target.is_required ? 'fixed' : 'skip',
      confirmed: false,
    };
  };

  const selectTarget = (index: number, targetId: string) => {
    const target = targetColumns.find(column => column.id === targetId);
    if (!target) return;
    setMappings(current => {
      const previousTargetId = current[index]?.target_column_id;
      const next = current
        .filter((item, itemIndex) => itemIndex === index || item.source_column || item.target_column_id !== targetId)
        .map(item => item);
      const currentIndex = next.findIndex(item => item === current[index]);
      next[currentIndex] = { ...next[currentIndex], target_column_id: target.id, target_column_name: target.name, target_display_name: target.display_name ?? target.name, target_type: target.type, target_required: target.is_required, action: 'map', confirmed: true };
      if (previousTargetId && previousTargetId !== targetId && !next.some(item => item.target_column_id === previousTargetId)) {
        const fallback = fallbackForTarget(previousTargetId);
        if (fallback) next.push(fallback);
      }
      return next;
    });
  };

  const skipSource = (index: number) => {
    setMappings(current => {
      const previousTargetId = current[index]?.target_column_id;
      const next = current.map((item, itemIndex) => itemIndex === index ? { ...item, action: 'skip' as const, target_column_id: '', confirmed: true } : item);
      if (previousTargetId && !next.some(item => item.target_column_id === previousTargetId)) {
        const fallback = fallbackForTarget(previousTargetId);
        if (fallback) next.push(fallback);
      }
      return next;
    });
  };

  const handlePreview = async () => {
    if (!analysis || !mappingReady) return;
    const nextDraft = { mappings, row_changes: rowChanges, skipped_rows: skippedRows, limit: 100, offset: 0 };
    try {
      const response = await preview.mutateAsync({ jobId: analysis.job_id, draft: nextDraft });
      setDraft(nextDraft); setPreviewData(response.data); setStep('preview');
    } catch (error) { toast.error(getErrorMessage(error)); }
  };

  const refreshPreview = async () => {
    if (!analysis || !draft) return false;
    const nextDraft = { ...draft, row_changes: rowChanges, skipped_rows: skippedRows };
    try {
      const response = await preview.mutateAsync({ jobId: analysis.job_id, draft: nextDraft });
      setDraft(nextDraft); setPreviewData(response.data);
      return true;
    } catch (error) { toast.error(getErrorMessage(error)); return false; }
  };

  const handleConfirm = async () => {
    if (!analysis) return;
    try {
      const response = await confirm.mutateAsync({ jobId: analysis.job_id });
      setResult(response.data); setStep('result'); onSuccess?.();
    } catch (error) { toast.error(getErrorMessage(error)); }
  };

  const prepareConfirmation = async () => {
    if (await refreshPreview()) setConfirmOpen(true);
  };

  const handleDownloadTemplate = async () => {
    try {
      await downloadTemplate();
      toast.success(t('downloadSuccess'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('downloadFailed'));
    }
  };

  const steps = ['file', 'mapping', 'preview', 'result'] as Step[];
  const activeStep = steps.indexOf(step);
  const statusCounts = {
    exact: mappings.filter(item => item.status === 'exact').length,
    possible: mappings.filter(item => item.status === 'possible').length,
    unmatched: mappings.filter(item => item.status === 'unmatched').length,
  };
  const showFirstMatchHint = !isEnriching && statusCounts.exact === 0 && statusCounts.possible > 0;
  const previewRows = useMemo(() => {
    const query = previewQuery.trim().toLowerCase();
    if (!query || !previewData) return previewData?.rows ?? [];
    return previewData.rows.filter(row => Object.values(row.cells).some(cell =>
      String(cell.transformed_value ?? cell.original_value ?? '').toLowerCase().includes(query)
    ));
  }, [previewData, previewQuery]);
  const previewColumnLabel = (name: string) => {
    const mapping = mappings.find(item => item.target_column_name === name);
    const businessLabel = mapping?.source_column?.trim() || mapping?.target_display_name?.trim() || name;
    return { businessLabel, sqlName: name };
  };

  const stepper = (
    <div className="flex items-center gap-3 text-xs text-muted-foreground max-sm:grid max-sm:w-full max-sm:grid-cols-4 max-sm:gap-1">
      {steps.map((item, index) => (
        <React.Fragment key={item}>
          <span className="flex items-center gap-2 whitespace-nowrap max-sm:justify-center">
            <span className={`flex size-6 items-center justify-center rounded-full ${index < activeStep ? 'bg-emerald-500 text-white' : index === activeStep ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}>
              {index < activeStep ? <Check className="size-3.5" /> : index + 1}
            </span>
            <span className={`max-sm:hidden ${index === activeStep ? 'font-medium text-foreground' : ''}`}>{t(`steps.${item}`)}</span>
          </span>
          {index < steps.length - 1 && <span className="h-px w-5 bg-border max-sm:hidden" />}
        </React.Fragment>
      ))}
    </div>
  );

  const wideStep = step === 'preview' || step === 'result';

  return (
    <>
      {!wideStep && <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent
          className={step === 'mapping'
            ? 'w-[900px] max-w-[calc(100vw-32px)] overflow-hidden'
            : 'w-[540px] max-w-[calc(100vw-32px)] overflow-hidden'}
        >
          {step === 'file' && (
            <>
              <DialogHeader><DialogTitle>{t('title')}</DialogTitle></DialogHeader>
              <DialogBody className="space-y-6 pb-5">
                <section className="space-y-3">
                  <h3 className="font-semibold">{t('step1Title')}</h3>
                  <p className="text-sm text-muted-foreground">{t('step1Desc')}</p>
                  <Button onClick={handleDownloadTemplate} disabled={isDownloading}>
                    {isDownloading ? <Loader className="mr-2 size-4 animate-spin" /> : <Download className="mr-2 size-4" />}
                    {t('downloadTemplate')}
                  </Button>
                </section>
                <section className="space-y-3">
                  <h3 className="font-semibold">{t('step2Title')}</h3>
                  <div className="mx-auto flex h-60 w-60 flex-col items-center justify-center rounded-2xl border bg-background text-center shadow-sm">
                    <FileSpreadsheet className="mb-4 size-9 text-muted-foreground" />
                    <p className="max-w-48 truncate text-sm font-semibold">{selectedFile?.name ?? t('dropOrClick')}</p>
                    <p className="mt-1 text-xs text-muted-foreground">{t('supportedFormats')}</p>
                    <Button className="mt-5" variant="outline" onClick={() => setFileSelectorOpen(true)}>{t('selectFile')}</Button>
                  </div>
                </section>
                {selectedFile && analyze.isPending && (
                  <section className="flex items-center justify-center gap-2 rounded-xl border bg-muted/20 p-4 text-sm text-muted-foreground">
                    <Loader className="size-4 animate-spin" />
                    {t('sheetLoading')}
                  </section>
                )}
                {availableSheets.length > 1 && (
                  <section className="space-y-3 rounded-xl border border-primary/20 bg-primary/[0.03] p-4">
                    <div><h3 className="font-semibold">{t('selectSheetTitle')}</h3><p className="mt-1 text-xs text-muted-foreground">{t('selectSheetDesc', { count: availableSheets.length })}</p></div>
                    <div className="grid gap-2 sm:grid-cols-2">
                      {availableSheets.map(sheet => {
                        const selected = selectedSheetName === sheet.name;
                        return (
                          <button
                            key={sheet.name}
                            type="button"
                            aria-pressed={selected}
                            className={`flex min-w-0 items-center gap-3 rounded-lg border p-3 text-left transition-colors ${selected ? 'border-primary/40 bg-primary/[0.06]' : 'bg-background hover:border-primary/30'}`}
                            onClick={() => setSelectedSheetName(sheet.name)}
                          >
                            <span className={`flex size-4 shrink-0 items-center justify-center rounded-full border ${selected ? 'border-primary' : 'border-muted-foreground/30'}`}>
                              {selected && <span className="size-2 rounded-full bg-primary" />}
                            </span>
                            <FileSpreadsheet className="size-4 shrink-0 text-primary" />
                            <span className="min-w-0">
                              <span className="block truncate text-sm font-medium">{sheet.name}</span>
                              <span className="block text-xs text-muted-foreground">{t('sheetSize', { rows: sheet.row_count, columns: sheet.column_count })}</span>
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  </section>
                )}
              </DialogBody>
              <DialogFooter className="border-t py-4">
                <Button variant="outline" onClick={() => handleOpenChange(false)}>{t('cancel')}</Button>
                <Button onClick={handleAnalyze} disabled={!selectedFile || analyze.isPending || !selectedSheetName}>
                  {analyze.isPending && <Loader className="mr-2 size-4 animate-spin" />}{t('import')}
                </Button>
              </DialogFooter>
            </>
          )}

          {step === 'mapping' && analysis && (
            <>
              <DialogHeader className="space-y-3 border-b pb-4">
                <DialogTitle>{t('mappingTitle')}</DialogTitle>
                <p className="text-sm text-muted-foreground">{isEnriching ? t('matchingSubtitle') : t('mappingSubtitle')}</p>
                {stepper}
              </DialogHeader>
              <DialogBody className="space-y-3 py-4">
                <div className="flex items-center justify-between rounded-xl border px-3 py-3">
                  <div className="flex min-w-0 items-center gap-3">
                    <FileSpreadsheet className="size-5 shrink-0 text-emerald-500" />
                    <div className="min-w-0"><p className="truncate text-sm font-semibold">{selectedFile?.name}</p><p className="text-xs text-muted-foreground">{t('selectedSheetMeta', { sheet: analysis.source.selection.sheet_name })}</p></div>
                  </div>
                  <Button variant="outline" size="sm" onClick={() => setStep('file')}>{t('changeFile')}</Button>
                </div>
                {isEnriching ? (
                  <div className="flex min-h-80 flex-col items-center justify-center rounded-xl border bg-muted/20 px-6 text-center">
                    <span className="flex size-12 items-center justify-center rounded-full bg-primary/10 text-primary"><Loader className="size-6 animate-spin" /></span>
                    <h3 className="mt-4 font-semibold">{t('matchingTitle')}</h3>
                    <p className="mt-1 max-w-md text-sm text-muted-foreground">{t('matchingDesc')}</p>
                  </div>
                ) : <>
                {analysis.low_match && <Alert variant="destructive"><AlertTriangle className="size-4" /><AlertTitle>{t('lowMatchTitle')}</AlertTitle><AlertDescription>{t('lowMatchDesc')}</AlertDescription></Alert>}
                {showFirstMatchHint && <Alert><AlertCircle className="size-4" /><AlertTitle>{t('firstMatchHintTitle')}</AlertTitle><AlertDescription>{t('firstMatchHintDesc')}</AlertDescription></Alert>}
                <div className="flex items-center justify-between">
                  <div className="flex rounded-lg bg-muted p-1 text-xs">
                    <span className="rounded-md bg-background px-3 py-1.5 shadow-sm">{t('allFields')} {mappings.length}</span>
                    <span className="px-3 py-1.5">{t('matchStatus.exact')} {statusCounts.exact}</span>
                    <span className="px-3 py-1.5">{t('matchStatus.possible')} {statusCounts.possible}</span>
                    <span className="px-3 py-1.5">{t('matchStatus.unmatched')} {statusCounts.unmatched}</span>
                  </div>
                  <div className="flex gap-3 text-xs"><span className="text-emerald-600">● {t('matchStatus.exact')}</span><span className="text-amber-600">● {t('needsConfirm')}</span><span className="text-destructive">● {t('mustHandle')}</span></div>
                </div>
                <div className="overflow-x-auto rounded-xl border">
                  <table className="w-full min-w-[820px] text-sm">
                    <thead className="bg-muted/30 text-left"><tr><th className="w-14 p-3">{t('importColumn')}</th><th className="p-3">{t('sourceField')}</th><th className="p-3">{t('sampleData')}</th><th className="p-3">{t('status')}</th><th className="w-52 p-3">{t('targetField')}</th><th className="w-20 p-3 text-center">{t('action')}</th></tr></thead>
                    <tbody>{mappings.map((mapping, index) => (
                      <tr key={`${mapping.source_column ?? 'target'}-${index}`} className="border-t">
                        <td className="p-3"><Checkbox checked={mapping.action !== 'skip'} disabled={!mapping.source_column && mapping.target_required} onCheckedChange={checked => checked ? undefined : skipSource(index)} /></td>
                        <td className="p-3 font-medium">{mapping.source_column ?? mapping.target_display_name}</td>
                        <td className="p-3"><div className="flex max-w-56 flex-wrap gap-1">{mapping.sample_values?.slice(0, 3).map(value => <span key={value} className="rounded-full border px-2 py-0.5 text-xs text-muted-foreground">{value}</span>)}</div></td>
                        <td className="p-3"><span className={`rounded-full px-2 py-1 text-xs ${mapping.confirmed ? 'bg-emerald-50 text-emerald-600' : mapping.status === 'possible' ? 'bg-amber-50 text-amber-600' : 'bg-red-50 text-destructive'}`}>{mapping.confirmed && mapping.status !== 'exact' ? t('confirmed') : t(`matchStatus.${mapping.status}`)}</span></td>
                        <td className="p-3">
                          {mapping.source_column ? <Select value={mapping.action === 'map' ? mapping.target_column_id : 'skip'} onValueChange={value => value === 'skip' ? skipSource(index) : selectTarget(index, value)}><SelectTrigger className={mapping.status === 'unmatched' && mapping.action !== 'skip' ? 'border-amber-500' : ''}><SelectValue placeholder={t('selectTarget')} /></SelectTrigger><SelectContent>{targetColumns.map(column => <SelectItem key={column.id} value={column.id} disabled={usedTargetIds.has(column.id) && column.id !== mapping.target_column_id}>{targetColumnLabel(column)}</SelectItem>)}<SelectItem value="skip">{t('skip')}</SelectItem></SelectContent></Select> : <><Select value={mapping.action} onValueChange={value => updateMapping(index, { action: value as 'fixed' | 'skip', confirmed: value === 'skip' && !mapping.target_required })}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="fixed">{t('fixedValue')}</SelectItem><SelectItem value="skip" disabled={mapping.target_required}>{t('skip')}</SelectItem></SelectContent></Select>{mapping.action === 'fixed' && <Input className="mt-2" value={mapping.fixed_value ?? ''} placeholder={t('enterFixedValue')} onChange={event => updateMapping(index, { fixed_value: event.target.value, confirmed: Boolean(event.target.value.trim()) })} />}</>}
                        </td>
                        <td className="p-3 text-center">{mapping.confirmed ? <CheckCircle2 className="mx-auto size-4 text-emerald-500" /> : mapping.action === 'map' && mapping.target_column_id ? <Button size="sm" variant="outline" onClick={() => updateMapping(index, { confirmed: true })}>{t('confirmMatch')}</Button> : <AlertCircle className="mx-auto size-4 text-destructive" />}</td>
                      </tr>
                    ))}</tbody>
                  </table>
                </div>
                </>}
              </DialogBody>
              <DialogFooter className="items-center justify-between border-t py-4 max-sm:flex-col max-sm:items-stretch">
                <p className={isEnriching ? 'text-sm text-muted-foreground' : mappingReady ? 'text-sm text-emerald-600' : 'text-sm text-destructive'}>{isEnriching ? t('matchingProgress') : mappingReady ? t('allHandled') : t('pendingMappingCount', { count: pendingMappingCount })}</p>
                <div className="flex gap-2"><Button variant="outline" onClick={() => setStep('file')} disabled={isEnriching}>{t('previous')}</Button><Button title={!isEnriching && !mappingReady ? t('pendingMappingCount', { count: pendingMappingCount }) : undefined} onClick={handlePreview} disabled={isEnriching || !mappingReady || preview.isPending}>{preview.isPending && <Loader className="mr-2 size-4 animate-spin" />}{t('previewAction')}</Button></div>
              </DialogFooter>
            </>
          )}

        </DialogContent>
      </Dialog>}

      {step === 'preview' && previewData && (
            <div className="absolute inset-0 z-40 flex min-h-0 flex-col bg-background p-3">
              <header className="flex items-start justify-between rounded-xl border p-4 shadow-sm max-md:flex-col max-md:gap-4">
                <div>
                  <Button className="mb-3 h-auto p-0 text-xs" variant="ghost" onClick={() => setStep('mapping')}><ArrowLeft className="mr-1 size-4" />{t('backToMapping')}</Button>
                  <h2 className="text-xl font-semibold">{t('previewTitle')}</h2>
                  <p className="mt-1 text-sm text-muted-foreground">{t('previewSubtitle')}</p>
                </div>
                {stepper}
              </header>
              <div className="mt-3 flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border shadow-sm">
                <div className="flex items-center justify-between gap-3 border-b p-3 max-sm:flex-col max-sm:items-stretch">
                  <div className="flex items-center gap-3">
                    <span className="rounded-lg border px-3 py-1.5 text-sm">{t('allFields')} {previewData.total_rows}</span>
                    <label className="relative block w-72 max-w-full">
                      <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                      <Input className="pl-9" value={previewQuery} placeholder={t('searchPreview')} onChange={event => setPreviewQuery(event.target.value)} />
                    </label>
                  </div>
                  <Button variant="outline" size="sm" onClick={() => setSkippedRows(previewData.rows.filter(row => row.status === 'failed').map(row => row.row_index))}>{t('onlyValidRows')}</Button>
                </div>
                <div className="min-h-0 flex-1 overflow-auto">
                  <table className="w-full min-w-max text-sm">
                    <thead className="sticky top-0 z-10 bg-background shadow-[0_1px_0_hsl(var(--border))]">
                      <tr>
                        <th className="w-14 p-3 text-left">{t('row')}</th>
                        {Object.keys(previewData.rows[0]?.cells ?? {}).map(name => {
                          const label = previewColumnLabel(name);
                          return <th key={name} className="min-w-44 p-3 text-left font-medium"><span>{label.businessLabel}</span>{label.businessLabel !== label.sqlName && <span className="ml-1 text-xs font-normal text-muted-foreground">{label.sqlName}</span>}</th>;
                        })}
                        <th className="w-24 p-3 text-left">{t('processingResult')}</th>
                        <th className="w-16 p-3">{t('importColumn')}</th>
                      </tr>
                    </thead>
                    <tbody>{previewRows.map(row => (
                      <tr key={row.row_index} className={`border-t ${row.status === 'skipped' ? 'bg-muted/40 text-muted-foreground' : ''}`}>
                        <td className="p-3 tabular-nums">{row.row_index}</td>
                        {Object.entries(row.cells).map(([name, cell]) => <td key={name} className={`min-w-44 p-2 align-top ${cell.error_message ? 'bg-red-50/70' : ''}`}><Input aria-invalid={Boolean(cell.error_message)} defaultValue={String(cell.transformed_value ?? cell.original_value ?? '')} onChange={event => setRowChanges(current => ({ ...current, [row.row_index]: { ...current[row.row_index], [name]: event.target.value } }))} />{cell.error_message && <p className="mt-1 text-xs text-destructive"><AlertCircle className="mr-1 inline size-3" />{cell.error_message}</p>}</td>)}
                        <td className="p-3"><span className={`rounded-full px-2 py-1 text-xs ${row.status === 'valid' ? 'bg-emerald-50 text-emerald-600' : row.status === 'failed' ? 'bg-red-50 text-destructive' : 'bg-muted text-muted-foreground'}`}>{t(`rowStatus.${row.status}`)}</span></td>
                        <td className="p-3 text-center"><Checkbox checked={!skippedRows.includes(row.row_index)} onCheckedChange={checked => setSkippedRows(current => checked ? current.filter(value => value !== row.row_index) : [...current, row.row_index])} /></td>
                      </tr>
                    ))}</tbody>
                  </table>
                </div>
                <footer className="flex items-center justify-between gap-3 border-t p-3 max-md:flex-col max-md:items-stretch"><p className={previewData.failed_rows ? 'text-sm text-destructive' : 'text-sm text-emerald-600'}>{previewData.failed_rows ? t('previewSummary', { failed: previewData.failed_rows }) : t('previewReady')}</p><div className="flex justify-end gap-2"><Button variant="outline" onClick={() => setStep('mapping')}>{t('previous')}</Button><Button variant="outline" onClick={refreshPreview} disabled={preview.isPending}>{t('revalidate')}</Button><Button onClick={() => void prepareConfirmation()} disabled={preview.isPending}>{t('confirmImport')}</Button></div></footer>
              </div>
            </div>
          )}

      {step === 'result' && result && (
            <div className="absolute inset-0 z-40 flex min-h-0 flex-col overflow-auto bg-background p-3">
              <header className="flex items-center justify-between rounded-xl border p-4 shadow-sm max-md:flex-col max-md:items-start max-md:gap-4"><div><h2 className="text-xl font-semibold">{t('resultTitle')}</h2><p className="mt-1 text-sm text-muted-foreground">{selectedFile?.name} {t('processed')}</p></div>{stepper}</header>
              <section className="mt-3 rounded-xl border p-5 shadow-sm"><div className="flex items-start gap-3"><CheckCircle2 className="size-7 text-emerald-500" /><div><h3 className="text-lg font-semibold">{t('importCompleted')}</h3><p className="text-sm text-muted-foreground">{t('existingDataUntouched')}</p></div></div><div className="mt-5 grid grid-cols-4 gap-3 max-sm:grid-cols-2">{[[t('totalRows'), result.total_rows, 'text-foreground'], [t('importedRows'), result.imported_rows, 'text-emerald-600'], [t('failedRows'), result.failed_rows, 'text-destructive'], [t('skippedRows'), result.skipped_rows, 'text-amber-600']].map(([label, value, color]) => <div key={label} className="rounded-xl border p-4"><p className="text-xs text-muted-foreground">{label}</p><p className={`mt-2 text-2xl font-semibold tabular-nums ${color}`}>{value}</p></div>)}</div></section>
              {result.failed_items.length > 0 && <section className="mt-3 overflow-hidden rounded-xl border shadow-sm"><div className="flex items-center justify-between border-b p-4"><div><h3 className="font-semibold">{t('failureDetails')}</h3><p className="text-sm text-muted-foreground">{t('failedDataDesc')}</p></div></div><table className="w-full text-sm"><thead className="bg-muted/30 text-left"><tr><th className="p-3">Excel {t('row')}</th><th className="p-3">{t('sourceField')}</th><th className="p-3">{t('failureReason')}</th></tr></thead><tbody>{result.failed_items.slice(0, 20).map((item, index) => <tr key={`${item.row_index}-${index}`} className="border-t"><td className="p-3">{item.row_index}</td><td className="p-3">{item.column_name ?? '-'}</td><td className="p-3 text-destructive">{item.error_message}</td></tr>)}</tbody></table></section>}
              <div className="mt-3 flex justify-end"><Button onClick={() => handleOpenChange(false)}>{t('backToTable')}</Button></div>
            </div>
          )}

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent className="w-[360px] max-w-[calc(100vw-32px)]" showCloseButton>
          <DialogBody className="flex flex-col items-center px-8 py-12 text-center"><span className="mb-5 flex size-11 items-center justify-center rounded-full bg-primary/10"><CheckCircle2 className="size-5 text-primary" /></span><h3 className="text-lg font-semibold">{t('confirmDialogTitle')}</h3><p className="mt-4 text-sm text-muted-foreground">{t('confirmDialogDesc', { imported: previewData?.valid_rows ?? 0, skipped: skippedRows.length })}</p></DialogBody>
          <DialogFooter className="grid grid-cols-2 border-t p-4"><Button variant="outline" onClick={() => setConfirmOpen(false)}>{t('cancel')}</Button><Button onClick={() => { setConfirmOpen(false); void handleConfirm(); }} disabled={confirm.isPending}>{t('startImport')}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <FileSelectorDialog open={fileSelectorOpen} onOpenChange={setFileSelectorOpen} onConfirm={handleFileConfirm} initSelectedFiles={selectedFile ? [selectedFile] : []} maxCount={1} acceptExt={['xlsx', 'xls']} />
    </>
  );
};

export default BatchImportDialog;
