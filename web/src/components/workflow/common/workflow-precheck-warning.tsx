'use client';

import React, { useEffect, useMemo, useState } from 'react';
import { X } from 'lucide-react';
import type { WorkflowPrecheckWarning } from '@/services/types/workflow';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import type { WorkflowBillingTranslationScope } from '@/utils/workflow/billing';
import {
  getWorkflowPrecheckNoticeCollapsed,
  setWorkflowPrecheckNoticeCollapsed,
} from '@/utils/ui-local';
import { cn } from '@/lib/utils';

interface WorkflowPrecheckWarningBannerProps {
  warnings: WorkflowPrecheckWarning[];
  scope: WorkflowBillingTranslationScope;
  storageKey: string;
  placement?: 'inline' | 'floating';
  className?: string;
  floatingOffset?: number;
}

/**
 * @component WorkflowPrecheckWarningBanner
 * @category Feature
 * @status Stable
 * @description Displays non-blocking workflow precheck warnings before a run starts.
 * @usage Render above run/chat surfaces when precheck returns warning status.
 * @example
 * <WorkflowPrecheckWarningBanner warnings={warnings} scope="agents" />
 */
export function WorkflowPrecheckWarningBanner({
  warnings,
  scope,
  storageKey,
  placement = 'floating',
  className,
  floatingOffset = 8,
}: WorkflowPrecheckWarningBannerProps) {
  const { getPrecheckWarningViews } = useWorkflowBillingFeedback(scope);
  const items = useMemo(
    () => getPrecheckWarningViews(warnings),
    [getPrecheckWarningViews, warnings]
  );
  const [collapsed, setCollapsed] = useState<boolean>(() =>
    getWorkflowPrecheckNoticeCollapsed(storageKey, false)
  );

  useEffect(() => {
    setCollapsed(getWorkflowPrecheckNoticeCollapsed(storageKey, false));
  }, [storageKey]);

  if (items.length === 0) {
    return null;
  }

  const resolvedFloatingOffset = Math.max(0, floatingOffset);

  const summary = items.map(item => `${item.title}: ${item.description}`).join('  ');

  const handleCollapse = () => {
    setCollapsed(true);
    setWorkflowPrecheckNoticeCollapsed(storageKey, true);
  };

  const handleExpand = () => {
    setCollapsed(false);
    setWorkflowPrecheckNoticeCollapsed(storageKey, false);
  };

  return (
    <div className={cn('pointer-events-none z-20', className)}>
      <button
        type="button"
        onClick={handleExpand}
        className={cn(
          'pointer-events-auto absolute right-2 top-2 flex h-5 w-5 items-center justify-center rounded-full border border-amber-300 bg-amber-500 text-[11px] font-black text-white shadow-sm transition-all duration-300 ease-out hover:scale-105 hover:bg-amber-600',
          collapsed
            ? 'translate-y-0 scale-100 opacity-100'
            : 'pointer-events-none -translate-y-1 scale-75 opacity-0'
        )}
        aria-label="Expand warning"
      >
        !
      </button>

        <div
        className={cn(
          'overflow-hidden transition-all duration-300 [transition-timing-function:cubic-bezier(0.22,1,0.36,1)]',
          placement === 'floating' ? 'absolute left-0 right-0' : 'relative mb-2',
          collapsed ? 'max-h-0 -translate-y-1 opacity-0' : 'max-h-14 translate-y-0 opacity-100'
        )}
        style={
          placement === 'floating'
            ? {
                bottom: `calc(100% + var(--workflow-precheck-floating-offset, ${resolvedFloatingOffset}px))`,
              }
            : undefined
        }
      >
        <div className="pointer-events-auto flex h-8 items-center gap-2 rounded-full border border-amber-200/90 bg-[linear-gradient(90deg,rgba(255,247,237,0.98),rgba(255,251,235,0.98))] px-3 pr-2 text-[11px] text-amber-950 shadow-[0_8px_24px_rgba(245,158,11,0.12)] backdrop-blur">
          <div className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-amber-500 text-[10px] font-black text-white">
            !
          </div>
          <p className="min-w-0 flex-1 truncate whitespace-nowrap font-medium">{summary}</p>
          <button
            type="button"
            onClick={handleCollapse}
            className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-amber-700 transition-colors hover:bg-amber-100 hover:text-amber-900"
            aria-label="Collapse warning"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}
