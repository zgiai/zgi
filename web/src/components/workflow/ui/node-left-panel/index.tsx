'use client';

import React, { useMemo, useState } from 'react';
import { Panel } from '@xyflow/react';
import { ChevronLeft, Rocket, Trash2 } from 'lucide-react';
import { useWorkflowStore } from '../../store';
import { AgentType } from '@/services/types/agent';
import { createNodeByTypeFactory } from '../create-node-modal/services/create-node';
import { useNodeTypesI18n, type NodeGroupKey } from '../create-node-modal/constants/node-types';
import {
  computeViewportCenterFlowPos,
  computeStaggeredPosition,
} from '../create-node-modal/services/placement';
import { cn } from '@/lib/utils';
import { usePanelStackItem, useWorkflowOperations } from '../../hooks';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useBuiltinTools } from '@/hooks/workflow/use-builtin-tools';
import { useLocale } from '@/hooks/use-locale';
import { pickLocale, mapParametersToFormFields, createInitialBindings } from '@/utils/tool-helpers';
import type { BuiltinToolProvider, BuiltinToolItem } from '@/services/types/tool';
import NodesTab from './nodes-tab';
import ToolsTab from './tool-tab';
import { NODE_TYPES } from '../../nodes';

interface NodeLeftPanelProps {
  focusModeActive?: boolean;
}

const NodeLeftPanel: React.FC<NodeLeftPanelProps> = ({ focusModeActive = false }) => {
  const {
    addStartNode,
    addKnowledgeRetrievalNode,
    addLLMNode,
    addHttpRequestNode,
    addSqlGeneratorNode,
    addCallDatabaseNode,
    addToolNode,
    addCreateScheduledTaskNode,
    addNotificationSMSNode,
    addEndNode,
    addIfElseNode,
    addCodeNode,
    addIterationNode,
    addAssignerNode,
    addAnswerNode,
    addDocumentExtractorNode,
    addParameterExtractorNode,
    addVariableAggregatorNode,
    addJsonParserNode,
    addImageGenNode,
    addApprovalNode,
    addAnnouncementNode,
    addQuestionAnswerNode,
    addLoopNode,
  } = useWorkflowOperations();

  const hasStartNode = useWorkflowStore(state =>
    state.nodes.some(n => n.data.type === NODE_TYPES.START)
  );
  const agentType = useWorkflowStore.use.agentType();
  const viewport = useWorkflowStore.use.viewport();
  const creationOffsetIndex = useWorkflowStore.use.creationOffsetIndex();
  const incrementCreationOffsetIndex = useWorkflowStore.use.incrementCreationOffsetIndex();
  const beginHistoryBatch = useWorkflowStore.use.beginHistoryBatch();
  const endHistoryBatch = useWorkflowStore.use.endHistoryBatch();

  const [isCollapsed, setIsCollapsed] = useState(false);
  const focusModeRestoreCollapsedRef = React.useRef<boolean | null>(null);
  const focusModeWasActiveRef = React.useRef(false);
  const [isDraggingOverCancel, setIsDraggingOverCancel] = useState(false);
  const draggingNodeType = useWorkflowStore.use.draggingNodeType();
  const updateDraggingNodePreviewClient = useWorkflowStore.use.updateDraggingNodePreviewClient();
  const clearDraggingNodePreview = useWorkflowStore.use.clearDraggingNodePreview();
  const collapsedWidth = 40;
  const panelWidth = 200;
  const draggingWidth = 80; // Size of the cancel area

  React.useEffect(() => {
    if (focusModeActive && !focusModeWasActiveRef.current) {
      focusModeWasActiveRef.current = true;
      focusModeRestoreCollapsedRef.current = isCollapsed;
      if (!isCollapsed) setIsCollapsed(true);
      return;
    }

    if (!focusModeActive && focusModeWasActiveRef.current) {
      focusModeWasActiveRef.current = false;
      const restoreCollapsed = focusModeRestoreCollapsedRef.current;
      focusModeRestoreCollapsedRef.current = null;
      if (restoreCollapsed !== null) setIsCollapsed(restoreCollapsed);
    }
  }, [focusModeActive, isCollapsed]);

  const createNodeByType = createNodeByTypeFactory({
    addStartNode,
    addKnowledgeRetrievalNode,
    addLLMNode,
    addHttpRequestNode,
    addSqlGeneratorNode,
    addCallDatabaseNode,
    addToolNode,
    addCreateScheduledTaskNode,
    addNotificationSMSNode,
    addEndNode,
    addIfElseNode,
    addCodeNode,
    addIterationNode,
    addAssignerNode,
    addAnswerNode,
    addDocumentExtractorNode,
    addParameterExtractorNode,
    addVariableAggregatorNode,
    addJsonParserNode,
    addImageGenNode,
    addApprovalNode,
    addAnnouncementNode,
    addQuestionAnswerNode,
    addLoopNode,
  });

  const i18nTypes = useNodeTypesI18n();
  const filteredTypes = useMemo(() => {
    return i18nTypes.filter(nt => {
      if (nt.type === 'loop-end') return false;
      if (nt.type === 'start') return !hasStartNode;
      if (nt.type === 'end') return agentType === AgentType.WORKFLOW;
      if (nt.type === 'answer') return agentType === AgentType.CONVERSATIONAL_AGENT;
      return true;
    });
  }, [i18nTypes, hasStartNode, agentType]);

  const t = useT('nodes');

  const labels = {
    quickAdd: t('ui.quickAdd'),
    noAvailable: t('ui.noAvailable'),
    collapsePanel: t('ui.collapsePanel'),
    tabNodes: t('ui.tabNodes'),
    tabTools: t('ui.tabTools'),
    toolsEmpty: t('ui.toolsEmpty'),
    group: {
      flow: t('groups.flow'),
      ai: t('groups.ai'),
      data: t('groups.data'),
      tool: t('groups.tool'),
    } as Record<NodeGroupKey, string>,
    cancel: t('ui.cancel'),
  };

  const groupOrder: NodeGroupKey[] = ['flow', 'ai', 'data', 'tool'];
  const grouped = groupOrder
    .map(key => ({ key, items: filteredTypes.filter(n => n.group === key) }))
    .filter(g => g.items.length > 0);

  const handleAddCentered = (type: string) => {
    const basePos = computeViewportCenterFlowPos(viewport);
    const pos = computeStaggeredPosition(basePos, creationOffsetIndex);
    beginHistoryBatch();
    try {
      const id = createNodeByType(type, pos);
      if (id) {
        useWorkflowStore.getState().selectNode(id);
        useWorkflowStore.getState().setSelectionSource('create');
        incrementCreationOffsetIndex();
      }
    } finally {
      endHistoryBatch();
    }
  };

  const { panelStyle } = usePanelStackItem({
    id: 'node-left-panel',
    position: 'top-left',
    order: 1,
    visible: true,
    width: draggingNodeType ? draggingWidth : isCollapsed ? collapsedWidth : panelWidth,
    gap: 8,
  });

  const { tools, isLoading: toolsLoading, isFetching: toolsFetching } = useBuiltinTools();
  const { locale } = useLocale();

  const handleAddBuiltinTool = (provider: BuiltinToolProvider, tool: BuiltinToolItem) => {
    const fields = mapParametersToFormFields(tool.parameters, locale);
    const bindings = createInitialBindings(fields);
    const nodeTitle = pickLocale(tool.label, locale, tool.name);
    const initialData = {
      provider_type: provider.type || 'builtin',
      provider_id: provider.id || provider.name || '',
      tool_name: tool.name,
      tool_parameters: bindings,
      title: nodeTitle,
    };

    const basePos = computeViewportCenterFlowPos(viewport);
    const pos = computeStaggeredPosition(basePos, creationOffsetIndex);
    beginHistoryBatch();
    try {
      const id = createNodeByType('tool', pos, undefined, initialData);
      if (!id) return;
      useWorkflowStore.getState().selectNode(id);
      useWorkflowStore.getState().setSelectionSource('create');
      incrementCreationOffsetIndex();
    } finally {
      endHistoryBatch();
    }
  };

  return (
    <Panel
      position="top-left"
      className={cn(
        'p-0 z-[100]',
        'transition-all duration-300 ease-in-out overflow-hidden',
        draggingNodeType
          ? 'h-20 rounded-xl shadow-premium border-2 border-dashed border-destructive/20 bg-destructive/5 backdrop-blur-sm'
          : isCollapsed
            ? 'h-10'
            : 'h-[calc(100%-200px)]'
      )}
      style={{
        ...panelStyle,
        width: draggingNodeType ? draggingWidth : isCollapsed ? collapsedWidth : panelWidth,
      }}
    >
      {/* dragging state - Cancel Area */}
      {draggingNodeType && (
        <div
          className={cn(
            'flex flex-col items-center justify-center h-full w-full gap-1 transition-all duration-300',
            'animate-in fade-in zoom-in-95 duration-300',
            isDraggingOverCancel && 'scale-110 bg-red-600/90'
          )}
          onDragEnterCapture={e => {
            e.preventDefault();
            e.stopPropagation();
            if (e.clientX || e.clientY) {
              updateDraggingNodePreviewClient({ x: e.clientX, y: e.clientY });
            }
            setIsDraggingOverCancel(true);
          }}
          onDragLeaveCapture={e => {
            e.preventDefault();
            e.stopPropagation();
            setIsDraggingOverCancel(false);
          }}
          onDragOverCapture={e => {
            e.preventDefault();
            e.stopPropagation();
            if (e.clientX || e.clientY) {
              updateDraggingNodePreviewClient({ x: e.clientX, y: e.clientY });
            }
            if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
          }}
          onDropCapture={e => {
            e.preventDefault();
            e.stopPropagation();
            setIsDraggingOverCancel(false);
            clearDraggingNodePreview();
            // Drop here = Cancel (already handled by stopping propagation and not being the canvas)
          }}
        >
          <div
            className={cn(
              'bg-red-500 text-white rounded-full p-1.5 shadow-lg animate-pulse transition-transform pointer-events-none',
              isDraggingOverCancel && 'bg-white text-red-600'
            )}
          >
            <Trash2 className="w-5 h-5" />
          </div>
          <span
            className={cn(
              'text-[10px] font-bold text-red-600 uppercase tracking-wider pointer-events-none',
              isDraggingOverCancel && 'text-white'
            )}
          >
            {labels.cancel}
          </span>
        </div>
      )}

      <div
        className={cn(
          'flex flex-col h-full w-full transition-all duration-300 ease-in-out',
          draggingNodeType && 'opacity-0 pointer-events-none scale-95'
        )}
        onDragOverCapture={e => {
          e.preventDefault();
          e.stopPropagation();
          if (e.dataTransfer) e.dataTransfer.dropEffect = 'none';
        }}
        onDropCapture={e => {
          e.preventDefault();
          e.stopPropagation();
        }}
      >
        <div
          className={cn(
            'flex items-center h-10 theme-surface border border-muted interactive-subtle',
            isCollapsed
              ? 'w-10 justify-center px-0 shadow-premium rounded-lg'
              : 'justify-start px-2 rounded-t-lg border-b-none'
          )}
          onClick={() => isCollapsed && setIsCollapsed(false)}
        >
          <div
            className={cn(
              'text-sm font-medium transition-all duration-300 ease-in-out truncate',
              isCollapsed ? 'w-0 h-0 overflow-hidden opacity-0' : 'opacity-100 grow'
            )}
          >
            <Rocket size={14} className="mr-1 inline text-highlight" /> {labels.quickAdd}
          </div>
          <Button
            variant="ghost"
            isIcon
            onClick={() => setIsCollapsed(!isCollapsed)}
            size="sm"
            aria-label={labels.collapsePanel}
          >
            <ChevronLeft
              size={16}
              className={cn(
                'transition-transform duration-300 ease-in-out',
                isCollapsed && 'rotate-180'
              )}
            />
          </Button>
        </div>

        {!isCollapsed && (
          <div className="flex-1 min-h-0 flex flex-col pb-2 theme-surface border-x border-b border-muted shadow-premium rounded-b-lg overflow-hidden animate-in fade-in slide-in-from-left-4 duration-300">
            <Tabs defaultValue="nodes" className="w-full flex-1 min-h-0 flex flex-col">
              <div className="px-2 py-1">
                <TabsList className="grid h-8 grid-cols-2 rounded-md p-0.5">
                  <TabsTrigger value="nodes" className="rounded px-2 py-0.5 text-xs">
                    {labels.tabNodes}
                  </TabsTrigger>
                  <TabsTrigger value="tools" className="rounded px-2 py-0.5 text-xs">
                    {labels.tabTools}
                  </TabsTrigger>
                </TabsList>
              </div>

              <TabsContent
                value="nodes"
                className="flex-1 min-h-0 space-y-2 px-2 pb-2 overflow-y-auto"
              >
                <NodesTab
                  grouped={grouped}
                  labels={{ group: labels.group, noAvailable: labels.noAvailable }}
                  onAddCentered={handleAddCentered}
                />
              </TabsContent>

              <TabsContent
                value="tools"
                className="flex-1 min-h-0 space-y-3 px-2 grow overflow-y-auto"
              >
                <ToolsTab
                  tools={tools}
                  isLoading={toolsLoading}
                  isFetching={toolsFetching}
                  labels={{ toolsEmpty: labels.toolsEmpty }}
                  locale={locale}
                  onAddTool={handleAddBuiltinTool}
                />
              </TabsContent>
            </Tabs>
          </div>
        )}
      </div>
    </Panel>
  );
};

export default NodeLeftPanel;
