'use client';

import { useEffect, type MouseEvent, type ReactNode } from 'react';
import Link from 'next/link';
import { useParams, usePathname, useRouter } from 'next/navigation';
import {
  BrainCircuit,
  FileText,
  History,
  House,
  ListTodo,
  Loader2,
  MessageSquare,
  Package,
  RadioTower,
  Server,
  Settings2,
} from 'lucide-react';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { useDigitalHumanDraftStore } from '@/components/digital-human/digital-human-draft-store';
import { DigitalHumanHeader } from '@/components/digital-human/digital-human-header';
import { DigitalHumanSidebar } from '@/components/digital-human/digital-human-sidebar';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

export default function DigitalHumanLayout({ children }: { children: ReactNode }) {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate>
      <DigitalHumanShell id={params.digitalHumanId}>{children}</DigitalHumanShell>
    </DigitalHumanFeatureGate>
  );
}

function DigitalHumanShell({ id, children }: { id: string; children: ReactNode }) {
  const t = useT('digitalHumans');
  const pathname = usePathname();
  const router = useRouter();
  const draftIsDirty = useDigitalHumanDraftStore(state => Boolean(state.editors[id]?.isDirty));
  const permissions = useAccountPermissions();
  const detailQuery = useDigitalHuman(id);
  const configurationHref = `/console/digital-humans/${id}/configuration`;
  const externalHref = `/console/digital-humans/${id}/external`;
  const chatHref = `/console/digital-humans/${id}/chat`;
  const isConfigurationPage = pathname.startsWith(configurationHref);
  const isExternalPage = pathname.startsWith(externalHref);
  const isInitialSetup = Boolean(detailQuery.data?.data && !detailQuery.data.data.published);
  const restrictToConfiguration = detailQuery.isLoading || isInitialSetup;

  useEffect(() => {
    if (!draftIsDirty) return;
    const warnBeforeUnload = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener('beforeunload', warnBeforeUnload);
    return () => window.removeEventListener('beforeunload', warnBeforeUnload);
  }, [draftIsDirty]);

  useEffect(() => {
    if (isInitialSetup && !isConfigurationPage) {
      router.replace(configurationHref);
    }
  }, [configurationHref, isConfigurationPage, isInitialSetup, router]);

  const navigateAfterSaving = async (event: MouseEvent<HTMLAnchorElement>, href: string) => {
    const draftEditor = useDigitalHumanDraftStore.getState().editors[id];
    if (!draftEditor || (!draftEditor.isDirty && draftEditor.saveState !== 'error')) return;
    event.preventDefault();
    const saved = await draftEditor.saveNow();
    if (saved) router.push(href);
  };

  const openChat = async () => {
    const draftEditor = useDigitalHumanDraftStore.getState().editors[id];
    if (draftEditor && (draftEditor.isDirty || draftEditor.saveState === 'error')) {
      const saved = await draftEditor.saveNow();
      if (!saved) return;
    }
    router.push(chatHref);
  };

  const configurationItem = {
    href: configurationHref,
    label: t('tabs.configuration'),
    icon: Settings2,
    active: isConfigurationPage,
  };
  const publishedMobileItems = [
    {
      href: `/console/digital-humans/${id}`,
      label: t('tabs.dashboard'),
      icon: House,
      active: pathname === `/console/digital-humans/${id}`,
    },
    ...(permissions.hasPermission('digital_human.run')
      ? [
          {
            href: chatHref,
            label: t('tabs.chat'),
            icon: MessageSquare,
            active: pathname.startsWith(chatHref),
          },
        ]
      : []),
    configurationItem,
    {
      href: externalHref,
      label: t('tabs.external'),
      icon: Server,
      active: isExternalPage,
    },
    {
      href: `/console/digital-humans/${id}/channels`,
      label: t('tabs.channels'),
      icon: RadioTower,
      active: pathname.startsWith(`/console/digital-humans/${id}/channels`),
    },
    {
      href: `/console/digital-humans/${id}/tasks`,
      label: t('tabs.tasks'),
      icon: ListTodo,
      active: pathname.startsWith(`/console/digital-humans/${id}/tasks`),
    },
    {
      href: `/console/digital-humans/${id}/reports`,
      label: t('tabs.reports'),
      icon: FileText,
      active: pathname.startsWith(`/console/digital-humans/${id}/reports`),
    },
    {
      href: `/console/digital-humans/${id}/artifacts`,
      label: t('tabs.artifacts'),
      icon: Package,
      active: pathname.startsWith(`/console/digital-humans/${id}/artifacts`),
    },
    {
      href: `/console/digital-humans/${id}/runs`,
      label: t('tabs.runs'),
      icon: History,
      active: pathname.startsWith(`/console/digital-humans/${id}/runs`),
    },
    ...(permissions.hasPermission('digital_human.memory.view')
      ? [
          {
            href: `/console/digital-humans/${id}/memory`,
            label: t('tabs.memory'),
            icon: BrainCircuit,
            active: pathname.startsWith(`/console/digital-humans/${id}/memory`),
          },
        ]
      : []),
  ];
  const mobileItems = restrictToConfiguration ? [configurationItem] : publishedMobileItems;
  const routeGuardPending = !isConfigurationPage && restrictToConfiguration;

  return (
    <div className="flex h-full min-h-0 min-w-0 bg-background">
      <DigitalHumanSidebar id={id} onNavigate={navigateAfterSaving} />
      <div className="flex h-full min-h-0 w-0 min-w-0 grow flex-col overflow-hidden">
        {isConfigurationPage || isExternalPage ? (
          <DigitalHumanHeader id={id} onOpenChat={() => void openChat()} />
        ) : null}
        <nav
          className="flex shrink-0 gap-1 overflow-x-auto border-b bg-background px-2 py-1 md:hidden"
          aria-label={t('tabs.navigation')}
        >
          {mobileItems.map(item => {
            const Icon = item.icon;
            return (
              <Link
                key={item.href}
                href={item.href}
                onClick={event => void navigateAfterSaving(event, item.href)}
                aria-current={item.active ? 'page' : undefined}
                className={cn(
                  'flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  item.active
                    ? 'bg-muted font-medium text-foreground'
                    : 'text-muted-foreground hover:text-foreground'
                )}
              >
                <Icon className="size-3.5" />
                {item.label}
              </Link>
            );
          })}
        </nav>
        <div className="min-h-0 flex-1 overflow-auto">
          {routeGuardPending ? (
            <div
              className="flex min-h-full items-center justify-center gap-2 text-sm text-muted-foreground"
              role="status"
            >
              <Loader2 className="size-4 animate-spin" />
              {isInitialSetup ? t('initialSetup.redirecting') : t('loading')}
            </div>
          ) : (
            children
          )}
        </div>
      </div>
    </div>
  );
}
