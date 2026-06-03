'use client';

import {
  DocumentPagePreview as SharedDocumentPagePreview,
  type DocumentPreviewPage,
} from '@/components/document-preview/document-page-preview';
import type { ParsedElement } from '@/services/types/content-parse';
import { useT } from '@/i18n';

interface DocumentPagePreviewProps {
  page: DocumentPreviewPage;
  elements: ParsedElement[];
  selectedElementId?: string;
  onSelectElement: (element: ParsedElement) => void;
}

export type { DocumentPreviewPage };

export function DocumentPagePreview({
  page,
  elements,
  selectedElementId,
  onSelectElement,
}: DocumentPagePreviewProps) {
  const t = useT('contentParse');
  const pageLabel = page.label || t('preview.page', { page: page.pageIndex + 1 });

  return (
    <SharedDocumentPagePreview
      page={page}
      elements={elements}
      selectedElementId={selectedElementId}
      onSelectElement={onSelectElement}
      pageLabel={pageLabel}
      boxesLabel={t('preview.boxes', { count: elements.filter(element => element.bbox).length })}
      formatElementType={type => formatPreviewElementType(type, t as ContentParseTranslator)}
    />
  );
}

type ContentParseTranslator = (key: string, values?: Record<string, unknown>) => string;

function formatPreviewElementType(type: string | undefined, t: ContentParseTranslator): string {
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
