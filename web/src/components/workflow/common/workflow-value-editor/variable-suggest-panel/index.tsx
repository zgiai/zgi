import { useEffect } from 'react';
import FloatingPanel from '@/components/ui/floating-panel';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { ChevronRight } from 'lucide-react';

type FloatingPanelPortalRoot = React.ComponentProps<typeof FloatingPanel>['portalRoot'];

// Local structural type compatible with VarOption used by editor
export interface VarOption {
  sourceId: string;
  sourceTitle: string;
  key: string;
  type: string;
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
    portalRoot,
  } = props;
  const hasActive = activeGroupIndex >= 0 && activeItemIndex >= 0;
  const activeKey = hasActive ? `g${activeGroupIndex}-i${activeItemIndex}` : '';
  const { labels } = props;

  // Ensure the active item is scrolled into view when using keyboard navigation
  useEffect(() => {
    if (!open || !hasActive) return;
    try {
      // Find the current active suggestion item within the open suggest container
      const container = document.querySelector(
        'div[data-wf-suggest="open"]'
      ) as HTMLDivElement | null;
      const activeEl = container?.querySelector(
        '[data-wf-suggest-item][data-active="true"]'
      ) as HTMLElement | null;
      if (!activeEl) return;

      // Find nearest scrollable ancestor (prefer FloatingPanel root)
      const getScrollContainer = (el: HTMLElement | null): HTMLElement | null => {
        let cur: HTMLElement | null = el;
        while (cur && cur !== document.body) {
          const style = getComputedStyle(cur);
          const overflowY = style.overflowY || style.overflow;
          const canScroll = /(auto|scroll)/.test(overflowY);
          if (canScroll && cur.scrollHeight > cur.clientHeight) return cur;
          cur = cur.parentElement as HTMLElement | null;
        }
        return null;
      };

      const scrollContainer =
        getScrollContainer(activeEl) ||
        (activeEl.closest('[data-slot="floating-panel"]') as HTMLElement | null);
      if (scrollContainer) {
        const cRect = scrollContainer.getBoundingClientRect();
        const iRect = activeEl.getBoundingClientRect();
        // Scroll up or down only as needed to keep the active element fully visible
        if (iRect.top < cRect.top) {
          scrollContainer.scrollTop -= cRect.top - iRect.top + 4; // small head margin
        } else if (iRect.bottom > cRect.bottom) {
          scrollContainer.scrollTop += iRect.bottom - cRect.bottom + 4; // small tail margin
        }
      } else {
        // Fallback to native behavior if no explicit container was found
        activeEl.scrollIntoView({ block: 'nearest' });
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
      className="w-[360px] max-h-[260px] overflow-y-auto"
      role="tooltip"
    >
      <div data-wf-suggest="open" className="flex flex-col h-full">
        {/* Header/Breadcrumb (Hide if only sourceId is present) */}
        {suggestPath.length > 1 && (
          <div className="flex items-center gap-1 p-2 border-b bg-muted/30 sticky top-0 z-10">
            <button
              onClick={e => {
                e.preventDefault();
                e.stopPropagation();
                onBack?.();
              }}
              className="p-1 hover:bg-accent rounded-md transition-colors"
            >
              <ChevronRight className="size-3 rotate-180" />
            </button>
            <div className="flex items-center gap-1 text-[10px] text-muted-foreground truncate font-medium">
              <span>{suggestPath[0]}</span>
              {suggestPath.length > 1 && (
                <span className="text-highlight">({suggestPath.slice(1).join('.')})</span>
              )}
            </div>
          </div>
        )}

        <div className="flex-1 overflow-y-auto">
          {groups.length === 0 || groups.every(g => g.items.length === 0) ? (
            <div className="px-2 py-6 text-center text-xs text-muted-foreground">
              {labels.empty}
            </div>
          ) : (
            groups.map((g, gi) => (
              <div key={g.title} className="mt-1 space-y-1">
                {g.title && (
                  <div className="px-2 py-1 text-xs text-muted-foreground font-bold">{g.title}</div>
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
                        'flex cursor-pointer items-center rounded-sm text-sm outline-none overflow-hidden pr-1',
                        isActive
                          ? 'bg-accent text-accent-foreground'
                          : 'hover:bg-accent hover:text-accent-foreground'
                      )}
                      onMouseEnter={() => onHover(gi, ii)}
                    >
                      {/* Left: Select Area */}
                      <div
                        className={cn(
                          'flex-1 flex items-center gap-2 py-1 truncate min-w-0',
                          it.hasChildren ? 'px-2' : 'pl-2'
                        )}
                        onMouseDown={e => {
                          e.preventDefault();
                          e.stopPropagation();
                          onSelect(it);
                        }}
                      >
                        <span className="font-medium truncate">{it.displayKey || it.key}</span>
                        <span className="ml-auto text-[10px] uppercase text-muted-foreground shrink-0 tabular-nums">
                          {it.type}
                        </span>
                      </div>

                      {/* Right: Expand Area (if has children) */}
                      {it.hasChildren && (
                        <div
                          className="p-1 hover:bg-black/5 dark:hover:bg-white/10 rounded-sm ml-px shrink-0 transition-colors"
                          onMouseDown={e => {
                            e.preventDefault();
                            e.stopPropagation();
                            onExpand?.(it);
                          }}
                          title="Expand fields"
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
