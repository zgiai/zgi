import React from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import type { Locale } from '@/lib/i18n';
import type { BuiltinToolProvider, BuiltinToolItem } from '@/services/types/tool';
import type { NodeGroupKey, NodeType } from './constants/node-types';
import { NodeCatalogList, ToolCatalogList } from '../node-catalog';

interface CreateNodePickerContentLabels {
  noAvailable: string;
  tabNodes: string;
  tabTools: string;
  toolsEmpty: string;
  group: Record<NodeGroupKey, string>;
}

interface CreateNodePickerContentProps {
  grouped: Array<{ key: NodeGroupKey; items: NodeType[] }>;
  labels: CreateNodePickerContentLabels;
  tools: BuiltinToolProvider[] | null | undefined;
  toolsLoading: boolean;
  toolsFetching: boolean;
  locale: Locale;
  density?: 'panel' | 'popover';
  onAddNode: (type: string) => void;
  onAddBuiltinTool: (provider: BuiltinToolProvider, tool: BuiltinToolItem) => void;
}

/**
 * @component CreateNodePickerContent
 * @category Workflow
 * @status Stable
 * @description Shared node creation picker content used by floating and dialog shells
 * @usage Render inside CreateNodeFloatingPicker
 * @example
 * <CreateNodePickerContent grouped={grouped} labels={labels} onAddNode={addNode} />
 */
export function CreateNodePickerContent({
  grouped,
  labels,
  tools,
  toolsLoading,
  toolsFetching,
  locale,
  density = 'popover',
  onAddNode,
  onAddBuiltinTool,
}: CreateNodePickerContentProps) {
  const isPopover = density === 'popover';

  return (
    <Tabs defaultValue="nodes" className="flex h-full max-h-[inherit] min-h-0 w-full flex-col">
      <div className={isPopover ? 'px-2 pb-1 pt-2' : 'px-4 pb-2 pt-3'}>
        <TabsList
          className={
            isPopover
              ? 'grid h-8 w-full grid-cols-2 rounded-md p-0.5'
              : 'grid h-9 w-full grid-cols-2 rounded-md p-0.5'
          }
        >
          <TabsTrigger value="nodes" className="rounded px-2 py-0.5 text-xs">
            {labels.tabNodes}
          </TabsTrigger>
          <TabsTrigger value="tools" className="rounded px-2 py-0.5 text-xs">
            {labels.tabTools}
          </TabsTrigger>
        </TabsList>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <TabsContent
          value="nodes"
          className={isPopover ? 'mt-0 px-2 pb-2 outline-none' : 'mt-0 px-4 pb-4 outline-none'}
        >
          <NodeCatalogList
            grouped={grouped}
            labels={{ group: labels.group, noAvailable: labels.noAvailable }}
            onSelect={onAddNode}
            density={density}
            interaction="select"
            tooltipSide="right"
          />
        </TabsContent>
        <TabsContent
          value="tools"
          className={isPopover ? 'mt-0 px-2 pb-2 outline-none' : 'mt-0 px-4 pb-4 outline-none'}
        >
          <ToolCatalogList
            tools={tools}
            isLoading={toolsLoading}
            isFetching={toolsFetching}
            labels={{ toolsEmpty: labels.toolsEmpty }}
            locale={locale}
            onSelect={onAddBuiltinTool}
            density={density}
            interaction="select"
            tooltipSide="right"
          />
        </TabsContent>
      </div>
    </Tabs>
  );
}
