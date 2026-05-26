'use client';

// Step 1: Prompt + file selection + preview + AI analysis result
// Self-contained component; emits analyzed columns to parent

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Separator } from '@/components/ui/separator';
import { Trash2, Loader, FileText, Check, Lightbulb, X, AlertTriangle } from 'lucide-react';
import { Switch } from '@/components/ui/switch';
import FileSelectorDialog from '@/components/files/file-selector-dialog';
import type { FileItem } from '@/services/types/file';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { useFilePreview } from '@/hooks/file/use-file-preview';
import { useAnalyzeFileForTable } from '@/hooks/db/use-analyze-file-for-table';
import type { DbTableColumn } from '@/services/types/db';
import { useTranslations, useLocale } from 'next-intl';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
} from '@/components/ui/dropdown-menu';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';

export interface StepOneProps {
  dataSourceId: string;
  onAnalyzeDone: (cols: DbTableColumn[]) => void;
  initialAiColumns?: DbTableColumn[];
}

// Prompt template type
interface PromptTemplateItem {
  id: string;
  key: 'general' | 'userManagement' | 'orderSystem' | 'inventory';
  title: string;
  description: string;
  text: string;
}

type SupportedLocale = 'en-US' | 'zh-Hans';

// Template texts for different locales
const TEMPLATE_TEXTS: Readonly<
  Record<SupportedLocale, Readonly<Record<PromptTemplateItem['key'], string>>>
> = {
  'en-US': {
    general:
      'Based on the data and business context, infer and design a suitable table structure. Identify key fields, determine appropriate data types, and mark required fields.',
    userManagement:
      'Create a user management table structure including: user ID, username, email, phone number, password hash, avatar URL, registration time, last login time, account status (active/disabled), role, and other common user fields. Ensure proper data types and required field settings.',
    orderSystem:
      'Create an e-commerce order table structure including: order ID, user ID, order number, order status (pending/paid/shipped/completed/cancelled), total amount, payment method, shipping address, recipient name, recipient phone, order creation time, payment time, shipping time, and remarks. Use appropriate data types.',
    inventory:
      'Create an inventory management table structure including: product ID, product name, SKU code, category, current stock quantity, minimum stock threshold, unit price, supplier, warehouse location, last restock time, and product status. Ensure numeric fields use appropriate types.',
  },
  'zh-Hans': {
    general:
      '请基于数据内容和业务场景，合理推断并创建合适的表结构。识别关键字段，确定合适的数据类型，并标记必填字段。',
    userManagement:
      '创建用户管理表结构，包含：用户ID、用户名、邮箱、手机号、密码哈希、头像URL、注册时间、最后登录时间、账户状态（活跃/禁用）、角色等常见用户字段。确保数据类型和必填字段设置正确。',
    orderSystem:
      '创建电商订单表结构，包含：订单ID、用户ID、订单编号、订单状态（待付款/已付款/已发货/已完成/已取消）、订单总额、支付方式、收货地址、收件人姓名、收件人电话、下单时间、支付时间、发货时间、备注。使用合适的数据类型。',
    inventory:
      '创建库存管理表结构，包含：商品ID、商品名称、SKU编码、分类、当前库存数量、最低库存阈值、单价、供应商、仓库位置、最近补货时间、商品状态。确保数值字段使用合适的类型。',
  },
};

export default function StepOne({ dataSourceId, onAnalyzeDone, initialAiColumns }: StepOneProps) {
  const t = useT();
  const locale = useLocale();
  const currentLocale: SupportedLocale = locale === 'zh-Hans' ? 'zh-Hans' : 'en-US';
  const MAX_COUNT = 1;
  const user = useCurrentUser();

  // Fetch default LLM model
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');

  // Initialize model selection from saved preference or default
  const [selectedModel, setSelectedModel] = useState<ModelSelectorValue | null>(() => {
    if (!user?.id) return null;
    const saved = getLastSelectedAiModel(user.id, 'create');
    return saved ? { provider: saved.provider, model: saved.model } : null;
  });

  // Update model when default loads (only if no saved preference)
  useEffect(() => {
    if (defaultModel && !selectedModel && user?.id) {
      const saved = getLastSelectedAiModel(user.id, 'create');
      if (!saved) {
        setSelectedModel({ provider: defaultModel.provider, model: defaultModel.model });
      }
    }
  }, [defaultModel, selectedModel, user?.id]);

  // Local inputs - initialize with default prompt
  const [prompt, setPrompt] = useState<string>('');
  const [referenceEnabled, setReferenceEnabled] = useState<boolean>(true);
  const [fileDialogOpen, setFileDialogOpen] = useState<boolean>(false);
  const [selectedFile, setSelectedFile] = useState<FileItem | null>(null);

  // Load default prompt on mount
  useEffect(() => {
    // Only set default prompt if prompt is empty (initial load)
    if (!prompt) {
      setPrompt(TEMPLATE_TEXTS[currentLocale].general);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Prompt template selector state
  const [templateMenuOpen, setTemplateMenuOpen] = useState<boolean>(false);
  const [previewTemplate, setPreviewTemplate] = useState<PromptTemplateItem | null>(null);

  // Build template list based on locale
  const templates: readonly PromptTemplateItem[] = useMemo(() => {
    const keys: ReadonlyArray<{ key: PromptTemplateItem['key']; id: string }> = [
      { key: 'general', id: 'general' },
      { key: 'userManagement', id: 'user-management' },
      { key: 'orderSystem', id: 'order-system' },
      { key: 'inventory', id: 'inventory' },
    ] as const;
    return keys.map(({ key, id }) => ({
      id,
      key,
      title: t(`dbs.createPage.promptTemplates.items.${key}.title`),
      description: t(`dbs.createPage.promptTemplates.items.${key}.description`),
      text: TEMPLATE_TEXTS[currentLocale][key],
    }));
  }, [t, currentLocale]);

  const handleTemplateClick = useCallback((template: PromptTemplateItem) => {
    setPreviewTemplate(template);
    setTemplateMenuOpen(false);
  }, []);

  const handleConfirmApply = useCallback(() => {
    if (previewTemplate) {
      setPrompt(previewTemplate.text);
      setPreviewTemplate(null);
    }
  }, [previewTemplate]);

  // AI result
  const [aiColumns, setAiColumns] = useState<DbTableColumn[]>(initialAiColumns ?? []);
  const [analyzedPreviewContent, setAnalyzedPreviewContent] = useState('');
  useEffect(() => {
    if (Array.isArray(initialAiColumns)) {
      setAiColumns(initialAiColumns);
    }
  }, [initialAiColumns]);

  const { analyze, isPending: isAnalyzing } = useAnalyzeFileForTable();
  useEffect(() => {
    if (!referenceEnabled || !selectedFile?.id) {
      setAnalyzedPreviewContent('');
    }
  }, [referenceEnabled, selectedFile?.id]);

  const canAnalyze = useMemo(() => {
    // Require prompt, model, and file (if reference enabled)
    const hasPrompt = prompt.trim().length > 0;
    const hasModel = !!selectedModel?.model;
    if (!referenceEnabled) return hasPrompt && hasModel;
    return hasPrompt && hasModel;
  }, [prompt, referenceEnabled, selectedModel]);

  const handleConfirmFiles = useCallback((files: FileItem[]) => {
    const first = files[0];
    if (first) {
      setSelectedFile(first);
      setAnalyzedPreviewContent('');
    }
    setFileDialogOpen(false);
  }, []);

  const handleAnalyze = useCallback(async () => {
    if (!canAnalyze || !selectedModel) return;
    const payload = {
      prompt,
      model: { provider: selectedModel.provider, name: selectedModel.model },
      data_source_id: dataSourceId,
      ...(referenceEnabled && selectedFile?.id ? { file_id: selectedFile.id } : {}),
    };
    const result = await analyze(payload);
    const cols = result.columns;
    setAiColumns(cols);
    setAnalyzedPreviewContent(result.content ?? '');
    onAnalyzeDone(cols);
  }, [
    analyze,
    canAnalyze,
    selectedFile,
    prompt,
    referenceEnabled,
    onAnalyzeDone,
    selectedModel,
    dataSourceId,
  ]);

  // File preview (markdown) for selected file
  const { content: previewContent, isLoading: isLoadingPreview } = useFilePreview(
    selectedFile?.id,
    {
      enabled: referenceEnabled && !!selectedFile?.id,
      staleTime: 60_000,
      gcTime: 5 * 60_000,
      refetchOnWindowFocus: false,
    }
  );
  const displayedPreviewContent = analyzedPreviewContent || previewContent;

  // Terms to highlight inside preview based on AI columns
  const highlightTerms = useMemo(() => {
    const set = new Set<string>();
    aiColumns.forEach(col => {
      const name = (col.name || '').trim();
      if (name.length > 0) set.add(name);
      const desc = (col.description || '').trim();
      if (desc.length > 0) {
        // Split description into tokens by common separators (handles Chinese/English)
        const tokens = desc
          .split(/[\s,，。；;:：]/)
          .map(t => t.trim())
          .filter(t => t.length >= 2);
        tokens.forEach(t => set.add(t));
      }
    });
    // Limit the number of terms to avoid performance issues
    return Array.from(set).slice(0, 40);
  }, [aiColumns]);

  return (
    <div className="flex h-0 grow border rounded-md overflow-hidden">
      {/* Left – prompt + file */}
      <div className="w-[420px] p-6 border-r flex flex-col gap-4">
        {/* Model Selector */}
        <div className="space-y-2">
          <label className="text-sm font-medium">
            {t('dbs.modelSelector.label')}
            <span className="text-destructive ml-1">*</span>
          </label>
          <ModelSelector
            modelType="text-chat"
            value={selectedModel ?? undefined}
            onChange={value => {
              setSelectedModel(value);
              if (user?.id) {
                saveLastSelectedAiModel(user.id, 'create', {
                  provider: value.provider,
                  model: value.model,
                });
              }
            }}
            placeholder={t('dbs.modelSelector.placeholder')}
          />
        </div>
        {/* Requirement description */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label className="text-sm font-medium">
              {t('dbs.createPage.requirementLabel')} <span className="text-red-500">*</span>
            </label>
            {/* Prompt Template Selector */}
            <DropdownMenu open={templateMenuOpen} onOpenChange={setTemplateMenuOpen}>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs gap-1"
                  aria-label={t('dbs.createPage.promptTemplates.selectTemplate')}
                >
                  <Lightbulb className="h-3.5 w-3.5" />
                  {t('dbs.createPage.promptTemplates.selectTemplate')}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="p-0 w-[320px] rounded-xl" sideOffset={6}>
                {/* Header */}
                <div className="flex items-center justify-between px-3 py-2 border-b">
                  <div className="text-sm font-medium">
                    {t('dbs.createPage.promptTemplates.title')}
                  </div>
                  <button
                    type="button"
                    className="inline-flex items-center justify-center rounded-md hover:bg-muted p-1"
                    aria-label={t('common.cancel')}
                    onClick={() => setTemplateMenuOpen(false)}
                  >
                    <X className="h-4 w-4" />
                  </button>
                </div>

                {/* List */}
                <div className="p-3 space-y-2">
                  {templates.map(item => (
                    <button
                      key={item.id}
                      type="button"
                      aria-label={item.title}
                      onClick={() => handleTemplateClick(item)}
                      className={cn(
                        'w-full text-left rounded-lg border hover:bg-accent hover:text-accent-foreground transition-colors',
                        'px-3 py-2 flex items-start gap-2'
                      )}
                    >
                      <span className="mt-0.5 inline-flex items-center justify-center rounded-md bg-muted text-muted-foreground p-1">
                        <FileText className="h-4 w-4" />
                      </span>
                      <span className="min-w-0 grow">
                        <div className="text-sm font-medium leading-none mb-1">{item.title}</div>
                        <div className="text-xs text-muted-foreground leading-relaxed">
                          {item.description}
                        </div>
                      </span>
                    </button>
                  ))}
                </div>

                {/* Footer tip */}
                <div className="px-3 py-2 border-t text-xs text-muted-foreground">
                  <span role="img" aria-label="tip" className="mr-1">
                    💡
                  </span>
                  {t('dbs.createPage.promptTemplates.footerTip')}
                </div>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
          <Textarea
            placeholder={t('dbs.createPage.requirementPlaceholder')}
            value={prompt}
            onChange={e => setPrompt(e.target.value)}
            className="min-h-[140px]"
          />
        </div>

        {/* Reference files header with switch */}
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium">{t('dbs.createPage.referenceFiles')}</span>
          <Switch checked={referenceEnabled} onCheckedChange={setReferenceEnabled} />
        </div>

        {/* Reference files area */}
        {referenceEnabled && (
          <div className="flex flex-col gap-3">
            {/* Selected file card inside dashed border */}
            <div className="rounded-md border border-dashed p-6">
              {selectedFile ? (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <FileText className="h-6 w-6 text-highlight" />
                    <div className="flex flex-col">
                      <span className="text-sm">{selectedFile.name}</span>
                      <span className="mt-1 text-xs text-success inline-flex items-center gap-1">
                        <Check className="h-3 w-3" /> {t('dbs.createPage.fileSelected')}
                      </span>
                    </div>
                  </div>
                  <Button
                    isIcon
                    variant="ghost"
                    onClick={() => {
                      setSelectedFile(null);
                      setAnalyzedPreviewContent('');
                    }}
                    className="h-8 w-8"
                    aria-label={t('dbs.createPage.removeFileAria')}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ) : (
                <div className="text-sm text-muted-foreground">
                  {t('dbs.createPage.noFileSelected')}
                </div>
              )}
            </div>

            {/* Choose from file manager entry */}
            <div
              className="rounded-md border border-dashed border-highlight bg-highlight/5 text-highlight p-4 cursor-pointer hover:bg-highlight/10"
              onClick={() => setFileDialogOpen(true)}
            >
              <div className="flex items-center gap-2">
                <FileText className="h-4 w-4" />
                <span className="text-sm font-medium">
                  {t('dbs.createPage.chooseFromFileManager')}
                </span>
              </div>
              <div className="mt-1 text-xs text-highlight/80">
                {t('dbs.createPage.chooseFromFileManagerDesc')}
              </div>
            </div>
          </div>
        )}

        <Separator />

        {/* Action button */}
        <Button onClick={handleAnalyze} disabled={!canAnalyze || isAnalyzing} className="w-full">
          {isAnalyzing ? (
            <span className="inline-flex items-center gap-2">
              <Loader className="h-4 w-4 animate-spin" /> {t('dbs.createPage.startAnalyzeLoading')}
            </span>
          ) : (
            t('dbs.createPage.startAnalyze')
          )}
        </Button>
      </div>

      {/* Right – preview + analysis result */}
      <div className="flex-1 p-4 h-full space-y-4 overflow-y-auto">
        {/* File Markdown preview */}
        {referenceEnabled && selectedFile?.id && (
          <div className="rounded-md border p-3 max-h-[280px] overflow-auto">
            {isLoadingPreview ? (
              <div className="space-y-2">
                {Array.from({ length: 8 }).map((_, i) => (
                  <Skeleton key={i} className="h-4 w-full" />
                ))}
              </div>
            ) : displayedPreviewContent ? (
              <MarkdownViewer content={displayedPreviewContent} highlights={highlightTerms} />
            ) : (
              <div className="text-sm text-muted-foreground">{t('dbs.createPage.noPreview')}</div>
            )}
          </div>
        )}

        {isAnalyzing ? (
          <div className="space-y-2">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        ) : aiColumns.length === 0 ? (
          <div className="h-full flex items-center justify-center text-muted-foreground">
            {t('dbs.createPage.emptyHint')}
          </div>
        ) : (
          <div className="border rounded-md overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('dbs.createPage.colFieldName')}</TableHead>
                  <TableHead>{t('dbs.createPage.colDescription')}</TableHead>
                  <TableHead>{t('dbs.createPage.colType')}</TableHead>
                  <TableHead>{t('dbs.createPage.colRequired')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {aiColumns.map((col, idx) => (
                  <TableRow key={`${col.name}-${idx}`}>
                    <TableCell className="font-mono text-sm">{col.name}</TableCell>
                    <TableCell>{col.description}</TableCell>
                    <TableCell>{String(col.type)}</TableCell>
                    <TableCell>
                      {col.is_required
                        ? t('dbs.createPage.requiredYes')
                        : t('dbs.createPage.requiredNo')}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      {/* File selection dialog */}
      <FileSelectorDialog
        open={fileDialogOpen}
        onOpenChange={setFileDialogOpen}
        onConfirm={handleConfirmFiles}
        initSelectedFiles={selectedFile ? [selectedFile] : []}
        maxCount={MAX_COUNT}
      />

      {/* Template Preview Dialog */}
      <Dialog open={!!previewTemplate} onOpenChange={isOpen => !isOpen && setPreviewTemplate(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <FileText className="h-5 w-5" />
              {previewTemplate?.title}
            </DialogTitle>
          </DialogHeader>

          <DialogBody className="space-y-4">
            {/* Description */}
            <p className="text-sm text-muted-foreground">
              {t('dbs.createPage.promptTemplates.preview.description')}
            </p>

            {/* Preview Label */}
            <div className="text-sm font-medium">
              {t('dbs.createPage.promptTemplates.preview.previewLabel')}
            </div>

            {/* Preview Content */}
            <div className="rounded-lg border bg-muted/50 p-4">
              <pre className="text-sm whitespace-pre-wrap font-mono leading-relaxed">
                {previewTemplate?.text}
              </pre>
            </div>

            {/* Warning */}
            <div className="flex items-center gap-2 text-sm text-amber-600 dark:text-amber-500">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              <span>{t('dbs.createPage.promptTemplates.preview.warning')}</span>
            </div>
          </DialogBody>

          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setPreviewTemplate(null)}>
              {t('dbs.createPage.promptTemplates.preview.cancel')}
            </Button>
            <Button onClick={handleConfirmApply}>
              {t('dbs.createPage.promptTemplates.preview.apply')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
