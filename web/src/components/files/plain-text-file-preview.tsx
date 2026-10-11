'use client';

import { useEffect, useState } from 'react';
import { AlertCircle, Loader2 } from 'lucide-react';
import { useT } from '@/i18n';

export const maxTextPreviewBytes = 1024 * 1024;

// Decode bounded bytes as text, never as an iframe document or HTML markup.
export async function readBoundedTextPreview(response: Response): Promise<string> {
  if (!response.ok) throw new Error('text preview request failed');
  if (Number(response.headers.get('content-length')) > maxTextPreviewBytes) {
    await response.body?.cancel();
    throw new RangeError('text preview exceeds byte limit');
  }
  if (!response.body) return '';

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let received = 0;
  let text = '';
  try {
    for (let chunk = await reader.read(); !chunk.done; chunk = await reader.read()) {
      const { value } = chunk;
      received += value.byteLength;
      if (received > maxTextPreviewBytes) {
        await reader.cancel();
        throw new RangeError('text preview exceeds byte limit');
      }
      text += decoder.decode(value, { stream: true });
    }
    return text + decoder.decode();
  } finally {
    reader.releaseLock();
  }
}

export function PlainTextFilePreview({ previewUrl }: { previewUrl: string }) {
  const t = useT('files');
  const [result, setResult] = useState<{
    url: string;
    text: string;
    error: 'large' | 'load' | null;
  } | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setResult(null);
    const load = async () => {
      try {
        const response = await fetch(previewUrl, {
          credentials: 'include',
          signal: controller.signal,
        });
        const text = await readBoundedTextPreview(response);
        if (!controller.signal.aborted) setResult({ url: previewUrl, text, error: null });
      } catch (error) {
        if (!controller.signal.aborted) {
          setResult({
            url: previewUrl,
            text: '',
            error: error instanceof RangeError ? 'large' : 'load',
          });
        }
      }
    };
    void load();
    return () => controller.abort();
  }, [previewUrl]);

  if (!result || result.url !== previewUrl) {
    return (
      <div role="status" className="flex min-h-[360px] items-center justify-center gap-2">
        <Loader2 className="size-5 animate-spin" />
        {t('preview.loading')}
      </div>
    );
  }
  if (result.error) {
    return (
      <div
        role="alert"
        className="flex min-h-[360px] flex-col items-center justify-center gap-2 p-6"
      >
        <AlertCircle className="size-5" />
        <div>{t(result.error === 'large' ? 'preview.textTooLargeTitle' : 'preview.loadError')}</div>
        <div className="text-sm text-muted-foreground">{t('preview.textFallback')}</div>
      </div>
    );
  }
  return (
    <pre className="h-full min-h-[60vh] overflow-auto whitespace-pre-wrap break-words bg-background p-4 text-sm">
      {result.text}
    </pre>
  );
}
