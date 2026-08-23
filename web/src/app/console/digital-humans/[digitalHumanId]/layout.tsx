'use client';

import { useEffect, useId, type MouseEvent, type ReactNode } from 'react';
import Link from 'next/link';
import { useParams, usePathname, useRouter, useSearchParams } from 'next/navigation';
import { BrainCircuit, History, RadioTower, Settings2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { DigitalHumanChat } from '@/components/digital-human/digital-human-chat';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { useDigitalHumanDraftStore } from '@/components/digital-human/digital-human-draft-store';
import { DigitalHumanHeader } from '@/components/digital-human/digital-human-header';
import { DigitalHumanSidebar } from '@/components/digital-human/digital-human-sidebar';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

const testChatParam = 'test-chat';
const conversationParam = 'conversation';

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
  const searchParams = useSearchParams();
  const testChatControlsPortalId = useId();
  const draftIsDirty = useDigitalHumanDraftStore(state => Boolean(state.editors[id]?.isDirty));
  const permissions = useAccountPermissions();
  const isConfigurationPage = pathname.startsWith(
    `/console/digital-humans/${id}/configuration`
  );

  useEffect(() => {
    if (!draftIsDirty) return;
    const warnBeforeUnload = (event: BeforeUnloadEvent) => event.preventDefault();
    window.addEventListener('beforeunload', warnBeforeUnload);
    return () => window.removeEventListener('beforeunload', warnBeforeUnload);
  }, [draftIsDirty]);

  const navigateAfterSaving = async (event: MouseEvent<HTMLAnchorElement>, href: string) => {
    const draftEditor = useDigitalHumanDraftStore.getState().editors[id];
    if (!draftEditor || (!draftEditor.isDirty && draftEditor.saveState !== 'error')) return;
    event.preventDefault();
    const saved = await draftEditor.saveNow();
    if (saved) router.push(href);
  };

  const buildCurrentHref = (params: URLSearchParams) => {
    const query = params.toString();
    return query ? `${pathname}?${query}` : pathname;
  };

  const openTestChat = () => {
    const params = new URLSearchParams(searchParams.toString());
    params.set(testChatParam, '1');
    router.push(buildCurrentHref(params), { scroll: false });
  };

  const closeTestChat = () => {
    const params = new URLSearchParams(searchParams.toString());
    params.delete(testChatParam);
    params.delete(conversationParam);
    router.replace(buildCurrentHref(params), { scroll: false });
  };

  const mobileItems = [
    {
      href: `/console/digital-humans/${id}/configuration`,
      label: t('tabs.configuration'),
      icon: Settings2,
      active: pathname.startsWith(`/console/digital-humans/${id}/configuration`),
    },
    {
      href: `/console/digital-humans/${id}/channels`,
      label: t('tabs.channels'),
      icon: RadioTower,
      active: pathname.startsWith(`/console/digital-humans/${id}/channels`),
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

  return (
    <div className="flex h-full min-h-0 min-w-0 bg-background">
      <DigitalHumanSidebar id={id} onNavigate={navigateAfterSaving} />
      <div className="flex h-full min-h-0 w-0 min-w-0 grow flex-col overflow-hidden">
        {isConfigurationPage ? (
          <DigitalHumanHeader
            id={id}
            onOpenTest={openTestChat}
          />
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
        <div className="min-h-0 flex-1 overflow-auto">{children}</div>
      </div>

      {permissions.hasPermission('digital_human.run') ? (
        <Sheet
          open={searchParams.get(testChatParam) === '1'}
          onOpenChange={open => {
            if (!open) closeTestChat();
          }}
        >
          <SheetContent
            side="right"
            showClose={false}
            className="flex h-full min-h-0 w-[min(720px,100vw)] max-w-none flex-col gap-0 p-0 sm:max-w-none"
          >
            <SheetHeader className="flex h-14 shrink-0 flex-row items-center justify-between space-y-0 border-b px-4 text-left">
              <div className="min-w-0">
                <SheetTitle className="truncate text-sm">{t('chat.sheetTitle')}</SheetTitle>
                <SheetDescription className="truncate text-xs">
                  {t('chat.sheetDescription')}
                </SheetDescription>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <div id={testChatControlsPortalId} className="flex items-center" />
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      isIcon
                      className="size-7"
                      aria-label={t('chat.close')}
                      onClick={closeTestChat}
                    >
                      <X className="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('chat.close')}</TooltipContent>
                </Tooltip>
              </div>
            </SheetHeader>
            <div className="min-h-0 flex-1">
              <DigitalHumanChat id={id} controlsPortalId={testChatControlsPortalId} />
            </div>
          </SheetContent>
        </Sheet>
      ) : null}
    </div>
  );
}
