import React from 'react';
import { useReactFlow, type Viewport } from '@xyflow/react';
import { useBuiltinTools } from '@/hooks/workflow/use-builtin-tools';
import { useLocale } from '@/hooks/use-locale';
import { pickLocale, mapParametersToFormFields, createInitialBindings } from '@/utils/tool-helpers';
import useWorkflowOperations from './use-workflow-operations';
import { useWorkflowStore } from '../store';
import { createNodeByTypeFactory } from '../ui/create-node-modal/services/create-node';
import { DEFAULT_DRAG_PREVIEW_ANCHOR } from '../ui/node-left-panel/drag-preview-utils';

interface UseDragCreateNodeParams {
  isReadOnly: boolean;
  viewViewport: Viewport;
}

interface ToolDragInfo {
  provider_id: string;
  tool_name: string;
  title?: string;
}

export function useDragCreateNode({ isReadOnly, viewViewport }: UseDragCreateNodeParams) {
  const { screenToFlowPosition } = useReactFlow();
  const { tools } = useBuiltinTools();
  const { locale } = useLocale();
  const draggingNodeType = useWorkflowStore.use.draggingNodeType();

  const {
    addStartNode,
    addKnowledgeRetrievalNode,
    addLLMNode,
    addHttpRequestNode,
    addSqlGeneratorNode,
    addToolNode,
    addCreateScheduledTaskNode,
    addNotificationSMSNode,
    addEndNode,
    addIfElseNode,
    addCodeNode,
    addIterationNode,
    addAssignerNode,
    addAnswerNode,
    addCallDatabaseNode,
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

  const createNodeByType = React.useMemo(
    () =>
      createNodeByTypeFactory({
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
      }),
    [
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
    ]
  );

  React.useEffect(() => {
    if (!draggingNodeType) return;

    const timeoutIds = new Set<number>();
    const clearDragState = () => {
      useWorkflowStore.getState().clearDraggingNodePreview();
    };
    const clearDragStateAfterCurrentEvent = () => {
      const timeoutId = window.setTimeout(() => {
        timeoutIds.delete(timeoutId);
        clearDragState();
      }, 0);
      timeoutIds.add(timeoutId);
    };
    const handleVisibilityChange = () => {
      if (document.visibilityState !== 'visible') clearDragState();
    };

    window.addEventListener('blur', clearDragState);
    window.addEventListener('pagehide', clearDragState);
    window.addEventListener('keydown', clearDragState, true);
    window.addEventListener('dragend', clearDragStateAfterCurrentEvent);
    window.addEventListener('drop', clearDragStateAfterCurrentEvent);
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      timeoutIds.forEach(timeoutId => window.clearTimeout(timeoutId));
      window.removeEventListener('blur', clearDragState);
      window.removeEventListener('pagehide', clearDragState);
      window.removeEventListener('keydown', clearDragState, true);
      window.removeEventListener('dragend', clearDragStateAfterCurrentEvent);
      window.removeEventListener('drop', clearDragStateAfterCurrentEvent);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [draggingNodeType]);

  const createToolInitialData = React.useCallback(
    (toolDragInfo: ToolDragInfo | null) => {
      if (!toolDragInfo) return undefined;

      try {
        const providers = Array.isArray(tools) ? tools : [];
        const provider = providers.find(
          p => p.id === toolDragInfo.provider_id || p.name === toolDragInfo.provider_id
        );
        const toolItem = provider?.tools?.find(t => t.name === toolDragInfo.tool_name);
        const fields = toolItem ? mapParametersToFormFields(toolItem.parameters, locale) : [];
        const bindings = createInitialBindings(fields);
        const nodeTitle = toolItem
          ? pickLocale(toolItem.label, locale, toolItem.name)
          : toolDragInfo.title || toolDragInfo.tool_name;

        return {
          provider_type: provider?.type || 'builtin',
          provider_id: toolDragInfo.provider_id,
          tool_name: toolDragInfo.tool_name,
          tool_parameters: bindings,
          title: nodeTitle,
        };
      } catch (_e) {
        return undefined;
      }
    },
    [locale, tools]
  );

  const handleDragOver = React.useCallback((event: React.DragEvent) => {
    event.preventDefault();
    if (event.clientX || event.clientY) {
      useWorkflowStore
        .getState()
        .updateDraggingNodePreviewClient({ x: event.clientX, y: event.clientY });
    }
    event.dataTransfer.dropEffect = 'copy';
  }, []);

  const handleDrop = React.useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const type = event.dataTransfer?.getData('application/x-workflow-node-type');
      const dragPreview = useWorkflowStore.getState().draggingNodePreview;
      if (!type || isReadOnly) {
        if (type || dragPreview) useWorkflowStore.getState().clearDraggingNodePreview();
        return;
      }

      const rawToolInfo = event.dataTransfer?.getData('application/x-workflow-tool-info');
      let toolDragInfo: ToolDragInfo | null = null;
      if (type === 'tool' && rawToolInfo) {
        try {
          toolDragInfo = JSON.parse(rawToolInfo);
        } catch (_e) {
          toolDragInfo = null;
        }
      }

      const initialData = type === 'tool' ? createToolInitialData(toolDragInfo) : undefined;
      const anchor = dragPreview?.anchor ?? DEFAULT_DRAG_PREVIEW_ANCHOR;
      const pos = screenToFlowPosition({
        x: event.clientX - anchor.x * viewViewport.zoom,
        y: event.clientY - anchor.y * viewViewport.zoom,
      });

      const id = createNodeByType(type, pos, undefined, initialData);
      if (id) {
        useWorkflowStore.getState().selectNode(id);
        useWorkflowStore.getState().setSelectionSource('create');
      }
      useWorkflowStore.getState().clearDraggingNodePreview();
    },
    [createNodeByType, createToolInitialData, isReadOnly, screenToFlowPosition, viewViewport.zoom]
  );

  return {
    draggingNodeType,
    handleDragOver,
    handleDrop,
  };
}
