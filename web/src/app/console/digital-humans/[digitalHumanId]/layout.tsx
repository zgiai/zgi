'use client';

import type { ReactNode } from 'react';
import Link from 'next/link';
import { useParams, usePathname } from 'next/navigation';
import {
  Activity,
  ArrowLeft,
  Bot,
  BrainCircuit,
  LayoutDashboard,
  MessageSquare,
  RadioTower,
  Settings2,
  Wrench,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

export default function DigitalHumanLayout({ children }: { children: ReactNode }) {
  const params = useParams<{ digitalHumanId: string }>();
  const id = params.digitalHumanId;
  return (
    <DigitalHumanFeatureGate>
      <DigitalHumanShell id={id}>{children}</DigitalHumanShell>
    </DigitalHumanFeatureGate>
  );
}

function DigitalHumanShell({ id, children }: { id: string; children: ReactNode }) {
  const t = useT('digitalHumans');
  const pathname = usePathname();
  const detail = useDigitalHuman(id);
  const permissions = useAccountPermissions();
  const value = detail.data?.data;
  const hasUnpublishedChanges = Boolean(
    value?.published && value.draft.revision !== value.published.revision
  );
  const tabs = [
    {
      href: `/console/digital-humans/${id}`,
      label: t('tabs.overview'),
      icon: LayoutDashboard,
      exact: true,
    },
    {
      href: `/console/digital-humans/${id}/settings`,
      label: t('tabs.settings'),
      icon: Settings2,
    },
    {
      href: `/console/digital-humans/${id}/capabilities`,
      label: t('tabs.capabilities'),
      icon: Wrench,
    },
    {
      href: `/console/digital-humans/${id}/channels`,
      label: t('tabs.channels'),
      icon: RadioTower,
    },
    { href: `/console/digital-humans/${id}/chat`, label: t('tabs.chat'), icon: MessageSquare },
    { href: `/console/digital-humans/${id}/runs`, label: t('tabs.runs'), icon: Activity },
    ...(permissions.hasPermission('digital_human.memory.view')
      ? [
          {
            href: `/console/digital-humans/${id}/memory`,
            label: t('tabs.memory'),
            icon: BrainCircuit,
          },
        ]
      : []),
  ];
  return (
    <div className="min-h-full">
      <div className="border-b bg-background px-4 pt-5 md:px-8">
        <div className="mx-auto flex max-w-7xl flex-col gap-5">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex min-w-0 items-center gap-3">
              <Link
                href="/console/digital-humans"
                className="flex size-10 shrink-0 items-center justify-center rounded-lg text-muted-foreground hover:bg-muted"
                aria-label={t('common.back')}
              >
                <ArrowLeft className="size-4" />
              </Link>
              <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <Bot className="size-5" />
              </div>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h1 className="truncate text-lg font-semibold">
                    {value?.definition.name ?? t('title')}
                  </h1>
                  <span className="rounded-full border bg-muted/30 px-2 py-0.5 text-xs text-muted-foreground">
                    {value?.published
                      ? t('header.publishedVersion', {
                          version: value.published.version_number ?? 1,
                        })
                      : t('header.draft')}
                  </span>
                  {hasUnpublishedChanges ? (
                    <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-xs text-amber-700 dark:text-amber-300">
                      {t('header.unpublishedChanges')}
                    </span>
                  ) : null}
                </div>
                <p className="mt-0.5 truncate text-sm text-muted-foreground">
                  {value?.definition.description || t('header.noDescription')}
                </p>
              </div>
            </div>
            <div className="flex shrink-0 gap-2 pl-[6.5rem] sm:pl-0">
              <Button variant="outline" size="sm" asChild>
                <Link href={`/console/digital-humans/${id}/chat`}>{t('header.test')}</Link>
              </Button>
              {permissions.hasPermission('digital_human.publish') ? (
                <Button size="sm" asChild>
                  <Link href={`/console/digital-humans/${id}#publish`}>
                    {value?.published ? t('header.publishChanges') : t('header.publish')}
                  </Link>
                </Button>
              ) : null}
            </div>
          </div>
          <nav className="flex gap-1 overflow-x-auto" aria-label={t('tabs.navigation')}>
            {tabs.map(tab => {
              const active = tab.exact ? pathname === tab.href : pathname.startsWith(tab.href);
              const Icon = tab.icon;
              return (
                <Link
                  key={tab.href}
                  href={tab.href}
                  aria-current={active ? 'page' : undefined}
                  className={cn(
                    'flex min-h-10 shrink-0 items-center gap-2 border-b-2 px-3 py-2 text-sm',
                    active
                      ? 'border-primary font-medium text-foreground'
                      : 'border-transparent text-muted-foreground hover:text-foreground'
                  )}
                >
                  <Icon className="size-4" />
                  {tab.label}
                </Link>
              );
            })}
          </nav>
        </div>
      </div>
      {children}
    </div>
  );
}
