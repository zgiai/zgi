'use client';

import { FileCode2, FileText, FolderClosed, TerminalSquare } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { useZgiMarketSkillManifest } from '@/hooks/use-zgi-market';
import type { MarketSkillManifest } from '@/services/types/zgi-market';
import { useMarketLabels } from './use-market-labels';
import { skillPackageSummary } from './market-format';

interface ZgiMarketSkillContentsProps {
  slug: string;
  /** Whether the organization activated the skill; the market describes the package only then. */
  activated: boolean;
}

function FileRow({ file }: { file: MarketSkillManifest['files'][number] }) {
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

/**
 * What a skill package holds: its files by directory, and whether it brings scripts. The market
 * serves the file list, not the text of SKILL.md, and only to an organization that has the skill.
 */
export function ZgiMarketSkillContents({ slug, activated }: ZgiMarketSkillContentsProps) {
  const { t, bytes } = useMarketLabels();
  const manifest = useZgiMarketSkillManifest(slug, activated);

  if (!activated) {
    return (
      <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
        {t('market.catalog.detail.skill.afterInstall')}
      </p>
    );
  }
  if (manifest.isPending) {
    return <Skeleton className="h-32 w-full" />;
  }
  if (manifest.isError || !manifest.data) {
    return (
      <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
        {t('market.catalog.detail.skill.unavailable')}
      </p>
    );
  }

  const summary = skillPackageSummary(manifest.data.files);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="outline">
          {t('market.catalog.detail.skill.fileCount', { count: manifest.data.files.length })}
        </Badge>
        <Badge variant="outline">{bytes(manifest.data.size_bytes)}</Badge>
        {summary.scripts.length > 0 ? (
          <Badge variant="warning" className="gap-1">
            <TerminalSquare className="size-3.5" aria-hidden="true" />
            {t('market.catalog.detail.skill.scripts', { count: summary.scripts.length })}
          </Badge>
        ) : (
          <Badge variant="subtle">{t('market.catalog.detail.skill.noScripts')}</Badge>
        )}
      </div>
      <p className="text-xs leading-5 text-muted-foreground">
        {summary.scripts.length > 0
          ? t('market.catalog.detail.skill.scriptsNote')
          : t('market.catalog.detail.skill.instructionsNote')}
      </p>
      <div className="overflow-hidden rounded-lg border bg-background">
        {summary.groups.map(group => (
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
    </div>
  );
}
