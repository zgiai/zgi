'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { ArrowRight, Info } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

const WORKSPACE_TABS = [
  {
    key: 'models',
    href: '/dashboard/provider',
    pathPrefix: '/dashboard/provider',
  },
  {
    key: 'channels',
    href: '/dashboard/channel',
    pathPrefix: '/dashboard/channel',
  },
] as const;

export default function ModelChannelWorkspaceNav(): JSX.Element {
  const pathname = usePathname();
  const t = useT('dashboard');
  const isChannelRoute = pathname.startsWith('/dashboard/channel');

  return (
    <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border/70 bg-background px-4 py-2">
      <nav
        aria-label={t('modelChannels.navigationLabel')}
        className="inline-flex items-center gap-1 rounded-md bg-muted/70 p-1"
      >
        {WORKSPACE_TABS.map(tab => {
          const active = pathname.startsWith(tab.pathPrefix);
          const showChannelCue = tab.key === 'channels' && !active;

          return (
            <Link
              key={tab.key}
              href={tab.href}
              aria-current={active ? 'page' : undefined}
              className={cn(
                'inline-flex h-8 items-center gap-2 rounded-sm px-3 text-sm font-medium transition-colors',
                active
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:bg-background/60 hover:text-foreground'
              )}
            >
              {t(`modelChannels.tabs.${tab.key}`)}
              {showChannelCue ? (
                <span className="hidden items-center gap-0.5 rounded-full bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary sm:inline-flex">
                  {t('modelChannels.channelCue')}
                  <ArrowRight className="h-2.5 w-2.5" />
                </span>
              ) : null}
            </Link>
          );
        })}
      </nav>
      <div className="hidden items-center gap-1.5 text-xs text-muted-foreground md:flex">
        <Info className="h-3.5 w-3.5 shrink-0" />
        <span>
          {t(isChannelRoute ? 'modelChannels.guidance.channels' : 'modelChannels.guidance.models')}
        </span>
      </div>
    </div>
  );
}
