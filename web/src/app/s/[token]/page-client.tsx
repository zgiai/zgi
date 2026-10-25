'use client';

import { useQuery } from '@tanstack/react-query';
import { Download, FileText, Loader2 } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { artifactShareService } from '@/services/artifact-share.service';

function formatSize(bytes: number) {
  if (!bytes) return '';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

const inlineImages = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];

/** A shared artifact, for anyone holding the link. */
export function ArtifactSharePageClient({ token }: { token: string }) {
  const t = useT('digitalHumans');
  const share = useQuery({
    queryKey: ['artifact-share', token],
    queryFn: () => artifactShareService.getShare(token),
    // Queries wait for sign-in to settle by default; a share needs none.
    enabled: Boolean(token),
    retry: false,
  });
  const data = share.data;
  const mime = data?.mime_type.toLowerCase() ?? '';
  const name = data?.file_name || (data ? `${data.title}.md` : '');

  return (
    <main className="min-h-screen bg-background px-4 py-6 text-foreground sm:px-6 sm:py-10">
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-4">
        {share.isPending ? (
          <div className="flex min-h-[320px] items-center justify-center rounded-lg border">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : share.isError || !data ? (
          <div className="rounded-lg border p-6 text-center">
            <h1 className="text-lg font-semibold">{t('artifacts.share.page.unavailable')}</h1>
            <p className="mt-2 text-sm text-muted-foreground">{t('artifacts.share.page.unavailableDescription')}</p>
          </div>
        ) : (
          <>
            <header className="flex flex-wrap items-start gap-3">
              <div className="min-w-0 flex-1">
                <h1 className="break-words text-xl font-semibold [overflow-wrap:anywhere] sm:text-2xl">{data.title}</h1>
                <p className="mt-1 text-xs text-muted-foreground">
                  {[`v${data.version}`, formatSize(data.size), data.expires_at
                    ? t('artifacts.share.page.expires', { time: new Date(data.expires_at).toLocaleString() })
                    : ''].filter(Boolean).join(' · ')}
                </p>
              </div>
              <Button asChild variant="outline" size="sm">
                <a href={artifactShareService.contentUrl(token, true)} download={name} rel="noreferrer">
                  <Download className="size-4" />
                  {t('artifacts.download')}
                </a>
              </Button>
            </header>
            {data.kind === 'document' && data.content !== undefined ? (
              <article className="rounded-lg border bg-card px-5 py-4 text-sm">
                <MarkdownViewer content={data.content} allowRawHtml={false} className="md-viewer max-w-full break-words" />
              </article>
            ) : mime === 'application/pdf' ? (
              <iframe title={data.title} src={artifactShareService.contentUrl(token)} className="h-[80vh] w-full rounded-lg border" />
            ) : inlineImages.includes(mime) ? (
              // A file from the share's own content endpoint.
              <img src={artifactShareService.contentUrl(token)} alt={data.title} className="max-h-[80vh] rounded-lg border object-contain" />
            ) : (
              <div className="flex items-center gap-3 rounded-lg border px-4 py-6">
                <FileText className="size-8 shrink-0 text-muted-foreground" />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{name}</p>
                  <p className="text-xs text-muted-foreground">{t('artifacts.share.page.noPreview')}</p>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </main>
  );
}
