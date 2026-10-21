'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { Download, FileImage, FileSpreadsheet, FileText, Files, Package, Search, Users } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { ProtectedGeneratedFileCard } from '@/components/chat/variants/aichat/protected-generated-file-card';
import { useOpenTaskReport } from '@/components/digital-human/task-report-drawer';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { useLocale } from '@/hooks/use-locale';
import { useMediaQuery } from '@/hooks/use-media-query';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { digitalHumanService } from '@/services/digital-human.service';
import type {
  DigitalHumanArtifact,
  DigitalHumanArtifactKind,
  DigitalHumanArtifactVersion,
} from '@/services/types/digital-human';

const kinds: DigitalHumanArtifactKind[] = ['document', 'file', 'image', 'table'];
const kindIcon = { document: FileText, file: Files, image: FileImage, table: FileSpreadsheet } as const;

function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = name;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/**
 * The digital human's artifact center (ADR 0009): what it kept across
 * conversations, each with its versions and where it came from.
 */
export function DigitalHumanArtifacts({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const selected = searchParams.get('selected');
  const wide = useMediaQuery('(min-width: 1024px)');
  const [search, setSearch] = useState('');
  const [mine, setMine] = useState(false);
  const [kind, setKind] = useState<DigitalHumanArtifactKind | null>(null);
  const query = useQuery({
    queryKey: ['digital-human-artifacts', id, search.trim(), mine, kind],
    queryFn: () => digitalHumanService.listArtifacts(id, { q: search.trim() || undefined, scope: mine ? 'mine' : undefined, kind: kind ?? undefined, page_size: 100 }),
  });
  const items = query.data?.data.items ?? [];

  const select = (artifactId: string | null) => {
    const params = new URLSearchParams(searchParams.toString());
    if (artifactId) params.set('selected', artifactId);
    else params.delete('selected');
    const next = params.toString();
    router.replace(next ? `${pathname}?${next}` : pathname, { scroll: false });
  };

  return (
    <main className="mx-auto flex min-h-full w-full max-w-[1600px] flex-col gap-5 px-4 py-6 md:px-8 md:py-8">
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <Package className="size-5 text-primary" />
          <h1 className="text-2xl font-semibold tracking-tight">{t('artifacts.title')}</h1>
        </div>
        <p className="max-w-3xl text-sm text-muted-foreground">{t('artifacts.description')}</p>
      </div>
      <div className="flex flex-col gap-3 xl:flex-row xl:items-center">
        <Input value={search} onChange={event => setSearch(event.target.value)} placeholder={t('artifacts.search')} leftIcon={<Search />} className="h-9 xl:max-w-sm" />
        <div className="flex flex-wrap gap-2">
          {[false, true].map(value => (
            <Button key={String(value)} size="sm" variant={mine === value ? 'default' : 'outline'} className="h-8 rounded-full" onClick={() => setMine(value)}>
              {value ? t('artifacts.scope.mine') : t('artifacts.scope.all')}
            </Button>
          ))}
          <span className="mx-1 w-px self-stretch bg-border" />
          {kinds.map(item => (
            <Button key={item} size="sm" variant={kind === item ? 'default' : 'outline'} className="h-8 rounded-full" onClick={() => setKind(kind === item ? null : item)}>
              {t(`artifacts.kind.${item}`)}
            </Button>
          ))}
        </div>
      </div>
      <div className="grid items-start gap-4 lg:grid-cols-[minmax(320px,420px)_minmax(0,1fr)]">
        {query.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-lg border border-dashed p-8 text-center">
            <Package className="mx-auto size-7 text-muted-foreground/50" />
            <p className="mt-2 text-sm font-medium">{t('artifacts.empty')}</p>
            <p className="mt-1 text-xs text-muted-foreground">{t('artifacts.emptyDescription')}</p>
          </div>
        ) : (
          <ul className="divide-y rounded-lg border">
            {items.map(item => {
              const Icon = kindIcon[item.kind];
              return (
                <li key={item.id}>
                  <button
                    type="button"
                    aria-current={selected === item.id ? 'true' : undefined}
                    className={cn('flex w-full gap-3 px-4 py-3 text-left hover:bg-muted/40', selected === item.id && 'bg-primary/5')}
                    onClick={() => select(item.id)}
                  >
                    <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">{item.title}</span>
                      <span className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                        <span>v{item.current_version}</span>
                        <span>{new Date(item.updated_at).toLocaleString(locale)}</span>
                        {item.visibility === 'team' ? (
                          <span className="inline-flex items-center gap-0.5">
                            <Users className="size-3" />
                            {t('artifacts.visibility.team')}
                          </span>
                        ) : null}
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
        {wide ? (
          <section className="sticky top-4 max-h-[calc(100vh-12rem)] overflow-y-auto rounded-xl border bg-background p-4">
            {selected ? (
              <ArtifactDetail key={selected} digitalHumanId={id} artifactId={selected} />
            ) : (
              <div className="flex min-h-60 flex-col items-center justify-center gap-2 text-center">
                <Package className="size-7 text-muted-foreground/50" />
                <p className="text-sm text-muted-foreground">{t('artifacts.selectHint')}</p>
              </div>
            )}
          </section>
        ) : (
          <Sheet open={Boolean(selected)} onOpenChange={open => !open && select(null)}>
            <SheetContent className="w-full overflow-y-auto p-4 sm:max-w-xl">
              <SheetTitle className="sr-only">{t('artifacts.title')}</SheetTitle>
              {selected ? <ArtifactDetail key={selected} digitalHumanId={id} artifactId={selected} /> : null}
            </SheetContent>
          </Sheet>
        )}
      </div>
    </main>
  );
}

function ArtifactDetail({ digitalHumanId, artifactId }: { digitalHumanId: string; artifactId: string }) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const openReport = useOpenTaskReport();
  const artifact = useQuery({
    queryKey: ['digital-human-artifact', digitalHumanId, artifactId],
    queryFn: () => digitalHumanService.getArtifact(digitalHumanId, artifactId),
  });
  const versions = useQuery({
    queryKey: ['digital-human-artifact-versions', digitalHumanId, artifactId],
    queryFn: () => digitalHumanService.listArtifactVersions(digitalHumanId, artifactId),
  });
  const [number, setNumber] = useState<number | null>(null);
  const data = artifact.data?.data;
  const items = versions.data?.data.items ?? [];
  const version = items.find(item => item.number === (number ?? data?.current_version)) ?? data?.latest;

  if (artifact.isLoading) return <Skeleton className="h-40 w-full" />;
  if (!data || !version) return <p className="text-sm text-muted-foreground">{t('artifacts.loadError')}</p>;
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <h2 className="text-base font-semibold">{data.title}</h2>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="outline">{t(`artifacts.kind.${data.kind}`)}</Badge>
          <Badge variant="outline">{t(`artifacts.visibility.${data.visibility}`)}</Badge>
          <span>{new Date(version.created_at).toLocaleString(locale)}</span>
          {data.source_task_id ? (
            <Link href={`/console/digital-humans/${digitalHumanId}/tasks?task=${data.source_task_id}`} className="text-primary hover:underline">
              {t('artifacts.openTask')}
            </Link>
          ) : null}
          {version.source_report_id ? (
            <button type="button" className="text-primary hover:underline" onClick={() => openReport(version.source_report_id ?? '')}>
              {t('artifacts.openReport')}
            </button>
          ) : null}
        </div>
      </div>
      {items.length > 1 ? (
        <div className="flex flex-wrap gap-1.5">
          {items.map(item => (
            <Button
              key={item.id}
              size="sm"
              variant={item.number === version.number ? 'default' : 'outline'}
              className="h-7 rounded-full px-2.5 text-xs"
              title={item.note || new Date(item.created_at).toLocaleString(locale)}
              onClick={() => setNumber(item.number)}
            >
              v{item.number}
            </Button>
          ))}
        </div>
      ) : null}
      {version.note ? <p className="text-xs text-muted-foreground">{version.note}</p> : null}
      <ArtifactPreview digitalHumanId={digitalHumanId} artifact={data} version={version} />
    </div>
  );
}

function ArtifactPreview({ digitalHumanId, artifact, version }: { digitalHumanId: string; artifact: DigitalHumanArtifact; version: DigitalHumanArtifactVersion }) {
  const t = useT('digitalHumans');
  const read = (signal?: AbortSignal) => digitalHumanService.readArtifactContent(digitalHumanId, artifact.id, version.number, signal);
  const inline = artifact.kind === 'document' || artifact.kind === 'image';
  const content = useQuery({
    queryKey: ['digital-human-artifact-content', digitalHumanId, artifact.id, version.number],
    queryFn: ({ signal }) => read(signal),
    enabled: inline,
    staleTime: Infinity,
  });
  const [text, setText] = useState<string | null>(null);
  const [imageUrl, setImageUrl] = useState<string | null>(null);
  useEffect(() => {
    const blob = content.data;
    if (!blob) return;
    if (artifact.kind === 'document') {
      void blob.text().then(setText);
      return;
    }
    const url = URL.createObjectURL(blob);
    setImageUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [artifact.kind, content.data]);

  if (!inline) {
    const name = version.file_name || artifact.title;
    return (
      <ProtectedGeneratedFileCard
        conversationId={artifact.id}
        readFile={(_, __, signal) => read(signal)}
        file={{
          file_id: version.id, filename: name, mime_type: version.mime_type, size: version.size, artifact_type: 'file', access_mode: 'authenticated',
          skill_id: '', tool_name: '', extension: name.includes('.') ? `.${name.split('.').pop()}` : '', url: '', transfer_method: 'tool_file', created_at: 0,
        }}
      />
    );
  }
  if (content.isLoading) return <Skeleton className="h-40 w-full" />;
  if (content.isError) return <p className="text-sm text-muted-foreground">{t('artifacts.loadError')}</p>;
  const download = () => content.data && saveBlob(content.data, version.file_name || `${artifact.title}.md`);
  return (
    <div className="space-y-2">
      <div className="flex justify-end">
        <Button size="sm" variant="ghost" onClick={download}>
          <Download className="size-4" />
          {t('artifacts.download')}
        </Button>
      </div>
      {artifact.kind === 'document' ? (
        <div className="rounded-md border bg-muted/20 p-4 text-sm">
          <MarkdownViewer content={text ?? ''} />
        </div>
      ) : imageUrl ? (
        // A blob URL from an authenticated read.
        <img src={imageUrl} alt={artifact.title} className="max-h-[60vh] rounded-md border object-contain" />
      ) : null}
    </div>
  );
}
