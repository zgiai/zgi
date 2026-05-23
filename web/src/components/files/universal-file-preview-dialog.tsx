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
import {
  getOriginalPreviewKind,
  isOriginalPreviewImage,
  isOriginalPreviewPdf,
  isOriginalPreviewSupported,
} from '@/utils/file-helpers';
import { cn } from '@/lib/utils';

const maxOfficePreviewBytes = 10 * 1024 * 1024;
const maxSpreadsheetPreviewRows = 200;
const maxSpreadsheetPreviewColumns = 100;

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
  const resolvedPreviewUrl = previewUrl || file?.previewUrl || '';
  const downloadUrl = file?.downloadUrl || resolvedPreviewUrl;
  const isSupported = isOriginalPreviewSupported(file?.extension, file?.mimeType);
  const extension = file?.extension?.replace(/^\./, '').toUpperCase() || '';
  const title = file?.name || t('preview.title');
  const isImage = isOriginalPreviewImage(file?.extension, file?.mimeType);
  const isPdf = isOriginalPreviewPdf(file?.extension, file?.mimeType);
  const previewKind = getOriginalPreviewKind(file?.extension, file?.mimeType);

  const renderPreview = () => {
    if (!file) {
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
          title={error || t('preview.loadError')}
        />
      );
    }

    if (isImage) {
      return (
        <div className="flex h-full min-h-0 items-center justify-center overflow-auto bg-muted/30 p-4">
          <img
            src={resolvedPreviewUrl}
            alt={file.name}
            className="max-h-full max-w-full object-contain"
          />
        </div>
      );
    }

    if (previewKind === 'office') {
      return <OfficePreview file={file} previewUrl={resolvedPreviewUrl} />;
    }

    if (isPdf || previewKind === 'browser' || previewKind === 'html') {
      return (
        <iframe
          src={resolvedPreviewUrl}
          title={file.name}
          sandbox={
            previewKind === 'html'
              ? 'allow-downloads allow-forms allow-popups allow-scripts'
              : undefined
          }
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
          {resolvedPreviewUrl ? (
            <Button variant="outline" asChild>
              <a href={resolvedPreviewUrl} target="_blank" rel="noreferrer">
                <ExternalLink className="mr-2 h-4 w-4" />
                {t('preview.openInNewTab')}
              </a>
            </Button>
          ) : null}
          {downloadUrl ? (
            <Button variant="outline" asChild>
              <a href={downloadUrl} download={file?.name}>
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

function DocxPreview({ previewUrl }: { previewUrl: string }) {
  const t = useT('files');
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let cancelled = false;
    setIsLoading(true);
    setError(null);
    container.innerHTML = '';

    const render = async () => {
      try {
        const [response, docxPreview] = await Promise.all([
          fetch(previewUrl, { credentials: 'include' }),
          import('docx-preview'),
        ]);
        if (!response.ok) throw new Error(t('preview.loadError'));

        const blob = await response.blob();
        if (cancelled) return;

        await docxPreview.renderAsync(blob, container, undefined, {
          className: 'zgi-docx-preview',
          inWrapper: false,
          ignoreWidth: false,
          ignoreHeight: false,
        });
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : t('preview.loadError'));
        }
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };

    void render();
    return () => {
      cancelled = true;
    };
  }, [previewUrl, t]);

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
    <div className="relative h-full min-h-[60vh] overflow-auto bg-muted/30 p-4">
      {isLoading ? (
        <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/70">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span>{t('preview.loading')}</span>
          </div>
        </div>
      ) : null}
      <div
        ref={containerRef}
        className="mx-auto w-fit max-w-full overflow-auto rounded-md bg-background shadow-sm [&_.zgi-docx-preview-wrapper]:bg-transparent"
      />
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

  useEffect(() => {
    let cancelled = false;
    setIsLoading(true);
    setError(null);

    const load = async () => {
      try {
        const [response, jszip] = await Promise.all([
          fetch(previewUrl, { credentials: 'include' }),
          import('jszip'),
        ]);
        if (!response.ok) throw new Error(t('preview.loadError'));

        const buffer = await response.arrayBuffer();
        const sheets = await readXlsxWorkbookPreview(buffer, jszip.default);
        const firstSheet = sheets[0];
        if (!cancelled) {
          if (!firstSheet) throw new Error(t('preview.emptyWorkbook'));

          setActiveSheet(firstSheet.name);
          setState({
            sheets,
            activeSheet: firstSheet.name,
          });
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : t('preview.loadError'));
        }
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [previewUrl, t]);

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

async function readXlsxWorkbookPreview(
  buffer: ArrayBuffer,
  JSZipCtor: typeof JSZip
): Promise<SpreadsheetSheetPreview[]> {
  const zip = await JSZipCtor.loadAsync(buffer);
  const workbookXml = await readZipText(zip, 'xl/workbook.xml');
  const workbookRelsXml = await readZipText(zip, 'xl/_rels/workbook.xml.rels');
  const sharedStrings = await readSharedStrings(zip);

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

      const worksheetXml = await readZipText(zip, resolveWorkbookTarget(target));
      if (!worksheetXml) return null;
      return readWorksheetPreview(name, worksheetXml, sharedStrings);
    })
  );

  return sheets.filter((sheet): sheet is SpreadsheetSheetPreview => sheet !== null);
}

async function readSharedStrings(zip: JSZip): Promise<string[]> {
  const sharedStringsXml = await readZipText(zip, 'xl/sharedStrings.xml');
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

      columnCount = Math.min(
        Math.max(columnCount, columnIndex),
        maxSpreadsheetPreviewColumns
      );
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

async function readZipText(zip: JSZip, path: string): Promise<string | null> {
  return (await zip.file(path)?.async('text')) ?? null;
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
  path.replace(/\\/g, '/')
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
