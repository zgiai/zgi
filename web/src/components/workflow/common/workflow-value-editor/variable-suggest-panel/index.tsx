import { useEffect, useMemo, useRef } from 'react';
import FloatingPanel from '@/components/ui/floating-panel';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { ChevronLeft, ChevronRight } from 'lucide-react';

type FloatingPanelPortalRoot = React.ComponentProps<typeof FloatingPanel>['portalRoot'];

// Local structural type compatible with VarOption used by editor
export interface VarOption {
  sourceId: string;
  sourceTitle: string;
  key: string;
  insertKey?: string;
  type: string;
  showType?: boolean;
  /** Pre-resolved description text for tooltip (caller should translate if needed) */
  description?: string;
  hasChildren?: boolean;
  displayKey?: string;
}

export interface VariableSuggestPanelProps {
  open: boolean;
  x: number;
  y: number;
  groups: Array<{ title: string; items: VarOption[] }>;
  activeGroupIndex: number;
  activeItemIndex: number;
  onHover: (groupIndex: number, itemIndex: number) => void;
  onOpenChange?: (open: boolean) => void;
  onSelect: (item: VarOption) => void;
  onExpand?: (item: VarOption) => void;
  onBack?: () => void;
  suggestPath?: string[];
  ownerId: string;
  portalRoot?: FloatingPanelPortalRoot;
  labels: {
    empty: string;
  };
}

// Pure UI component for rendering variable suggestion dropdown
export default function VariableSuggestPanel(props: VariableSuggestPanelProps) {
  const {
    open,
    x,
    y,
    groups,
    activeGroupIndex,
    activeItemIndex,
    onHover,
    onOpenChange,
    onSelect,
    onExpand,
    onBack,
    suggestPath = [],
    ownerId,
    portalRoot,
  } = props;
  const hasActive = activeGroupIndex >= 0 && activeItemIndex >= 0;
  const activeKey = hasActive ? `g${activeGroupIndex}-i${activeItemIndex}` : '';
  const { labels } = props;
  const listRef = useRef<HTMLDivElement | null>(null);
  const pathKey = suggestPath.join('\0');
  const groupKey = useMemo(
    () =>
      groups
        .map(group => `${group.title}:${group.items.map(item => item.key).join(',')}`)
        .join('|'),
    [groups]
  );

  useEffect(() => {
    if (!open) return;
    if (listRef.current) {
      listRef.current.scrollTop = 0;
    }
  }, [open, pathKey, groupKey]);

  // Ensure the active item is scrolled into view when using keyboard navigation
  useEffect(() => {
    if (!open || !hasActive) return;
    try {
      const container = listRef.current;
      const activeEl = container?.querySelector(
        '[data-wf-suggest-item][data-active="true"]'
      ) as HTMLElement | null;
      if (!container || !activeEl) return;

      const cRect = container.getBoundingClientRect();
      const iRect = activeEl.getBoundingClientRect();
      if (iRect.top < cRect.top) {
        container.scrollTop -= cRect.top - iRect.top + 4;
      } else if (iRect.bottom > cRect.bottom) {
        container.scrollTop += iRect.bottom - cRect.bottom + 4;
      }
    } catch (err) {
      console.error('Failed to keep active suggestion visible:', err);
    }
    // Re-run when selection changes while panel is open
  }, [open, hasActive, activeKey]);

  return (
    <FloatingPanel
      open={open}
      onOpenChange={onOpenChange || (() => {})}
      x={x}
      y={y}
      portalRoot={portalRoot}
      maxWidth={420}
      maxHeight={280}
      className="overflow-hidden p-0"
      role="tooltip"
    >
      <div
        data-wf-suggest="open"
        data-wf-suggest-owner={ownerId}
        className="flex h-full min-h-0 flex-col"
      >
        {/* Header/Breadcrumb (Hide if only sourceId is present) */}
        {suggestPath.length > 1 && (
          <div className="sticky top-0 z-10 flex items-center gap-2 border-b bg-popover/95 p-2 backdrop-blur">
            <button
              onClick={e => {
                e.preventDefault();
                e.stopPropagation();
                onBack?.();
              }}
              className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              aria-label="Back"
            >
              <ChevronLeft className="size-3.5" />
            </button>
            <div className="flex min-w-0 items-center gap-1 truncate text-xs font-medium text-muted-foreground">
              <span>{suggestPath[0]}</span>
              {suggestPath.length > 1 && (
                <>
                  <ChevronRight className="size-3 shrink-0 text-muted-foreground/60" />
                  <span className="truncate text-foreground">{suggestPath.at(-1)}</span>
                </>
              )}
            </div>
          </div>
        )}

        <div
          ref={listRef}
          data-wf-suggest-list="true"
          className="min-h-0 flex-1 overflow-y-auto p-1"
        >
          {groups.length === 0 || groups.every(g => g.items.length === 0) ? (
            <div className="px-2 py-6 text-center text-xs text-muted-foreground">
              {labels.empty}
            </div>
          ) : (
            groups.map((g, gi) => (
              <div key={g.title} className="space-y-1">
                {g.title && (
                  <div className="sticky top-0 z-10 bg-popover/95 px-2 py-1 text-xs font-semibold text-muted-foreground backdrop-blur">
                    {g.title}
                  </div>
                )}
                {g.items.map((it, ii) => {
                  const key = `g${gi}-i${ii}`;
                  const isActive = key === activeKey;
                  const itemContent = (
                    <div
                      key={key}
                      data-wf-suggest-item
                      role="option"
                      aria-selected={isActive}
                      data-active={isActive}
                      className={cn(
                        'flex cursor-pointer items-stretch rounded-sm text-sm outline-none overflow-hidden pr-1',
                        isActive
                          ? 'bg-accent text-accent-foreground'
                          : 'hover:bg-accent hover:text-accent-foreground'
                      )}
                      onMouseEnter={() => onHover(gi, ii)}
                    >
                      {/* Left: Select Area */}
                      <div
                        className={cn(
                          'flex-1 flex min-w-0 items-start gap-2 py-2',
                          it.hasChildren ? 'px-2' : 'pl-2 pr-2'
                        )}
                        onMouseDown={e => {
                          e.preventDefault();
                          e.stopPropagation();
                          onSelect(it);
                        }}
                      >
                        <div className="min-w-0 flex-1">
                          <div className="flex min-w-0 items-center gap-2">
                            <span className="truncate text-[13px] font-medium leading-4">
                              {it.displayKey || it.key}
                            </span>
                            {it.showType === false ? null : (
                              <span className="ml-auto shrink-0 text-[10px] uppercase text-muted-foreground tabular-nums">
                                {it.type}
                              </span>
                            )}
                          </div>
                          {it.description ? (
                            <div className="mt-0.5 truncate text-xs leading-4 text-muted-foreground">
                              {it.description}
                            </div>
                          ) : null}
                        </div>
                      </div>

                      {/* Right: Expand Area (if has children) */}
                      {it.hasChildren && (
                        <div
                          className="ml-px flex w-8 shrink-0 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-black/5 hover:text-foreground dark:hover:bg-white/10"
                          onMouseDown={e => {
                            e.preventDefault();
                            e.stopPropagation();
                            onExpand?.(it);
                          }}
                          title="Enter"
                        >
                          <ChevronRight className="size-3 text-muted-foreground" />
                        </div>
                      )}
                    </div>
                  );
                  return it.description ? (
                    <Tooltip key={key}>
                      <TooltipTrigger asChild>{itemContent}</TooltipTrigger>
                      <TooltipContent side="right" className="max-w-xs text-xs">
                        <div className="text-muted-foreground">{it.description}</div>
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    itemContent
                  );
                })}
              </div>
            ))
          )}
        </div>
      </div>
    </FloatingPanel>
  );
}
