'use client';

import { useEffect, useState, type ReactNode } from 'react';
import Link from 'next/link';
import { useDigitalHumanObjectHref } from '@/components/digital-human/digital-human-object-link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { Archive, ArchiveRestore, Download, FileImage, FileSpreadsheet, FileText, Files, History, Package, Search, Share2, UserRound, Users } from 'lucide-react';
import { ArtifactShareDialog } from '@/components/digital-human/artifact-share-dialog';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { ProtectedGeneratedFileCard } from '@/components/chat/variants/aichat/protected-generated-file-card';
import { useOpenTaskReport } from '@/components/digital-human/task-report-drawer';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { useLocale } from '@/hooks/use-locale';
import { useMediaQuery } from '@/hooks/use-media-query';
import { useT } from '@/i18n';
import type { ScopedTranslations } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { formatTaskTime } from '@/components/digital-human/digital-human-task-presentation';
import { digitalHumanService } from '@/services/digital-human.service';
import { getErrorMessage } from '@/utils/error-notifications';
import type {
  DigitalHumanArtifact,
  DigitalHumanArtifactKind,
  DigitalHumanArtifactVersion,
} from '@/services/types/digital-human';

const kinds: DigitalHumanArtifactKind[] = ['document', 'file', 'image', 'table'];
const kindIcon = { document: FileText, file: Files, image: FileImage, table: FileSpreadsheet } as const;
const kindTone = {
  document: 'bg-primary/10 text-primary',
  file: 'bg-muted text-muted-foreground',
  image: 'bg-success/10 text-success',
  table: 'bg-warning/10 text-warning',
} as const;
// Text files read as documents, whatever kind they were kept as.
const textExtensions = ['md', 'markdown', 'txt', 'csv', 'json', 'log', 'yaml', 'yml'];
const maxTextPreviewBytes = 1024 * 1024;

function readsAsText(version: DigitalHumanArtifactVersion, title: string) {
  if (version.size > maxTextPreviewBytes) return false;
  const name = (version.file_name || title).toLowerCase();
  const extension = name.includes('.') ? name.split('.').pop() ?? '' : '';
  return version.mime_type?.startsWith('text/') || version.mime_type === 'application/json' || textExtensions.includes(extension);
}

/** Whom an artifact belongs to, in words: a member, an end user, or the team. */
export function artifactOwnerLabel(t: ScopedTranslations<'digitalHumans'>, artifact: DigitalHumanArtifact) {
  switch (artifact.owner_kind) {
    case 'workspace':
      return t('artifacts.owner.team');
    case 'application':
      return t('artifacts.owner.application');
    case 'api_end_user':
      return artifact.owner_name
        ? t('artifacts.owner.endUserNamed', { name: artifact.owner_name })
        : t('artifacts.owner.endUser', { id: artifact.owner_id.slice(0, 8) });
    default:
      return artifact.owner_name || t('artifacts.owner.member');
  }
}

function formatSize(bytes: number) {
  if (!bytes) return '';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function KindTile({ kind, size = 'md' }: { kind: DigitalHumanArtifactKind; size?: 'md' | 'lg' }) {
  const Icon = kindIcon[kind];
  return (
    <span className={cn('flex shrink-0 items-center justify-center rounded-lg', kindTone[kind], size === 'lg' ? 'size-10' : 'size-8')}>
      <Icon className={size === 'lg' ? 'size-5' : 'size-4'} />
    </span>
  );
}

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
  const [archived, setArchived] = useState(false);
  const query = useQuery({
    queryKey: ['digital-human-artifacts', id, search.trim(), mine, kind, archived],
    queryFn: () =>
      digitalHumanService.listArtifacts(id, {
        q: search.trim() || undefined,
        scope: mine ? 'mine' : undefined,
        kind: kind ?? undefined,
        archived: archived ? 'archived' : undefined,
        page_size: 100,
      }),
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
          <span className="mx-1 w-px self-stretch bg-border" />
          <Button size="sm" variant={archived ? 'default' : 'outline'} className="h-8 rounded-full" onClick={() => setArchived(!archived)}>
            <Archive className="size-3.5" />
            {t('artifacts.showArchived')}
          </Button>
        </div>
      </div>
      <div className="grid items-start gap-4 lg:grid-cols-[minmax(320px,420px)_minmax(0,1fr)]">
        {query.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        ) : items.length === 0 ? (
          <div className="rounded-xl border border-dashed p-8 text-center">
            <Package className="mx-auto size-7 text-muted-foreground/50" />
            <p className="mt-2 text-sm font-medium">{archived ? t('artifacts.emptyArchived') : t('artifacts.empty')}</p>
            {archived ? null : <p className="mt-1 text-xs text-muted-foreground">{t('artifacts.emptyDescription')}</p>}
          </div>
        ) : (
          <ul className="divide-y overflow-hidden rounded-xl border bg-background">
            {items.map(item => (
              <li key={item.id}>
                <button
                  type="button"
                  aria-current={selected === item.id ? 'true' : undefined}
                  className={cn(
                    'flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/40',
                    selected === item.id && 'bg-primary/5 hover:bg-primary/5'
                  )}
                  onClick={() => select(item.id)}
                >
                  <KindTile kind={item.kind} />
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                      <span className="min-w-0 flex-1 truncate text-sm font-medium">{item.title}</span>
                      <span className="shrink-0 text-xs tabular-nums text-muted-foreground" title={new Date(item.updated_at).toLocaleString(locale)}>
                        {formatTaskTime(item.updated_at, locale)}
                      </span>
                    </span>
                    {item.latest?.note ? <span className="mt-0.5 line-clamp-1 block text-xs text-muted-foreground">{item.latest.note}</span> : null}
                    <span className="mt-1 flex flex-wrap items-center gap-x-2 text-[11px] text-muted-foreground/80">
                      <span>{t(`artifacts.kind.${item.kind}`)}</span>
                      <span>·</span>
                      <span>{t('artifacts.versionCount', { count: item.current_version })}</span>
                      <span>·</span>
                      <span className="inline-flex min-w-0 items-center gap-0.5">
                        {item.owner_kind === 'workspace' ? <Users className="size-3 shrink-0" /> : <UserRound className="size-3 shrink-0" />}
                        <span className="truncate">{artifactOwnerLabel(t, item)}</span>
                      </span>
                      {item.visibility === 'team' && item.owner_kind !== 'workspace' ? (
                        <span className="inline-flex items-center gap-0.5">
                          <Users className="size-3" />
                          {t('artifacts.visibility.team')}
                        </span>
                      ) : null}
                      {item.archived_at ? (
                        <span className="inline-flex items-center gap-0.5">
                          <Archive className="size-3" />
                          {t('artifacts.archived')}
                        </span>
                      ) : null}
                    </span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
        {wide ? (
          <section className="sticky top-4 max-h-[calc(100vh-12rem)] overflow-y-auto rounded-xl border bg-background p-5">
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

export function ArtifactDetail({ digitalHumanId, artifactId }: { digitalHumanId: string; artifactId: string }) {
  const t = useT('digitalHumans');
  const objectHref = useDigitalHumanObjectHref();
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
  const [sharing, setSharing] = useState(false);
  const queryClient = useQueryClient();
  const archive = useMutation({
    mutationFn: (archived: boolean) => digitalHumanService.setArtifactArchived(digitalHumanId, artifactId, archived),
    onSuccess: (result, archived) => {
      queryClient.setQueryData(['digital-human-artifact', digitalHumanId, artifactId], result);
      void queryClient.invalidateQueries({ queryKey: ['digital-human-artifacts', digitalHumanId] });
      toast.success(archived ? t('artifacts.archiveDone') : t('artifacts.restoreDone'));
    },
    onError: error => toast.error(getErrorMessage(error) || t('artifacts.archiveError')),
  });
  const data = artifact.data?.data;
  const items = versions.data?.data.items ?? [];
  const version = items.find(item => item.number === (number ?? data?.current_version)) ?? data?.latest;

  if (artifact.isLoading) return <Skeleton className="h-40 w-full" />;
  if (!data || !version) return <p className="text-sm text-muted-foreground">{t('artifacts.loadError')}</p>;
  const viewingOld = version.number !== data.current_version;
  return (
    <div className="space-y-5">
      <div className="flex items-start gap-3">
        <KindTile kind={data.kind} size="lg" />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-lg font-semibold leading-7">{data.title}</h2>
          <p className="text-xs text-muted-foreground">
            {t('artifacts.versionLabel', { number: version.number })}
            {viewingOld ? ` · ${t('artifacts.oldVersion')}` : ''}
            {' · '}
            {formatTaskTime(version.created_at, locale)}
          </p>
        </div>
        {(data.can_share || data.can_manage) && !data.archived_at ? (
          <Button size="sm" variant="outline" className="shrink-0" onClick={() => setSharing(true)}>
            <Share2 className="size-4" />
            {t('artifacts.share.action')}
          </Button>
        ) : null}
        {data.can_manage ? (
          <Button size="sm" variant="outline" className="shrink-0" disabled={archive.isPending} onClick={() => archive.mutate(!data.archived_at)}>
            {data.archived_at ? <ArchiveRestore className="size-4" /> : <Archive className="size-4" />}
            {data.archived_at ? t('artifacts.restore') : t('artifacts.archive')}
          </Button>
        ) : null}
      </div>
      {data.archived_at ? <p className="rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">{t('artifacts.archivedNotice')}</p> : null}
      {!data.archived_at && data.sharing_disabled_at ? (
        <p className="rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">{t('artifacts.share.stoppedNotice')}</p>
      ) : null}
      {version.note ? <p className="rounded-lg border-l-2 border-primary/60 bg-muted/40 px-3 py-2 text-sm">{version.note}</p> : null}
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 rounded-lg border px-4 py-3 text-sm sm:grid-cols-3">
        <ArtifactFact label={t('artifacts.facts.owner')} value={artifactOwnerLabel(t, data)} />
        <ArtifactFact label={t('artifacts.facts.kind')} value={t(`artifacts.kind.${data.kind}`)} />
        <ArtifactFact label={t('artifacts.facts.visibility')} value={t(`artifacts.visibility.${data.visibility}`)} />
        <ArtifactFact label={t('artifacts.facts.size')} value={formatSize(version.size) || '—'} />
        <ArtifactFact
          label={t('artifacts.facts.source')}
          value={
            data.source_task_id || version.source_report_id ? (
              <span className="flex flex-wrap gap-x-2">
                {data.source_task_id ? (
                  <Link href={objectHref('task', data.source_task_id, digitalHumanId)} scroll={false} className="text-primary hover:underline">
                    {t('artifacts.openTask')}
                  </Link>
                ) : null}
                {version.source_report_id ? (
                  <button type="button" className="text-primary hover:underline" onClick={() => openReport(version.source_report_id ?? '')}>
                    {t('artifacts.openReport')}
                  </button>
                ) : null}
              </span>
            ) : (
              t('artifacts.facts.conversation')
            )
          }
        />
      </dl>
      <ArtifactPreview digitalHumanId={digitalHumanId} artifact={data} version={version} />
      <ArtifactShareDialog digitalHumanId={digitalHumanId} artifact={data} version={version.number} open={sharing} onOpenChange={setSharing} />
      {items.length > 1 ? (
        <section>
          <h3 className="flex items-center gap-1.5 text-sm font-semibold">
            <History className="size-4 text-muted-foreground" />
            {t('artifacts.history')}
          </h3>
          <ol className="mt-2 divide-y rounded-lg border">
            {items.map(item => (
              <li key={item.id}>
                <button
                  type="button"
                  aria-current={item.number === version.number ? 'true' : undefined}
                  className={cn('flex w-full items-start gap-3 px-3 py-2 text-left text-sm hover:bg-muted/40', item.number === version.number && 'bg-primary/5')}
                  onClick={() => setNumber(item.number)}
                >
                  <span className="w-8 shrink-0 font-medium tabular-nums">v{item.number}</span>
                  <span className="min-w-0 flex-1 truncate text-muted-foreground">{item.note || '—'}</span>
                  <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{formatTaskTime(item.created_at, locale)}</span>
                </button>
              </li>
            ))}
          </ol>
        </section>
      ) : null}
    </div>
  );
}

function ArtifactFact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 truncate">{value}</dd>
    </div>
  );
}

function ArtifactPreview({ digitalHumanId, artifact, version }: { digitalHumanId: string; artifact: DigitalHumanArtifact; version: DigitalHumanArtifactVersion }) {
  const t = useT('digitalHumans');
  const read = (signal?: AbortSignal) => digitalHumanService.readArtifactContent(digitalHumanId, artifact.id, version.number, signal);
  const asText = artifact.kind === 'document' || readsAsText(version, artifact.title);
  const inline = asText || artifact.kind === 'image';
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
    if (asText) {
      void blob.text().then(setText);
      return;
    }
    const url = URL.createObjectURL(blob);
    setImageUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [asText, content.data]);

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
  const markdown = artifact.kind === 'document' || /\.(md|markdown)$/i.test(version.file_name || artifact.title);
  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold">{t('artifacts.preview')}</h3>
        <Button size="sm" variant="outline" onClick={download}>
          <Download className="size-4" />
          {t('artifacts.download')}
        </Button>
      </div>
      {asText ? (
        <div className="max-h-[60vh] overflow-auto rounded-lg border px-5 py-4 text-sm">
          {markdown ? <MarkdownViewer content={text ?? ''} /> : <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-5">{text}</pre>}
        </div>
      ) : imageUrl ? (
        // A blob URL from an authenticated read.
        <img src={imageUrl} alt={artifact.title} className="max-h-[60vh] rounded-lg border object-contain" />
      ) : null}
    </section>
  );
}
