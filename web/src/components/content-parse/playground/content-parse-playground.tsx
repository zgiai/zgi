'use client';

import * as React from 'react';
import {
  Activity,
  BoxSelect,
  ChevronDown,
  Clock3,
  Copy,
  Cpu,
  FileSearch,
  FileText,
  Hash,
  History,
  Info,
  Loader2,
  Play,
  RefreshCw,
  Save,
  Share2,
  SlidersHorizontal,
  Sparkles,
  UploadCloud,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Select, SelectContent, SelectItem, SelectTrigger } from '@/components/ui/select';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { contentParseService } from '@/services/content-parse.service';
import type {
  ContentParsePlaygroundOCREngineStatus,
  ContentParsePlaygroundParseResponse,
  ContentParsePlaygroundProviderSummary,
  ContentParsePlaygroundProviderStatus,
  ContentParsePlaygroundSavedRun,
  ParseOCREngine,
  ParsedElement,
  ParseProfile,
  ParseProviderKey,
} from '@/services/types/content-parse';
import { DocumentPagePreview, type DocumentPreviewPage } from './document-page-preview';

type PDFJSModule = typeof import('pdfjs-dist/legacy/build/pdf.mjs');

let pdfjsModulePromise: Promise<PDFJSModule> | null = null;

type ProviderOptionView = {
  value: ParseProviderKey;
  label: string;
  hint: string;
  explanation: string;
};

type ProfileOptionView = {
  value: ParseProfile;
  label: string;
  description: string;
};

type OCREngineOptionView = {
  value: ParseOCREngine;
  label: string;
  description: string;
};

export function ContentParsePlayground() {
  const t = useT('contentParse');
  const [file, setFile] = React.useState<File | null>(null);
  const [provider, setProvider] = React.useState<ParseProviderKey>('auto');
  const [profile, setProfile] = React.useState<ParseProfile>('auto');
  const [ocrEngine, setOcrEngine] = React.useState<ParseOCREngine>('auto');
  const [result, setResult] = React.useState<ContentParsePlaygroundParseResponse | null>(null);
  const [savedRun, setSavedRun] = React.useState<ContentParsePlaygroundSavedRun | null>(null);
  const [historyRuns, setHistoryRuns] = React.useState<ContentParsePlaygroundSavedRun[]>([]);
  const [compareRuns, setCompareRuns] = React.useState<ContentParsePlaygroundSavedRun[]>([]);
  const [providerSummaries, setProviderSummaries] = React.useState<
    ContentParsePlaygroundProviderSummary[]
  >([]);
  const [providerStatuses, setProviderStatuses] = React.useState<
    Partial<Record<ParseProviderKey, ContentParsePlaygroundProviderStatus>>
  >({});
  const [ocrStatuses, setOcrStatuses] = React.useState<
    Partial<Record<ParseOCREngine, ContentParsePlaygroundOCREngineStatus>>
  >({});
  const [providersLoading, setProvidersLoading] = React.useState(false);
  const [providerSource, setProviderSource] = React.useState<string | undefined>();
  const [previewPages, setPreviewPages] = React.useState<DocumentPreviewPage[]>([]);
  const [previewError, setPreviewError] = React.useState<string | null>(null);
  const [selectedElementId, setSelectedElementId] = React.useState<string | undefined>();
  const [outputTab, setOutputTab] = React.useState('blocks');
  const [isParsing, setIsParsing] = React.useState(false);
  const [isSaving, setIsSaving] = React.useState(false);
  const [isLoadingHistory, setIsLoadingHistory] = React.useState(false);
  const [isLoadingCompare, setIsLoadingCompare] = React.useState(false);
  const [isLoadingProviderSummary, setIsLoadingProviderSummary] = React.useState(false);
  const [isRenderingPreview, setIsRenderingPreview] = React.useState(false);
  const [isPending, startTransition] = React.useTransition();
  const elementCardRefs = React.useRef(new Map<string, HTMLDivElement>());
  const pageRefs = React.useRef(new Map<number, HTMLDivElement>());
  const loadedShareTokenRef = React.useRef<string | null>(null);
  const savedPreviewRequestRef = React.useRef(0);

  const providerOptions = React.useMemo<ProviderOptionView[]>(
    () => [
      {
        value: 'auto',
        label: t('providers.auto.label'),
        hint: t('providers.auto.hint'),
        explanation: t('providers.auto.explanation'),
      },
      {
        value: 'local',
        label: t('providers.local.label'),
        hint: t('providers.local.hint'),
        explanation: t('providers.local.explanation'),
      },
      {
        value: 'vlm',
        label: t('providers.vlm.label'),
        hint: t('providers.vlm.hint'),
        explanation: t('providers.vlm.explanation'),
      },
      {
        value: 'reducto',
        label: t('providers.reducto.label'),
        hint: t('providers.reducto.hint'),
        explanation: t('providers.reducto.explanation'),
      },
      {
        value: 'mineru',
        label: t('providers.mineru.label'),
        hint: t('providers.mineru.hint'),
        explanation: t('providers.mineru.explanation'),
      },
      {
        value: 'hyperparse_api',
        label: t('providers.hyperparseApi.label'),
        hint: t('providers.hyperparseApi.hint'),
        explanation: t('providers.hyperparseApi.explanation'),
      },
    ],
    [t]
  );
  const profileOptions = React.useMemo<ProfileOptionView[]>(
    () => [
      { value: 'auto', label: t('profiles.auto'), description: t('profiles.descriptions.auto') },
      {
        value: 'high_quality',
        label: t('profiles.highQuality'),
        description: t('profiles.descriptions.highQuality'),
      },
      {
        value: 'layout_first',
        label: t('profiles.layoutFirst'),
        description: t('profiles.descriptions.layoutFirst'),
      },
      { value: 'fast', label: t('profiles.fast'), description: t('profiles.descriptions.fast') },
      {
        value: 'local_first',
        label: t('profiles.localFirst'),
        description: t('profiles.descriptions.localFirst'),
      },
    ],
    [t]
  );
  const ocrEngineOptions = React.useMemo<OCREngineOptionView[]>(
    () => [
      { value: 'auto', label: t('ocr.auto'), description: t('ocr.descriptions.auto') },
      {
        value: 'tesseract',
        label: t('ocr.tesseract'),
        description: t('ocr.descriptions.tesseract'),
      },
      {
        value: 'paddleocr',
        label: t('ocr.paddleocr'),
        description: t('ocr.descriptions.paddleocr'),
      },
    ],
    [t]
  );
  const previewRenderFailedMessage = t('preview.renderFailed');
  const sourceUnavailableMessage = t('preview.sourceUnavailable');
  const selectedProviderStatus = providerStatuses[provider];
  const selectedProfile = profileOptions.find(item => item.value === profile);
  const selectedOCROption = ocrEngineOptions.find(item => item.value === ocrEngine);
  const ocrApplies = provider === 'local' || provider === 'auto';
  const elements = React.useMemo(() => result?.artifact?.elements || [], [result]);
  const deferredElements = React.useDeferredValue(elements);
  const pages = React.useMemo(
    () => buildPages(previewPages, result?.quality_summary.page_count, deferredElements),
    [deferredElements, previewPages, result?.quality_summary.page_count]
  );
  const selectedElement = React.useMemo(
    () => deferredElements.find(element => elementKey(element) === selectedElementId),
    [deferredElements, selectedElementId]
  );
  const activePreviewFileName = file?.name || savedRun?.file_name || result?.file.name;
  const hasPreviewContext = Boolean(file || savedRun || result);

  const loadSavedRunSourcePreview = React.useCallback(
    async (run: ContentParsePlaygroundSavedRun, shareToken?: string) => {
      const requestID = savedPreviewRequestRef.current + 1;
      savedPreviewRequestRef.current = requestID;
      setIsRenderingPreview(true);
      setPreviewPages([]);
      setPreviewError(null);
      try {
        const response = shareToken
          ? await contentParseService.renderSharedRunSourcePreview(shareToken, 20)
          : await contentParseService.renderSavedRunSourcePreview(run.id, 20);
        if (savedPreviewRequestRef.current !== requestID) return;
        const renderedPages = response.data.pages.filter(page => page.startsWith('data:image/'));
        const nextPages = await Promise.all(
          renderedPages.map(async (imageUrl, index) => {
            const dimensions = await readImageDimensions(imageUrl);
            return {
              pageIndex: index,
              imageUrl,
              aspectRatio: dimensions.width / dimensions.height,
            };
          })
        );
        if (savedPreviewRequestRef.current === requestID) {
          setPreviewPages(nextPages);
        }
      } catch {
        if (savedPreviewRequestRef.current === requestID) {
          setPreviewError(sourceUnavailableMessage);
        }
      } finally {
        if (savedPreviewRequestRef.current === requestID) {
          setIsRenderingPreview(false);
        }
      }
    },
    [sourceUnavailableMessage]
  );

  React.useEffect(() => {
    let cancelled = false;
    setProvidersLoading(true);

    async function loadProviders() {
      try {
        const response = await contentParseService.listPlaygroundProviders();
        if (cancelled) return;
        const nextStatuses: Partial<
          Record<ParseProviderKey, ContentParsePlaygroundProviderStatus>
        > = {};
        response.data.providers.forEach(item => {
          nextStatuses[item.key] = item;
        });
        const nextOCRStatuses: Partial<
          Record<ParseOCREngine, ContentParsePlaygroundOCREngineStatus>
        > = {};
        response.data.ocr_engines?.forEach(item => {
          nextOCRStatuses[item.key] = item;
        });
        setProviderStatuses(nextStatuses);
        setOcrStatuses(nextOCRStatuses);
        setProviderSource(response.data.source);
      } catch {
        if (!cancelled) {
          setProviderStatuses({});
          setOcrStatuses({});
          setProviderSource(undefined);
        }
      } finally {
        if (!cancelled) {
          setProvidersLoading(false);
        }
      }
    }

    void loadProviders();
    return () => {
      cancelled = true;
    };
  }, []);

  React.useEffect(() => {
    if (typeof window === 'undefined') return;
    const token = new URLSearchParams(window.location.search).get('share');
    if (!token || loadedShareTokenRef.current === token) return;
    const shareToken = token;
    loadedShareTokenRef.current = shareToken;

    let cancelled = false;
    async function loadSharedRun() {
      try {
        const response = await contentParseService.getPlaygroundShare(shareToken);
        if (cancelled) return;
        const run = response.data;
        const nextResult = savedRunToParseResult(run);
        const firstWithBox = nextResult.artifact?.elements?.find(hasReliableBox);
        setFile(null);
        setSavedRun(run);
        setResult(nextResult);
        setCompareRuns([]);
        setHistoryRuns([]);
        setSelectedElementId(firstWithBox ? elementKey(firstWithBox) : undefined);
        setOutputTab('blocks');
        void loadSavedRunSourcePreview(run, shareToken);
        toast.success(t('toast.shareLoaded'));
      } catch (error) {
        if (!cancelled) {
          toast.error(error instanceof Error ? error.message : t('toast.shareLoadFailed'));
        }
      }
    }
    void loadSharedRun();
    return () => {
      cancelled = true;
    };
  }, [loadSavedRunSourcePreview, t]);

  React.useEffect(() => {
    const status = providerStatuses[provider];
    if (!status || status.selectable) return;
    const localStatus = providerStatuses.local;
    setProvider(localStatus?.selectable === false ? 'auto' : 'local');
  }, [provider, providerStatuses]);

  React.useEffect(() => {
    const status = ocrStatuses[ocrEngine];
    if (!status || status.available) return;
    setOcrEngine('auto');
  }, [ocrEngine, ocrStatuses]);

  React.useEffect(() => {
    if (!file) {
      setPreviewPages([]);
      setPreviewError(null);
      return;
    }

    let cancelled = false;
    setPreviewPages([]);
    setPreviewError(null);

    async function renderPreview() {
      if (!file) return;
      setIsRenderingPreview(true);
      try {
        const nextPages = await renderFilePreview(file);
        if (!cancelled) {
          setPreviewPages(nextPages);
        }
      } catch (error) {
        if (!cancelled) {
          setPreviewError(error instanceof Error ? error.message : previewRenderFailedMessage);
        }
      } finally {
        if (!cancelled) {
          setIsRenderingPreview(false);
        }
      }
    }

    void renderPreview();
    return () => {
      cancelled = true;
    };
  }, [file, previewRenderFailedMessage]);

  React.useEffect(() => {
    return () => {
      previewPages.forEach(page => {
        if (page.imageUrl?.startsWith('blob:')) {
          URL.revokeObjectURL(page.imageUrl);
        }
      });
    };
  }, [previewPages]);

  const handleFile = React.useCallback((nextFile: File | null) => {
    if (!nextFile) return;
    savedPreviewRequestRef.current += 1;
    setFile(nextFile);
    setResult(null);
    setSavedRun(null);
    setCompareRuns([]);
    setSelectedElementId(undefined);
    setOutputTab('blocks');
  }, []);

  const setElementCardRef = React.useCallback(
    (key: string) => (node: HTMLDivElement | null) => {
      if (node) {
        elementCardRefs.current.set(key, node);
        return;
      }
      elementCardRefs.current.delete(key);
    },
    []
  );

  const setPageRef = React.useCallback(
    (pageIndex: number) => (node: HTMLDivElement | null) => {
      if (node) {
        pageRefs.current.set(pageIndex, node);
        return;
      }
      pageRefs.current.delete(pageIndex);
    },
    []
  );

  const scrollToElementCard = React.useCallback((key: string) => {
    window.setTimeout(() => {
      elementCardRefs.current.get(key)?.scrollIntoView({
        block: 'center',
        behavior: 'smooth',
      });
    }, 80);
  }, []);

  const scrollToPreviewPage = React.useCallback((element: ParsedElement) => {
    window.setTimeout(() => {
      pageRefs.current.get(element.page)?.scrollIntoView({
        block: 'center',
        behavior: 'smooth',
      });
    }, 40);
  }, []);

  const handleSelectElement = React.useCallback(
    (element: ParsedElement, source: 'preview' | 'list') => {
      const key = elementKey(element);
      setSelectedElementId(key);
      if (source === 'preview') {
        setOutputTab('blocks');
        scrollToElementCard(key);
        return;
      }
      scrollToPreviewPage(element);
    },
    [scrollToElementCard, scrollToPreviewPage]
  );

  const handleParse = React.useCallback(async () => {
    if (!file) {
      toast.error(t('toast.selectFile'));
      return;
    }

    setIsParsing(true);
    try {
      const effectiveOCREngine = ocrApplies ? ocrEngine : 'auto';
      const response = await contentParseService.parsePlayground({
        file,
        provider,
        profile,
        intent: 'preview',
        fresh: true,
        ocrEngine: effectiveOCREngine,
      });
      startTransition(() => {
        setResult(response.data);
        setSavedRun(null);
        setCompareRuns([]);
        const firstWithBox = response.data.artifact?.elements?.find(hasReliableBox);
        setSelectedElementId(firstWithBox ? elementKey(firstWithBox) : undefined);
        setOutputTab('blocks');
      });
      toast.success(t('toast.parseDone'));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('toast.parseFailed'));
    } finally {
      setIsParsing(false);
    }
  }, [file, ocrApplies, ocrEngine, profile, provider, t]);

  const handleSaveRun = React.useCallback(async () => {
    if (!file || !result) {
      toast.error(t('toast.saveRequiresResult'));
      return;
    }
    setIsSaving(true);
    try {
      const effectiveOCREngine = ocrApplies ? ocrEngine : 'auto';
      const response = await contentParseService.savePlayground({
        file,
        provider,
        profile,
        intent: 'preview',
        fresh: true,
        ocrEngine: effectiveOCREngine,
        parseResult: result,
      });
      setSavedRun(response.data.run);
      setResult(response.data.parse_result);
      toast.success(t('toast.saveDone'));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('toast.saveFailed'));
    } finally {
      setIsSaving(false);
    }
  }, [file, ocrApplies, ocrEngine, profile, provider, result, t]);

  const handleLoadHistory = React.useCallback(async () => {
    setIsLoadingHistory(true);
    try {
      const response = await contentParseService.listPlaygroundRuns({ limit: 30 });
      setHistoryRuns(response.data.items || []);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('toast.historyLoadFailed'));
    } finally {
      setIsLoadingHistory(false);
    }
  }, [t]);

  const handleLoadCompare = React.useCallback(async () => {
    const sourceHash = result?.file.sha256 || savedRun?.source_content_hash;
    if (!sourceHash) {
      toast.error(t('toast.compareRequiresHash'));
      return;
    }
    setIsLoadingCompare(true);
    try {
      const response = await contentParseService.comparePlaygroundHash(sourceHash, 50);
      setCompareRuns(response.data.items || []);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('toast.compareLoadFailed'));
    } finally {
      setIsLoadingCompare(false);
    }
  }, [result?.file.sha256, savedRun?.source_content_hash, t]);

  const handleLoadProviderSummary = React.useCallback(async () => {
    setIsLoadingProviderSummary(true);
    try {
      const response = await contentParseService.getPlaygroundProviderSummary(200);
      setProviderSummaries(response.data.items || []);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('toast.providerSummaryLoadFailed'));
    } finally {
      setIsLoadingProviderSummary(false);
    }
  }, [t]);

  const handleOpenSavedRun = React.useCallback(
    (run: ContentParsePlaygroundSavedRun) => {
      const nextResult = savedRunToParseResult(run);
      const firstWithBox = nextResult.artifact?.elements?.find(hasReliableBox);
      startTransition(() => {
        setFile(null);
        setSavedRun(run);
        setResult(nextResult);
        setSelectedElementId(firstWithBox ? elementKey(firstWithBox) : undefined);
        setOutputTab('blocks');
      });
      void loadSavedRunSourcePreview(run);
    },
    [loadSavedRunSourcePreview, startTransition]
  );

  const handleCopyShareLink = React.useCallback(() => {
    if (!savedRun?.share_token || typeof window === 'undefined') {
      toast.error(t('toast.saveBeforeShare'));
      return;
    }
    const url = `${window.location.origin}${window.location.pathname}?share=${savedRun.share_token}`;
    void navigator.clipboard.writeText(url);
    toast.success(t('toast.shareCopied'));
  }, [savedRun?.share_token, t]);

  const selectedProvider = providerOptions.find(item => item.value === provider);
  const isBusy = isParsing || isPending || isSaving;

  return (
    <div className="min-h-full bg-bg-canvas px-4 py-3 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-3">
        <header className="rounded-2xl border border-border/80 bg-card/80 p-3 shadow-sm">
          <div className="grid gap-3 xl:grid-cols-[minmax(220px,0.65fr)_minmax(260px,0.72fr)_minmax(360px,1fr)_minmax(380px,0.95fr)] xl:items-center">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2 text-xs font-medium text-muted-foreground">
                <span className="uppercase tracking-[0.12em]">{t('page.eyebrow')}</span>
                <span className="text-border">/</span>
                <Badge variant={result ? 'success' : isBusy ? 'info' : 'subtle'}>
                  <span
                    className={cn(
                      'size-1.5 rounded-full',
                      result ? 'bg-success' : isBusy ? 'bg-info' : 'bg-muted-foreground'
                    )}
                    aria-hidden="true"
                  />
                  {result ? t('status.parsed') : isBusy ? t('status.parsing') : t('status.ready')}
                </Badge>
              </div>
              <h1 className="mt-1 truncate text-xl font-semibold tracking-tight text-foreground lg:text-2xl">
                {t('page.title')}
              </h1>
              <p className="mt-1 line-clamp-1 text-xs leading-5 text-muted-foreground">
                {t('page.description')}
              </p>
            </div>

            <FileDropZone file={file} onFile={handleFile} compact />

            <CompactRunSummary
              result={result}
              isBusy={isBusy}
              isRenderingPreview={isRenderingPreview}
            />

            <div className="flex flex-wrap items-center gap-2 xl:justify-end">
              <Select
                value={provider}
                onValueChange={value => setProvider(value as ParseProviderKey)}
              >
                <SelectTrigger className="h-9 min-w-[190px] bg-background">
                  <div className="flex min-w-0 items-center justify-between gap-2">
                    <span className="truncate">{selectedProvider?.label || provider}</span>
                    <ProviderStatusBadge
                      status={selectedProviderStatus}
                      loading={providersLoading}
                      compact
                    />
                  </div>
                </SelectTrigger>
                <SelectContent>
                  {providerOptions.map(item => {
                    const status = providerStatuses[item.value];
                    const selectable = isProviderSelectable(status);
                    return (
                      <SelectItem
                        key={item.value}
                        value={item.value}
                        disabled={!selectable}
                        textValue={item.label}
                      >
                        <div className="flex w-full min-w-56 items-center justify-between gap-3">
                          <span>{item.label}</span>
                          <ProviderStatusBadge status={status} loading={providersLoading} compact />
                        </div>
                      </SelectItem>
                    );
                  })}
                </SelectContent>
              </Select>

              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-9 px-2 text-xs text-muted-foreground"
                  >
                    <SlidersHorizontal className="size-3.5" />
                    {t('advanced.open')}
                  </Button>
                </PopoverTrigger>
                <PopoverContent
                  align="end"
                  className="w-[560px] max-w-[calc(100vw-2rem)] rounded-xl p-3"
                >
                  <div className="mb-3 text-xs leading-5 text-muted-foreground">
                    {t('advanced.providerCompareHint')}
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1.5">
                      <div className="text-xs font-medium text-muted-foreground">
                        {t('advanced.profileLabel')}
                      </div>
                      <Select
                        value={profile}
                        onValueChange={value => setProfile(value as ParseProfile)}
                      >
                        <SelectTrigger className="bg-background">
                          <span className="truncate">{selectedProfile?.label}</span>
                        </SelectTrigger>
                        <SelectContent>
                          {profileOptions.map(item => (
                            <SelectItem key={item.value} value={item.value} textValue={item.label}>
                              <div className="flex min-w-64 flex-col gap-0.5">
                                <span>{item.label}</span>
                                <span className="text-xs text-muted-foreground">
                                  {item.description}
                                </span>
                              </div>
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <p className="text-xs leading-5 text-muted-foreground">
                        {selectedProfile?.description}
                      </p>
                    </div>

                    <div className="space-y-1.5">
                      <div className="text-xs font-medium text-muted-foreground">
                        {t('advanced.ocrLabel')}
                      </div>
                      <Select
                        value={ocrEngine}
                        onValueChange={value => setOcrEngine(value as ParseOCREngine)}
                        disabled={!ocrApplies}
                      >
                        <SelectTrigger className="bg-background">
                          <div className="flex min-w-0 items-center justify-between gap-2">
                            <span className="truncate">{selectedOCROption?.label}</span>
                            <OCRStatusBadge status={ocrStatuses[ocrEngine]} compact />
                          </div>
                        </SelectTrigger>
                        <SelectContent>
                          {ocrEngineOptions.map(item => {
                            const status = ocrStatuses[item.value];
                            const selectable = isOCREngineSelectable(status);
                            return (
                              <SelectItem
                                key={item.value}
                                value={item.value}
                                disabled={!selectable}
                                textValue={item.label}
                              >
                                <div className="flex min-w-64 items-center justify-between gap-3">
                                  <div className="flex flex-col gap-0.5">
                                    <span>{item.label}</span>
                                    <span className="text-xs text-muted-foreground">
                                      {item.description}
                                    </span>
                                  </div>
                                  <OCRStatusBadge status={status} compact />
                                </div>
                              </SelectItem>
                            );
                          })}
                        </SelectContent>
                      </Select>
                      <p className="text-xs leading-5 text-muted-foreground">
                        {ocrApplies ? selectedOCROption?.description : t('ocr.notApplicable')}
                      </p>
                    </div>
                  </div>
                </PopoverContent>
              </Popover>

              <Popover>
                <PopoverTrigger asChild>
                  <Button type="button" variant="outline" size="sm" className="h-9 px-2 text-xs">
                    <Info className="size-3.5" />
                    {t('help.detailsToggle')}
                    <ChevronDown className="size-3.5" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent
                  align="end"
                  className="w-[640px] max-w-[calc(100vw-2rem)] rounded-xl p-3"
                >
                  <ProviderQualityHelp
                    provider={selectedProvider}
                    status={selectedProviderStatus}
                    className="lg:grid-cols-2"
                  />
                </PopoverContent>
              </Popover>

              <Popover onOpenChange={open => open && void handleLoadProviderSummary()}>
                <PopoverTrigger asChild>
                  <Button type="button" variant="ghost" size="sm" className="h-9 px-2 text-xs">
                    <Activity className="size-3.5" />
                    {t('actions.health')}
                  </Button>
                </PopoverTrigger>
                <PopoverContent
                  align="end"
                  className="w-[520px] max-w-[calc(100vw-2rem)] rounded-xl p-3"
                >
                  <ProviderSummaryPanel
                    items={providerSummaries}
                    loading={isLoadingProviderSummary}
                  />
                </PopoverContent>
              </Popover>

              <Popover onOpenChange={open => open && void handleLoadHistory()}>
                <PopoverTrigger asChild>
                  <Button type="button" variant="ghost" size="sm" className="h-9 px-2 text-xs">
                    <History className="size-3.5" />
                    {t('actions.history')}
                  </Button>
                </PopoverTrigger>
                <PopoverContent
                  align="end"
                  className="w-[520px] max-w-[calc(100vw-2rem)] rounded-xl p-3"
                >
                  <HistoryPanel
                    items={historyRuns}
                    loading={isLoadingHistory}
                    onOpenRun={handleOpenSavedRun}
                  />
                </PopoverContent>
              </Popover>

              <Popover onOpenChange={open => open && void handleLoadCompare()}>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-9 px-2 text-xs"
                    disabled={!result?.file.sha256 && !savedRun?.source_content_hash}
                  >
                    <Hash className="size-3.5" />
                    {t('actions.compare')}
                  </Button>
                </PopoverTrigger>
                <PopoverContent
                  align="end"
                  className="w-[560px] max-w-[calc(100vw-2rem)] rounded-xl p-3"
                >
                  <ComparePanel
                    items={compareRuns}
                    loading={isLoadingCompare}
                    onOpenRun={handleOpenSavedRun}
                  />
                </PopoverContent>
              </Popover>

              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-9"
                onClick={handleSaveRun}
                loading={isSaving}
                disabled={!file || !result}
              >
                <Save className="size-3.5" />
                {savedRun ? t('actions.saved') : t('actions.save')}
              </Button>

              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-9"
                onClick={handleCopyShareLink}
                disabled={!savedRun?.share_token}
              >
                <Share2 className="size-3.5" />
                {t('actions.share')}
              </Button>

              <Button
                type="button"
                onClick={handleParse}
                className="h-9"
                loading={isBusy}
                disabled={
                  !file || isRenderingPreview || !isProviderSelectable(selectedProviderStatus)
                }
              >
                <Play className="size-4" />
                {t('actions.run')}
              </Button>
            </div>
          </div>
        </header>

        <section className="grid min-h-[640px] gap-5 xl:h-[calc(100vh-190px)] xl:max-h-[920px] xl:grid-cols-[minmax(0,1fr)_440px] xl:items-stretch">
          <Card className="flex min-h-[640px] flex-col overflow-hidden border-border/80 shadow-sm xl:h-full">
            <CardHeader className="shrink-0 border-b border-border/70 p-4">
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0">
                  <CardTitle className="text-base">{t('preview.title')}</CardTitle>
                  <CardDescription className="mt-1 truncate">
                    {activePreviewFileName || t('empty.previewDescription')}
                  </CardDescription>
                </div>
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant="outline" className="bg-background">
                    {selectedProvider?.hint || t('providers.auto.hint')}
                  </Badge>
                  {providerSource ? (
                    <Badge variant="outline" className="bg-background">
                      {formatProviderSource(providerSource, t)}
                    </Badge>
                  ) : null}
                  {result?.file.sha256 ? (
                    <Badge variant="outline" className="bg-background">
                      <Hash className="size-3" />
                      {result.file.sha256.slice(0, 12)}
                    </Badge>
                  ) : null}
                  {isRenderingPreview ? (
                    <Badge variant="outline" className="bg-background">
                      <Loader2 className="size-3 animate-spin" />
                      {t('preview.rendering')}
                    </Badge>
                  ) : null}
                </div>
              </div>
            </CardHeader>

            <CardContent className="min-h-0 flex-1 p-0">
              <ScrollArea className="h-full">
                <div className="space-y-6 p-4 lg:p-6">
                  {!hasPreviewContext ? (
                    <EmptyPreview />
                  ) : (
                    <>
                      {previewError ? (
                        <div className="rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
                          {t('preview.renderFailed')}
                          {previewError && previewError !== previewRenderFailedMessage
                            ? ` ${previewError}`
                            : null}
                        </div>
                      ) : null}
                      {pages.map(page => (
                        <div key={page.pageIndex} ref={setPageRef(page.pageIndex)}>
                          <DocumentPagePreview
                            page={page}
                            elements={deferredElements.filter(
                              element => element.page === page.pageIndex
                            )}
                            selectedElementId={selectedElementId}
                            onSelectElement={element => handleSelectElement(element, 'preview')}
                          />
                        </div>
                      ))}
                    </>
                  )}
                </div>
              </ScrollArea>
            </CardContent>
          </Card>

          <Card className="flex min-h-[640px] flex-col overflow-hidden border-border/80 shadow-sm xl:h-full">
            <CardHeader className="shrink-0 border-b border-border/70 p-4">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <CardTitle className="text-base">{t('output.title')}</CardTitle>
                  <CardDescription className="mt-1">{t('output.description')}</CardDescription>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleParse}
                  disabled={!file || isBusy || !isProviderSelectable(selectedProviderStatus)}
                >
                  <RefreshCw className={cn('size-3.5', isBusy && 'animate-spin')} />
                  {t('actions.rerun')}
                </Button>
              </div>
            </CardHeader>

            <Tabs
              value={outputTab}
              onValueChange={setOutputTab}
              className="flex min-h-0 flex-1 flex-col"
            >
              <div className="border-b border-border/70 px-4 py-3">
                <TabsList className="grid w-full grid-cols-4 bg-muted/60">
                  <TabsTrigger value="blocks" className="text-xs">
                    {t('output.tabs.blocks')}
                  </TabsTrigger>
                  <TabsTrigger value="markdown" className="text-xs">
                    {t('output.tabs.markdown')}
                  </TabsTrigger>
                  <TabsTrigger value="json" className="text-xs">
                    {t('output.tabs.json')}
                  </TabsTrigger>
                  <TabsTrigger value="route" className="text-xs">
                    {t('output.tabs.route')}
                  </TabsTrigger>
                </TabsList>
              </div>

              <TabsContent value="blocks" className="m-0 min-h-0 flex-1">
                <ScrollArea className="h-full">
                  <div className="space-y-2 p-4">
                    {deferredElements.length === 0 ? (
                      <EmptyResult text={t('empty.blocks')} />
                    ) : (
                      deferredElements.map(element => {
                        const key = elementKey(element);
                        return (
                          <div key={key} ref={setElementCardRef(key)}>
                            <ElementCard
                              element={element}
                              selected={key === selectedElementId}
                              onSelect={() => handleSelectElement(element, 'list')}
                            />
                          </div>
                        );
                      })
                    )}
                  </div>
                </ScrollArea>
              </TabsContent>

              <TabsContent value="markdown" className="m-0 min-h-0 flex-1">
                <CodePanel
                  title={t('output.tabs.markdown')}
                  value={result?.artifact?.markdown || result?.artifact?.text || ''}
                  empty={t('empty.markdown')}
                />
              </TabsContent>

              <TabsContent value="json" className="m-0 min-h-0 flex-1">
                <CodePanel
                  title={t('output.tabs.json')}
                  value={result ? JSON.stringify(result.artifact, null, 2) : ''}
                  empty={t('empty.json')}
                />
              </TabsContent>

              <TabsContent value="route" className="m-0 min-h-0 flex-1">
                <CodePanel
                  title={t('output.tabs.route')}
                  value={
                    result
                      ? JSON.stringify(
                          {
                            route_plan: result.route_plan,
                            chunk_plan: result.chunk_plan,
                            quality_summary: result.quality_summary,
                            selected_element: selectedElement,
                          },
                          null,
                          2
                        )
                      : ''
                  }
                  empty={t('empty.route')}
                />
              </TabsContent>
            </Tabs>
          </Card>
        </section>
      </div>
    </div>
  );
}

function CompactRunSummary({
  result,
  isBusy,
  isRenderingPreview,
  className,
}: {
  result: ContentParsePlaygroundParseResponse | null;
  isBusy: boolean;
  isRenderingPreview: boolean;
  className?: string;
}) {
  const t = useT('contentParse');
  const summary = result?.quality_summary;
  const metrics = [
    {
      id: 'quality',
      icon: Sparkles,
      label: t('metrics.quality'),
      value: summary?.quality_level ? formatQualityLevel(summary.quality_level, t) : '-',
    },
    {
      id: 'bbox',
      icon: BoxSelect,
      label: t('metrics.bbox'),
      value: summary
        ? `${summary.reliable_bbox_count || summary.bbox_count}/${summary.element_count}`
        : '-',
    },
    {
      id: 'engine',
      icon: Cpu,
      label: t('metrics.engine'),
      value: summary?.engine_used || '-',
    },
    {
      id: 'time',
      icon: Clock3,
      label: t('metrics.time'),
      value: summary
        ? `${summary.duration_ms}ms`
        : isRenderingPreview
          ? t('metrics.rendering')
          : '-',
    },
  ];

  return (
    <div className={cn('grid min-w-0 grid-cols-2 gap-1.5 sm:grid-cols-4', className)}>
      {metrics.map(metric => {
        const Icon = metric.icon;
        return (
          <div
            key={metric.label}
            className="min-w-0 rounded-lg border border-border/70 bg-background/70 px-2 py-1.5"
          >
            <div className="flex items-center gap-1 text-[10px] font-medium uppercase tracking-[0.08em] text-muted-foreground">
              {isBusy && metric.id === 'time' ? (
                <Loader2 className="size-2.5 animate-spin" />
              ) : (
                <Icon className="size-2.5" />
              )}
              {metric.label}
            </div>
            <div className="mt-0.5 truncate text-xs font-semibold text-foreground">
              {metric.value}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function HistoryPanel({
  items,
  loading,
  onOpenRun,
}: {
  items: ContentParsePlaygroundSavedRun[];
  loading: boolean;
  onOpenRun: (run: ContentParsePlaygroundSavedRun) => void;
}) {
  const t = useT('contentParse');

  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-semibold text-foreground">{t('history.title')}</div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('history.description')}</p>
      </div>
      <RunList items={items} loading={loading} empty={t('history.empty')} onOpenRun={onOpenRun} />
    </div>
  );
}

function ComparePanel({
  items,
  loading,
  onOpenRun,
}: {
  items: ContentParsePlaygroundSavedRun[];
  loading: boolean;
  onOpenRun: (run: ContentParsePlaygroundSavedRun) => void;
}) {
  const t = useT('contentParse');

  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-semibold text-foreground">{t('compare.title')}</div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('compare.description')}</p>
      </div>
      <RunList items={items} loading={loading} empty={t('compare.empty')} onOpenRun={onOpenRun} />
    </div>
  );
}

function ProviderSummaryPanel({
  items,
  loading,
}: {
  items: ContentParsePlaygroundProviderSummary[];
  loading: boolean;
}) {
  const t = useT('contentParse');

  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-semibold text-foreground">{t('providerSummary.title')}</div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          {t('providerSummary.description')}
        </p>
      </div>
      {loading ? (
        <LoadingState text={t('providerSummary.loading')} />
      ) : items.length === 0 ? (
        <EmptyInline text={t('providerSummary.empty')} />
      ) : (
        <ScrollArea className="max-h-80 pr-3">
          <div className="space-y-2">
            {items.map(item => (
              <div
                key={item.provider_key}
                className="rounded-lg border border-border/70 bg-background/70 p-3"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium text-foreground">
                      {item.provider_key}
                    </div>
                    <div className="mt-1 truncate text-xs text-muted-foreground">
                      {item.adapter_name || '-'} · {item.engine_name || '-'}
                    </div>
                  </div>
                  <Badge variant={item.failed_count > 0 ? 'warning' : 'success'}>
                    {t('providerSummary.runs', { count: item.run_count })}
                  </Badge>
                </div>
                <div className="mt-3 grid grid-cols-4 gap-2 text-xs">
                  <MetricPill label={t('providerSummary.success')} value={item.success_count} />
                  <MetricPill label={t('providerSummary.degraded')} value={item.degraded_count} />
                  <MetricPill label={t('providerSummary.failed')} value={item.failed_count} />
                  <MetricPill label={t('providerSummary.fallback')} value={item.fallback_count} />
                </div>
                <div className="mt-2 grid grid-cols-3 gap-2 text-xs">
                  <MetricPill
                    label={t('providerSummary.avgTime')}
                    value={`${Math.round(item.avg_duration_ms || 0)}ms`}
                  />
                  <MetricPill
                    label={t('providerSummary.avgText')}
                    value={Math.round(item.avg_text_length || 0)}
                  />
                  <MetricPill
                    label={t('providerSummary.cost')}
                    value={`${item.estimated_cost || 0} ${item.cost_currency || ''}`.trim()}
                  />
                </div>
              </div>
            ))}
          </div>
        </ScrollArea>
      )}
    </div>
  );
}

function RunList({
  items,
  loading,
  empty,
  onOpenRun,
}: {
  items: ContentParsePlaygroundSavedRun[];
  loading: boolean;
  empty: string;
  onOpenRun: (run: ContentParsePlaygroundSavedRun) => void;
}) {
  const t = useT('contentParse');

  if (loading) {
    return <LoadingState text={t('history.loading')} />;
  }
  if (items.length === 0) {
    return <EmptyInline text={empty} />;
  }
  return (
    <ScrollArea className="max-h-80 pr-3">
      <div className="space-y-2">
        {items.map(item => (
          <button
            type="button"
            key={item.id}
            onClick={() => onOpenRun(item)}
            className="w-full rounded-lg border border-border/70 bg-background/70 p-3 text-left transition-colors hover:border-primary/50 hover:bg-muted/30"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium text-foreground">{item.file_name}</div>
                <div className="mt-1 truncate text-xs text-muted-foreground">
                  {formatProviderRunLabel(item)} · {formatDateTime(item.created_at)}
                </div>
              </div>
              <Badge variant={item.status === 'failed' ? 'destructive' : 'outline'}>
                {formatQualityLevel(item.quality_level, t)}
              </Badge>
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>{formatBytes(item.file_size)}</span>
              <span>sha256:{item.source_content_hash.slice(0, 10)}</span>
              {item.duration_ms != null ? <span>{item.duration_ms}ms</span> : null}
            </div>
          </button>
        ))}
      </div>
    </ScrollArea>
  );
}

function MetricPill({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-border/60 bg-muted/20 px-2 py-1">
      <div className="truncate text-[10px] text-muted-foreground">{label}</div>
      <div className="mt-0.5 truncate font-semibold text-foreground">{value}</div>
    </div>
  );
}

function LoadingState({ text }: { text: string }) {
  return (
    <div className="flex items-center justify-center gap-2 rounded-lg border border-border/70 bg-muted/20 p-5 text-sm text-muted-foreground">
      <Loader2 className="size-4 animate-spin" />
      {text}
    </div>
  );
}

function EmptyInline({ text }: { text: string }) {
  return (
    <div className="rounded-lg border border-dashed border-border bg-muted/20 p-5 text-center text-sm text-muted-foreground">
      {text}
    </div>
  );
}

function ProviderQualityHelp({
  provider,
  status,
  className,
}: {
  provider: ProviderOptionView | undefined;
  status: ContentParsePlaygroundProviderStatus | undefined;
  className?: string;
}) {
  const t = useT('contentParse');

  return (
    <div className={cn('grid gap-2 text-xs text-muted-foreground', className)}>
      <div className="rounded-lg border border-border/70 bg-muted/20 p-3">
        <div className="font-medium text-foreground">
          {t('help.providerTitle')}
          {provider ? ` · ${provider.label}` : null}
        </div>
        <p className="mt-1 leading-5">{provider?.explanation || t('providers.auto.explanation')}</p>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <ProviderStatusBadge status={status} />
          <span>{t('help.providerStatusDescription')}</span>
        </div>
      </div>
      <div className="rounded-lg border border-border/70 bg-muted/20 p-3">
        <div className="font-medium text-foreground">{t('help.qualityTitle')}</div>
        <p className="mt-1 leading-5">{t('help.qualityDescription')}</p>
      </div>
    </div>
  );
}

function ProviderStatusBadge({
  status,
  loading = false,
  compact = false,
}: {
  status: ContentParsePlaygroundProviderStatus | undefined;
  loading?: boolean;
  compact?: boolean;
}) {
  const t = useT('contentParse');
  const label = loading ? t('providerStatus.loading') : formatProviderStatus(status, t);

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-medium',
        providerStatusClass(status, loading),
        compact && 'px-1.5 py-0 text-[10px]'
      )}
    >
      <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
      {label}
    </span>
  );
}

function OCRStatusBadge({
  status,
  compact = false,
}: {
  status: ContentParsePlaygroundOCREngineStatus | undefined;
  compact?: boolean;
}) {
  const t = useT('contentParse');
  const label = !status
    ? t('providerStatus.unknown')
    : status.available
      ? t('providerStatus.available')
      : t('providerStatus.unavailable');

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-medium',
        !status
          ? 'border-border bg-muted/50 text-muted-foreground'
          : status.available
            ? 'border-success/30 bg-success/10 text-success'
            : 'border-warning/30 bg-warning/10 text-warning',
        compact && 'px-1.5 py-0 text-[10px]'
      )}
    >
      <span className="size-1.5 rounded-full bg-current" aria-hidden="true" />
      {label}
    </span>
  );
}

function FileDropZone({
  file,
  onFile,
  compact = false,
}: {
  file: File | null;
  onFile: (file: File | null) => void;
  compact?: boolean;
}) {
  const t = useT('contentParse');

  return (
    <label
      className={cn(
        'group flex cursor-pointer items-center gap-3 rounded-xl border border-dashed border-border bg-background transition-colors hover:border-primary/60 hover:bg-muted/30',
        compact ? 'min-h-14 px-3 py-2' : 'min-h-20 px-4 py-3'
      )}
      onDragOver={event => event.preventDefault()}
      onDrop={event => {
        event.preventDefault();
        onFile(event.dataTransfer.files?.[0] || null);
      }}
    >
      <input
        type="file"
        className="sr-only"
        accept=".pdf,.doc,.docx,.ppt,.pptx,.xls,.xlsx,.csv,.txt,.md,.markdown,.png,.jpg,.jpeg,.webp,.gif,.bmp,.tif,.tiff"
        onChange={event => onFile(event.target.files?.[0] || null)}
      />
      <div
        className={cn(
          'flex items-center justify-center rounded-lg bg-primary/10 text-primary',
          compact ? 'size-8' : 'size-11'
        )}
      >
        <UploadCloud className={compact ? 'size-4' : 'size-5'} />
      </div>
      <div className="min-w-0">
        <div className="truncate text-sm font-semibold text-foreground">
          {file ? file.name : t('upload.title')}
        </div>
        <div className="mt-1 text-xs text-muted-foreground">
          {file ? formatBytes(file.size) : t('upload.subtitle')}
        </div>
      </div>
    </label>
  );
}

function ElementCard({
  element,
  selected,
  onSelect,
}: {
  element: ParsedElement;
  selected: boolean;
  onSelect: () => void;
}) {
  const t = useT('contentParse');

  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        'w-full rounded-xl border p-3 text-left transition-colors',
        selected
          ? 'border-primary bg-primary/5'
          : 'border-border bg-card hover:border-primary/40 hover:bg-muted/30'
      )}
    >
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="bg-background">
            {formatElementType(element.type, t)}
          </Badge>
          <span className="text-xs text-muted-foreground">
            {t('element.pageShort', { page: element.page + 1 })}
          </span>
        </div>
        {element.confidence != null ? (
          <span className="text-xs text-muted-foreground">
            {Math.round(element.confidence * 100)}%
          </span>
        ) : element.precision ? (
          <span className="text-xs text-muted-foreground">
            {formatPrecision(element.precision, t)}
          </span>
        ) : null}
      </div>
      <p className="line-clamp-3 text-xs leading-5 text-muted-foreground">
        {element.content || element.metadata?.markdown?.toString() || t('element.noText')}
      </p>
      {element.bbox ? (
        <div className="mt-2 font-mono text-[10px] text-muted-foreground/70">
          [{round(element.bbox.left)}, {round(element.bbox.top)}, {round(element.bbox.right)},{' '}
          {round(element.bbox.bottom)}]
        </div>
      ) : null}
    </button>
  );
}

function CodePanel({ title, value, empty }: { title: string; value: string; empty: string }) {
  const t = useT('contentParse');

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center justify-between border-b border-border/70 px-4 py-3">
        <span className="text-sm font-semibold text-foreground">{title}</span>
        <Button
          type="button"
          variant="outline"
          size="xs"
          disabled={!value}
          onClick={() => {
            void navigator.clipboard.writeText(value);
            toast.success(t('toast.copied'));
          }}
        >
          <Copy className="size-3" />
          {t('actions.copy')}
        </Button>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        {value ? (
          <pre className="whitespace-pre-wrap break-words p-4 font-mono text-xs leading-5 text-muted-foreground">
            {value}
          </pre>
        ) : (
          <EmptyResult text={empty} />
        )}
      </ScrollArea>
    </div>
  );
}

function EmptyPreview() {
  const t = useT('contentParse');

  return (
    <div className="flex min-h-[560px] items-center justify-center rounded-xl border border-dashed border-border bg-card">
      <div className="max-w-sm text-center">
        <div className="mx-auto mb-4 flex size-14 items-center justify-center rounded-xl bg-primary/10 text-primary">
          <FileSearch className="size-7" />
        </div>
        <h3 className="text-base font-semibold text-foreground">{t('empty.previewTitle')}</h3>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {t('empty.previewDescription')}
        </p>
      </div>
    </div>
  );
}

function EmptyResult({ text }: { text: string }) {
  return (
    <div className="flex h-full min-h-[320px] items-center justify-center p-6 text-center text-sm text-muted-foreground">
      <div>
        <FileText className="mx-auto mb-3 size-8 text-muted-foreground/50" />
        {text}
      </div>
    </div>
  );
}

async function renderFilePreview(file: File): Promise<DocumentPreviewPage[]> {
  if (isImageFile(file)) {
    return renderImagePreview(file);
  }
  if (isPDFFile(file)) {
    return renderPDFPreview(file);
  }
  return [];
}

async function renderImagePreview(file: File): Promise<DocumentPreviewPage[]> {
  const url = URL.createObjectURL(file);
  try {
    const dimensions = await readImageDimensions(url);
    return [
      {
        pageIndex: 0,
        imageUrl: url,
        aspectRatio: dimensions.width / dimensions.height,
      },
    ];
  } catch (error) {
    URL.revokeObjectURL(url);
    throw error;
  }
}

async function renderPDFPreview(file: File): Promise<DocumentPreviewPage[]> {
  try {
    const response = await contentParseService.renderPDFPlayground(file, 20);
    const renderedPages = response.data.pages.filter(page => page.startsWith('data:image/'));
    if (renderedPages.length > 0) {
      return Promise.all(
        renderedPages.map(async (imageUrl, index) => {
          const dimensions = await readImageDimensions(imageUrl);
          return {
            pageIndex: index,
            imageUrl,
            aspectRatio: dimensions.width / dimensions.height,
          };
        })
      );
    }
  } catch {
    // Keep the playground usable even if the server-side preview renderer is missing locally.
  }

  const pdfjs = await loadPDFJSModule();
  const bytes = new Uint8Array(await file.arrayBuffer());
  const loadingTask = pdfjs.getDocument({ data: bytes });
  const pdf = await loadingTask.promise;
  const pageLimit = Math.min(pdf.numPages, 20);
  const pages: DocumentPreviewPage[] = [];

  for (let index = 1; index <= pageLimit; index += 1) {
    const page = await pdf.getPage(index);
    const viewport = page.getViewport({ scale: 1.35 });
    const canvas = document.createElement('canvas');
    const context = canvas.getContext('2d');
    if (!context) {
      throw new Error('Canvas is not available');
    }
    canvas.width = Math.ceil(viewport.width);
    canvas.height = Math.ceil(viewport.height);
    await page.render({ canvas, canvasContext: context, viewport }).promise;
    pages.push({
      pageIndex: index - 1,
      imageUrl: canvas.toDataURL('image/png'),
      aspectRatio: viewport.width / viewport.height,
    });
  }

  return pages;
}

async function loadPDFJSModule(): Promise<PDFJSModule> {
  if (!pdfjsModulePromise) {
    pdfjsModulePromise = import('pdfjs-dist/legacy/build/pdf.mjs').then(mod => {
      const workerSrc = new URL('pdfjs-dist/legacy/build/pdf.worker.min.mjs', import.meta.url);
      if (!mod.GlobalWorkerOptions.workerSrc) {
        mod.GlobalWorkerOptions.workerSrc = workerSrc.toString();
      }
      return mod;
    });
  }
  return pdfjsModulePromise;
}

function readImageDimensions(url: string): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () =>
      resolve({ width: image.naturalWidth || 1, height: image.naturalHeight || 1 });
    image.onerror = () => reject(new Error('Image preview failed'));
    image.src = url;
  });
}

function buildPages(
  previewPages: DocumentPreviewPage[],
  reportedPageCount: number | undefined,
  elements: ParsedElement[]
): DocumentPreviewPage[] {
  const pages = [...previewPages];
  const maxElementPage = elements.reduce((max, element) => Math.max(max, element.page), -1);
  const neededCount = Math.max(reportedPageCount || 0, maxElementPage + 1, previewPages.length, 1);
  for (let index = pages.length; index < neededCount; index += 1) {
    pages.push({
      pageIndex: index,
      aspectRatio: 0.72,
    });
  }
  return pages;
}

function savedRunToParseResult(
  run: ContentParsePlaygroundSavedRun
): ContentParsePlaygroundParseResponse {
  return {
    file: {
      name: run.file_name,
      size: run.file_size,
      sha256: run.source_content_hash,
    },
    route_plan: run.route_plan_json,
    artifact: run.artifact_json,
    chunk_source: run.chunk_source_json,
    chunk_plan: run.chunk_plan_json,
    quality_summary:
      run.quality_summary_json ||
      ({
        status: run.status,
        quality_level: run.quality_level,
        engine_used: run.engine_name,
        fallback_used: run.fallback_used,
        duration_ms: run.duration_ms || 0,
        text_length: Number(run.summary_json?.text_length || 0),
        markdown_length: Number(run.summary_json?.markdown_length || 0),
        element_count: Number(run.summary_json?.element_count || 0),
        bbox_count: Number(run.summary_json?.bbox_count || 0),
        reliable_bbox_count: 0,
        unreliable_bbox_count: 0,
        bbox_ratio: 0,
        reliable_bbox_ratio: 0,
        page_count: Number(run.summary_json?.page_count || 0),
        ocr_engine: run.ocr_engine,
      } satisfies ContentParsePlaygroundParseResponse['quality_summary']),
  };
}

type ContentParseTranslator = (key: any, values?: any) => string;

function isProviderSelectable(status: ContentParsePlaygroundProviderStatus | undefined): boolean {
  if (!status) return true;
  return status.selectable;
}

function isOCREngineSelectable(status: ContentParsePlaygroundOCREngineStatus | undefined): boolean {
  if (!status) return true;
  return status.available;
}

function formatProviderStatus(
  status: ContentParsePlaygroundProviderStatus | undefined,
  t: ContentParseTranslator
): string {
  if (!status) return t('providerStatus.unknown');
  switch (status.status) {
    case 'available':
      return t('providerStatus.available');
    case 'fallback':
      return t('providerStatus.fallback');
    case 'not_configured':
      return t('providerStatus.notConfigured');
    case 'unavailable':
      return t('providerStatus.unavailable');
    default:
      return t('providerStatus.unknown');
  }
}

function providerStatusClass(
  status: ContentParsePlaygroundProviderStatus | undefined,
  loading: boolean
): string {
  if (loading || !status) {
    return 'border-border bg-muted/50 text-muted-foreground';
  }
  switch (status.status) {
    case 'available':
      return 'border-success/30 bg-success/10 text-success';
    case 'fallback':
      return 'border-info/30 bg-info/10 text-info';
    case 'not_configured':
      return 'border-warning/30 bg-warning/10 text-warning';
    case 'unavailable':
      return 'border-destructive/30 bg-destructive/10 text-destructive';
    default:
      return 'border-border bg-muted/50 text-muted-foreground';
  }
}

function formatProviderSource(source: string, t: ContentParseTranslator): string {
  switch (source) {
    case 'default_catalog':
      return t('providerStatus.sourceDefaultCatalog');
    default:
      return source;
  }
}

function formatQualityLevel(level: string, t: ContentParseTranslator): string {
  switch (level) {
    case 'high':
      return t('qualityLevels.high');
    case 'standard':
      return t('qualityLevels.standard');
    case 'degraded':
      return t('qualityLevels.degraded');
    case 'failed':
      return t('qualityLevels.failed');
    default:
      return level;
  }
}

function formatElementType(type: string | undefined, t: ContentParseTranslator): string {
  const normalized = (type || '').replace(/[_-]/g, '').toLowerCase();
  switch (normalized) {
    case 'title':
      return t('element.types.title');
    case 'heading':
      return t('element.types.heading');
    case 'text':
      return t('element.types.text');
    case 'paragraph':
      return t('element.types.paragraph');
    case 'table':
      return t('element.types.table');
    case 'figure':
      return t('element.types.figure');
    case 'image':
      return t('element.types.image');
    case 'formula':
      return t('element.types.formula');
    case 'list':
      return t('element.types.list');
    case 'listitem':
      return t('element.types.listItem');
    case 'code':
      return t('element.types.code');
    default:
      return type || t('element.types.element');
  }
}

function formatPrecision(precision: string, t: ContentParseTranslator): string {
  switch (precision) {
    case 'reliable':
      return t('element.precision.reliable');
    case 'unreliable':
      return t('element.precision.unreliable');
    case 'estimated':
      return t('element.precision.estimated');
    default:
      return precision;
  }
}

function elementKey(element: ParsedElement): string {
  return element.id || `${element.page}-${element.ordinal}-${element.type}`;
}

function hasReliableBox(element: ParsedElement): boolean {
  return Boolean(element.bbox && element.precision !== 'unreliable');
}

function isPDFFile(file: File): boolean {
  return file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf');
}

function isImageFile(file: File): boolean {
  return file.type.startsWith('image/') && !file.name.toLowerCase().match(/\.tiff?$/);
}

function formatBytes(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

function formatProviderRunLabel(run: ContentParsePlaygroundSavedRun): string {
  const provider = run.final_provider_key || run.requested_provider_key || 'unknown';
  const engine = run.engine_name || run.adapter_name;
  return engine ? `${provider} / ${engine}` : provider;
}

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function round(value: number): string {
  return Number.isFinite(value) ? value.toFixed(3) : '0.000';
}
