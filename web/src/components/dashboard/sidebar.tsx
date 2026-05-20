'use client';

import * as React from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { IS_CLOUD } from '@/lib/config';
import { Icons } from '@/components/ui/icons';
import {
  Activity,
  AppWindow,
  ArrowLeft,
  ArrowRightToLine,
  Bot,
  Brain,
  Building2,
  ChevronDown,
  ContactRound,
  CreditCard,
  KeyRound,
  RadioTower,
  ReceiptText,
  Settings,
  ShieldCheck,
  Users,
  X,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { getSidebarCollapsed, saveSidebarCollapsed } from '@/utils/ui-local';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Logo } from '@/components/logo';

interface NavItem {
  key: string;
  title: string;
  href: string;
  icon?: React.ElementType;
}

interface NavGroup {
  key: string;
  title: string;
  icon?: React.ElementType;
  items: NavItem[];
  isRoot?: boolean;
  defaultOpen?: boolean;
}

const STORAGE_KEY = 'zgi:dashboard:sidebar:groups';

function buildDashboardGroups(t: ReturnType<typeof useT<'dashboard'>>) {
  return [
    {
      key: 'root',
      title: t('items.dashboard'),
      icon: Icons.LayoutDashboard,
      items: [
        {
          key: 'dashboard',
          title: t('items.dashboard'),
          href: '/dashboard',
          icon: Icons.LayoutDashboard,
        },
      ],
      isRoot: true,
    },
    {
      key: 'usage-overview',
      title: t('usage.title'),
      icon: Activity,
      items: [
        {
          key: 'usage-overview',
          title: t('groups.usage'),
          href: '/dashboard/usage/overview',
          icon: Activity,
        },
      ],
    },
    {
      key: 'org',
      title: t('groups.org'),
      icon: Users,
      defaultOpen: true,
      items: [
        {
          key: 'workspaces',
          title: t('items.workspaces'),
          href: '/dashboard/organization/workspaces',
          icon: Building2,
        },
        {
          key: 'contacts',
          title: t('items.contacts'),
          href: '/dashboard/organization/contacts',
          icon: ContactRound,
        },
        {
          key: 'permissions',
          title: t('items.permissions'),
          href: '/dashboard/organization/permissions',
          icon: ShieldCheck,
        },
      ],
    },
    {
      key: 'llm',
      title: t('groups.llm'),
      icon: Brain,
      defaultOpen: true,
      items: [
        {
          key: 'providers',
          title: t('items.llmProviders'),
          href: '/dashboard/provider',
          icon: Brain,
        },
        {
          key: 'model-settings',
          title: t('items.modelSettings'),
          href: '/dashboard/settings/model',
          icon: Settings,
        },
        {
          key: 'channel',
          title: t('items.channel'),
          href: '/dashboard/channel',
          icon: RadioTower,
        },
        {
          key: 'api-keys',
          title: t('items.apiKeys'),
          href: '/dashboard/api-keys',
          icon: KeyRound,
        },
        {
          key: 'aichat-skills',
          title: t('items.aichatSkills'),
          href: '/dashboard/organization/aichat-skills',
          icon: Bot,
        },
      ],
    },
    ...(IS_CLOUD
      ? [
          {
            key: 'billing',
            title: t('groups.billing'),
            icon: CreditCard,
            defaultOpen: true,
            items: [
              {
                key: 'billing-overview',
                title: t('items.billingOverview'),
                href: '/dashboard/cost-center/overview',
                icon: CreditCard,
              },
              {
                key: 'billing-bills',
                title: t('items.billingBills'),
                href: '/dashboard/cost-center/bills',
                icon: ReceiptText,
              },
            ],
          },
        ]
      : []),
    {
      key: 'settings',
      title: t('groups.settings'),
      defaultOpen: true,
      icon: Settings,
      items: [
        {
          key: 'marketplace',
          title: t('items.marketplace'),
          href: '/dashboard/market/plugins',
          icon: AppWindow,
        },
      ],
    },
  ] as NavGroup[];
}

function getDefaultOpenState(groups: NavGroup[]): Record<string, boolean> {
  return groups.reduce<Record<string, boolean>>((state, group) => {
    if (group.defaultOpen) {
      state[group.key] = true;
    }

    return state;
  }, {});
}

export function DashboardSidebar(): JSX.Element {
  const pathname = usePathname();
  const t = useT('dashboard');
  const tNav = useT('navigation');
  const [isCollapsed, setIsCollapsed] = React.useState<boolean>(() =>
    getSidebarCollapsed('dashboard', false)
  );

  React.useEffect(() => {
    saveSidebarCollapsed('dashboard', isCollapsed);
  }, [isCollapsed]);

  const toggleCollapse = () => setIsCollapsed(prev => !prev);

  // Build nav structure with i18n labels
  const groups = React.useMemo<NavGroup[]>(() => buildDashboardGroups(t), [t]);

  // Open state per group, persisted in localStorage
  const [open, setOpen] = React.useState<Record<string, boolean>>(() => {
    const defaultOpen = getDefaultOpenState(groups);
    if (typeof window === 'undefined') return {};
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      return raw
        ? { ...defaultOpen, ...(JSON.parse(raw) as Record<string, boolean>) }
        : defaultOpen;
    } catch {
      return defaultOpen;
    }
  });

  React.useEffect(() => {
    const defaultOpen = getDefaultOpenState(groups);
    setOpen(prev => {
      let changed = false;
      const next = { ...prev };

      Object.entries(defaultOpen).forEach(([key, value]) => {
        if (!(key in next)) {
          next[key] = value;
          changed = true;
        }
      });

      return changed ? next : prev;
    });
  }, [groups]);

  React.useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(open));
    } catch {
      // ignore storage errors
    }
  }, [open]);

  const toggleGroup = (key: string) => setOpen(prev => ({ ...prev, [key]: !prev[key] }));

  return (
    <aside
      className={cn(
        'hidden shrink-0 border-r border-border/60 bg-background text-sidebar-foreground transition-all duration-300 md:flex md:flex-col',
        isCollapsed ? 'w-12' : 'w-44'
      )}
    >
      {/* Back to console */}
      <div className={cn('shrink-0 px-2 py-2', isCollapsed && 'flex justify-center')}>
        <Link
          href="/console"
          className={cn(
            'flex h-8 shrink-0 items-center rounded-md text-[13px] transition-colors',
            isCollapsed ? 'w-8 justify-center px-0' : 'w-full justify-start gap-2 px-2',
            'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
          )}
          title={t('backToConsole')}
        >
          <ArrowLeft size={16} className="shrink-0 text-foreground/65" />
          <span className={cn('truncate font-normal', isCollapsed && 'hidden')}>
            {t('backToConsole')}
          </span>
        </Link>
      </div>
      {/* Content */}
      <nav
        className={cn(
          'flex flex-1 flex-col gap-1 overflow-y-auto overflow-x-hidden px-2 py-1 scrollbar-none transition-all duration-300',
          isCollapsed ? 'items-center' : 'items-start'
        )}
      >
        {groups.map(group => {
          const Icon = group.icon || Icons.LayoutDashboard;
          const isSingleLink = group.items.length === 1;
          if (isSingleLink) {
            const item = group.items[0];
            const ItemIcon = item.icon || Icon;
            // Use strict match for root dashboard to prevent always-active state
            const isActive =
              group.key === 'root'
                ? pathname === item.href
                : pathname === item.href || pathname.startsWith(item.href + '/');
            return (
              <Link
                key={item.key}
                href={item.href}
                className={cn(
                  'flex shrink-0 items-center rounded-md py-1.5 text-[13px] transition-colors',
                  isCollapsed ? 'w-8 justify-center px-0' : 'w-full justify-start gap-2 px-2',
                  isActive
                    ? 'bg-muted/80 text-foreground'
                    : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
                )}
                title={item.title}
              >
                <ItemIcon
                  size={16}
                  className={cn('shrink-0 text-foreground/65', isActive && 'text-foreground')}
                />
                <span className={cn('truncate font-medium', isCollapsed && 'hidden')}>
                  {item.title}
                </span>
              </Link>
            );
          }

          if (isCollapsed) {
            return group.items.map(item => {
              const ItemIcon = item.icon || Icon;
              const isActive = pathname === item.href || pathname.startsWith(item.href + '/');
              return (
                <Link
                  key={item.key}
                  href={item.href}
                  className={cn(
                    'flex w-8 items-center justify-center rounded-md py-1.5 text-[13px] font-medium transition-colors',
                    isActive
                      ? 'bg-muted/80 text-foreground'
                      : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
                  )}
                  title={item.title}
                >
                  <ItemIcon
                    size={16}
                    className={cn('shrink-0 text-foreground/65', isActive && 'text-foreground')}
                  />
                </Link>
              );
            });
          }

          const isExpanded = open[group.key] ?? false;
          return (
            <div key={group.key} className="w-full pt-2">
              <button
                type="button"
                onClick={() => toggleGroup(group.key)}
                className={cn(
                  'flex w-full items-center justify-between rounded-md px-2 py-1 text-[12px]',
                  'font-medium text-foreground/55 hover:bg-muted/60 hover:text-foreground/80'
                )}
                aria-expanded={isExpanded}
              >
                <span className="truncate">{group.title}</span>
                <ChevronDown
                  className={cn(
                    'h-3.5 w-3.5 shrink-0 text-foreground/45 transition-transform duration-200',
                    !isExpanded && '-rotate-90'
                  )}
                />
              </button>
              {isExpanded && (
                <div className="mt-1 space-y-0.5">
                  {group.items.map(item => {
                    const ItemIcon = item.icon || Icon;
                    const isActive = pathname === item.href || pathname.startsWith(item.href + '/');
                    return (
                      <Link
                        key={item.key}
                        href={item.href}
                        className={cn(
                          'flex items-center gap-2 rounded-md px-2 py-1.5 text-[13px] transition-colors',
                          isActive
                            ? 'bg-muted/80 text-foreground'
                            : 'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
                        )}
                      >
                        <ItemIcon
                          size={16}
                          className={cn(
                            'shrink-0 text-foreground/60',
                            isActive && 'text-foreground'
                          )}
                        />
                        <span className="truncate font-medium">{item.title}</span>
                      </Link>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })}
      </nav>
      <div className={cn('shrink-0 flex p-2 pt-1', isCollapsed && 'justify-center')}>
        <Button
          onClick={toggleCollapse}
          variant="ghost"
          size="xs"
          aria-label={isCollapsed ? tNav('expand') : tNav('collapse')}
          className={cn(
            'flex h-7 items-center rounded-md py-0 text-[13px] font-medium transition-colors gap-0',
            isCollapsed ? 'w-8 justify-center px-0' : 'w-full justify-start px-2',
            'text-foreground/70 hover:bg-muted/70 hover:text-foreground'
          )}
        >
          <ArrowRightToLine
            size={16}
            className={cn(
              'shrink-0 transition-transform duration-300',
              !isCollapsed && 'rotate-180'
            )}
          />
          <span
            className={cn(
              'ml-2 truncate font-normal opacity-100 transition-all duration-300',
              isCollapsed && 'ml-0 hidden w-0 opacity-0'
            )}
          >
            {isCollapsed ? tNav('expand') : tNav('collapse')}
          </span>
        </Button>
      </div>
    </aside>
  );
}

export function DashboardMobileSidebar({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const pathname = usePathname();
  const t = useT('dashboard');
  const navGroups = React.useMemo<NavGroup[]>(() => buildDashboardGroups(t), [t]);
  const [openGroups, setOpenGroups] = React.useState<Record<string, boolean>>(() =>
    getDefaultOpenState(navGroups)
  );

  React.useEffect(() => {
    const defaults = getDefaultOpenState(navGroups);
    setOpenGroups(prev => ({ ...defaults, ...prev }));
  }, [navGroups]);

  const closeSidebar = () => onOpenChange(false);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="left" className="w-[86vw] max-w-80 p-0">
        <SheetHeader className="flex h-14 flex-row items-center justify-between space-y-0 border-b px-4 py-2">
          <SheetTitle className="sr-only">{t('items.dashboard')}</SheetTitle>
          <Logo routerToHome={false} showName={false} />
          <Button variant="ghost" size="sm" onClick={closeSidebar}>
            <X className="h-4 w-4" />
          </Button>
        </SheetHeader>
        <div className="flex h-full flex-col overflow-hidden bg-background">
          <div className="shrink-0 px-3 py-2">
            <Link
              href="/console"
              onClick={closeSidebar}
              className="flex h-9 items-center gap-2 rounded-md px-3 text-sm text-foreground/75 transition-colors hover:bg-muted/70 hover:text-foreground"
            >
              <ArrowLeft className="h-4 w-4 shrink-0" />
              <span className="truncate">{t('backToConsole')}</span>
            </Link>
          </div>
          <nav className="flex-1 space-y-3 overflow-y-auto px-3 py-2">
            {navGroups.map(group => {
              const Icon = group.icon || Icons.LayoutDashboard;
              const isSingleLink = group.items.length === 1;

              if (isSingleLink) {
                const item = group.items[0];
                const ItemIcon = item.icon || Icon;
                const isActive =
                  group.key === 'root'
                    ? pathname === item.href
                    : pathname === item.href || pathname.startsWith(item.href + '/');

                return (
                  <Link
                    key={item.key}
                    href={item.href}
                    onClick={closeSidebar}
                    className={cn(
                      'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                      isActive
                        ? 'bg-muted/80 text-foreground'
                        : 'text-foreground/75 hover:bg-muted/70 hover:text-foreground'
                    )}
                  >
                    <ItemIcon className="h-4 w-4 shrink-0" />
                    <span className="truncate">{item.title}</span>
                  </Link>
                );
              }

              const isExpanded = openGroups[group.key] ?? false;
              return (
                <div key={group.key}>
                  <button
                    type="button"
                    onClick={() =>
                      setOpenGroups(prev => ({ ...prev, [group.key]: !(prev[group.key] ?? false) }))
                    }
                    className="flex w-full items-center justify-between rounded-md px-3 py-1 text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground hover:bg-muted/60 hover:text-foreground/80"
                  >
                    <span className="truncate">{group.title}</span>
                    <ChevronDown
                      className={cn(
                        'h-3.5 w-3.5 shrink-0 transition-transform duration-200',
                        !isExpanded && '-rotate-90'
                      )}
                    />
                  </button>
                  {isExpanded ? (
                    <div className="mt-1 space-y-0.5">
                      {group.items.map(item => {
                        const ItemIcon = item.icon || Icon;
                        const isActive = pathname === item.href || pathname.startsWith(item.href + '/');
                        return (
                          <Link
                            key={item.key}
                            href={item.href}
                            onClick={closeSidebar}
                            className={cn(
                              'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
                              isActive
                                ? 'bg-muted/80 text-foreground'
                                : 'text-foreground/75 hover:bg-muted/70 hover:text-foreground'
                            )}
                          >
                            <ItemIcon className="h-4 w-4 shrink-0" />
                            <span className="truncate">{item.title}</span>
                          </Link>
                        );
                      })}
                    </div>
                  ) : null}
                </div>
              );
            })}
          </nav>
        </div>
      </SheetContent>
    </Sheet>
  );
}
