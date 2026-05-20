'use client';

import * as React from 'react';
import Link from 'next/link';
import { ArrowLeft, PanelLeftClose, PanelLeftOpen, Pencil } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

export interface ResourceSidebarNavItem {
  title: string;
  href: string;
  icon: LucideIcon;
  children?: ResourceSidebarNavItem[];
  isActive?: (pathname: string) => boolean;
}

interface ResourceSidebarProps {
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  expandLabel: string;
  collapseLabel: string;
  header: React.ReactNode;
  navItems?: ResourceSidebarNavItem[];
  pathname?: string;
  isNavigationHidden?: boolean;
  expandedWidthClassName?: string;
  children?: React.ReactNode;
}

interface ResourceSidebarHeaderProps {
  isCollapsed: boolean;
  isLoading?: boolean;
  loadingLabel?: string;
  iconType?: string | null;
  icon?: string;
  iconBackground: string;
  iconSrc?: string;
  name?: string;
  description?: string;
  showIdentity?: boolean;
  backHref?: string;
  backLabel?: string;
  iconActionLabel?: string;
  onIconClick?: () => void;
}

interface ResourceSidebarChromeContextValue {
  onToggleCollapse: () => void;
  toggleLabel: string;
  ToggleIcon: LucideIcon;
}

const ResourceSidebarChromeContext = React.createContext<ResourceSidebarChromeContextValue | null>(
  null
);

/**
 * @component ResourceSidebar
 * @category Common
 * @status Stable
 * @description Shared desktop sidebar shell for resource detail pages with an edge collapse handle, header, and optional navigation.
 * @usage Use for agent, dataset, and database detail sidebars that need consistent collapse behavior.
 * @example
 * <ResourceSidebar header={<ResourceSidebarHeader ... />} navItems={items} />
 */
export function ResourceSidebar({
  isCollapsed,
  onToggleCollapse,
  expandLabel,
  collapseLabel,
  header,
  navItems = [],
  pathname = '',
  isNavigationHidden = false,
  expandedWidthClassName = 'w-36',
  children,
}: ResourceSidebarProps) {
  const toggleLabel = isCollapsed ? expandLabel : collapseLabel;
  const ToggleIcon = isCollapsed ? PanelLeftOpen : PanelLeftClose;

  return (
    <aside
      className={cn(
        'relative hidden md:flex md:flex-col shrink-0 border-r border-border bg-background text-sidebar-foreground transition-all duration-300',
        isCollapsed ? 'w-11' : expandedWidthClassName
      )}
    >
      <ResourceSidebarChromeContext.Provider value={{ onToggleCollapse, toggleLabel, ToggleIcon }}>
        {header}
      </ResourceSidebarChromeContext.Provider>

      {isNavigationHidden ? <div className="flex-1" /> : null}

      {!isNavigationHidden && navItems.length > 0 ? (
        <nav className="flex flex-1 flex-col gap-[3px] px-1 py-2 items-center">
          {navItems.map(item => {
            const Icon = item.icon;
            const isActive = item.isActive
              ? item.isActive(pathname)
              : pathname === item.href || pathname.startsWith(`${item.href}/`);

            return (
              <div key={item.href} className="w-full">
                <Link
                  href={item.href}
                  className={cn(
                    'relative flex items-center rounded-md py-1.5 text-xs font-medium transition-colors',
                    isActive
                      ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                      : 'text-muted-foreground hover:bg-background/70 hover:text-foreground',
                    isCollapsed ? 'justify-center px-0 w-8' : 'px-2 w-full'
                  )}
                >
                  {isActive && !isCollapsed ? (
                    <span
                      className="absolute bottom-1.5 left-0 top-1.5 w-0.5 rounded-r-full bg-foreground/70"
                      aria-hidden="true"
                    />
                  ) : null}
                  <Icon size={18} className="shrink-0" />
                  <span
                    className={cn(
                      'truncate font-normal opacity-100 transition-all duration-300',
                      isCollapsed ? 'ml-0 w-0 overflow-hidden opacity-0' : 'ml-1 w-full'
                    )}
                  >
                    {item.title}
                  </span>
                </Link>
                {!isCollapsed && item.children?.length ? (
                  <div className="mt-1 flex flex-col gap-[3px] pl-3">
                    {item.children.map(child => {
                      const ChildIcon = child.icon;
                      const isChildActive = child.isActive
                        ? child.isActive(pathname)
                        : pathname === child.href || pathname.startsWith(`${child.href}/`);

                      return (
                        <Link
                          key={child.href}
                          href={child.href}
                          className={cn(
                            'relative flex w-full items-center rounded-md px-2 py-1.5 text-xs transition-colors',
                            isChildActive
                              ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                              : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                          )}
                        >
                          {isChildActive ? (
                            <span
                              className="absolute bottom-1.5 left-0 top-1.5 w-0.5 rounded-r-full bg-foreground/70"
                              aria-hidden="true"
                            />
                          ) : null}
                          <ChildIcon size={15} className="shrink-0" />
                          <span className="ml-1 truncate font-normal">{child.title}</span>
                        </Link>
                      );
                    })}
                  </div>
                ) : null}
              </div>
            );
          })}
        </nav>
      ) : null}

      {!isNavigationHidden ? children : null}
    </aside>
  );
}

/**
 * @component ResourceSidebarHeader
 * @category Common
 * @status Stable
 * @description Shared resource identity header with icon, name, description, and loading state.
 * @usage Place inside ResourceSidebar header prop.
 * @example
 * <ResourceSidebarHeader isCollapsed={collapsed} name={name} iconBackground={ICON_BG} />
 */
export function ResourceSidebarHeader({
  isCollapsed,
  isLoading = false,
  loadingLabel,
  iconType,
  icon,
  iconBackground,
  iconSrc,
  name,
  description,
  showIdentity = true,
  backHref,
  backLabel,
  iconActionLabel,
  onIconClick,
}: ResourceSidebarHeaderProps) {
  const chrome = React.useContext(ResourceSidebarChromeContext);
  const t = useT('common');
  const displayDescription = description?.trim() || t('noDescription');

  if (isLoading) {
    return (
      <div
        className={cn(
          'flex items-center border-b border-border',
          isCollapsed ? 'justify-center gap-0 px-1 py-1.5' : 'gap-2 px-1.5 py-2'
        )}
      >
        <Skeleton className="h-9 w-9" />
        <div className={cn('min-w-0 flex-1 space-y-1 transition-all', isCollapsed && 'hidden')}>
          <Skeleton className="h-3.5 w-20" />
          <Skeleton className="h-2.5 w-24" />
        </div>
      </div>
    );
  }

  const iconPreview = (
    <IconPreview
      iconType={iconType === 'image' ? 'image' : 'text'}
      src={iconType === 'image' ? iconSrc : ''}
      icon={icon}
      iconBackground={iconBackground}
      editable={false}
      size={isCollapsed ? 'sidebar' : 'sidebarExpanded'}
    />
  );

  const titleBlock = (
    <div className={cn('min-w-0 transition-all px-0.5', isCollapsed && 'hidden')}>
      <div className="truncate text-[13px] font-semibold leading-4" title={name}>
        {name || loadingLabel || '-'}
      </div>
      <div
        className="mt-0.5 line-clamp-2 break-all text-[11px] leading-[15px] text-muted-foreground"
        title={displayDescription}
      >
        {displayDescription}
      </div>
    </div>
  );

  if (!isCollapsed) {
    return (
      <div className="border-b border-border px-1.5 py-2">
        <div className="flex min-w-0 flex-col gap-2 rounded-md text-left">
          <div className="flex min-w-0 items-center justify-between gap-1">
            {backHref && backLabel ? (
              <Link
                href={backHref}
                aria-label={backLabel}
                title={backLabel}
                className="flex h-7 min-w-0 items-center gap-1 rounded-[4px] px-1.5 text-xs text-muted-foreground transition-colors hover:bg-primary/5 hover:text-primary"
              >
                <ArrowLeft size={14} className="shrink-0" />
                <span className="truncate">{backLabel}</span>
              </Link>
            ) : (
              <div />
            )}
            {chrome ? (
              <Button
                type="button"
                variant="ghost"
                isIcon
                size="sm"
                aria-label={chrome.toggleLabel}
                title={chrome.toggleLabel}
                onClick={chrome.onToggleCollapse}
                className="h-7 w-7 shrink-0 rounded-[4px] bg-transparent p-0 shadow-none transition-colors hover:bg-primary/5 hover:text-primary"
              >
                <chrome.ToggleIcon size={16} className="shrink-0" />
              </Button>
            ) : null}
          </div>
          {showIdentity ? (
            <div className="flex min-w-0 items-start justify-between gap-2 px-0.5">
              <div className="min-w-0 flex-1">{titleBlock}</div>
              <div className="flex shrink-0 items-start gap-1">
                <div className="shrink-0">{iconPreview}</div>
                <div className="flex shrink-0 items-center gap-1">
                  {onIconClick ? (
                    <Button
                      type="button"
                      variant="ghost"
                      isIcon
                      size="sm"
                      aria-label={iconActionLabel}
                      title={iconActionLabel}
                      onClick={onIconClick}
                      className="h-7 w-7 rounded-md bg-transparent p-0 shadow-none transition-colors hover:bg-primary/5 hover:text-primary"
                    >
                      <Pencil size={16} className="shrink-0" />
                    </Button>
                  ) : null}
                </div>
              </div>
            </div>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <div className="border-b border-border px-0.5 py-1">
      <div className="flex w-10 flex-col items-center gap-1">
        <div className="flex flex-col items-center gap-1">
          {backHref && backLabel ? (
            <Link
              href={backHref}
              aria-label={backLabel}
              title={backLabel}
              className="flex h-7 w-7 items-center justify-center rounded-[4px] text-muted-foreground transition-colors hover:bg-primary/5 hover:text-primary"
            >
              <ArrowLeft className="h-4 w-4" />
            </Link>
          ) : null}
          {chrome ? (
            <Button
              type="button"
              variant="ghost"
              isIcon
              size="sm"
              aria-label={chrome.toggleLabel}
              title={chrome.toggleLabel}
              onClick={chrome.onToggleCollapse}
              className="h-7 w-7 rounded-[4px] bg-transparent p-0 text-muted-foreground shadow-none transition-colors hover:bg-primary/5 hover:text-primary"
            >
              <chrome.ToggleIcon className="h-4 w-4" />
            </Button>
          ) : null}
        </div>
        {showIdentity ? (
          onIconClick ? (
            <button
              type="button"
              aria-label={iconActionLabel}
              title={iconActionLabel}
              onClick={onIconClick}
              className="flex h-9 w-9 items-center justify-center rounded-md p-0 transition-colors hover:bg-primary/5 hover:text-primary"
            >
              {iconPreview}
            </button>
          ) : (
            iconPreview
          )
        ) : null}
      </div>
    </div>
  );
}
