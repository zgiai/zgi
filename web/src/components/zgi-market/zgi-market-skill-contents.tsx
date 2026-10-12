'use client';

import { FileCode2, FileText, FolderClosed, TerminalSquare } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { useZgiMarketSkillManifest } from '@/hooks/use-zgi-market';
import type { MarketSkillSummary } from '@/services/types/zgi-market';
import { useMarketLabels } from './use-market-labels';
import { skillPackageSummary } from './market-format';

interface ZgiMarketSkillContentsProps {
  slug: string;
  /** What the market says about the package to anyone who may see the listing; null when it says nothing. */
  summary: MarketSkillSummary | null;
  /** The description the page already shows above; the package's own is not repeated when it says the same. */
  shownDescription?: string;
  /** Whether the organization activated the skill; the market serves the whole file list only then. */
  activated: boolean;
}

interface PackageFile {
  path: string;
  size: number;
}

function FileRow({ file }: { file: PackageFile }) {
  const { bytes } = useMarketLabels();
  const script = skillPackageSummary([file]).scripts.length > 0;
  const Icon = script ? FileCode2 : FileText;
  return (
    <li className="flex items-center justify-between gap-3 px-4 py-2">
      <span className="flex min-w-0 items-center gap-2">
        <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="break-all font-mono text-xs">{file.path}</span>
      </span>
      <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
        {bytes(file.size)}
      </span>
    </li>
  );
}

/** Whether the page already says this: the same words, or the beginning of them. */
function sameText(text: string | undefined, shown: string | undefined): boolean {
  const a = (text ?? '').replace(/\s+/g, ' ').trim();
  const b = (shown ?? '').replace(/\s+/g, ' ').trim();
  return a !== '' && b !== '' && (a === b || a.startsWith(b));
}

function Notice({ children }: { children: React.ReactNode }) {
  return (
    <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
      {children}
    </p>
  );
}

/**
 * What a skill package is, before anyone installs it: what SKILL.md declares, the beginning of its
 * text, whether it brings scripts, and its files by directory. The market caps the summary's file
 * list; an organization that has the skill gets the whole list from the package manifest.
 */
export function ZgiMarketSkillContents({
  slug,
  summary,
  shownDescription,
  activated,
}: ZgiMarketSkillContentsProps) {
  const { t, bytes } = useMarketLabels();
  const needsManifest = activated && (!summary || (summary.files_omitted ?? 0) > 0);
  const manifest = useZgiMarketSkillManifest(slug, needsManifest);

  if (!summary) {
    if (!activated) {
      return <Notice>{t('market.catalog.detail.skill.afterInstall')}</Notice>;
    }
    if (manifest.isPending) {
      return <Skeleton className="h-32 w-full" />;
    }
    if (manifest.isError || !manifest.data) {
      return <Notice>{t('market.catalog.detail.skill.unavailable')}</Notice>;
    }
  }

  const files: PackageFile[] = manifest.data?.files ?? summary?.files ?? [];
  const fileCount = manifest.data?.files.length ?? summary?.file_count ?? files.length;
  const totalBytes = manifest.data?.size_bytes ?? summary?.total_bytes ?? 0;
  const omitted = manifest.data ? 0 : (summary?.files_omitted ?? 0);
  const grouped = skillPackageSummary(files);
  const hasScripts = summary ? summary.has_scripts : grouped.scripts.length > 0;
  const interpreters = summary?.interpreters ?? [];
  const description = sameText(summary?.description, shownDescription) ? '' : summary?.description;
  const declared = summary?.name || description || summary?.when_to_use;

  return (
    <div className="flex flex-col gap-4">
      {declared ? (
        <dl className="flex flex-col gap-3 rounded-lg border bg-background px-4 py-3 text-sm">
          {summary?.name ? (
            <div className="flex flex-col gap-1">
              <dt className="text-xs text-muted-foreground">
                {t('market.catalog.detail.skill.declaredName')}
              </dt>
              <dd className="break-all font-mono text-xs">{summary.name}</dd>
            </div>
          ) : null}
          {description ? (
            <dd className="whitespace-pre-line break-words leading-6">{description}</dd>
          ) : null}
          {summary?.when_to_use ? (
            <div className="flex flex-col gap-1">
              <dt className="text-xs text-muted-foreground">
                {t('market.catalog.detail.skill.whenToUse')}
              </dt>
              <dd className="whitespace-pre-line break-words leading-6">{summary.when_to_use}</dd>
            </div>
          ) : null}
        </dl>
      ) : null}

      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <Badge variant="outline">
            {t('market.catalog.detail.skill.fileCount', { count: fileCount })}
          </Badge>
          <Badge variant="outline">{bytes(totalBytes)}</Badge>
          {hasScripts ? (
            <Badge variant="warning" className="gap-1">
              <TerminalSquare className="size-3.5" aria-hidden="true" />
              {t('market.catalog.detail.skill.hasScripts')}
            </Badge>
          ) : (
            <Badge variant="subtle">{t('market.catalog.detail.skill.noScripts')}</Badge>
          )}
          {interpreters.map(interpreter => (
            <Badge key={interpreter} variant="outline" className="font-mono">
              {interpreter}
            </Badge>
          ))}
        </div>
        <p className="text-xs leading-5 text-muted-foreground">
          {hasScripts
            ? t('market.catalog.detail.skill.scriptsNote')
            : t('market.catalog.detail.skill.instructionsNote')}
        </p>
      </div>

      {summary?.excerpt ? (
        <div className="flex flex-col gap-2">
          <h3 className="text-sm font-medium">{t('market.catalog.detail.skill.excerptTitle')}</h3>
          <div className="max-h-[28rem] overflow-y-auto rounded-lg border bg-background px-4 py-3">
            {/* SKILL.md is written by whoever published the skill: raw HTML is never rendered. */}
            <MarkdownViewer content={summary.excerpt} allowRawHtml={false} />
          </div>
          {summary.excerpt_truncated ? (
            <p className="text-xs text-muted-foreground">
              {t('market.catalog.detail.skill.excerptTruncated')}
            </p>
          ) : null}
        </div>
      ) : null}

      <div className="flex flex-col gap-2">
        <h3 className="text-sm font-medium">{t('market.catalog.detail.skill.filesTitle')}</h3>
        <div className="max-h-[28rem] overflow-y-auto rounded-lg border bg-background">
          {grouped.groups.map(group => (
            <div key={group.directory || '/'} className="border-b last:border-b-0">
              {group.directory ? (
                <p className="flex items-center gap-2 bg-muted/40 px-4 py-1.5 text-xs font-medium text-muted-foreground">
                  <FolderClosed className="size-3.5" aria-hidden="true" />
                  <span className="font-mono">{group.directory}/</span>
                </p>
              ) : null}
              <ul className="divide-y">
                {group.files.map(file => (
                  <FileRow key={file.path} file={file} />
                ))}
              </ul>
            </div>
          ))}
        </div>
        {omitted > 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('market.catalog.detail.skill.filesOmitted', { count: omitted })}
          </p>
        ) : null}
      </div>
    </div>
  );
}
