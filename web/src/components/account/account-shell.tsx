'use client';

import type { ReactNode } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { ArrowLeft, Bell, Brain, UserRound, type LucideIcon } from 'lucide-react';
import { UserMenu } from '@/components/console/user-menu';
import { Logo } from '@/components/logo';
import { SafeAvatar } from '@/components/ui/avatar';
import { ROUTES } from '@/constants/routes';
import { useAutoProfile } from '@/hooks/use-profile';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { useAuthStore } from '@/store/auth-store';
import { getUserContactDisplay } from '@/utils/account-display';

const sections: Array<{
  href: string;
  label: 'profile' | 'memory' | 'notifications';
  icon: LucideIcon;
}> = [
  { href: '/profile', label: 'profile', icon: UserRound },
  { href: '/profile/memory', label: 'memory', icon: Brain },
  { href: '/profile/notifications', label: 'notifications', icon: Bell },
];

// The personal pages (profile, memory, notifications) share one frame: a
// header to get back to work and a navigation between them. It does not use
// the console shell, which requires an organization; people arriving from a
// DigitalHuman or WebApp may not have one.
export function AccountShell({ children }: { children: ReactNode }) {
  const tNav = useT('navigation');
  const pathname = usePathname();
  const user = useAuthStore.use.user();
  const { data: profile } = useAutoProfile({ staleTime: 1_800_000 });
  const name = profile?.name || user?.name || '';
  const contact = getUserContactDisplay(profile, user);
  const active = (href: string) =>
    href === '/profile' ? pathname === '/profile' : pathname.startsWith(href);

  return (
    <div className="min-h-screen bg-muted/30">
      <header className="sticky top-0 z-30 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="mx-auto flex h-14 max-w-6xl items-center gap-4 px-4 sm:px-6">
          <Logo showName={false} />
          <span aria-hidden="true" className="h-5 w-px bg-border" />
          <Link
            href={ROUTES.CONSOLE.HOME}
            className="flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            <ArrowLeft className="size-4" />
            {tNav('backToWorkspace')}
          </Link>
          <div className="ml-auto flex items-center gap-3">
            <span className="hidden text-sm font-medium sm:inline">{tNav('accountCenter')}</span>
            <UserMenu />
          </div>
        </div>
      </header>

      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-6 sm:px-6 lg:flex-row lg:gap-8">
        <aside className="shrink-0 lg:sticky lg:top-20 lg:w-52 lg:self-start">
          <div className="mb-4 hidden items-center gap-3 px-2 lg:flex">
            <SafeAvatar
              src={profile?.avatar_url || user?.avatar_url || null}
              alt={name || null}
              fallback={name || null}
              size="md"
            />
            <div className="min-w-0">
              <div className="truncate text-sm font-medium">{name}</div>
              {contact ? (
                <div className="truncate text-xs text-muted-foreground">{contact}</div>
              ) : null}
            </div>
          </div>
          <nav
            aria-label={tNav('accountCenter')}
            className="-mx-1 flex gap-1 overflow-x-auto px-1 lg:mx-0 lg:flex-col lg:overflow-visible lg:px-0"
          >
            {sections.map(section => {
              const Icon = section.icon;
              const current = active(section.href);
              return (
                <Link
                  key={section.href}
                  href={section.href}
                  aria-current={current ? 'page' : undefined}
                  className={cn(
                    'flex shrink-0 items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors',
                    current
                      ? 'bg-background font-medium text-foreground shadow-sm ring-1 ring-border'
                      : 'text-muted-foreground hover:bg-background/60 hover:text-foreground'
                  )}
                >
                  <Icon className="size-4" />
                  {tNav(section.label)}
                </Link>
              );
            })}
          </nav>
        </aside>
        <main className="min-w-0 flex-1">{children}</main>
      </div>
    </div>
  );
}

/** The title row every personal page starts with. */
export function AccountPageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0 flex-1 basis-64">
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description ? <p className="mt-1 text-sm text-muted-foreground">{description}</p> : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  );
}
