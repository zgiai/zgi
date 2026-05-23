'use client';

import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import type JSZip from 'jszip';
import {
  AlertCircle,
  Download,
  ExternalLink,
  FileText,
  Image as ImageIcon,
  Loader2,
  RotateCcw,
  ZoomIn,
  ZoomOut,
} from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  getOriginalPreviewKind,
  isOriginalPreviewImage,
  isOriginalPreviewPdf,
  isOriginalPreviewSupported,
} from '@/utils/file-helpers';
import { cn } from '@/lib/utils';

const maxOfficePreviewBytes = 10 * 1024 * 1024;
const maxOfficeZipEntries = 256;
const maxOfficeZipUncompressedBytes = 50 * 1024 * 1024;
const maxOfficeZipEntryBytes = 10 * 1024 * 1024;
const maxOfficeXmlEntryBytes = 8 * 1024 * 1024;
const maxCsvPreviewBytes = 1024 * 1024;
const maxHtmlPreviewBytes = 2 * 1024 * 1024;
const maxSpreadsheetPreviewRows = 200;
const maxSpreadsheetPreviewColumns = 100;
const maxCsvPreviewRows = 500;
const maxCsvPreviewColumns = 100;
const defaultDocxPreviewZoom = 1.15;
const minDocxPreviewZoom = 0.75;
const maxDocxPreviewZoom = 1.8;
const docxPreviewZoomStep = 0.1;
const htmlPreviewCsp = [
  'default-src \'none\'',
  'script-src \'unsafe-inline\'',
  'style-src \'unsafe-inline\' https://fonts.googleapis.com',
  'font-src https://fonts.gstatic.com data:',
  'img-src data: blob:',
  'connect-src \'none\'',
  'form-action \'none\'',
  'frame-src \'none\'',
  'base-uri \'none\'',
  'navigate-to \'none\'',
].join('; ');
const htmlPreviewFallbackStyle = `
  .reveal {
    opacity: 1 !important;
    transform: none !important;
  }
`;

export interface UniversalFilePreviewDescriptor {
  id?: string;
  name: string;
  extension?: string | null;
  mimeType?: string | null;
  size?: number | null;
  previewUrl?: string;
  downloadUrl?: string;
}

export interface UniversalFilePreviewDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  file: UniversalFilePreviewDescriptor | null;
  previewUrl?: string;
  isLoading?: boolean;
  error?: string | null;
  onDownload?: () => void;
  isDownloading?: boolean;
}

/**
 * @component UniversalFilePreviewDialog
 * @category Feature
 * @status Stable
 * @description Shared original-file preview dialog for file manager and generated file artifacts.
 */
export function UniversalFilePreviewDialog({
  open,
  onOpenChange,
  file,
  previewUrl,
  isLoading = false,
  error = null,
  onDownload,
  isDownloading = false,
}: UniversalFilePreviewDialogProps) {
  const t = useT('files');
  const [previewSession, setPreviewSession] = useState<{
    file: UniversalFilePreviewDescriptor;
    previewUrl: string;
    downloadUrl: string;
  } | null>(null);
  const [htmlOpenConfirmOpen, setHtmlOpenConfirmOpen] = useState(false);

  useEffect(() => {
    if (!open) {
      setPreviewSession(null);
      setHtmlOpenConfirmOpen(false);
      return;
    }
    if (!file) return;

    setPreviewSession(current => {
      if (current) return current;

      const sessionPreviewUrl = previewUrl || file.previewUrl || '';
      return {
        file,
        previewUrl: sessionPreviewUrl,
        downloadUrl: file.downloadUrl || sessionPreviewUrl,
      };
    });
  }, [file, open, previewUrl]);

  const activeFile = previewSession?.file ?? file;
  const resolvedPreviewUrl = previewSession?.previewUrl ?? previewUrl ?? file?.previewUrl ?? '';
  const downloadUrl = previewSession?.downloadUrl ?? file?.downloadUrl ?? resolvedPreviewUrl;
  const isSupported = isOriginalPreviewSupported(activeFile?.extension, activeFile?.mimeType);
  const extension = activeFile?.extension?.replace(/^\./, '').toUpperCase() || '';
  const title = activeFile?.name || t('preview.title');
  const isImage = isOriginalPreviewImage(activeFile?.extension, activeFile?.mimeType);
  const isPdf = isOriginalPreviewPdf(activeFile?.extension, activeFile?.mimeType);
  const previewKind = getOriginalPreviewKind(activeFile?.extension, activeFile?.mimeType);
  const htmlExternalOpenUrl = previewKind === 'html' ? resolvedPreviewUrl : '';
  const canOpenHtmlExternally = Boolean(htmlExternalOpenUrl);

  const openHtmlInNewTab = () => {
    if (!htmlExternalOpenUrl) return;
    window.open(htmlExternalOpenUrl, '_blank', 'noopener,noreferrer');
  };

  const renderPreview = () => {
    if (!activeFile) {
      return (
        <PreviewMessage
          icon={<AlertCircle className="h-5 w-5" />}
          title={t('preview.noFileSelected')}
        />
      );
    }

    if (!isSupported) {
      return (
        <PreviewMessage
          icon={<AlertCircle className="h-5 w-5" />}
          title={t('preview.unsupportedTitle')}
          description={t('preview.unsupportedDescription')}
        />
      );
    }

    if (isLoading) {
      return (
        <PreviewMessage
          icon={<Loader2 className="h-5 w-5 animate-spin" />}
          title={t('preview.loading')}
        />
      );
    }

    if (error || !resolvedPreviewUrl) {
      return (
        <PreviewMessage
          icon={<AlertCircle className="h-5 w-5" />}
          title={error || t('preview.unavailableTitle')}
          description={t('preview.downloadOnlyDescription')}
        />
      );
    }

    if (isImage) {
      return (
        <div className="flex h-full min-h-0 items-center justify-center overflow-auto bg-muted/30 p-4">
          <img
            src={resolvedPreviewUrl}
            alt={activeFile.name}
            className="max-h-full max-w-full object-contain"
          />
        </div>
      );
    }

    if (previewKind === 'office') {
      return <OfficePreview file={activeFile} previewUrl={resolvedPreviewUrl} />;
    }

    if (previewKind === 'csv') {
      return <CsvPreview previewUrl={resolvedPreviewUrl} />;
    }

    if (previewKind === 'html') {
      return <HtmlPreview previewUrl={resolvedPreviewUrl} title={activeFile.name} />;
    }

    if (isPdf) {
      return (
        <iframe
          src={resolvedPreviewUrl}
          title={activeFile.name}
          referrerPolicy="no-referrer"
          className="h-full min-h-[60vh] w-full border-0 bg-background"
        />
      );
    }

    if (previewKind === 'browser') {
      return (
        <iframe
          src={resolvedPreviewUrl}
          title={activeFile.name}
          sandbox=""
          referrerPolicy="no-referrer"
          className="h-full min-h-[60vh] w-full border-0 bg-background"
        />
      );
    }

    return (
      <PreviewMessage
        icon={<AlertCircle className="h-5 w-5" />}
        title={t('preview.unsupportedTitle')}
        description={t('preview.unsupportedDescription')}
      />
    );
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="full" className="gap-0 overflow-hidden p-0">
          <DialogHeader className="border-b px-5 py-4">
            <div className="flex min-w-0 items-start gap-3 pr-8">
              <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                {isImage ? <ImageIcon className="h-4 w-4" /> : <FileText className="h-4 w-4" />}
              </div>
              <div className="min-w-0">
                <DialogTitle className="truncate text-base leading-6">{title}</DialogTitle>
                <DialogDescription className="mt-1">
                  {extension ? t('preview.fileMeta', { extension }) : t('preview.description')}
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>

          <DialogBody className="min-h-0 overflow-hidden p-0">{renderPreview()}</DialogBody>

          <DialogFooter className="border-t px-5 py-3">
            {canOpenHtmlExternally ? (
              <Button variant="outline" onClick={() => setHtmlOpenConfirmOpen(true)}>
                <ExternalLink className="mr-2 h-4 w-4" />
                {t('preview.openInNewTab')}
              </Button>
            ) : null}
            {downloadUrl ? (
              <Button variant="outline" asChild>
                <a href={downloadUrl} download={activeFile?.name}>
                  <Download className="mr-2 h-4 w-4" />
                  {t('actions.downloadFile')}
                </a>
              </Button>
            ) : file && onDownload ? (
              <Button variant="outline" onClick={onDownload} disabled={isDownloading}>
                <Download className="mr-2 h-4 w-4" />
                {t('actions.downloadFile')}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={htmlOpenConfirmOpen}
        onOpenChange={setHtmlOpenConfirmOpen}
        title={t('preview.htmlOpenRiskTitle')}
        description={t('preview.htmlOpenRiskDescription')}
        confirmText={t('preview.htmlOpenRiskConfirm')}
        cancelText={t('preview.htmlOpenRiskCancel')}
        variant="warning"
        onConfirm={openHtmlInNewTab}
      />
    </>
  );
}

interface OfficePreviewProps {
  file: UniversalFilePreviewDescriptor;
  previewUrl: string;
}

function OfficePreview({ file, previewUrl }: OfficePreviewProps) {
  const t = useT('files');
  const extension = file.extension?.toLowerCase().replace(/^\./, '') ?? '';
  if (file.size && file.size > maxOfficePreviewBytes) {
    return (
      <PreviewMessage
        icon={<AlertCircle className="h-5 w-5" />}
        title={t('preview.officeTooLargeTitle')}
        description={t('preview.officeFallback')}
      />
    );
  }
  if (extension === 'docx') {
    return <DocxPreview previewUrl={previewUrl} />;
  }
  if (extension === 'xlsx') {
    return <SpreadsheetPreview previewUrl={previewUrl} />;
  }

  return (
    <PreviewMessage
      icon={<AlertCircle className="h-5 w-5" />}
      title={t('preview.officeUnsupportedTitle')}
      description={t('preview.officeFallback')}
    />
  );
}

function HtmlPreview({ previewUrl, title }: { previewUrl: string; title: string }) {
  const t = useT('files');
  const [srcDoc, setSrcDoc] = useState('');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const loadErrorText = t('preview.loadError');
  const htmlTooLargeText = t('preview.htmlTooLargeTitle');

  useEffect(() => {
    const abortController = new AbortController();
    let cancelled = false;
    setIsLoading(true);
    setError(null);
    setSrcDoc('');

    const loadHtml = async () => {
      try {
        const response = await fetch(previewUrl, {
          credentials: 'include',
          signal: abortController.signal,
        });
        if (!response.ok) throw new Error(loadErrorText);

        const contentLength = Number(response.headers.get('content-length') || 0);
        if (contentLength > maxHtmlPreviewBytes) {
          throw new Error(htmlTooLargeText);
        }

        const html = await response.text();
        if (new Blob([html]).size > maxHtmlPreviewBytes) {
          throw new Error(htmlTooLargeText);
        }
        if (cancelled) return;

        setSrcDoc(buildIsolatedHtmlPreview(html));
      } catch (err) {
        if (abortController.signal.aborted || cancelled) return;
        setError(err instanceof Error ? err.message : loadErrorText);
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };

    void loadHtml();
    return () => {
      cancelled = true;
      abortController.abort();
    };
  }, [previewUrl, loadErrorText, htmlTooLargeText]);

  return (
    <div className="flex h-full min-h-[60vh] flex-col bg-background">
      <div className="border-b bg-muted/30 px-4 py-3 text-xs text-muted-foreground">
        <div className="font-medium text-foreground">{t('preview.htmlLimitedTitle')}</div>
        <div className="mt-1">{t('preview.htmlLimitedDescription')}</div>
      </div>
      {isLoading ? (
        <PreviewMessage
          icon={<Loader2 className="h-5 w-5 animate-spin" />}
          title={t('preview.loading')}
        />
      ) : error || !srcDoc ? (
        <PreviewMessage
          icon={<AlertCircle className="h-5 w-5" />}
          title={error || loadErrorText}
          description={t('preview.downloadOnlyDescription')}
        />
      ) : (
        <iframe
          srcDoc={srcDoc}
          title={title}
          sandbox="allow-scripts"
          referrerPolicy="no-referrer"
          className="min-h-0 flex-1 border-0 bg-background"
        />
      )}
    </div>
  );
}

function buildIsolatedHtmlPreview(html: string): string {
  const parser = new DOMParser();
  const document = parser.parseFromString(html, 'text/html');
  const head = document.head || document.documentElement.insertBefore(document.createElement('head'), document.body);

  document.querySelectorAll('base, iframe, object, embed, form, meta[http-equiv="refresh"]').forEach(node => {
    node.remove();
  });
  document.querySelectorAll('script[src], script[type="module"], script[type="importmap"]').forEach(node => {
    node.remove();
  });
  document.querySelectorAll('*').forEach(element => {
    for (const attribute of Array.from(element.attributes)) {
      const name = attribute.name.toLowerCase();
      if (name.startsWith('on')) {
        element.removeAttribute(attribute.name);
        continue;
      }

      if (['action', 'formaction', 'href', 'src', 'xlink:href'].includes(name)) {
        const value = attribute.value.trim();
        if (!isSafeHtmlPreviewUrl(value, name)) {
          element.removeAttribute(attribute.name);
        }
      }
    }
  });

  const csp = document.createElement('meta');
  csp.setAttribute('http-equiv', 'Content-Security-Policy');
  csp.setAttribute('content', htmlPreviewCsp);
  head.prepend(csp);

  const fallbackStyle = document.createElement('style');
  fallbackStyle.textContent = htmlPreviewFallbackStyle;
  head.appendChild(fallbackStyle);

  return `<!doctype html>\n${document.documentElement.outerHTML}`;
}

function isSafeHtmlPreviewUrl(value: string, attributeName: string): boolean {
  if (!value) return true;
  const normalized = value.replace(/\s+/g, '').replace(/\u007f/g, '').toLowerCase();
  if (
    normalized.startsWith('javascript:') ||
    normalized.startsWith('vbscript:') ||
    normalized.startsWith('file:')
  ) {
    return false;
  }

  if (attributeName === 'href' || attributeName === 'xlink:href') {
    return value.startsWith('#');
  }

  if (attributeName === 'src') {
    return normalized.startsWith('data:image/') || normalized.startsWith('blob:');
  }

  return false;
}

function DocxPreview({ previewUrl }: { previewUrl: string }) {
  const t = useT('files');
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [zoom, setZoom] = useState(defaultDocxPreviewZoom);
  const loadErrorText = t('preview.loadError');
  const officeTooLargeText = t('preview.officeTooLargeTitle');

  const updateZoom = (next: number) => {
    setZoom(Math.min(maxDocxPreviewZoom, Math.max(minDocxPreviewZoom, Number(next.toFixed(2)))));
  };

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let cancelled = false;
    const abortController = new AbortController();
    setIsLoading(true);
    setError(null);
    container.innerHTML = '';
    setZoom(defaultDocxPreviewZoom);

    const render = async () => {
      try {
        const [response, docxPreview, jszip] = await Promise.all([
          fetch(previewUrl, { credentials: 'include', signal: abortController.signal }),
          import('docx-preview'),
          import('jszip'),
        ]);
        if (!response.ok) throw new Error(loadErrorText);

        const buffer = await readOfficePreviewBuffer(response, officeTooLargeText);
        await loadOfficeZipPreview(buffer, jszip.default, officeTooLargeText);
        if (cancelled) return;

        const blob = new Blob([buffer], {
          type:
            response.headers.get('content-type') ||
            'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        });
        await docxPreview.renderAsync(blob, container, undefined, {
          className: 'zgi-docx-preview',
          inWrapper: false,
          ignoreWidth: false,
          ignoreHeight: false,
        });
      } catch (err) {
        if (abortController.signal.aborted) return;
        if (!cancelled) {
          setError(err instanceof Error ? err.message : loadErrorText);
        }
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };

    void render();
    return () => {
      cancelled = true;
      abortController.abort();
    };
  }, [previewUrl, loadErrorText, officeTooLargeText]);

  if (error) {
    return (
      <PreviewMessage
        icon={<AlertCircle className="h-5 w-5" />}
        title={error}
        description={t('preview.officeFallback')}
      />
    );
  }

  return (
    <div className="relative flex h-full min-h-[60vh] flex-col overflow-hidden bg-[#f5f6f8]">
      <div className="flex min-h-12 items-center justify-center border-b bg-background/90 px-4">
        <div className="inline-flex items-center gap-1 rounded-md border bg-background p-1 shadow-sm">
          <Button
            type="button"
            variant="ghost"
            size="xs"
            isIcon
            aria-label="Zoom out"
            title="Zoom out"
            disabled={zoom <= minDocxPreviewZoom}
            onClick={() => updateZoom(zoom - docxPreviewZoomStep)}
          >
            <ZoomOut className="h-3.5 w-3.5" />
          </Button>
          <div className="min-w-14 px-2 text-center text-xs font-medium text-muted-foreground">
            {Math.round(zoom * 100)}%
          </div>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            isIcon
            aria-label="Zoom in"
            title="Zoom in"
            disabled={zoom >= maxDocxPreviewZoom}
            onClick={() => updateZoom(zoom + docxPreviewZoomStep)}
          >
            <ZoomIn className="h-3.5 w-3.5" />
          </Button>
          <div className="mx-1 h-5 w-px bg-border" />
          <Button
            type="button"
            variant="ghost"
            size="xs"
            isIcon
            aria-label="Reset zoom"
            title="Reset zoom"
            onClick={() => updateZoom(defaultDocxPreviewZoom)}
          >
            <RotateCcw className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      {isLoading ? (
        <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/70">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span>{t('preview.loading')}</span>
          </div>
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-auto px-6 py-8">
        <div
          className="mx-auto w-fit max-w-full origin-top"
          style={{ zoom } as React.CSSProperties}
        >
          <div
            ref={containerRef}
            className={cn(
              'mx-auto w-fit max-w-full',
              '[&_.zgi-docx-preview-wrapper]:bg-transparent',
              '[&_.zgi-docx-preview]:!m-0',
              '[&_.zgi-docx-preview]:box-border',
              '[&_.zgi-docx-preview]:px-[72px]',
              '[&_.zgi-docx-preview]:py-16',
              '[&_.zgi-docx-preview]:overflow-hidden',
              '[&_.zgi-docx-preview]:rounded-[3px]',
              '[&_.zgi-docx-preview]:border',
              '[&_.zgi-docx-preview]:border-black/5',
              '[&_.zgi-docx-preview]:bg-white',
              '[&_.zgi-docx-preview]:shadow-[0_18px_45px_rgba(15,23,42,0.12),0_1px_3px_rgba(15,23,42,0.10)]',
              '[&_.zgi-docx-preview+_.zgi-docx-preview]:mt-8'
            )}
          />
        </div>
      </div>
    </div>
  );
}

interface SpreadsheetRowPreview {
  number: number;
  cells: string[];
}

interface SpreadsheetSheetPreview {
  name: string;
  rows: SpreadsheetRowPreview[];
  columnCount: number;
  totalRowCount: number;
}

interface SpreadsheetState {
  sheets: SpreadsheetSheetPreview[];
  activeSheet: string;
}

function SpreadsheetPreview({ previewUrl }: { previewUrl: string }) {
  const t = useT('files');
  const [state, setState] = useState<SpreadsheetState | null>(null);
  const [activeSheet, setActiveSheet] = useState('');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const loadErrorText = t('preview.loadError');
  const emptyWorkbookText = t('preview.emptyWorkbook');
  const officeTooLargeText = t('preview.officeTooLargeTitle');

  useEffect(() => {
    let cancelled = false;
    const abortController = new AbortController();
    setIsLoading(true);
    setError(null);

    const load = async () => {
      try {
        const [response, jszip] = await Promise.all([
          fetch(previewUrl, { credentials: 'include', signal: abortController.signal }),
          import('jszip'),
        ]);
        if (!response.ok) throw new Error(loadErrorText);

        const buffer = await readOfficePreviewBuffer(response, officeTooLargeText);
        const sheets = await readXlsxWorkbookPreview(buffer, jszip.default, officeTooLargeText);
        const firstSheet = sheets[0];
        if (!cancelled) {
          if (!firstSheet) throw new Error(emptyWorkbookText);

          setActiveSheet(firstSheet.name);
          setState({
            sheets,
            activeSheet: firstSheet.name,
          });
        }
      } catch (err) {
        if (abortController.signal.aborted) return;
        if (!cancelled) {
          setError(err instanceof Error ? err.message : loadErrorText);
        }
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };

    void load();
    return () => {
      cancelled = true;
      abortController.abort();
    };
  }, [previewUrl, loadErrorText, emptyWorkbookText, officeTooLargeText]);

  const currentSheet = useMemo(
    () => state?.sheets.find(item => item.name === activeSheet) ?? state?.sheets[0],
    [activeSheet, state]
  );
  const visibleRows = currentSheet?.rows ?? [];
  const maxColumns = currentSheet?.columnCount ?? 0;

  if (isLoading && !state) {
    return (
      <PreviewMessage
        icon={<Loader2 className="h-5 w-5 animate-spin" />}
        title={t('preview.loading')}
      />
    );
  }

  if (error || !state) {
    return (
      <PreviewMessage
        icon={<AlertCircle className="h-5 w-5" />}
        title={error || t('preview.loadError')}
        description={t('preview.officeFallback')}
      />
    );
  }

  return (
    <div className="flex h-full min-h-[60vh] flex-col bg-background">
      <div className="flex min-h-11 items-center gap-2 overflow-x-auto border-b px-3">
        {state.sheets.map(sheet => (
          <button
            key={sheet.name}
            type="button"
            className={cn(
              'h-8 shrink-0 rounded-md px-3 text-xs font-medium transition-colors',
              (currentSheet?.name ?? activeSheet) === sheet.name
                ? 'bg-primary text-primary-foreground'
                : 'bg-muted text-muted-foreground hover:text-foreground'
            )}
            onClick={() => setActiveSheet(sheet.name)}
          >
            {sheet.name}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-3">
        <table className="w-max border-collapse text-xs">
          <thead>
            <tr>
              <th className="sticky left-0 top-0 z-20 h-8 w-12 min-w-12 border bg-muted/80 px-2 text-center font-medium text-muted-foreground" />
              {Array.from({ length: maxColumns }).map((_, columnIndex) => (
                <th
                  key={`column-${columnIndex}`}
                  className="sticky top-0 z-10 h-8 min-w-28 border bg-muted/80 px-2 text-center font-medium text-muted-foreground"
                >
                  {getSpreadsheetColumnLabel(columnIndex + 1)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visibleRows.length > 0 ? (
              visibleRows.map((row, rowIndex) => (
                <tr key={`row-${rowIndex}`} className={rowIndex === 0 ? 'bg-muted/60' : ''}>
                  <th className="sticky left-0 z-10 h-8 w-12 min-w-12 border bg-muted/80 px-2 text-right font-medium text-muted-foreground">
                    {row.number}
                  </th>
                  {Array.from({ length: maxColumns }).map((_, columnIndex) => (
                    <td
                      key={`${row.number}-${columnIndex}`}
                      className="h-8 min-w-28 max-w-72 border bg-background px-2 py-1 align-top"
                    >
                      <div className="max-h-24 overflow-hidden whitespace-pre-wrap break-words">
                        {row.cells[columnIndex] ?? ''}
                      </div>
                    </td>
                  ))}
                </tr>
              ))
            ) : (
              <tr>
                <td
                  colSpan={Math.max(maxColumns, 1) + 1}
                  className="p-8 text-center text-sm text-muted-foreground"
                >
                  {t('preview.emptySheet')}
                </td>
              </tr>
            )}
          </tbody>
        </table>
        {currentSheet && currentSheet.totalRowCount > visibleRows.length ? (
          <div className="mt-3 text-xs text-muted-foreground">
            {t('preview.rowLimit', { count: visibleRows.length })}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function CsvPreview({ previewUrl }: { previewUrl: string }) {
  const t = useT('files');
  const [rows, setRows] = useState<string[][] | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const loadErrorText = t('preview.loadError');
  const textTooLargeText = t('preview.textTooLargeTitle');

  useEffect(() => {
    let cancelled = false;
    const abortController = new AbortController();
    setIsLoading(true);
    setError(null);

    const load = async () => {
      try {
        const response = await fetch(previewUrl, {
          credentials: 'include',
          signal: abortController.signal,
        });
        if (!response.ok) throw new Error(loadErrorText);

        const text = await readCsvPreviewText(response, abortController, textTooLargeText);
        if (!cancelled) {
          setRows(parseCsvPreviewRows(text));
        }
      } catch (err) {
        if (abortController.signal.aborted) return;
        if (!cancelled) {
          setError(err instanceof Error ? err.message : loadErrorText);
        }
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };

    void load();
    return () => {
      cancelled = true;
      abortController.abort();
    };
  }, [previewUrl, loadErrorText, textTooLargeText]);

  if (isLoading && !rows) {
    return (
      <PreviewMessage
        icon={<Loader2 className="h-5 w-5 animate-spin" />}
        title={t('preview.loading')}
      />
    );
  }

  if (error || !rows) {
    return (
      <PreviewMessage
        icon={<AlertCircle className="h-5 w-5" />}
        title={error || t('preview.loadError')}
        description={t('preview.textFallback')}
      />
    );
  }

  const maxColumns = Math.min(Math.max(0, ...rows.map(row => row.length)), maxCsvPreviewColumns);
  const visibleRows = rows.slice(0, maxCsvPreviewRows);

  return (
    <div className="flex h-full min-h-[60vh] flex-col bg-background">
      <div className="min-h-0 flex-1 overflow-auto p-3">
        {visibleRows.length > 0 && maxColumns > 0 ? (
          <table className="w-max border-collapse text-xs">
            <tbody>
              {visibleRows.map((row, rowIndex) => (
                <tr key={`csv-row-${rowIndex}`} className={rowIndex === 0 ? 'bg-muted/60' : ''}>
                  <th className="sticky left-0 z-10 h-8 w-12 min-w-12 border bg-muted/80 px-2 text-right font-medium text-muted-foreground">
                    {rowIndex + 1}
                  </th>
                  {Array.from({ length: maxColumns }).map((_, columnIndex) => (
                    <td
                      key={`${rowIndex}-${columnIndex}`}
                      className={cn(
                        'h-8 min-w-28 max-w-72 border bg-background px-2 py-1 align-top',
                        rowIndex === 0 ? 'font-medium text-foreground' : 'text-foreground'
                      )}
                    >
                      <div className="max-h-24 overflow-hidden whitespace-pre-wrap break-words">
                        {row[columnIndex] ?? ''}
                      </div>
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <div className="flex h-full min-h-[360px] items-center justify-center text-sm text-muted-foreground">
            {t('preview.emptySheet')}
          </div>
        )}
        {rows.length > visibleRows.length ? (
          <div className="mt-3 text-xs text-muted-foreground">
            {t('preview.rowLimit', { count: visibleRows.length })}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function parseCsvPreviewRows(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = '';
  let inQuotes = false;

  for (let index = 0; index < text.length; index++) {
    const char = text[index];
    const next = text[index + 1];

    if (char === '"') {
      if (inQuotes && next === '"') {
        cell += '"';
        index++;
      } else {
        inQuotes = !inQuotes;
      }
      continue;
    }

    if (char === ',' && !inQuotes) {
      row.push(cell);
      cell = '';
      continue;
    }

    if ((char === '\n' || char === '\r') && !inQuotes) {
      if (char === '\r' && next === '\n') index++;
      row.push(cell);
      if (row.some(value => value.trim() !== '')) {
        rows.push(row.slice(0, maxCsvPreviewColumns));
      }
      row = [];
      cell = '';
      if (rows.length >= maxCsvPreviewRows + 1) break;
      continue;
    }

    cell += char;
  }

  if (cell !== '' || row.length > 0) {
    row.push(cell);
    if (row.some(value => value.trim() !== '')) {
      rows.push(row.slice(0, maxCsvPreviewColumns));
    }
  }

  return rows;
}

async function readCsvPreviewText(
  response: Response,
  abortController: AbortController,
  limitErrorText: string
): Promise<string> {
  const contentLength = Number(response.headers.get('content-length') ?? 0);
  if (contentLength > maxCsvPreviewBytes && !response.body) {
    throw new Error(limitErrorText);
  }
  if (!response.body) {
    return response.text();
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let receivedBytes = 0;
  let text = '';

  try {
    let shouldRead = true;
    while (shouldRead) {
      const { done, value } = await reader.read();
      if (done) {
        shouldRead = false;
        continue;
      }
      if (!value) continue;

      receivedBytes += value.byteLength;
      if (receivedBytes > maxCsvPreviewBytes) {
        throw new Error(limitErrorText);
      }

      text += decoder.decode(value, { stream: true });
      if (countCsvPreviewRows(text, maxCsvPreviewRows + 1) >= maxCsvPreviewRows + 1) {
        abortController.abort();
        shouldRead = false;
      }
    }
    text += decoder.decode();
    return text;
  } finally {
    reader.releaseLock();
  }
}

function countCsvPreviewRows(text: string, limit: number): number {
  let rows = 0;
  let inQuotes = false;

  for (let index = 0; index < text.length; index++) {
    const char = text[index];
    const next = text[index + 1];

    if (char === '"') {
      if (inQuotes && next === '"') {
        index++;
      } else {
        inQuotes = !inQuotes;
      }
      continue;
    }

    if ((char === '\n' || char === '\r') && !inQuotes) {
      if (char === '\r' && next === '\n') index++;
      rows++;
      if (rows >= limit) return rows;
    }
  }

  return rows;
}

async function readOfficePreviewBuffer(
  response: Response,
  limitErrorText: string
): Promise<ArrayBuffer> {
  return readLimitedResponseArrayBuffer(response, maxOfficePreviewBytes, limitErrorText);
}

async function readLimitedResponseArrayBuffer(
  response: Response,
  maxBytes: number,
  limitErrorText: string
): Promise<ArrayBuffer> {
  const contentLength = readContentLength(response);
  if (contentLength !== null && contentLength > maxBytes) {
    throw new Error(limitErrorText);
  }

  if (!response.body) {
    if (contentLength === null) throw new Error(limitErrorText);

    const buffer = await response.arrayBuffer();
    if (buffer.byteLength > maxBytes) throw new Error(limitErrorText);
    return buffer;
  }

  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let receivedBytes = 0;

  try {
    let shouldRead = true;
    while (shouldRead) {
      const { done, value } = await reader.read();
      if (done) {
        shouldRead = false;
        continue;
      }
      if (!value) continue;

      if (receivedBytes + value.byteLength > maxBytes) {
        await reader.cancel();
        throw new Error(limitErrorText);
      }

      chunks.push(value);
      receivedBytes += value.byteLength;
    }
  } finally {
    reader.releaseLock();
  }

  const merged = new Uint8Array(receivedBytes);
  let offset = 0;
  chunks.forEach(chunk => {
    merged.set(chunk, offset);
    offset += chunk.byteLength;
  });
  return merged.buffer;
}

function readContentLength(response: Response): number | null {
  const raw = response.headers.get('content-length');
  if (!raw) return null;

  const size = Number(raw);
  if (!Number.isFinite(size) || size < 0) return null;
  return size;
}

async function readXlsxWorkbookPreview(
  buffer: ArrayBuffer,
  JSZipCtor: typeof JSZip,
  limitErrorText: string
): Promise<SpreadsheetSheetPreview[]> {
  const zip = await loadOfficeZipPreview(buffer, JSZipCtor, limitErrorText);
  const workbookXml = await readZipText(zip, 'xl/workbook.xml', limitErrorText);
  const workbookRelsXml = await readZipText(zip, 'xl/_rels/workbook.xml.rels', limitErrorText);
  const sharedStrings = await readSharedStrings(zip, limitErrorText);

  if (!workbookXml || !workbookRelsXml) return [];

  const workbookDoc = parseXml(workbookXml);
  const relsDoc = parseXml(workbookRelsXml);
  const relationshipTargets = new Map<string, string>();

  getElementsByLocalName(relsDoc, 'Relationship').forEach(relationship => {
    const id = relationship.getAttribute('Id');
    const target = relationship.getAttribute('Target');
    if (id && target) relationshipTargets.set(id, target);
  });

  const sheets = await Promise.all(
    getElementsByLocalName(workbookDoc, 'sheet').map(async sheetNode => {
      const name = sheetNode.getAttribute('name') || 'Sheet';
      const relationshipId =
        sheetNode.getAttribute('r:id') ||
        sheetNode.getAttributeNS(
          'http://schemas.openxmlformats.org/officeDocument/2006/relationships',
          'id'
        );
      const target = relationshipId ? relationshipTargets.get(relationshipId) : null;
      if (!target) return null;

      const worksheetXml = await readZipText(zip, resolveWorkbookTarget(target), limitErrorText);
      if (!worksheetXml) return null;
      return readWorksheetPreview(name, worksheetXml, sharedStrings);
    })
  );

  return sheets.filter((sheet): sheet is SpreadsheetSheetPreview => sheet !== null);
}

async function readSharedStrings(zip: JSZip, limitErrorText: string): Promise<string[]> {
  const sharedStringsXml = await readZipText(zip, 'xl/sharedStrings.xml', limitErrorText);
  if (!sharedStringsXml) return [];

  const sharedStringsDoc = parseXml(sharedStringsXml);
  return getElementsByLocalName(sharedStringsDoc, 'si').map(item =>
    getElementsByLocalName(item, 't')
      .map(text => text.textContent ?? '')
      .join('')
  );
}

function readWorksheetPreview(
  name: string,
  worksheetXml: string,
  sharedStrings: string[]
): SpreadsheetSheetPreview {
  const worksheetDoc = parseXml(worksheetXml);
  const rows: SpreadsheetRowPreview[] = [];
  const rowNodes = getElementsByLocalName(worksheetDoc, 'row');
  let columnCount = Math.min(readDimensionColumnCount(worksheetDoc), maxSpreadsheetPreviewColumns);

  rowNodes.forEach((rowNode, rowIndex) => {
    const cells = Array.from({ length: maxSpreadsheetPreviewColumns }, () => '');
    const cellNodes = getDirectChildrenByLocalName(rowNode, 'c');
    const rowNumber = Number(rowNode.getAttribute('r')) || rowIndex + 1;

    cellNodes.forEach((cellNode, cellIndex) => {
      const columnIndex = readColumnIndex(cellNode.getAttribute('r')) || cellIndex + 1;
      if (columnIndex > maxSpreadsheetPreviewColumns) return;

      columnCount = Math.min(Math.max(columnCount, columnIndex), maxSpreadsheetPreviewColumns);
      cells[columnIndex - 1] = readCellValue(cellNode, sharedStrings);
    });

    if (rows.length < maxSpreadsheetPreviewRows) {
      rows.push({ number: rowNumber, cells });
    }
  });

  return {
    name,
    rows,
    columnCount,
    totalRowCount: rowNodes.length,
  };
}

function readCellValue(cellNode: Element, sharedStrings: string[]): string {
  const type = cellNode.getAttribute('t');
  if (type === 'inlineStr') {
    return getElementsByLocalName(cellNode, 't')
      .map(text => text.textContent ?? '')
      .join('');
  }

  const value = getDirectChildrenByLocalName(cellNode, 'v')[0]?.textContent ?? '';
  if (!value) {
    const formula = getDirectChildrenByLocalName(cellNode, 'f')[0]?.textContent;
    return formula ? `=${formula}` : '';
  }

  if (type === 's') return sharedStrings[Number(value)] ?? '';
  if (type === 'b') return value === '1' ? 'TRUE' : 'FALSE';
  return value;
}

function getSpreadsheetColumnLabel(index: number): string {
  let column = '';
  let current = index;
  while (current > 0) {
    const remainder = (current - 1) % 26;
    column = String.fromCharCode(65 + remainder) + column;
    current = Math.floor((current - 1) / 26);
  }
  return column;
}

async function loadOfficeZipPreview(
  buffer: ArrayBuffer,
  JSZipCtor: typeof JSZip,
  limitErrorText: string
): Promise<JSZip> {
  const zip = await JSZipCtor.loadAsync(buffer);
  validateOfficeZipPreviewBudget(zip, limitErrorText);
  return zip;
}

function validateOfficeZipPreviewBudget(zip: JSZip, limitErrorText: string) {
  const entries = Object.values(zip.files).filter(entry => !entry.dir);
  if (entries.length > maxOfficeZipEntries) throw new Error(limitErrorText);

  let totalUncompressedSize = 0;
  entries.forEach(entry => {
    const size = getZipEntryUncompressedSize(entry, limitErrorText);
    if (size > maxOfficeZipEntryBytes) throw new Error(limitErrorText);
    totalUncompressedSize += size;
  });
  if (totalUncompressedSize > maxOfficeZipUncompressedBytes) throw new Error(limitErrorText);
}

async function readZipText(
  zip: JSZip,
  path: string,
  limitErrorText: string
): Promise<string | null> {
  const entry = zip.file(path);
  if (!entry) return null;

  const size = getZipEntryUncompressedSize(entry, limitErrorText);
  if (size > maxOfficeXmlEntryBytes) throw new Error(limitErrorText);

  return entry.async('text');
}

function getZipEntryUncompressedSize(entry: JSZip.JSZipObject, limitErrorText: string): number {
  const maybeEntry = entry as JSZip.JSZipObject & {
    _data?: { uncompressedSize?: number };
  };
  const size = maybeEntry._data?.uncompressedSize;
  if (typeof size !== 'number' || !Number.isFinite(size)) {
    throw new Error(limitErrorText);
  }
  return size;
}

function parseXml(xml: string): XMLDocument {
  return new DOMParser().parseFromString(xml, 'application/xml');
}

function getElementsByLocalName(root: ParentNode, localName: string): Element[] {
  return Array.from(root.querySelectorAll('*')).filter(element => element.localName === localName);
}

function getDirectChildrenByLocalName(element: Element, localName: string): Element[] {
  return Array.from(element.children).filter(child => child.localName === localName);
}

function resolveWorkbookTarget(target: string): string {
  const path = target.startsWith('/') ? target.slice(1) : `xl/${target}`;
  const normalizedSegments: string[] = [];
  path
    .replace(/\\/g, '/')
    .split('/')
    .forEach(segment => {
      if (!segment || segment === '.') return;
      if (segment === '..') {
        normalizedSegments.pop();
        return;
      }
      normalizedSegments.push(segment);
    });
  return normalizedSegments.join('/');
}

function readDimensionColumnCount(worksheetDoc: XMLDocument): number {
  const ref = getElementsByLocalName(worksheetDoc, 'dimension')[0]?.getAttribute('ref');
  if (!ref) return 0;
  return readColumnIndex(ref.split(':').pop() ?? '') || 0;
}

function readColumnIndex(cellRef: string | null): number {
  const columnRef = cellRef?.match(/[A-Z]+/i)?.[0].toUpperCase();
  if (!columnRef) return 0;

  return columnRef.split('').reduce((index, char) => index * 26 + char.charCodeAt(0) - 64, 0);
}

interface PreviewMessageProps {
  icon: ReactNode;
  title: string;
  description?: string;
}

function PreviewMessage({ icon, title, description }: PreviewMessageProps) {
  return (
    <div className="flex h-full min-h-[360px] items-center justify-center p-6 text-center">
      <div className="max-w-sm">
        <div className="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          {icon}
        </div>
        <div className="text-sm font-medium text-foreground">{title}</div>
        {description ? (
          <div className="mt-2 text-sm leading-6 text-muted-foreground">{description}</div>
        ) : null}
      </div>
    </div>
  );
}
