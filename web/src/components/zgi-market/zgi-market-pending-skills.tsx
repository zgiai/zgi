'use client';

import Link from 'next/link';
import { Download, Store } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  useInstallZgiMarketSkills,
  useZgiMarketInstalledSkills,
  useZgiMarketMeta,
  useZgiMarketPendingSkills,
  type MarketSkillBatchResult,
} from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';

/** Tells an administrator what a batch of installs came to, one line for each failure. */
export function useReportMarketSkillInstalls() {
  const t = useT();
  return (result: MarketSkillBatchResult) => {
    if (result.installed.length === 1 && result.failed.length === 0) {
      toast.success(t('market.installs.installedToast', { name: result.installed[0] }));
    } else if (result.installed.length > 0) {
      toast.success(t('market.installs.installedCount', { count: result.installed.length }));
    }
    for (const failure of result.failed) {
      toast.error(
        failure.reason === 'conflict'
          ? t('market.installs.conflict', { name: failure.slug, existing: failure.detail })
          : failure.reason === 'refused'
            ? t('market.installs.refused', { name: failure.slug, message: failure.detail })
            : t('market.installs.failed', { name: failure.slug, message: failure.detail })
      );
    }
  };
}

/**
 * The market skills the organization activated and has not installed, on the page that lists its
 * skills. Nothing is installed until an administrator asks for it.
 */
export function ZgiMarketPendingSkills() {
  const t = useT();
  const meta = useZgiMarketMeta();
  const { pending, isLoading, isError } = useZgiMarketPendingSkills();
  const install = useInstallZgiMarketSkills();
  const report = useReportMarketSkillInstalls();
  const canInstall = meta.data?.admin === true;

  // An instance without ZGI Market, and an organization with nothing waiting, show nothing.
  if (isLoading || isError || pending.length === 0) return null;

  const run = (slugs: string[]) => install.mutate(slugs, { onSuccess: report });

  return (
    <section
      className="rounded-xl border bg-card p-4 shadow-sm"
      aria-labelledby="zgi-market-pending-skills"
      data-testid="zgi-market-pending-skills"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2
            id="zgi-market-pending-skills"
            className="flex items-center gap-2 text-sm font-semibold"
          >
            <Store className="size-4 text-primary" aria-hidden="true" />
            {t('market.installs.title')}
            <Badge variant="warning">{t('market.installs.pending')}</Badge>
            <Badge variant="subtle">{pending.length}</Badge>
          </h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('market.installs.description')}
          </p>
        </div>
        {canInstall && pending.length > 1 ? (
          <Button
            size="sm"
            disabled={install.isPending}
            onClick={() => run(pending.map(skill => skill.slug))}
          >
            <Download className="size-4" aria-hidden="true" />
            {install.isPending
              ? t('market.installs.installing')
              : t('market.installs.installAll', { count: pending.length })}
          </Button>
        ) : null}
      </div>
      <ul className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        {pending.map(skill => (
          <li
            key={skill.slug}
            className="flex items-center justify-between gap-3 rounded-md border bg-background px-3 py-2"
          >
            <Link
              href={`/dashboard/market/catalog/skill/${skill.slug}`}
              className="min-w-0 truncate font-mono text-xs hover:underline"
            >
              {skill.slug}
            </Link>
            {canInstall ? (
              <Button
                size="sm"
                variant="outline"
                className="shrink-0"
                disabled={install.isPending}
                aria-label={`${t('market.installs.install')} ${skill.slug}`}
                onClick={() => run([skill.slug])}
              >
                {t('market.installs.install')}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Marks a skill of the organization that was installed from ZGI Market. */
export function ZgiMarketSkillOriginBadge({ skillId }: { skillId: string }) {
  const t = useT();
  const installed = useZgiMarketInstalledSkills();
  const match = (installed.data ?? []).find(skill => skill.skill_id === skillId);
  if (!match) return null;
  return (
    <Badge variant="outline" className="h-5 shrink-0 gap-1 rounded-md px-1.5 font-normal">
      <Store className="size-3" aria-hidden="true" />
      {t('market.installs.origin')}
    </Badge>
  );
}
