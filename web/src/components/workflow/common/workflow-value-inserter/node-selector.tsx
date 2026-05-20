import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ChevronDown } from 'lucide-react';
import NodeTab from './node-tab';
import type { WorkflowVariableCatalogGroup } from '../../hooks';

export interface NodeSelectorProps {
  upstreamNodes: WorkflowVariableCatalogGroup[];
  activeNodeId: string | null;
  onSelect: (nodeId: string) => void;
  maxTabsWidth?: number;
  className?: string;
}

/**
 * NodeSelector - renders node tabs with overflow into a dropdown, with width measurement.
 */
const NodeSelector: React.FC<NodeSelectorProps> = ({
  upstreamNodes,
  activeNodeId,
  onSelect,
  maxTabsWidth,
  className,
}) => {
  const t = useT();

  const getDisplayTitle = (node: WorkflowVariableCatalogGroup) => node.sourceTitle;

  const [visibleNodes, setVisibleNodes] = useState<WorkflowVariableCatalogGroup[]>([]);
  const [overflowNodes, setOverflowNodes] = useState<WorkflowVariableCatalogGroup[]>([]);
  const tabsContainerRef = useRef<HTMLDivElement>(null);
  const measureRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const moreMeasureRef = useRef<HTMLDivElement>(null);

  const calculateVisibleNodes = useCallback(() => {
    if (!tabsContainerRef.current || upstreamNodes.length === 0) {
      setVisibleNodes(upstreamNodes);
      setOverflowNodes([]);
      return;
    }

    const containerEl = tabsContainerRef.current;
    const containerWidth = containerEl.offsetWidth;
    const baseWidth =
      typeof maxTabsWidth === 'number' && maxTabsWidth > 0
        ? Math.min(containerWidth, maxTabsWidth)
        : containerWidth;
    const styles = window.getComputedStyle(containerEl);
    const parsePx = (val: string | null) => {
      if (!val) return 0;
      const n = Number.parseFloat(val);
      return Number.isFinite(n) ? n : 0;
    };
    const gapPx = Math.max(0, parsePx(styles.columnGap || styles.gap));
    const paddingLeft = parsePx(styles.paddingLeft);
    const paddingRight = parsePx(styles.paddingRight);
    const horizontalPadding = paddingLeft + paddingRight;
    const safetyPx = 12;
    const effectiveWidth = Math.max(0, baseWidth - horizontalPadding - safetyPx);

    const compute = (maxWidth: number) => {
      let currentWidth = 0;
      let count = 0;
      const visible: WorkflowVariableCatalogGroup[] = [];
      const overflow: WorkflowVariableCatalogGroup[] = [];

      upstreamNodes.forEach(node => {
        const measureEl = measureRefs.current
          .get(node.sourceId)
          ?.querySelector('button') as HTMLButtonElement | null;
        const tabWidth = measureEl ? measureEl.offsetWidth : 120;

        const nextWidth = currentWidth + tabWidth + (count > 0 ? gapPx : 0);
        if (nextWidth <= maxWidth && visible.length < 6) {
          visible.push(node);
          currentWidth = nextWidth;
          count++;
        } else {
          overflow.push(node);
        }
      });

      return { visible, overflow };
    };

    let { visible, overflow } = compute(effectiveWidth);

    if (overflow.length > 0) {
      const moreBtn = moreMeasureRef.current?.querySelector('button') as HTMLButtonElement | null;
      const moreWidth = moreBtn ? moreBtn.offsetWidth : 60;
      const reserveMore = moreWidth + (visible.length > 0 ? gapPx : 0);
      const { visible: v2, overflow: o2 } = compute(Math.max(0, effectiveWidth - reserveMore));
      visible = v2;
      overflow = o2;
    }

    setVisibleNodes(visible);
    setOverflowNodes(overflow);
  }, [upstreamNodes, maxTabsWidth]);

  useEffect(() => {
    calculateVisibleNodes();

    const handleResize = () => calculateVisibleNodes();
    window.addEventListener('resize', handleResize);
    let ro: ResizeObserver | null = null;
    if (tabsContainerRef.current && 'ResizeObserver' in window) {
      ro = new ResizeObserver(() => {
        calculateVisibleNodes();
      });
      ro.observe(tabsContainerRef.current);
    }

    return () => {
      window.removeEventListener('resize', handleResize);
      ro?.disconnect();
    };
  }, [calculateVisibleNodes]);

  if (upstreamNodes.length === 0) return null;

  return (
    <div className={cn('border rounded-md bg-background', className)}>
      <div ref={tabsContainerRef} className="flex items-center gap-2 p-1 min-h-9">
        {/* Hidden measurement container */}
        <div className="absolute opacity-0 pointer-events-none -z-10">
          {upstreamNodes.map(node => (
            <div
              key={`measure-${node.sourceId}`}
              ref={el => {
                if (el) {
                  measureRefs.current.set(node.sourceId, el);
                } else {
                  measureRefs.current.delete(node.sourceId);
                }
              }}
            >
              <Button
                variant="outline"
                size="sm"
                className="h-7 px-3 text-xs font-medium rounded-sm"
              >
                {getDisplayTitle(node)}
              </Button>
            </div>
          ))}
          <div ref={moreMeasureRef}>
            <Button variant="ghost" size="sm" className="h-7 px-3 text-xs rounded-sm">
              <span>{t('common.more')}</span>
              <ChevronDown className="h-3 w-3 ml-1" />
            </Button>
          </div>
        </div>

        {/* Visible node tabs */}
        {visibleNodes.map(node => (
          <NodeTab
            key={node.sourceId}
            node={{
              ...node,
              sourceTitle: getDisplayTitle(node),
            }}
            isActive={activeNodeId === node.sourceId}
            onClick={() => onSelect(node.sourceId)}
            ariaLabel={t('nodes.valueInserter.aria.nodeTab', {
              title: getDisplayTitle(node),
            })}
          />
        ))}

        {/* More dropdown for overflow nodes */}
        {overflowNodes.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger className="ml-auto" asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 px-3 text-xs rounded-sm"
                aria-label={t('nodes.valueInserter.aria.more')}
              >
                <span className="ml-1">{t('common.more')}</span>
                <ChevronDown className="h-3 w-3 ml-1" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              {overflowNodes.map(node => (
                <DropdownMenuItem
                  key={node.sourceId}
                  onClick={() => onSelect(node.sourceId)}
                  className={cn('cursor-pointer', activeNodeId === node.sourceId && 'bg-accent')}
                >
                  {getDisplayTitle(node)}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </div>
  );
};

export default React.memo(NodeSelector);
