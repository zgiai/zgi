'use client';

import { useQuery } from '@tanstack/react-query';
import { fileManageService } from '@/services/file-manage.service';
import type { ApiResponseData } from '@/services/types/common';
import type { FileSourcePreviewPagesResponse } from '@/services/types/file';
import type { DocumentPreviewPage } from '@/components/document-preview/document-page-preview';
import type { PDFDocumentProxy } from 'pdfjs-dist/types/src/display/api';

export const FILE_SOURCE_PREVIEW_PAGES_QUERY_KEY = 'file-source-preview-pages';

interface PDFJSModule {
  getDocument: (source: { data: Uint8Array }) => { promise: Promise<PDFDocumentProxy> };
  GlobalWorkerOptions: {
    workerSrc?: string;
  };
}

let pdfjsModulePromise: Promise<PDFJSModule> | null = null;

export const getFileSourcePreviewPagesKey = (fileId: string, maxPages: number) => [
  FILE_SOURCE_PREVIEW_PAGES_QUERY_KEY,
  fileId,
  maxPages,
];

export interface FileSourcePreviewPagesView extends FileSourcePreviewPagesResponse {
  preview_pages: DocumentPreviewPage[];
}

export function useFileSourcePreviewPages(
  fileId: string,
  options: { enabled?: boolean; maxPages?: number } = {}
) {
  const { enabled = true, maxPages = 20 } = options;

  return useQuery<ApiResponseData<FileSourcePreviewPagesView>>({
    queryKey: getFileSourcePreviewPagesKey(fileId, maxPages),
    enabled: enabled && Boolean(fileId),
    retry: false,
    queryFn: async () => {
      try {
        const response = await fileManageService.getSourcePreviewPages(fileId, maxPages);
        const previewPages = await dataUrlsToPreviewPages(response.data.pages);

        return {
          ...response,
          data: {
            ...response.data,
            preview_pages: previewPages,
          },
        };
      } catch (error) {
        const fallback = await renderOriginalPDFPreview(fileId, maxPages).catch(() => null);
        if (fallback) {
          return fallback;
        }
        throw error;
      }
    },
  });
}

async function dataUrlsToPreviewPages(pages: string[]): Promise<DocumentPreviewPage[]> {
  return Promise.all(
    pages
      .filter(page => page.startsWith('data:image/'))
      .map(async (imageUrl, index) => {
        const dimensions = await readImageDimensions(imageUrl);
        return {
          pageIndex: index,
          imageUrl,
          aspectRatio: dimensions.width / dimensions.height,
        };
      })
  );
}

async function renderOriginalPDFPreview(
  fileId: string,
  maxPages: number
): Promise<ApiResponseData<FileSourcePreviewPagesView> | null> {
  const blob = await fileManageService.downloadFile(fileId);
  const bytes = new Uint8Array(await blob.arrayBuffer());
  if (!isPDFBytes(bytes)) {
    return null;
  }

  const pages = await renderPDFBytesToPreviewPages(bytes, maxPages);
  if (pages.length === 0) {
    return null;
  }

  return {
    code: '0',
    message: 'success',
    data: {
      engine: 'browser_pdfjs_fallback',
      page_count: pages.length,
      pages: pages.map(page => page.imageUrl || '').filter(Boolean),
      preview_pages: pages,
    },
  };
}

async function renderPDFBytesToPreviewPages(
  bytes: Uint8Array,
  maxPages: number
): Promise<DocumentPreviewPage[]> {
  const pdfjs = await loadPDFJSModule();
  const loadingTask = pdfjs.getDocument({ data: bytes });
  const pdf = await loadingTask.promise;
  const pageLimit = Math.min(pdf.numPages, Math.max(maxPages, 1));
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

function isPDFBytes(bytes: Uint8Array): boolean {
  return (
    bytes.length >= 4 &&
    bytes[0] === 0x25 &&
    bytes[1] === 0x50 &&
    bytes[2] === 0x44 &&
    bytes[3] === 0x46
  );
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
