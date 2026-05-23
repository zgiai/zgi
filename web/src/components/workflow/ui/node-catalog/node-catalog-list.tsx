'use client';

import React from 'react';
import { Plus } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useWorkflowStore } from '../../store';
import type { NodeGroupKey, NodeType } from '../create-node-modal/constants/node-types';
import {
  DEFAULT_DRAG_PREVIEW_ANCHOR,
  getDragClientPosition,
  setTransparentDragImage,
} from '../node-left-panel/drag-preview-utils';
import {
  createDragPreviewNodeData,
  resolveDragPreviewNodeTitle,
} from '../node-left-panel/drag-preview-data';
import type { CatalogDensity, CatalogInteraction, CatalogTooltipSide } from './types';

interface NodeCatalogListLabels {
  group: Record<NodeGroupKey, string>;
  noAvailable: string;
}

interface NodeCatalogListProps {
  grouped: Array<{ key: NodeGroupKey; items: NodeType[] }>;
  labels: NodeCatalogListLabels;
  onSelect: (type: string) => void;
  density?: CatalogDensity;
  interaction?: CatalogInteraction;
  tooltipSide?: CatalogTooltipSide;
  className?: string;
}

const densityClassNames = {
  panel: {
    root: 'space-y-4',
    group: 'space-y-1.5',
    heading: 'px-2 text-[11px] font-bold text-muted-foreground uppercase tracking-wider',
    grid: 'grid grid-cols-1 gap-1',
    row: 'gap-2.5 rounded-md px-2 py-1.5',
    icon:
      'h-8 w-8 rounded-xl text-white shadow-sm group-hover:scale-105 [&_svg]:h-4 [&_svg]:w-4',
    title: 'text-xs font-medium',
    addButton: 'h-6 w-6',
    plus: 'h-3 w-3',
    empty: 'text-[11px] px-2 py-4',
    tooltip: 'max-w-[200px] text-[11px]',
  },
  popover: {
    root: 'space-y-2.5',
    group: 'space-y-1',
    heading: 'px-1.5 text-[10px] font-bold text-muted-foreground uppercase tracking-wider',
    grid: 'grid grid-cols-1 gap-0.5',
    row: 'gap-2 rounded-md px-1.5 py-1',
    icon:
      'h-7 w-7 rounded-lg text-white shadow-sm group-hover:scale-105 [&_svg]:h-3.5 [&_svg]:w-3.5',
    title: 'text-[11px] font-medium',
    addButton: 'h-5 w-5',
    plus: 'h-3 w-3',
    empty: 'text-[11px] px-2 py-4',
    tooltip: 'max-w-[220px] text-[11px]',
  },
} satisfies Record<CatalogDensity, Record<string, string>>;

/**
 * @component NodeCatalogList
 * @category Workflow
 * @status Stable
 * @description Shared workflow node catalog list for drag panels and compact selection popovers
 * @usage Use with interaction="drag" in the left panel and interaction="select" in creation pickers
 * @example
 * <NodeCatalogList grouped={groups} labels={labels} onSelect={addNode} />
 */
export function NodeCatalogList({
  grouped,
  labels,
  onSelect,
  density = 'panel',
  interaction = 'select',
  tooltipSide = 'right',
  className,
}: NodeCatalogListProps) {
  const t = useT('nodes');
  const { value: defaultLlm } = useDefaultModelByUseCase('text-chat');
  const nodes = useWorkflowStore.use.nodes();
  const setDraggingNodeType = useWorkflowStore.use.setDraggingNodeType();
  const setDraggingNodePreview = useWorkflowStore.use.setDraggingNodePreview();
  const updateDraggingNodePreviewClient = useWorkflowStore.use.updateDraggingNodePreviewClient();
  const clearDraggingNodePreview = useWorkflowStore.use.clearDraggingNodePreview();
  const classes = densityClassNames[density];
  const isDragInteraction = interaction === 'drag';

  const getExistingTitles = React.useCallback(() => {
    return new Set(nodes.map(node => node.data?.title).filter(Boolean)) as Set<string>;
  }, [nodes]);

  const createPreviewDataForNode = React.useCallback(
    (nodeType: NodeType) => {
      const previewTitle = resolveDragPreviewNodeTitle(
        nodeType.type,
        t,
        nodeType.title,
        getExistingTitles()
      );
      const data = createDragPreviewNodeData({
        type: nodeType.type,
        title: previewTitle,
        defaultLlmProvider: defaultLlm?.provider,
        defaultLlmName: defaultLlm?.model,
        approvalLabels: {
          content: t('approval.defaults.content'),
          approveLabel: t('approval.defaults.approveLabel'),
          rejectLabel: t('approval.defaults.rejectLabel'),
          emailSubject: t('approval.defaults.emailSubject'),
          emailBody: t('approval.defaults.emailBody'),
        },
      });

      return { previewTitle, data };
    },
    [defaultLlm?.model, defaultLlm?.provider, getExistingTitles, t]
  );

  const handleNodeDragStart = React.useCallback(
    (event: React.DragEvent<HTMLDivElement>, nodeType: NodeType) => {
      if (!isDragInteraction) return;
      if (nodeType.disabledReason) {
        event.preventDefault();
        toast.warning(nodeType.disabledReason);
        return;
      }
      event.dataTransfer?.setData('application/x-workflow-node-type', nodeType.type);
      event.dataTransfer.effectAllowed = 'copy';
      setTransparentDragImage(event.dataTransfer);

      const client = getDragClientPosition(event) ?? { x: 0, y: 0 };
      const { previewTitle, data } = createPreviewDataForNode(nodeType);

      setDraggingNodePreview({
        type: nodeType.type,
        title: previewTitle,
        description: nodeType.description,
        data: data ?? undefined,
        client,
        anchor: DEFAULT_DRAG_PREVIEW_ANCHOR,
      });

      setTimeout(() => setDraggingNodeType(nodeType.type), 0);
    },
    [createPreviewDataForNode, isDragInteraction, setDraggingNodePreview, setDraggingNodeType]
  );

  const handleNodeDrag = React.useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      if (!isDragInteraction) return;
      const client = getDragClientPosition(event);
      if (client) updateDraggingNodePreviewClient(client);
    },
    [isDragInteraction, updateDraggingNodePreviewClient]
  );

  const handleNodeDragEnd = React.useCallback(() => {
    if (!isDragInteraction) return;
    clearDraggingNodePreview();
  }, [clearDraggingNodePreview, isDragInteraction]);

  const handleUnavailableNode = React.useCallback((reason: string) => {
    toast.warning(reason);
  }, []);

  return (
    <>
      <div className={cn(classes.root, className)}>
        {grouped.map(group => (
          <div key={group.key} className={classes.group}>
            <div className={classes.heading}>{labels.group[group.key]}</div>
            <div className={classes.grid}>
              {group.items.map(nodeType => (
                <Tooltip key={nodeType.type}>
                  <TooltipTrigger asChild>
                    <div
                      aria-disabled={Boolean(nodeType.disabledReason)}
                      className={cn(
                        'group flex w-full items-center text-left transition-all duration-200',
                        classes.row,
                        nodeType.disabledReason
                          ? 'cursor-not-allowed opacity-60 hover:bg-muted/40'
                          : isDragInteraction
                            ? 'cursor-grab hover:bg-muted/80 hover:shadow-sm active:scale-[0.98]'
                            : 'cursor-pointer hover:bg-accent hover:text-accent-foreground'
                      )}
                      draggable={isDragInteraction}
                      onClick={() => {
                        if (nodeType.disabledReason) {
                          handleUnavailableNode(nodeType.disabledReason);
                          return;
                        }
                        if (!isDragInteraction) onSelect(nodeType.type);
                      }}
                      onDragStart={event => handleNodeDragStart(event, nodeType)}
                      onDrag={handleNodeDrag}
                      onDragEnd={handleNodeDragEnd}
                      role="button"
                      tabIndex={0}
                      onKeyDown={event => {
                        if (event.key === 'Enter' || event.key === ' ') {
                          event.preventDefault();
                          if (nodeType.disabledReason) {
                            handleUnavailableNode(nodeType.disabledReason);
                            return;
                          }
                          onSelect(nodeType.type);
                        }
                      }}
                    >
                      <div
                        className={cn(
                          'flex shrink-0 items-center justify-center transition-transform',
                          classes.icon,
                          nodeType.bgColor
                        )}
                      >
                        {nodeType.icon}
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className={cn('truncate text-foreground/90', classes.title)}>
                          {nodeType.title}
                        </div>
                      </div>
                      {isDragInteraction ? (
                        <Button
                          variant="ghost"
                          isIcon
                          className={cn(
                            'rounded-full opacity-0 transition-all hover:bg-primary/10 hover:text-primary group-hover:opacity-100',
                            classes.addButton
                          )}
                          onClick={event => {
                            event.stopPropagation();
                            if (nodeType.disabledReason) {
                              handleUnavailableNode(nodeType.disabledReason);
                              return;
                            }
                            onSelect(nodeType.type);
                          }}
                          aria-label={nodeType.title}
                        >
                          <Plus className={classes.plus} />
                        </Button>
                      ) : null}
                    </div>
                  </TooltipTrigger>
                  <TooltipContent side={tooltipSide} className={classes.tooltip}>
                    <p className="mb-1 font-semibold">{nodeType.title}</p>
                    <p className="leading-relaxed text-muted-foreground">{nodeType.description}</p>
                    {nodeType.disabledReason ? (
                      <p className="mt-2 leading-relaxed text-foreground">
                        {nodeType.disabledReason}
                      </p>
                    ) : null}
                  </TooltipContent>
                </Tooltip>
              ))}
            </div>
          </div>
        ))}
      </div>
      {grouped.length === 0 ? (
        <div className={cn('text-center italic text-muted-foreground', classes.empty)}>
          {labels.noAvailable}
        </div>
      ) : null}
    </>
  );
}
