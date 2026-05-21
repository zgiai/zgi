import React from 'react';
import { useWorkflowOperations } from '../../hooks';
import { NODE_TYPES } from '../../nodes';
import { useCreateNodeModal } from '../../hooks/use-create-node-modal';
import { useWorkflowStore } from '../../store';
import { resolveContainerContextId } from './services/context';
import { createNodeByTypeFactory } from './services/create-node';
import { useNodeTypesI18n } from './constants/node-types';
import { AgentType } from '@/services/types/agent';
import { useBuiltinTools } from '@/hooks/workflow/use-builtin-tools';
import { useLocale } from '@/hooks/use-locale';
import type { OriginHandleProp } from './types';
import { useCreationActions } from './hooks/use-creation-actions';
import { useT } from '@/i18n';
import { CreateNodePickerContent } from './create-node-picker-content';
import { CreateNodeFloatingPicker } from './create-node-floating-picker';

interface CreateNodeModalProps {
  isOpen: boolean;
  onClose: () => void;
  position: { x: number; y: number } | null;
  anchorClientPosition: { x: number; y: number } | null;
  originatingHandle: OriginHandleProp | null;
}

const CreateNodeModal: React.FC<CreateNodeModalProps> = ({
  isOpen,
  onClose,
  position,
  anchorClientPosition,
  originatingHandle,
}) => {
  const {
    addStartNode,
    addKnowledgeRetrievalNode,
    addLLMNode,
    addHttpRequestNode,
    addCallDatabaseNode,
    addSqlGeneratorNode,
    addToolNode,
    addCreateScheduledTaskNode,
    addNotificationSMSNode,
    addEndNode,
    addLoopEndNode,
    addIfElseNode,
    addCodeNode,
    addIterationNode,
    addLoopNode,
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
  } = useWorkflowOperations();
  const hasStartNode = useWorkflowStore(state =>
    state.nodes.some(n => n.data.type === NODE_TYPES.START)
  );
  const agentType = useWorkflowStore.use.agentType();
  const { originatingEdge: storeEdge } = useCreateNodeModal();

  // Freeze state when closing to prevent flash during exit animation
  const frozenState = React.useRef({
    originatingHandle,
    originatingEdge: storeEdge,
    position,
    anchorClientPosition,
  });

  if (isOpen) {
    frozenState.current = {
      originatingHandle,
      originatingEdge: storeEdge,
      position,
      anchorClientPosition,
    };
  }

  const {
    originatingHandle: activeHandle,
    originatingEdge: activeEdge,
    position: activePosition,
    anchorClientPosition: activeAnchorClientPosition,
  } = frozenState.current;

  // Detect whether the creation context is inside a container node
  const containerContextId: string | undefined = resolveContainerContextId({
    originatingHandle: activeHandle,
    originatingEdge: activeEdge,
  });

  const createNodeByType = React.useMemo(
    () =>
      createNodeByTypeFactory({
        addStartNode,
        addKnowledgeRetrievalNode,
        addLLMNode,
        addHttpRequestNode,
        addCallDatabaseNode,
        addSqlGeneratorNode,
        addToolNode,
        addCreateScheduledTaskNode,
        addNotificationSMSNode,
        addEndNode,
        addLoopEndNode,
        addIfElseNode,
        addCodeNode,
        addIterationNode,
        addLoopNode,
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
      }),
    [
      addStartNode,
      addKnowledgeRetrievalNode,
      addLLMNode,
      addHttpRequestNode,
      addCallDatabaseNode,
      addSqlGeneratorNode,
      addToolNode,
      addCreateScheduledTaskNode,
      addNotificationSMSNode,
      addEndNode,
      addLoopEndNode,
      addIfElseNode,
      addCodeNode,
      addIterationNode,
      addLoopNode,
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
    ]
  );

  const { handleAddNode, handleAddBuiltinTool, handleOpenChange } = useCreationActions({
    position: activePosition,
    originatingHandle: activeHandle,
    originatingEdge: activeEdge,
    onClose,
    createNodeByType,
  });

  const i18nTypes = useNodeTypesI18n();
  const t = useT();

  const labels = React.useMemo(() => {
    return {
      selectNodeType: t('nodes.ui.selectNodeType'),
      noAvailable: t('nodes.ui.noAvailable'),
      tabNodes: t('nodes.ui.tabNodes'),
      tabTools: t('nodes.ui.tabTools'),
      toolsEmpty: t('nodes.ui.toolsEmpty'),
      group: {
        flow: t('nodes.groups.flow'),
        ai: t('nodes.groups.ai'),
        data: t('nodes.groups.data'),
        tool: t('nodes.groups.tool'),
      },
    };
  }, [t]);

  const { tools, isLoading: toolsLoading, isFetching: toolsFetching } = useBuiltinTools();
  const { locale } = useLocale();

  // Create a snapshot of all data needed for the current render.
  // We freeze this when the modal starts closing so transitions stay visually stable.
  const snapshot = React.useMemo(() => {
    // Only allow nodes with both input and output when opened from an edge
    const baseTypes = activeEdge ? i18nTypes.filter(n => n.io) : i18nTypes;

    // Filter out types that cannot be added
    const filteredTypes = baseTypes.filter(nt => {
      if (containerContextId) {
        if (
          nt.type === 'start' ||
          nt.type === 'end' ||
          nt.type === 'approval' ||
          nt.type === 'announcement' ||
          nt.type === 'question-answer' ||
          nt.type === 'iteration' ||
          nt.type === 'loop'
        ) {
          return false;
        }
        return true;
      }
      if (nt.type === 'loop-end') return false;
      if (nt.type === 'start') return !hasStartNode;
      if (nt.type === 'end') return agentType === AgentType.WORKFLOW;
      if (nt.type === 'answer') return agentType === AgentType.CONVERSATIONAL_AGENT;
      return true;
    });

    const groupOrder: Array<'flow' | 'ai' | 'data' | 'tool'> = ['flow', 'ai', 'data', 'tool'];
    const grouped = groupOrder
      .map(key => ({ key, items: filteredTypes.filter(n => n.group === key) }))
      .filter(g => g.items.length > 0);

    return {
      filteredTypes,
      grouped,
      labels,
      tools,
      toolsLoading,
      toolsFetching,
    };
  }, [
    activeEdge,
    i18nTypes,
    containerContextId,
    hasStartNode,
    agentType,
    labels,
    tools,
    toolsLoading,
    toolsFetching,
  ]);

  const renderRef = React.useRef(snapshot);
  if (isOpen) {
    renderRef.current = snapshot;
  }

  const activeRender = renderRef.current;

  const pickerContent = (
    <CreateNodePickerContent
      grouped={activeRender.grouped}
      labels={{
        noAvailable: activeRender.labels.noAvailable,
        tabNodes: activeRender.labels.tabNodes,
        tabTools: activeRender.labels.tabTools,
        toolsEmpty: activeRender.labels.toolsEmpty,
        group: activeRender.labels.group as Record<'flow' | 'ai' | 'data' | 'tool', string>,
      }}
      tools={activeRender.tools}
      toolsLoading={activeRender.toolsLoading}
      toolsFetching={activeRender.toolsFetching}
      locale={locale}
      density="popover"
      onAddNode={handleAddNode}
      onAddBuiltinTool={handleAddBuiltinTool}
    />
  );

  return (
    <CreateNodeFloatingPicker
      open={isOpen}
      onOpenChange={handleOpenChange}
      anchorClientPosition={activeAnchorClientPosition}
    >
      {pickerContent}
    </CreateNodeFloatingPicker>
  );
};

export default React.memo(CreateNodeModal);
