'use client';

import React from 'react';
import { Plus, Wrench } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import ImageSafe from '@/components/common/image-safe';
import { cn } from '@/lib/utils';
import { pickLocale, pickToolDescription } from '@/utils/tool-helpers';
import type { Locale } from '@/lib/i18n';
import type {
  BuiltinToolsResponse,
  BuiltinToolProvider,
  BuiltinToolItem,
} from '@/services/types/tool';
import { useWorkflowStore } from '../../store';
import { getNextNodeTitle } from '../../store/helpers/titles';
import {
  DEFAULT_DRAG_PREVIEW_ANCHOR,
  getDragClientPosition,
  setTransparentDragImage,
} from '../node-left-panel/drag-preview-utils';
import { createToolDragPreviewNodeData } from '../node-left-panel/drag-preview-data';
import type { CatalogDensity, CatalogInteraction, CatalogTooltipSide } from './types';

interface ToolCatalogListLabels {
  toolsEmpty: string;
}

interface ToolCatalogListProps {
  tools: BuiltinToolsResponse | null | undefined;
  isLoading: boolean;
  isFetching: boolean;
  labels: ToolCatalogListLabels;
  locale: Locale;
  onSelect: (provider: BuiltinToolProvider, tool: BuiltinToolItem) => void;
  density?: CatalogDensity;
  interaction?: CatalogInteraction;
  tooltipSide?: CatalogTooltipSide;
  className?: string;
}

const densityClassNames = {
  panel: {
    root: 'space-y-5',
    loadingRoot: 'space-y-4 px-1',
    provider: 'space-y-2',
    headingWrap: 'flex items-center gap-2 px-1',
    heading: 'text-[10px] font-bold text-muted-foreground uppercase tracking-widest',
    grid: 'grid grid-cols-1 gap-1',
    row: 'gap-2 rounded-md px-2 py-1',
    icon: 'h-7 w-7 rounded-md group-hover:scale-105',
    title: 'text-[11px] font-medium',
    addButton: 'h-6 w-6',
    plus: 'h-3 w-3',
    empty: 'text-[11px] px-2 py-8',
    tooltip: 'max-w-[200px] text-[11px]',
    skeleton: 'h-8 w-full rounded-md',
  },
  popover: {
    root: 'space-y-3',
    loadingRoot: 'space-y-2 px-1',
    provider: 'space-y-1',
    headingWrap: 'flex items-center gap-2 px-1',
    heading: 'text-[10px] font-bold text-muted-foreground uppercase tracking-wider',
    grid: 'grid grid-cols-1 gap-0.5',
    row: 'gap-1.5 rounded-md px-1.5 py-1',
    icon: 'h-6 w-6 rounded-md group-hover:scale-105',
    title: 'text-[11px] font-medium',
    addButton: 'h-5 w-5',
    plus: 'h-3 w-3',
    empty: 'text-[11px] px-2 py-6',
    tooltip: 'max-w-[220px] text-[11px]',
    skeleton: 'h-7 w-full rounded-md',
  },
} satisfies Record<CatalogDensity, Record<string, string>>;

/**
 * @component ToolCatalogList
 * @category Workflow
 * @status Stable
 * @description Shared builtin tool catalog list for drag panels and compact selection popovers
 * @usage Use with interaction="drag" in the left panel and interaction="select" in creation pickers
 * @example
 * <ToolCatalogList tools={tools} onSelect={addTool} labels={labels} locale={locale} />
 */
export function ToolCatalogList({
  tools,
  isLoading,
  isFetching,
  labels,
  locale,
  onSelect,
  density = 'panel',
  interaction = 'select',
  tooltipSide = 'right',
  className,
}: ToolCatalogListProps) {
  const setDraggingNodeType = useWorkflowStore.use.setDraggingNodeType();
  const setDraggingNodePreview = useWorkflowStore.use.setDraggingNodePreview();
  const updateDraggingNodePreviewClient = useWorkflowStore.use.updateDraggingNodePreviewClient();
  const clearDraggingNodePreview = useWorkflowStore.use.clearDraggingNodePreview();
  const nodes = useWorkflowStore.use.nodes();
  const classes = densityClassNames[density];
  const loading = isLoading || isFetching;
  const isDragInteraction = interaction === 'drag';

  if (loading) {
    return (
      <div className={cn(classes.loadingRoot, className)}>
        {Array.from({ length: density === 'popover' ? 4 : 5 }).map((_, index) => (
          <div key={index} className="space-y-2">
            <Skeleton className="h-3 w-16" />
            <Skeleton className={classes.skeleton} />
            <Skeleton className={classes.skeleton} />
          </div>
        ))}
      </div>
    );
  }

  if (!tools || tools.length === 0) {
    return (
      <div
        className={cn(
          'rounded-lg bg-muted/30 text-center italic text-muted-foreground',
          classes.empty,
          className
        )}
      >
        {labels.toolsEmpty}
      </div>
    );
  }

  return (
    <div className={cn(classes.root, className)}>
      {tools.map((provider, providerIndex) => {
        const providerName = pickLocale(provider.label, locale, provider.name);
        const iconUrl = provider.icon;
        const providerKey = provider.id || provider.name || `provider-${providerIndex}`;

        return (
          <div key={providerKey} className={classes.provider}>
            <div className={classes.headingWrap}>
              <div className="h-px flex-1 bg-muted" />
              <div className={cn(classes.heading, 'whitespace-nowrap')}>{providerName}</div>
              <div className="h-px flex-1 bg-muted" />
            </div>
            <div className={classes.grid}>
              {(provider.tools ?? []).map((tool, toolIndex) => {
                const title = pickLocale(tool?.label, locale, tool?.name);
                const description = pickToolDescription(tool.description, locale, title);
                const toolKey = tool?.name || `tool-${toolIndex}`;

                return (
                  <Tooltip key={`${providerKey}-${toolKey}`}>
                    <TooltipTrigger asChild>
                      <div
                        className={cn(
                          'group flex w-full items-center text-left transition-all duration-200',
                          classes.row,
                          isDragInteraction
                            ? 'cursor-grab hover:bg-muted/80 hover:shadow-sm active:scale-[0.98]'
                            : 'cursor-pointer hover:bg-accent hover:text-accent-foreground'
                        )}
                        draggable={isDragInteraction}
                        onClick={() => {
                          if (!isDragInteraction) onSelect(provider, tool);
                        }}
                        onDragStart={event => {
                          if (!isDragInteraction) return;
                          event.dataTransfer?.setData('application/x-workflow-node-type', 'tool');
                          const existingTitles = new Set(
                            nodes.map(node => node.data?.title).filter(Boolean)
                          ) as Set<string>;
                          const previewTitle = getNextNodeTitle(title, existingTitles);
                          const toolInfo = {
                            provider_id: provider.id || provider.name || '',
                            tool_name: tool.name,
                            title: previewTitle,
                            iconUrl: provider.icon ?? undefined,
                          };
                          event.dataTransfer?.setData(
                            'application/x-workflow-tool-info',
                            JSON.stringify(toolInfo)
                          );
                          event.dataTransfer.effectAllowed = 'copy';
                          setTransparentDragImage(event.dataTransfer);
                          const client = getDragClientPosition(event) ?? { x: 0, y: 0 };
                          setDraggingNodePreview({
                            type: 'tool',
                            title: previewTitle,
                            description,
                            iconUrl: provider.icon ?? undefined,
                            data: createToolDragPreviewNodeData({
                              title: previewTitle,
                              providerId: toolInfo.provider_id,
                              toolName: toolInfo.tool_name,
                            }),
                            toolInfo: {
                              provider_id: toolInfo.provider_id,
                              tool_name: toolInfo.tool_name,
                            },
                            client,
                            anchor: DEFAULT_DRAG_PREVIEW_ANCHOR,
                          });
                          setTimeout(() => setDraggingNodeType('tool'), 0);
                        }}
                        onDrag={event => {
                          if (!isDragInteraction) return;
                          const client = getDragClientPosition(event);
                          if (client) updateDraggingNodePreviewClient(client);
                        }}
                        onDragEnd={() => {
                          if (!isDragInteraction) return;
                          clearDraggingNodePreview();
                        }}
                        role="button"
                        tabIndex={0}
                        onKeyDown={event => {
                          if (event.key === 'Enter' || event.key === ' ') {
                            event.preventDefault();
                            onSelect(provider, tool);
                          }
                        }}
                      >
                        <div
                          className={cn(
                            'flex shrink-0 items-center justify-center overflow-hidden border border-border/50 bg-white shadow-sm transition-transform',
                            classes.icon
                          )}
                        >
                          {iconUrl ? (
                            <ImageSafe
                              src={iconUrl}
                              alt={providerName}
                              className="h-full w-full object-cover"
                              loading="lazy"
                              referrerPolicy="no-referrer"
                              fallbackComponent={
                                <div className="flex h-full w-full items-center justify-center bg-primary/10 p-1">
                                  <Wrench className="h-3.5 w-3.5 text-primary" />
                                </div>
                              }
                            />
                          ) : (
                            <div className="flex h-full w-full items-center justify-center bg-primary/10 p-1">
                              <Wrench className="h-3.5 w-3.5 text-primary" />
                            </div>
                          )}
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className={cn('truncate text-foreground/90', classes.title)}>
                            {title}
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
                              onSelect(provider, tool);
                            }}
                            aria-label={title}
                          >
                            <Plus className={classes.plus} />
                          </Button>
                        ) : null}
                      </div>
                    </TooltipTrigger>
                    <TooltipContent side={tooltipSide} className={classes.tooltip}>
                      <p className="mb-1 font-semibold">{title}</p>
                      <p className="leading-relaxed text-muted-foreground">{description}</p>
                    </TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}
