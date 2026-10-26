'use client';

import * as React from 'react';
import { useT } from '@/i18n';

/**
 * @component RuntimePageHeader
 * @category Feature
 * @status Beta
 * @description The header every External access page shares: the section
 * eyebrow, the page title with an optional status, a description, and the
 * page's actions on the right.
 * @usage Rendered at the top of each Runtime Workspace console section.
 * @example
 * <RuntimePageHeader title="End users" description="…" actions={<Button>…</Button>} />
 */
export function RuntimePageHeader({
  title,
  description,
  badge,
  actions,
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  badge?: React.ReactNode;
  actions?: React.ReactNode;
}) {
  const t = useT('workspace');
  return (
    <header className="flex flex-col gap-4 border-b border-border/70 pb-5 @3xl/console:flex-row @3xl/console:items-end @3xl/console:justify-between">
      <div className="min-w-0">
        <div className="mb-1.5 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {t('runtime.pageTitle')}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          {badge}
        </div>
        {description ? (
          <p className="mt-1 max-w-3xl text-sm leading-6 text-muted-foreground">{description}</p>
        ) : null}
      </div>
      {actions ? <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div> : null}
    </header>
  );
}
