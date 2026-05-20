'use client';

import * as React from 'react';
import { Sheet, SheetContent } from '@/components/ui/sheet';
import { cn } from '@/lib/utils';

interface TaskPanelHostProps {
  open: boolean;
  isMobile: boolean;
  onOpenChange: (open: boolean) => void;
  children: React.ReactNode;
}

/**
 * @component TaskPanelHost
 * @category Feature
 * @status Stable
 * @description Shared host for the automation detail and editor panel across desktop and mobile layouts.
 * @usage Wrap panel content so the same content renders in an aside on desktop and a sheet on mobile.
 */
export function TaskPanelHost({
  open,
  isMobile,
  onOpenChange,
  children,
}: TaskPanelHostProps) {
  if (isMobile) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          side="left"
          className="w-full max-w-none gap-0 overflow-hidden p-0"
          showClose={false}
        >
          {open ? children : null}
        </SheetContent>
      </Sheet>
    );
  }

  if (!open) {
    return null;
  }

  return (
    <div className="pointer-events-none absolute inset-y-5 right-5 z-30 hidden md:block">
      <aside
        data-task-panel="true"
        className={cn(
          'pointer-events-auto h-full w-[min(42vw,560px)] overflow-hidden rounded-[28px] border border-border bg-background shadow-[0_24px_80px_rgba(15,23,42,0.14)]'
        )}
      >
        {children}
      </aside>
    </div>
  );
}
