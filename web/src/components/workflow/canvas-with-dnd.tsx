'use client';

import React from 'react';
import {
  ReactFlow,
  Background,
  BackgroundVariant,
  StraightEdge,
  StepEdge,
  SmoothStepEdge,
  SimpleBezierEdge,
  type Node,
  type Edge,
  type Viewport,
  type OnNodesChange,
  type OnEdgesChange,
  type OnConnect,
  PanOnScrollMode,
} from '@xyflow/react';
import { CreateNodeModal, WorkflowContextMenu, CustomEdge, EdgeDescriptionEditor } from './ui';
import { useWorkflowStore } from './store';
import { nodeTypes } from './nodes';
import GlobalContainerOverlay from './global-container-overlay';
import { WorkflowNodeDragPreview } from './ui/workflow-node-drag-preview';
import { useCreateNodeModal } from './hooks/use-create-node-modal';
import { useCanvasInteractionGuard } from './hooks/use-canvas-interaction-guard';
import { useConnectDropCreate } from './hooks/use-connect-drop-create';
import { useDragCreateNode } from './hooks/use-drag-create-node';
import WorkflowCanvasPanels from './ui/workflow-canvas-panels';

interface CanvasWithDndProps {
  viewNodes: Node[];
  viewEdges: Edge[];
  viewViewport: Viewport;
  isReadOnly: boolean;
  agentType: string;
  agentId: string;
  agentName?: string;
  agentIconType?: string;
  agentIcon?: string;
  agentIconUrl?: string;
  onNodesChange?: OnNodesChange;
  onEdgesChange?: OnEdgesChange;
  onConnect: OnConnect;
  onNodeClick: (event: React.MouseEvent, node: { id: string }) => void;
  onNodeContextMenu: (event: React.MouseEvent, node: { id: string }) => void;
  onPaneClick: (event: React.MouseEvent) => void;
  onViewportChange: (vp: Viewport) => void;
  fitOnMount?: boolean;
}

const EDGE_TYPES = {
  custom: CustomEdge,
  straight: StraightEdge,
  step: StepEdge,
  smoothstep: SmoothStepEdge,
  simplebezier: SimpleBezierEdge,
};

const CanvasWithDnd: React.FC<CanvasWithDndProps> = ({
  viewNodes,
  viewEdges,
  viewViewport,
  isReadOnly,
  agentType,
  agentId,
  agentName,
  agentIconType,
  agentIcon,
  agentIconUrl,
  onNodesChange,
  onEdgesChange,
  onConnect,
  onNodeClick,
  onNodeContextMenu,
  onPaneClick,
  onViewportChange,
  fitOnMount = false,
}) => {
  const createNodePickerOpen = useCreateNodeModal(state => state.open);

  const interactionMode = useWorkflowStore.use.interactionMode();
  const {
    isConnecting,
    isCanvasInteracting,
    beginInteraction,
    finishInteraction,
    beginConnection,
    finishConnection,
  } = useCanvasInteractionGuard();
  const { handleConnectStart, handleConnectEnd } = useConnectDropCreate({
    isReadOnly,
    beginConnection,
    finishConnection,
  });
  const { draggingNodeType, handleDragOver, handleDrop } = useDragCreateNode({
    isReadOnly,
    viewViewport,
  });
  const hideRightPanels = isCanvasInteracting || Boolean(draggingNodeType) || createNodePickerOpen;

  const onConnectWrapper = React.useMemo(
    () => (isReadOnly ? () => {} : onConnect),
    [isReadOnly, onConnect]
  );

  const setEdgeDescId = useWorkflowStore.use.setEdgeDescId();
  const setEdgeDescPosition = useWorkflowStore.use.setEdgeDescPosition();

  const handleEdgeDoubleClick = React.useCallback(
    (event: React.MouseEvent, edge: Edge) => {
      if (isReadOnly) return;
      // Record mouse client position for the floating editor
      setEdgeDescId(edge.id);
      setEdgeDescPosition({ x: event.clientX, y: event.clientY });
    },
    [isReadOnly, setEdgeDescId, setEdgeDescPosition]
  );

  return (
    <WorkflowContextMenu disabled={isReadOnly}>
      <ReactFlow
        nodes={viewNodes}
        edges={viewEdges}
        nodeTypes={nodeTypes}
        edgeTypes={EDGE_TYPES}
        onNodesChange={isReadOnly ? undefined : onNodesChange}
        onEdgesChange={isReadOnly ? undefined : onEdgesChange}
        onConnect={onConnectWrapper}
        onEdgeDoubleClick={handleEdgeDoubleClick}
        onConnectStart={handleConnectStart}
        onConnectEnd={handleConnectEnd}
        onNodeDragStart={(_e, node) => {
          useWorkflowStore.setState({ suppressNextLayoutDirty: false });
          beginInteraction('node-drag');
          const st = useWorkflowStore.getState() as unknown as {
            selectNode?: (id: string | null) => void;
            setSelectionSource?: (src: string) => void;
          };
          st.selectNode?.(node.id);
          st.setSelectionSource?.('drag');
        }}
        onNodeDragStop={(_e, node) => {
          finishInteraction('node-drag');
          const st = useWorkflowStore.getState() as unknown as {
            selectNode?: (id: string | null) => void;
            setSelectionSource?: (src: string) => void;
          };
          // Ensure selection persists
          st.selectNode?.(node.id);
          // Defer setting to 'click' so React Flow finishes internal drag state and click suppression window passes
          if (typeof window !== 'undefined') {
            window.requestAnimationFrame(() => {
              st.setSelectionSource?.('click');
            });
          } else {
            st.setSelectionSource?.('click');
          }
        }}
        onNodeClick={onNodeClick}
        onNodeContextMenu={onNodeContextMenu}
        onPaneClick={onPaneClick}
        onViewportChange={onViewportChange}
        onDragOver={handleDragOver}
        onDrop={handleDrop}
        onMoveStart={() => {
          // Cancel auto-follow only for user-initiated moves (not programmatic pan)
          try {
            const st = useWorkflowStore.getState() as unknown as {
              isProgrammaticPan?: boolean;
              setAutoFollow?: (enabled: boolean) => void;
            };
            if (!st?.isProgrammaticPan && typeof st?.setAutoFollow === 'function') {
              useWorkflowStore.setState({ suppressNextViewportDirty: false });
              st.setAutoFollow(false);
              beginInteraction('move');
            }
          } catch {
            // no-op
          }
        }}
        onMoveEnd={() => {
          finishInteraction('move');
        }}
        onSelectionDragStart={() => {
          beginInteraction('selection-drag');
        }}
        onSelectionDragStop={() => {
          finishInteraction('selection-drag');
        }}
        viewport={viewViewport}
        fitView={fitOnMount}
        attributionPosition="bottom-left"
        proOptions={{ hideAttribution: true }}
        connectionLineStyle={{ stroke: '#6366f1', strokeWidth: 2 }}
        snapToGrid
        snapGrid={[5, 5]}
        // Enforce our own keyboard logic by disabling RF defaults
        deleteKeyCode={null}
        selectionKeyCode={null}
        multiSelectionKeyCode={null}
        // Interaction based on mode and creation state
        elementsSelectable
        nodesDraggable={!isReadOnly}
        nodesConnectable={!isReadOnly}
        selectionOnDrag={!isReadOnly && interactionMode !== 'hand'}
        panOnDrag={interactionMode === 'hand' && !isConnecting}
        zoomOnScroll
        panOnScroll={interactionMode === 'hand' && !isConnecting}
        panOnScrollMode={PanOnScrollMode.Free}
        preventScrolling={false}
      >
        <Background
          variant={BackgroundVariant.Lines}
          bgColor="#f9fafb"
          gap={20}
          size={2}
          color="#e5e7eb"
        />

        <GlobalContainerOverlay />

        <WorkflowCanvasPanels
          agentType={agentType}
          agentId={agentId}
          agentName={agentName}
          agentIconType={agentIconType}
          agentIcon={agentIcon}
          agentIconUrl={agentIconUrl}
          isReadOnly={isReadOnly}
          draggingNodeType={draggingNodeType}
          temporarilyHidden={hideRightPanels}
        />
      </ReactFlow>
      {/* Workflow Modals */}
      <CreateNodeModal />
      <EdgeDescriptionEditor />
      <WorkflowNodeDragPreview />
    </WorkflowContextMenu>
  );
};

export default React.memo(CanvasWithDnd);
