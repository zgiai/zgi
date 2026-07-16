import React from 'react';
import { useReactFlow } from '@xyflow/react';
import { useWorkflowStore } from '../../../store';
import { canPlaceNodeInContainer } from '../../../store/helpers/container-rules';
import type { WorkflowNode } from '../../../store';
import { useCreateNodeModal, type OriginatingEdgeInfo } from '../../../hooks/use-create-node-modal';
import {
  computeNewChildPlacement,
  applyPlacementFirstPass,
  applyPlacementSecondPass,
  getNodeBoxFromGraph,
  computeViewportCenterFlowPos,
  computeStaggeredPosition,
  screenToFlowPositionManual,
} from '../services/placement';
import { shiftDownstreamNodes } from '../services/placement';
import { resolveContainerContextId, resolveAnchorIdForAppend } from '../services/context';
import { autoConnectFromHandle, reconnectForInsert } from '../services/create-node';
import type { OriginHandleProp } from '../types';
import type { BuiltinToolProvider, BuiltinToolItem } from '@/services/types/tool';
import { pickLocale, mapParametersToFormFields, createInitialBindings } from '@/utils/tool-helpers';
import { useLocale } from '@/hooks/use-locale';
import { PLACE_GAP_X, PLACE_GAP_Y } from '../constants/place';
import { PAD_X, PAD_Y } from '../constants/iteration-layout';
import type { ToolNodeData } from '../../../nodes/tool/config';

const isWorkflowCreationReadOnly = () => {
  const { mode, canEdit } = useWorkflowStore.getState();
  return mode === 'history' || !canEdit;
};

interface CreationActionsParams {
  position: { x: number; y: number } | null;
  originatingHandle: OriginHandleProp | null;
  originatingEdge: OriginatingEdgeInfo | null;
  onClose: () => void;
  isReadOnly: boolean;
  createNodeByType: (
    type: string,
    pos: { x: number; y: number },
    parentId?: string,
    initialData?: Partial<ToolNodeData>
  ) => string | null;
}

export const useCreationActions = ({
  position,
  originatingHandle,
  originatingEdge,
  onClose,
  isReadOnly,
  createNodeByType,
}: CreationActionsParams) => {
  const rf = useReactFlow();
  const { locale } = useLocale();
  const isSelectingRef = React.useRef(false);

  const setEdges = useWorkflowStore.use.setEdges();
  const edges = useWorkflowStore.use.edges();
  const incrementCreationOffsetIndex = useWorkflowStore.use.incrementCreationOffsetIndex();
  const { clearOriginatingHandle, clearOriginatingEdge } = useCreateNodeModal();
  const setIsCreatingNode = useWorkflowStore.use.setIsCreatingNode();

  const getNodeBox = React.useCallback(
    (id: string) =>
      getNodeBoxFromGraph(
        rf as unknown as {
          getNode: (id: string) =>
            | {
                width?: number;
                height?: number;
                position?: { x: number; y: number };
                data?: { type?: string };
              }
            | undefined;
        },
        id
      ),
    [rf]
  );

  const containerContextId: string | undefined = React.useMemo(
    () =>
      resolveContainerContextId({
        originatingHandle,
        originatingEdge,
      }),
    [originatingHandle, originatingEdge]
  );
  const containerContextType = useWorkflowStore(state =>
    containerContextId
      ? ((state.nodes.find(n => n.id === containerContextId)?.data || {}) as { type?: string }).type
      : undefined
  );

  const createNodeByTypeWrapper = React.useCallback(
    (type: string, pos: { x: number; y: number }, initialData?: Partial<ToolNodeData>) => {
      if (isReadOnly || isWorkflowCreationReadOnly()) {
        return null;
      }
      if (containerContextId && !canPlaceNodeInContainer(type, containerContextType)) {
        return null;
      }
      return createNodeByType(type, pos, containerContextId, initialData);
    },
    [containerContextId, containerContextType, createNodeByType, isReadOnly]
  );

  const handleOpenChange = React.useCallback(
    (open: boolean) => {
      if (!open) {
        onClose();
        if (!isSelectingRef.current) {
          clearOriginatingHandle();
          clearOriginatingEdge();
        }
      }
    },
    [onClose, clearOriginatingHandle, clearOriginatingEdge]
  );

  const handleAddNode = React.useCallback(
    (type: string) => {
      if (isReadOnly || isWorkflowCreationReadOnly()) {
        onClose();
        return;
      }
      if (originatingEdge) {
        isSelectingRef.current = true;
        onClose();
        setIsCreatingNode(true);
        setTimeout(() => {
          if (isWorkflowCreationReadOnly()) {
            useWorkflowStore.getState().setDraggingNodeType(null);
            isSelectingRef.current = false;
            setIsCreatingNode(false);
            return;
          }
          useWorkflowStore.getState().beginHistoryBatch();
          try {
            const insertCenter = (originatingEdge.midPoint ?? position) as { x: number; y: number };
            const newNodeId = createNodeByTypeWrapper(type, insertCenter);
            if (!newNodeId) return;

            if (containerContextId) {
              const parentData1 = (useWorkflowStore
                .getState()
                .nodes.find(n => n.id === containerContextId)?.data || {}) as {
                _children?: string[];
              };
              const prevChildren1 = Array.isArray(parentData1._children)
                ? parentData1._children
                : [];
              if (!prevChildren1.includes(newNodeId)) {
                useWorkflowStore.getState().updateNodeData(containerContextId, {
                  _children: [...prevChildren1, newNodeId],
                } as unknown as WorkflowNode['data']);
              }
              const parentNode = rf.getNode(containerContextId);
              const anchor = getNodeBox(originatingEdge.sourceId);
              const newSize = getNodeBox(newNodeId);
              const target = getNodeBox(originatingEdge.targetId);
              const placement = computeNewChildPlacement({
                parent: { w: parentNode?.width ?? 600, h: parentNode?.height ?? 420 },
                anchor,
                newSize: { w: newSize.w, h: newSize.h },
                mode: 'insert',
                target,
              });
              applyPlacementFirstPass({
                updateNode: useWorkflowStore.getState().updateNode,
                parentId: containerContextId,
                targetId: originatingEdge.targetId,
                placement,
                newNodeId,
                targetBox: target,
              });
              applyPlacementSecondPass({
                rf,
                updateNode: useWorkflowStore.getState().updateNode,
                parentId: containerContextId,
                newNodeId,
                placement,
                mode: 'insert',
                parentBaseW: parentNode?.width ?? 600,
                targetId: originatingEdge.targetId,
                targetBox: target,
              });
              requestAnimationFrame(() => {
                const tNode = rf.getNode(originatingEdge.targetId);
                const finalTx = tNode?.position?.x ?? target.x;
                const dx = finalTx - target.x;
                if (dx > 0) {
                  const edgeLite = edges.map(e => ({ source: e.source, target: e.target }));
                  shiftDownstreamNodes({
                    rf: rf as unknown as {
                      getNode: (id: string) =>
                        | {
                            width?: number;
                            height?: number;
                            position?: { x: number; y: number };
                            data?: { type?: string };
                            parentNode?: string;
                          }
                        | undefined;
                    },
                    updateNode: useWorkflowStore.getState().updateNode,
                    edges: edgeLite,
                    rootId: originatingEdge.targetId,
                    parentId: containerContextId,
                    dx,
                    dy: 0,
                    skipRoot: true,
                  });
                }
              });
            }
            reconnectForInsert({
              edges,
              setEdges,
              edge: {
                edgeId: originatingEdge.edgeId,
                sourceId: originatingEdge.sourceId,
                targetId: originatingEdge.targetId,
                sourceHandle: originatingEdge.sourceHandle,
                targetHandle: originatingEdge.targetHandle,
              },
              newNodeId,
            });
            if (!containerContextId) {
              const anchor = getNodeBox(originatingEdge.sourceId);
              const newSize = getNodeBox(newNodeId);
              const target = getNodeBox(originatingEdge.targetId);
              const placement = computeNewChildPlacement({
                parent: { w: 0, h: 0 },
                anchor,
                newSize: { w: newSize.w, h: newSize.h },
                mode: 'insert',
                target,
              });
              useWorkflowStore.getState().updateNode(newNodeId, {
                position: { x: placement.nx, y: placement.ny },
              } as unknown as WorkflowNode);
              if (typeof placement.targetShiftX === 'number') {
                const dx = placement.targetShiftX - target.x;
                useWorkflowStore.getState().updateNode(originatingEdge.targetId, {
                  position: { x: placement.targetShiftX, y: target.y },
                } as unknown as WorkflowNode);
                if (dx > 0) {
                  const edgeLite = edges.map(e => ({ source: e.source, target: e.target }));
                  shiftDownstreamNodes({
                    rf: rf as unknown as {
                      getNode: (id: string) =>
                        | {
                            width?: number;
                            height?: number;
                            position?: { x: number; y: number };
                            data?: { type?: string };
                            parentNode?: string;
                          }
                        | undefined;
                    },
                    updateNode: useWorkflowStore.getState().updateNode,
                    edges: edgeLite,
                    rootId: originatingEdge.targetId,
                    parentId: undefined,
                    dx,
                    dy: 0,
                    skipRoot: true,
                  });
                }
                requestAnimationFrame(() => {
                  const n2 = rf.getNode(newNodeId);
                  const nw = (n2?.width as number | undefined) ?? newSize.w;
                  const tx2 = Math.max(target.x, placement.nx + nw + PLACE_GAP_X);
                  const curT = rf.getNode(originatingEdge.targetId);
                  const curTx = curT?.position?.x ?? placement.targetShiftX ?? target.x;
                  const d2 = tx2 - curTx;
                  if (d2 !== 0) {
                    useWorkflowStore.getState().updateNode(originatingEdge.targetId, {
                      position: { x: tx2, y: target.y },
                    } as unknown as WorkflowNode);
                    const edgeLite2 = edges.map(e => ({ source: e.source, target: e.target }));
                    if (d2 > 0) {
                      shiftDownstreamNodes({
                        rf: rf as unknown as {
                          getNode: (id: string) =>
                            | {
                                width?: number;
                                height?: number;
                                position?: { x: number; y: number };
                                data?: { type?: string };
                                parentNode?: string;
                              }
                            | undefined;
                        },
                        updateNode: useWorkflowStore.getState().updateNode,
                        edges: edgeLite2,
                        rootId: originatingEdge.targetId,
                        parentId: undefined,
                        dx: d2,
                        dy: 0,
                        skipRoot: true,
                      });
                    }
                  }
                });
              }
            }
            useWorkflowStore.getState().selectNode(newNodeId);
            useWorkflowStore.getState().setSelectionSource('create');
            clearOriginatingEdge();
          } finally {
            useWorkflowStore.getState().setDraggingNodeType(null);
            useWorkflowStore.getState().endHistoryBatch();
            isSelectingRef.current = false;
            setIsCreatingNode(false);
          }
        }, 100);
        return;
      }

      isSelectingRef.current = true;
      onClose();
      setIsCreatingNode(true);
      setTimeout(() => {
        if (isWorkflowCreationReadOnly()) {
          useWorkflowStore.getState().setDraggingNodeType(null);
          isSelectingRef.current = false;
          setIsCreatingNode(false);
          return;
        }
        useWorkflowStore.getState().beginHistoryBatch();
        try {
          const initialPos = position ?? { x: 0, y: 0 };
          if (containerContextId) {
            const createdId = createNodeByTypeWrapper(type, initialPos);
            if (!createdId) return;

            const parentData2 = (useWorkflowStore
              .getState()
              .nodes.find(n => n.id === containerContextId)?.data || {}) as {
              _children?: string[];
            };
            const prevChildren2 = Array.isArray(parentData2._children) ? parentData2._children : [];
            if (!prevChildren2.includes(createdId)) {
              useWorkflowStore.getState().updateNodeData(containerContextId, {
                _children: [...prevChildren2, createdId],
              } as unknown as WorkflowNode['data']);
            }
            const anchorId = resolveAnchorIdForAppend({
              originatingHandle,
              iterationParentId: containerContextId,
              createdId,
            });
            const parentNode2 = rf.getNode(containerContextId);
            const anchor2 = getNodeBox(anchorId);
            const newSize2 = getNodeBox(createdId);
            let placement2: { nx: number; ny: number; nextPw: number; nextPh: number };
            if (originatingHandle) {
              const isSource = originatingHandle.handleType === 'source';
              const sameHandleEdges = edges.filter(e =>
                isSource
                  ? e.source === originatingHandle.nodeId &&
                    (e.sourceHandle ? e.sourceHandle === originatingHandle.handleId : true)
                  : e.target === originatingHandle.nodeId &&
                    (e.targetHandle ? e.targetHandle === originatingHandle.handleId : true)
              );
              const connectedNodeIds = sameHandleEdges.map(e => (isSource ? e.target : e.source));
              if (connectedNodeIds.length === 0) {
                const base = computeNewChildPlacement({
                  parent: { w: parentNode2?.width ?? 600, h: parentNode2?.height ?? 420 },
                  anchor: anchor2,
                  newSize: { w: newSize2.w, h: newSize2.h },
                  mode: 'append',
                });
                placement2 = base;
              } else {
                const boxes = connectedNodeIds.map(nid => getNodeBox(nid));
                const maxBox = boxes.reduce((acc, b) => (b.y + b.h > acc.y + acc.h ? b : acc));
                const nx = maxBox.x;
                const ny = maxBox.y + maxBox.h + PLACE_GAP_Y;
                const requiredRight = nx + newSize2.w + PAD_X * 2;
                const requiredBottom = ny + newSize2.h + PAD_Y * 2;
                const nextPw = Math.max(parentNode2?.width ?? 600, requiredRight);
                const nextPh = Math.max(parentNode2?.height ?? 420, requiredBottom);
                placement2 = { nx, ny, nextPw, nextPh };
              }
            } else {
              placement2 = computeNewChildPlacement({
                parent: { w: parentNode2?.width ?? 600, h: parentNode2?.height ?? 420 },
                anchor: anchor2,
                newSize: { w: newSize2.w, h: newSize2.h },
                mode: 'append',
              });
            }
            applyPlacementFirstPass({
              updateNode: useWorkflowStore.getState().updateNode,
              parentId: containerContextId,
              placement: placement2,
              newNodeId: createdId,
            });
            applyPlacementSecondPass({
              rf,
              updateNode: useWorkflowStore.getState().updateNode,
              parentId: containerContextId,
              newNodeId: createdId,
              placement: placement2,
              mode: 'append',
              parentBaseW: parentNode2?.width ?? 600,
            });

            if (originatingHandle) {
              const srcIsOrigin = originatingHandle.handleType === 'source';
              const sourceId = srcIsOrigin ? originatingHandle.nodeId : createdId;
              const targetId = srcIsOrigin ? createdId : originatingHandle.nodeId;
              autoConnectFromHandle({
                edges,
                setEdges,
                sourceId,
                targetId,
                sourceHandle: srcIsOrigin ? originatingHandle.handleId : undefined,
                targetHandle: srcIsOrigin ? undefined : originatingHandle.handleId,
                inLoop: true,
              });
            }
            useWorkflowStore.getState().selectNode(createdId);
            useWorkflowStore.getState().setSelectionSource('create');
          } else if (originatingHandle) {
            let placePos: { x: number; y: number };

            if (position) {
              placePos = screenToFlowPositionManual(position, useWorkflowStore.getState().viewport);
            } else {
              const anchorBox = getNodeBox(originatingHandle.nodeId);
              const isSource = originatingHandle.handleType === 'source';
              const sameHandleEdges = edges.filter(e =>
                isSource
                  ? e.source === originatingHandle.nodeId &&
                    (e.sourceHandle ? e.sourceHandle === originatingHandle.handleId : true)
                  : e.target === originatingHandle.nodeId &&
                    (e.targetHandle ? e.targetHandle === originatingHandle.handleId : true)
              );
              const connectedNodeIds = sameHandleEdges.map(e => (isSource ? e.target : e.source));
              if (connectedNodeIds.length === 0) {
                placePos = { x: anchorBox.x + anchorBox.w + PLACE_GAP_X, y: anchorBox.y };
              } else {
                const boxes = connectedNodeIds.map(nid => getNodeBox(nid));
                const maxBox = boxes.reduce((acc, b) => (b.y + b.h > acc.y + acc.h ? b : acc));
                placePos = { x: maxBox.x, y: maxBox.y + maxBox.h + PLACE_GAP_Y };
              }
            }
            const createdId = createNodeByTypeWrapper(type, placePos);
            if (!createdId) return;

            const srcIsOrigin = originatingHandle.handleType === 'source';
            const sourceId = srcIsOrigin ? originatingHandle.nodeId : createdId;
            const targetId = srcIsOrigin ? createdId : originatingHandle.nodeId;
            autoConnectFromHandle({
              edges,
              setEdges,
              sourceId,
              targetId,
              sourceHandle: srcIsOrigin ? originatingHandle.handleId : undefined,
              targetHandle: srcIsOrigin ? undefined : originatingHandle.handleId,
              inLoop: false,
            });
            useWorkflowStore.getState().selectNode(createdId);
            useWorkflowStore.getState().setSelectionSource('create');
            clearOriginatingHandle();
          } else {
            let pos: { x: number; y: number };
            if (position) {
              pos = screenToFlowPositionManual(position, useWorkflowStore.getState().viewport);
            } else {
              const viewportLatest = useWorkflowStore.getState().viewport;
              const indexLatest = useWorkflowStore.getState().creationOffsetIndex;
              const basePos = computeViewportCenterFlowPos(viewportLatest);
              pos = computeStaggeredPosition(basePos, indexLatest);
            }
            const createdId = createNodeByTypeWrapper(type, pos);
            if (!createdId) return;
            useWorkflowStore.getState().selectNode(createdId);
            useWorkflowStore.getState().setSelectionSource('create');
            if (!position) incrementCreationOffsetIndex();
          }
        } finally {
          useWorkflowStore.getState().setDraggingNodeType(null);
          useWorkflowStore.getState().endHistoryBatch();
          isSelectingRef.current = false;
          setIsCreatingNode(false);
        }
      }, 100);
    },
    [
      originatingEdge,
      isReadOnly,
      onClose,
      setIsCreatingNode,
      position,
      createNodeByTypeWrapper,
      containerContextId,
      getNodeBox,
      rf,
      edges,
      setEdges,
      clearOriginatingEdge,
      originatingHandle,
      clearOriginatingHandle,
      incrementCreationOffsetIndex,
    ]
  );

  const handleAddBuiltinTool = React.useCallback(
    (provider: BuiltinToolProvider, tool: BuiltinToolItem) => {
      if (isReadOnly || isWorkflowCreationReadOnly()) {
        onClose();
        return;
      }
      if (originatingEdge) {
        isSelectingRef.current = true;
        onClose();
        setIsCreatingNode(true);
        setTimeout(() => {
          if (isWorkflowCreationReadOnly()) {
            useWorkflowStore.getState().setDraggingNodeType(null);
            isSelectingRef.current = false;
            setIsCreatingNode(false);
            return;
          }
          useWorkflowStore.getState().beginHistoryBatch();
          try {
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

            const insertCenter = (originatingEdge.midPoint ?? position) as { x: number; y: number };
            const newNodeId = createNodeByTypeWrapper('tool', insertCenter, initialData);
            if (!newNodeId) return;

            if (containerContextId) {
              const parentData3 = (useWorkflowStore
                .getState()
                .nodes.find(n => n.id === containerContextId)?.data || {}) as {
                _children?: string[];
              };
              const prevChildren3 = Array.isArray(parentData3._children)
                ? parentData3._children
                : [];
              if (!prevChildren3.includes(newNodeId)) {
                useWorkflowStore.getState().updateNodeData(containerContextId, {
                  _children: [...prevChildren3, newNodeId],
                } as unknown as WorkflowNode['data']);
              }
              const parentNode = rf.getNode(containerContextId);
              const anchor = getNodeBox(originatingEdge.sourceId);
              const newSize = getNodeBox(newNodeId);
              const target = getNodeBox(originatingEdge.targetId);
              const placement = computeNewChildPlacement({
                parent: { w: parentNode?.width ?? 600, h: parentNode?.height ?? 420 },
                anchor,
                newSize: { w: newSize.w, h: newSize.h },
                mode: 'insert',
                target,
              });
              applyPlacementFirstPass({
                updateNode: useWorkflowStore.getState().updateNode,
                parentId: containerContextId,
                targetId: originatingEdge.targetId,
                placement,
                newNodeId,
                targetBox: target,
              });
              applyPlacementSecondPass({
                rf,
                updateNode: useWorkflowStore.getState().updateNode,
                parentId: containerContextId,
                newNodeId,
                placement,
                mode: 'insert',
                parentBaseW: parentNode?.width ?? 600,
                targetId: originatingEdge.targetId,
                targetBox: target,
              });
              requestAnimationFrame(() => {
                const tNode = rf.getNode(originatingEdge.targetId);
                const finalTx = tNode?.position?.x ?? target.x;
                const dx = finalTx - target.x;
                if (dx > 0) {
                  const edgeLite = edges.map(e => ({ source: e.source, target: e.target }));
                  shiftDownstreamNodes({
                    rf: rf as unknown as {
                      getNode: (id: string) =>
                        | {
                            width?: number;
                            height?: number;
                            position?: { x: number; y: number };
                            data?: { type?: string };
                            parentNode?: string;
                          }
                        | undefined;
                    },
                    updateNode: useWorkflowStore.getState().updateNode,
                    edges: edgeLite,
                    rootId: originatingEdge.targetId,
                    parentId: containerContextId,
                    dx,
                    dy: 0,
                    skipRoot: true,
                  });
                }
              });
            }
            reconnectForInsert({
              edges,
              setEdges,
              edge: {
                edgeId: originatingEdge.edgeId,
                sourceId: originatingEdge.sourceId,
                targetId: originatingEdge.targetId,
                sourceHandle: originatingEdge.sourceHandle,
                targetHandle: originatingEdge.targetHandle,
              },
              newNodeId,
            });
            if (!containerContextId) {
              const anchor = getNodeBox(originatingEdge.sourceId);
              const newSize = getNodeBox(newNodeId);
              const target = getNodeBox(originatingEdge.targetId);
              const placement = computeNewChildPlacement({
                parent: { w: 0, h: 0 },
                anchor,
                newSize: { w: newSize.w, h: newSize.h },
                mode: 'insert',
                target,
              });
              useWorkflowStore.getState().updateNode(newNodeId, {
                position: { x: placement.nx, y: placement.ny },
              } as unknown as WorkflowNode);
              if (typeof placement.targetShiftX === 'number') {
                const dx = placement.targetShiftX - target.x;
                useWorkflowStore.getState().updateNode(originatingEdge.targetId, {
                  position: { x: placement.targetShiftX, y: target.y },
                } as unknown as WorkflowNode);
                if (dx > 0) {
                  const edgeLite = edges.map(e => ({ source: e.source, target: e.target }));
                  shiftDownstreamNodes({
                    rf: rf as unknown as {
                      getNode: (id: string) =>
                        | {
                            width?: number;
                            height?: number;
                            position?: { x: number; y: number };
                            data?: { type?: string };
                            parentNode?: string;
                          }
                        | undefined;
                    },
                    updateNode: useWorkflowStore.getState().updateNode,
                    edges: edgeLite,
                    rootId: originatingEdge.targetId,
                    parentId: undefined,
                    dx,
                    dy: 0,
                    skipRoot: true,
                  });
                }
                requestAnimationFrame(() => {
                  const n2 = rf.getNode(newNodeId);
                  const nw = (n2?.width as number | undefined) ?? newSize.w;
                  const tx2 = Math.max(target.x, placement.nx + nw + PLACE_GAP_X);
                  const curT = rf.getNode(originatingEdge.targetId);
                  const curTx = curT?.position?.x ?? placement.targetShiftX ?? target.x;
                  const d2 = tx2 - curTx;
                  if (d2 !== 0) {
                    useWorkflowStore.getState().updateNode(originatingEdge.targetId, {
                      position: { x: tx2, y: target.y },
                    } as unknown as WorkflowNode);
                    const edgeLite2 = edges.map(e => ({ source: e.source, target: e.target }));
                    if (d2 > 0) {
                      shiftDownstreamNodes({
                        rf: rf as unknown as {
                          getNode: (id: string) =>
                            | {
                                width?: number;
                                height?: number;
                                position?: { x: number; y: number };
                                data?: { type?: string };
                                parentNode?: string;
                              }
                            | undefined;
                        },
                        updateNode: useWorkflowStore.getState().updateNode,
                        edges: edgeLite2,
                        rootId: originatingEdge.targetId,
                        parentId: undefined,
                        dx: d2,
                        dy: 0,
                        skipRoot: true,
                      });
                    }
                  }
                });
              }
            }
            // Data already passed via initialData in createNodeByTypeWrapper
            useWorkflowStore.getState().selectNode(newNodeId);
            useWorkflowStore.getState().setSelectionSource('create');
            clearOriginatingEdge();
          } finally {
            useWorkflowStore.getState().setDraggingNodeType(null);
            useWorkflowStore.getState().endHistoryBatch();
            isSelectingRef.current = false;
            setIsCreatingNode(false);
          }
        }, 100);
        return;
      }

      isSelectingRef.current = true;
      onClose();
      setIsCreatingNode(true);
      setTimeout(() => {
        if (isWorkflowCreationReadOnly()) {
          useWorkflowStore.getState().setDraggingNodeType(null);
          isSelectingRef.current = false;
          setIsCreatingNode(false);
          return;
        }
        useWorkflowStore.getState().beginHistoryBatch();
        try {
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
          const initialPos = position ?? { x: 0, y: 0 };

          if (containerContextId) {
            const createdId = createNodeByTypeWrapper('tool', initialPos, initialData);
            if (!createdId) return;

            const parentData4 = (useWorkflowStore
              .getState()
              .nodes.find(n => n.id === containerContextId)?.data || {}) as {
              _children?: string[];
            };
            const prevChildren4 = Array.isArray(parentData4._children) ? parentData4._children : [];
            if (!prevChildren4.includes(createdId)) {
              useWorkflowStore.getState().updateNodeData(containerContextId, {
                _children: [...prevChildren4, createdId],
              } as unknown as WorkflowNode['data']);
            }
            const anchorId = resolveAnchorIdForAppend({
              originatingHandle,
              iterationParentId: containerContextId,
              createdId,
            });
            const parentNode4 = rf.getNode(containerContextId);
            const anchor4 = getNodeBox(anchorId);
            const newSize4 = getNodeBox(createdId);
            let placement4: { nx: number; ny: number; nextPw: number; nextPh: number };
            if (originatingHandle) {
              const isSource = originatingHandle.handleType === 'source';
              const sameHandleEdges = edges.filter(e =>
                isSource
                  ? e.source === originatingHandle.nodeId &&
                    (e.sourceHandle ? e.sourceHandle === originatingHandle.handleId : true)
                  : e.target === originatingHandle.nodeId &&
                    (e.targetHandle ? e.targetHandle === originatingHandle.handleId : true)
              );
              const connectedNodeIds = sameHandleEdges.map(e => (isSource ? e.target : e.source));
              if (connectedNodeIds.length === 0) {
                const base = computeNewChildPlacement({
                  parent: { w: parentNode4?.width ?? 600, h: parentNode4?.height ?? 420 },
                  anchor: anchor4,
                  newSize: { w: newSize4.w, h: newSize4.h },
                  mode: 'append',
                });
                placement4 = base;
              } else {
                const boxes = connectedNodeIds.map(nid => getNodeBox(nid));
                const maxBox = boxes.reduce((acc, b) => (b.y + b.h > acc.y + acc.h ? b : acc));
                const nx = maxBox.x;
                const ny = maxBox.y + maxBox.h + PLACE_GAP_Y;
                const requiredRight = nx + newSize4.w + PAD_X * 2;
                const requiredBottom = ny + newSize4.h + PAD_Y * 2;
                const nextPw = Math.max(parentNode4?.width ?? 600, requiredRight);
                const nextPh = Math.max(parentNode4?.height ?? 420, requiredBottom);
                placement4 = { nx, ny, nextPw, nextPh };
              }
            } else {
              placement4 = computeNewChildPlacement({
                parent: { w: parentNode4?.width ?? 600, h: parentNode4?.height ?? 420 },
                anchor: anchor4,
                newSize: { w: newSize4.w, h: newSize4.h },
                mode: 'append',
              });
            }
            applyPlacementFirstPass({
              updateNode: useWorkflowStore.getState().updateNode,
              parentId: containerContextId,
              placement: placement4,
              newNodeId: createdId,
            });
            applyPlacementSecondPass({
              rf,
              updateNode: useWorkflowStore.getState().updateNode,
              parentId: containerContextId,
              newNodeId: createdId,
              placement: placement4,
              mode: 'append',
              parentBaseW: parentNode4?.width ?? 600,
            });

            if (originatingHandle) {
              const srcIsOrigin = originatingHandle.handleType === 'source';
              const sourceId = srcIsOrigin ? originatingHandle.nodeId : createdId;
              const targetId = srcIsOrigin ? createdId : originatingHandle.nodeId;
              autoConnectFromHandle({
                edges,
                setEdges,
                sourceId,
                targetId,
                sourceHandle: srcIsOrigin ? originatingHandle.handleId : undefined,
                targetHandle: srcIsOrigin ? undefined : originatingHandle.handleId,
                inLoop: true,
              });
            }
            useWorkflowStore.getState().selectNode(createdId);
            useWorkflowStore.getState().setSelectionSource('create');
          } else if (originatingHandle) {
            let pos: { x: number; y: number };
            if (position) {
              pos = screenToFlowPositionManual(position, useWorkflowStore.getState().viewport);
            } else {
              const anchorBox = getNodeBox(originatingHandle.nodeId);
              const isSource = originatingHandle.handleType === 'source';
              const sameHandleEdges = edges.filter(e =>
                isSource
                  ? e.source === originatingHandle.nodeId &&
                    (e.sourceHandle ? e.sourceHandle === originatingHandle.handleId : true)
                  : e.target === originatingHandle.nodeId &&
                    (e.targetHandle ? e.targetHandle === originatingHandle.handleId : true)
              );
              const connectedNodeIds = sameHandleEdges.map(e => (isSource ? e.target : e.source));
              if (connectedNodeIds.length === 0) {
                pos = { x: anchorBox.x + anchorBox.w + PLACE_GAP_X, y: anchorBox.y };
              } else {
                const boxes = connectedNodeIds.map(nid => getNodeBox(nid));
                const maxBox = boxes.reduce((acc, b) => (b.y + b.h > acc.y + acc.h ? b : acc));
                pos = { x: maxBox.x, y: maxBox.y + maxBox.h + PLACE_GAP_Y };
              }
            }

            const createdId = createNodeByTypeWrapper('tool', pos, initialData);
            if (!createdId) return;

            const srcIsOrigin = originatingHandle.handleType === 'source';
            const sourceId = srcIsOrigin ? originatingHandle.nodeId : createdId;
            const targetId = srcIsOrigin ? createdId : originatingHandle.nodeId;
            autoConnectFromHandle({
              edges,
              setEdges,
              sourceId,
              targetId,
              sourceHandle: srcIsOrigin ? originatingHandle.handleId : undefined,
              targetHandle: srcIsOrigin ? undefined : originatingHandle.handleId,
              inLoop: false,
            });
            useWorkflowStore.getState().selectNode(createdId);
            useWorkflowStore.getState().setSelectionSource('create');
            clearOriginatingHandle();
          } else {
            let pos: { x: number; y: number };
            if (position) {
              pos = screenToFlowPositionManual(position, useWorkflowStore.getState().viewport);
            } else {
              const viewportLatest = useWorkflowStore.getState().viewport;
              const indexLatest = useWorkflowStore.getState().creationOffsetIndex;
              const basePos = computeViewportCenterFlowPos(viewportLatest);
              pos = computeStaggeredPosition(basePos, indexLatest);
            }
            const createdId = createNodeByTypeWrapper('tool', pos, initialData);
            if (!createdId) return;
            useWorkflowStore.getState().selectNode(createdId);
            useWorkflowStore.getState().setSelectionSource('create');
            if (!position) incrementCreationOffsetIndex();
          }
        } finally {
          useWorkflowStore.getState().setDraggingNodeType(null);
          useWorkflowStore.getState().endHistoryBatch();
          isSelectingRef.current = false;
          setIsCreatingNode(false);
        }
      }, 100);
    },
    [
      originatingEdge,
      isReadOnly,
      onClose,
      setIsCreatingNode,
      locale,
      position,
      createNodeByTypeWrapper,
      containerContextId,
      getNodeBox,
      rf,
      edges,
      setEdges,
      clearOriginatingEdge,
      originatingHandle,
      clearOriginatingHandle,
      incrementCreationOffsetIndex,
    ]
  );

  return { handleAddNode, handleAddBuiltinTool, handleOpenChange };
};
