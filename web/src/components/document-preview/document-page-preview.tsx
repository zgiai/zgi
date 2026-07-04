'use client';

import { cn } from '@/lib/utils';

export interface DocumentPreviewPage {
  pageIndex: number;
  imageUrl?: string;
  aspectRatio: number;
  label?: string;
}

export interface DocumentPreviewBoundingBox {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export interface DocumentPreviewElement {
  id?: string;
  type: string;
  subtype?: string;
  page: number;
  content?: string;
  bbox?: DocumentPreviewBoundingBox;
  ordinal: number;
  precision?: string;
  confidence?: number;
  metadata?: Record<string, unknown>;
}

interface DocumentPagePreviewProps<ElementType extends DocumentPreviewElement> {
  page: DocumentPreviewPage;
  elements: ElementType[];
  selectedElementId?: string;
  onSelectElement: (element: ElementType) => void;
  pageLabel: string;
  boxesLabel?: string;
  formatElementType: (type: string | undefined) => string;
  className?: string;
}

const elementTone: Record<string, string> = {
  title: 'border-amber-400 bg-amber-300/20 text-amber-900',
  heading: 'border-amber-400 bg-amber-300/20 text-amber-900',
  text: 'border-sky-400 bg-sky-300/15 text-sky-900',
  paragraph: 'border-sky-400 bg-sky-300/15 text-sky-900',
  table: 'border-emerald-400 bg-emerald-300/15 text-emerald-900',
  figure: 'border-fuchsia-400 bg-fuchsia-300/15 text-fuchsia-900',
  image: 'border-fuchsia-400 bg-fuchsia-300/15 text-fuchsia-900',
  formula: 'border-rose-400 bg-rose-300/15 text-rose-900',
};

export function DocumentPagePreview<ElementType extends DocumentPreviewElement>({
  page,
  elements,
  selectedElementId,
  onSelectElement,
  pageLabel,
  boxesLabel,
  formatElementType,
  className,
}: DocumentPagePreviewProps<ElementType>) {
  const boxes: Array<{ element: ElementType; box: DocumentPreviewBoundingBox }> = [];
  elements.forEach(element => {
    if (!element.bbox || element.precision === 'unreliable') return;
    const box = normalizeBox(element.bbox);
    if (!box) return;
    boxes.push({ element, box });
  });
  boxes.sort((a, b) => boxArea(b.box) - boxArea(a.box));

  return (
    <section className={cn('mx-auto w-full max-w-5xl', className)}>
      <div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
        <span>{pageLabel}</span>
        {boxesLabel ? <span>{boxesLabel}</span> : null}
      </div>
      <div
        className="relative overflow-hidden rounded-xl border border-border bg-background shadow-sm"
        style={{ aspectRatio: page.aspectRatio }}
      >
        {page.imageUrl ? (
          <img
            src={page.imageUrl}
            alt={pageLabel}
            className="absolute inset-0 h-full w-full object-contain"
          />
        ) : (
          <div className="absolute inset-0 bg-[linear-gradient(0deg,hsl(var(--border)/0.45)_1px,transparent_1px),linear-gradient(90deg,hsl(var(--border)/0.45)_1px,transparent_1px)] bg-[size:24px_24px]" />
        )}

        <div className="absolute inset-0">
          {boxes.map(({ element, box }) => {
            const key = documentPreviewElementKey(element);
            const isSelected = key === selectedElementId;
            const tone =
              elementTone[element.type] || 'border-indigo-400 bg-indigo-300/15 text-indigo-900';
            return (
              <button
                type="button"
                key={key}
                title={element.content || element.type}
                onClick={() => onSelectElement(element)}
                className={cn(
                  'absolute rounded-[4px] border text-[10px] font-semibold leading-none transition-all',
                  'hover:z-30 hover:bg-background/70 hover:shadow-[0_0_0_2px_hsl(var(--border))]',
                  tone,
                  isSelected && 'z-40 border-2 border-primary bg-background/80 shadow-sm'
                )}
                style={{
                  left: `${box.left * 100}%`,
                  top: `${box.top * 100}%`,
                  width: `${Math.max((box.right - box.left) * 100, 0.6)}%`,
                  height: `${Math.max((box.bottom - box.top) * 100, 0.6)}%`,
                }}
              >
                {isSelected ? (
                  <span className="absolute -left-px -top-5 rounded bg-slate-950 px-1.5 py-0.5 text-[10px] text-white">
                    {formatElementType(element.type)}
                  </span>
                ) : null}
              </button>
            );
          })}
        </div>
      </div>
    </section>
  );
}

export function documentPreviewElementKey(element: DocumentPreviewElement): string {
  return element.id || `${element.page}-${element.ordinal}-${element.type}`;
}

function normalizeBox(box: DocumentPreviewBoundingBox): DocumentPreviewBoundingBox | null {
  const scale = box.right > 1 || box.bottom > 1 || box.left > 1 || box.top > 1 ? 100 : 1;
  const normalized = {
    left: clamp(box.left / scale),
    top: clamp(box.top / scale),
    right: clamp(box.right / scale),
    bottom: clamp(box.bottom / scale),
  };
  if (normalized.right <= normalized.left || normalized.bottom <= normalized.top) {
    return null;
  }
  return normalized;
}

function clamp(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(1, Math.max(0, value));
}

function boxArea(box: DocumentPreviewBoundingBox): number {
  return Math.max(box.right - box.left, 0) * Math.max(box.bottom - box.top, 0);
}
