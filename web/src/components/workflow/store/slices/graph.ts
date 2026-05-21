// Graph slice: nodes/edges state and graph operations
import type { WorkflowEdge, WorkflowNode, WorkflowNodeData } from '../type';
import type {
  EdgeChange,
  NodeChange,
  OnConnect,
  OnEdgesChange,
  OnNodesChange,
} from '@xyflow/react';
import { addEdge, applyEdgeChanges, applyNodeChanges } from '@xyflow/react';
import { adjustContainerLayout, getDescendants, computeNodeIdToTitle } from '../helpers/graph';
import {
  getValidSourceHandles,
  getValidTargetHandles,
  hasDynamicHandles,
} from '../helpers/handles';

import { getDefaultTitleByType, getNextNodeTitle } from '../helpers/titles';
import {
  getIncomingSources as graphGetIncomingSources,
  getAncestors as graphGetAncestors,
  getUpstreamVariables as graphGetUpstreamVariables,
  getUpstreamWritableVariables as graphGetUpstreamWritableVariables,
  type UpstreamExportItem,
  dedupeEdges as graphDedupeEdges,
} from '../helpers/graph';
import {
  generateNodeId as nodeGenerateNodeId,
  canAddNode as nodeCanAddNode,
  getUniqueNodeTypes as nodeGetUniqueNodeTypes,
  sanitizeNodes as nodeSanitizeNodes,
} from '../helpers/nodes';
import { isContainerNode, isContainerStartNode } from '../type';
import { pushHistory as histPushHistory } from '../helpers/history';
import type { GraphSnapshot, RunGraphSnapshot } from '../helpers/history';
import type { AgentType } from '@/services/types/agent';
import { AgentType as AgentTypeEnum } from '@/services/types/agent';
import { NODE_THEMES } from '../../nodes/custom/config';

export interface GraphSlice {
  // Graph state
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];

  // Graph operations
  setNodes: (nodes: WorkflowNode[]) => void;
  setEdges: (edges: WorkflowEdge[]) => void;
  onNodesChange: OnNodesChange;
  onEdgesChange: OnEdgesChange;
  onConnect: OnConnect;

  addNode: (
    nodeData: Partial<WorkflowNodeData>,
    position: { x: number; y: number },
    parentId?: string
  ) => string | null;
  addNodes: (
    items: Array<{ data: Partial<WorkflowNodeData>; position: { x: number; y: number } }>,
    parentId?: string
  ) => string[];
  deleteNode: (nodeId: string) => void;
  deleteNodes: (nodeIds: string[]) => void;

  updateNode: (nodeId: string, updates: Partial<WorkflowNode>) => void;
  updateNodeData: (nodeId: string, data: Partial<WorkflowNodeData>) => void;
  updateNodesData: (
    updates: Array<{ nodeId: string; data: Partial<WorkflowNodeData> }>,
    options?: { markDirty?: boolean; pushHistory?: boolean }
  ) => void;
  updateEdgeData: (edgeId: string, data: Partial<WorkflowEdge['data']>) => void;

  // Utilities
  generateNodeId: () => string;
  canAddNode: (nodeType: string) => boolean;
  getUniqueNodeTypes: () => string[];

  // Graph analysis
  getIncomingSources: (nodeId: string) => string[];
  getAncestors: (nodeId: string) => string[];
  getUpstreamVariables: (nodeId: string) => UpstreamExportItem[];
  getUpstreamWritableVariables: (nodeId: string) => UpstreamExportItem[];
}

// Minimal store shape needed by this slice's get()
interface GraphGet {
  // Core
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  mode: 'edit' | 'history';
  historyPast: GraphSnapshot[];
  historyFuture: GraphSnapshot[];
  agentType: AgentType;
  viewport: { x: number; y: number; zoom: number };
  // UI flags
  isDirty: boolean;
  hasLayoutChanges: boolean;
  suppressNextLayoutDirty?: boolean;
  // History batch flag
  isHistoryBatching: boolean;
  // Run-status slice (for resetting visual run states on edits)
  resetRunStatus: (nodeIds?: string[]) => void;
  resetActiveOutputHandles: (nodeIds?: string[]) => void;
  currentRunningNodeId: string | null;
  setCurrentRunningNodeId: (nodeId: string | null) => void;
  // Performance: sync cached runnable sets
  syncRunnableSets: () => void;
  syncRunnableSetsDebounced: () => void;
  graphVersion: number;
  selectedRunId: string | null;
  historySnapshots: Record<string, RunGraphSnapshot>;
  _analysisCache: {
    upstreamVariables: Map<string, UpstreamExportItem[]>;
    ancestors: Map<string, string[]>;
    version: number;
    runId: string | null;
  };
}

// Setter type kept generic to avoid coupling with the whole store type
// partial can be any shape of the store, consumers ensure correctness
// Replace flag and action name follow Zustand signature
export type StoreSet = (partial: unknown, replace?: boolean, action?: string) => void;

export function createGraphSlice(set: StoreSet, get: () => GraphGet): GraphSlice {
  return {
    // Initial state
    nodes: [],
    edges: [],

    setNodes(nodes: WorkflowNode[]) {
      if (get().mode === 'history') return;
      const shouldPush = !get().isHistoryBatching;
      const { past, future } = shouldPush
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };
      const sanitizedNodes = nodeSanitizeNodes(nodes);
      set(
        {
          historyPast: past,
          historyFuture: future,
          nodes: sanitizedNodes,
          nodeIdToTitle: computeNodeIdToTitle(sanitizedNodes),
          isDirty: true,
          graphVersion: get().graphVersion + 1,
        },
        false,
        'setNodes'
      );
      // Sync cached runnable sets after topology change
      get().syncRunnableSets();
    },

    setEdges(edges: WorkflowEdge[]) {
      if (get().mode === 'history') return;
      const shouldPush = !get().isHistoryBatching;
      const { past, future } = shouldPush
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };
      const uniqueEdges = graphDedupeEdges(edges);
      set(
        {
          historyPast: past,
          historyFuture: future,
          edges: uniqueEdges,
          isDirty: true,
          graphVersion: get().graphVersion + 1,
        },
        false,
        'setEdges'
      );
      // Sync cached runnable sets after topology change
      get().syncRunnableSets();
    },

    onNodesChange(changes) {
      if (get().mode === 'history') return;

      // Filter out removal of protected nodes (Start node, iteration-start etc.)
      const filteredChanges = (changes as Array<NodeChange<WorkflowNode>>).filter(change => {
        if (change.type === 'remove') {
          const node = get().nodes.find(n => n.id === change.id);
          const type = (node?.data as WorkflowNodeData | undefined)?.type;
          if (type === 'start' || isContainerStartNode(type)) {
            return false;
          }
        }
        return true;
      });

      let nextNodes = applyNodeChanges<WorkflowNode>(filteredChanges, get().nodes);
      // Filter out selection changes targeting container-start to keep it unselectable
      nextNodes = nextNodes.map(n => {
        if (isContainerStartNode((n.data as WorkflowNodeData)?.type)) {
          return { ...n, selected: false } as WorkflowNode;
        }
        return n;
      });
      if (nextNodes === get().nodes) return;
      const nodeChanges = changes as Array<NodeChange<WorkflowNode>>;
      const hasLayoutUpdate = nodeChanges.some(
        c => c.type === 'position' || c.type === 'dimensions'
      );
      const adjustedNodes = adjustContainerLayout(nextNodes, nodeChanges);
      // Sync selectedNodeId based on React Flow selection (single select only)
      const selectedNodesList = adjustedNodes.filter(n => (n as WorkflowNode).selected);
      const nextSelectedId = selectedNodesList.length === 1 ? selectedNodesList[0].id : null;
      // Determine selection source for programmatic selection (e.g., box-select)
      const prevSelSrc = (get() as unknown as { selectionSource?: string }).selectionSource as
        | 'none'
        | 'click'
        | 'create'
        | 'drag'
        | 'program'
        | undefined;
      // Keep 'drag' during position/size updates to avoid flicker in panel visibility while dragging
      let nextSelSrc: 'none' | 'click' | 'create' | 'drag' | 'program';
      if (!nextSelectedId) {
        // When no single node is selected (deselect or multi-select), use 'none'
        // so that NodeFloatingPanel closes cleanly.
        nextSelSrc = 'none';
      } else if (prevSelSrc === 'drag') {
        nextSelSrc = 'drag';
      } else if (prevSelSrc === 'click' || prevSelSrc === 'create') {
        nextSelSrc = prevSelSrc;
      } else {
        nextSelSrc = 'program';
      }
      if (hasLayoutUpdate) {
        const suppress = (get() as unknown as { suppressNextLayoutHistoryPush?: boolean })
          .suppressNextLayoutHistoryPush;

        // Check if there are any non-position/dimensions changes (like title or type changes in data)
        const hasStructuralChanges = changes.some(
          c => c.type !== 'position' && c.type !== 'dimensions'
        );

        const suppressLayoutDirty =
          !hasStructuralChanges &&
          !nodeChanges.some(c => c.type === 'position' && 'dragging' in c && c.dragging) &&
          Boolean(get().suppressNextLayoutDirty);

        const nextState: Partial<GraphSlice> & Record<string, unknown> = {
          nodes: adjustedNodes,
          hasLayoutChanges: suppressLayoutDirty ? get().hasLayoutChanges : true,
          selectedNodeId: nextSelectedId,
          selectionSource: nextSelSrc,
          suppressNextLayoutHistoryPush: suppress ? false : undefined,
          ...(get().suppressNextLayoutDirty ? { suppressNextLayoutDirty: false } : {}),
        };

        if (hasStructuralChanges) {
          nextState.nodeIdToTitle = computeNodeIdToTitle(adjustedNodes);
          nextState.graphVersion = get().graphVersion + 1;
        }

        // Apply autoHeight logic: strip height from changes for auto-height nodes to avoid "stickiness"
        const filteredNextNodes = adjustedNodes.map(node => {
          const type = (node.data as WorkflowNodeData).type;
          const theme = NODE_THEMES[type] || NODE_THEMES.default;
          if (theme.autoHeight) {
            // Ensure height remains undefined so browser can calculate it (height: auto)
            if (node.height !== undefined) {
              return { ...node, height: undefined };
            }
          }
          return node;
        });

        nextState.nodes = filteredNextNodes;

        set(nextState, false, 'onNodesChange');
      } else {
        // Even without layout updates, ensure autoHeight nodes don't have a fixed height
        const nextNodesWithAuto = adjustedNodes.map(node => {
          const type = (node.data as WorkflowNodeData).type;
          const theme = NODE_THEMES[type] || NODE_THEMES.default;
          if (theme.autoHeight && node.height !== undefined) {
            return { ...node, height: undefined };
          }
          return node;
        });

        set(
          {
            nodes: nextNodesWithAuto,
            nodeIdToTitle: computeNodeIdToTitle(nextNodesWithAuto),
            selectedNodeId: nextSelectedId,
            selectionSource: nextSelSrc,
            graphVersion: get().graphVersion + 1,
          },
          false,
          'onNodesChange'
        );
      }
    },

    onEdgesChange(changes) {
      if (get().mode === 'history') return;
      const hasSemanticChange = (changes as Array<EdgeChange<WorkflowEdge>>).some(
        c => c.type === 'add' || c.type === 'remove'
      );
      const nextEdges = applyEdgeChanges<WorkflowEdge>(
        changes as Array<EdgeChange<WorkflowEdge>>,
        get().edges
      );
      // filter out cross-scope edges
      const nodes = get().nodes;
      const filteredEdges = nextEdges.filter(e => {
        const sNode = nodes.find(n => n.id === e.source);
        const tNode = nodes.find(n => n.id === e.target);
        const sPid = (sNode as unknown as { parentId?: string })?.parentId;
        const tPid = (tNode as unknown as { parentId?: string })?.parentId;
        if ((sPid && !tPid) || (!sPid && tPid)) return false;
        if (sPid && tPid && sPid !== tPid) return false;
        return true;
      });
      if (filteredEdges === get().edges) return;
      if (hasSemanticChange) {
        const shouldPush = !get().isHistoryBatching;
        const { past, future } = shouldPush
          ? histPushHistory(get().historyPast, get().nodes, get().edges)
          : { past: get().historyPast, future: get().historyFuture };
        set(
          {
            historyPast: past,
            historyFuture: future,
            edges: filteredEdges,
            isDirty: true,
            graphVersion: get().graphVersion + 1,
          },
          false,
          'onEdgesChange'
        );
      } else {
        set({ edges: filteredEdges }, false, 'onEdgesChange');
      }
      // Sync cached runnable sets after edge topology change
      if (hasSemanticChange) {
        get().syncRunnableSets();
      }
    },

    onConnect(connection) {
      if (get().mode === 'history') return;
      if (!connection.source || !connection.target) return;
      if (connection.source === connection.target) return;

      // Require explicit handle ids to avoid creating invalid edges
      const sourceHandle = connection.sourceHandle;
      const targetHandle = connection.targetHandle;
      if (!sourceHandle || !targetHandle) return;
      const newKey = `${connection.source}|${sourceHandle}|${connection.target}|${targetHandle}`;
      const alreadyExists = get().edges.some(e => {
        // Require exact match including handles to allow multiple branches to same node
        const key = `${e.source}|${e.sourceHandle}|${e.target}|${e.targetHandle}`;
        return key === newKey;
      });
      if (alreadyExists) return;

      // Forbid cross-scope edges
      const nodes = get().nodes;
      const srcNode = nodes.find(n => n.id === connection.source);
      const tgtNode = nodes.find(n => n.id === connection.target);
      const srcPid = (srcNode as unknown as { parentId?: string })?.parentId;
      const tgtPid = (tgtNode as unknown as { parentId?: string })?.parentId;
      const crossScope =
        (srcPid && !tgtPid) || (!srcPid && tgtPid) || (srcPid && tgtPid && srcPid !== tgtPid);
      if (crossScope) return;

      const shouldPush = !get().isHistoryBatching;
      const { past, future } = shouldPush
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };

      const edge: WorkflowEdge = {
        id: `${connection.source}-${sourceHandle}-${connection.target}-${targetHandle}`,
        source: connection.source,
        target: connection.target,
        sourceHandle,
        targetHandle,
        type: 'custom',
        data: {
          sourceType: 'default',
          targetType: 'default',
          isInLoop: Boolean(srcPid && tgtPid && srcPid === tgtPid),
        },
      };

      const nextEdges = graphDedupeEdges(addEdge<WorkflowEdge>(edge, get().edges));
      set(
        {
          historyPast: past,
          historyFuture: future,
          edges: nextEdges,
          isDirty: true,
          graphVersion: get().graphVersion + 1,
        },
        false,
        'onConnect'
      );
      // Sync cached runnable sets after new connection
      get().syncRunnableSets();
    },

    addNode(
      nodeData: Partial<WorkflowNodeData>,
      position: { x: number; y: number },
      parentId?: string
    ) {
      if (get().mode === 'history') return null;
      const existingNodes = get().nodes;

      let id = nodeGenerateNodeId();
      // Safeguard against duplicate IDs
      if (existingNodes.some(n => n.id === id)) {
        console.warn(`[WorkflowStore] Duplicate ID detected: ${id}. Regenerating...`);
        id = `${Date.now()}_${Math.random().toString(36).substring(2, 9)}`;
      }

      const x = Number.isFinite(position.x) ? position.x : 0;
      const y = Number.isFinite(position.y) ? position.y : 0;

      const baseTitle =
        (nodeData as { title?: string }).title ||
        getDefaultTitleByType((nodeData as { type?: string }).type);
      const finalTitle = getNextNodeTitle(
        baseTitle,
        new Set(
          existingNodes.map(n => (n.data as { title?: string }).title).filter(Boolean) as string[]
        )
      );

      const theme = (
        NODE_THEMES as Record<string, { width?: number; height?: number; autoHeight?: boolean }>
      )[((nodeData as { type?: string }).type as string) || 'default'];
      const width = theme && typeof theme.width === 'number' ? theme.width : 280;
      const height = theme && typeof theme.height === 'number' ? theme.height : 120;

      const finalData = {
        ...(nodeData as WorkflowNodeData),
        title: finalTitle,
      } as WorkflowNodeData;
      // If parent is a container, ensure initial position respects padding (Header 40 + Pad 12 = 52)
      let finalPos = { x, y };
      if (parentId) {
        finalPos = {
          x: Math.max(x, 12),
          y: Math.max(y, 52),
        };
      }

      const nodeType = (nodeData as { type?: string }).type;
      const node: WorkflowNode = {
        id,
        type:
          nodeType === 'note' ||
          nodeType === 'approval' ||
          nodeType === 'announcement' ||
          nodeType === 'question-answer'
            ? nodeType
            : 'custom',
        position: finalPos,
        width,
        height: theme && theme.autoHeight ? undefined : height,
        data: finalData,
        ...(parentId ? { parentId, extent: 'parent' as const } : {}),
      } as WorkflowNode;

      const { past, future } = !get().isHistoryBatching
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };
      const nextNodes = [...get().nodes, node];
      set(
        {
          historyPast: past,
          historyFuture: future,
          nodes: nextNodes,
          nodeIdToTitle: computeNodeIdToTitle(nextNodes),
          isDirty: true,
          graphVersion: get().graphVersion + 1,
        },
        false,
        'addNode'
      );
      // Sync cached runnable sets after adding node
      get().syncRunnableSets();
      return id;
    },

    addNodes(
      items: Array<{ data: Partial<WorkflowNodeData>; position: { x: number; y: number } }>,
      parentId?: string
    ) {
      if (get().mode === 'history') return [];
      const existingNodes = get().nodes;
      const titles = new Set<string>();
      for (const n of existingNodes) {
        const t = (n.data as { title?: string }).title;
        if (typeof t === 'string') titles.add(t);
      }

      const ids: string[] = [];
      const toAdd: WorkflowNode[] = [];
      const hasStartAlready = existingNodes.some(
        n => (n.data as { type?: string })?.type === 'start'
      );

      for (const item of items) {
        const dataType = (item.data as { type?: string }).type;
        // Enforce unique start node
        if (
          dataType === 'start' &&
          (hasStartAlready || toAdd.some(n => (n.data as { type?: string })?.type === 'start'))
        ) {
          continue;
        }
        // Agent-type constraints
        const agentType = get().agentType;
        if (dataType === 'end' && agentType === AgentTypeEnum.CONVERSATIONAL_AGENT) continue;
        if (dataType === 'answer' && agentType !== AgentTypeEnum.CONVERSATIONAL_AGENT) continue;

        const id = nodeGenerateNodeId();
        const x = Number.isFinite(item.position.x) ? item.position.x : 0;
        const y = Number.isFinite(item.position.y) ? item.position.y : 0;

        const baseTitle =
          (item.data as { title?: string }).title || getDefaultTitleByType(dataType);
        const finalTitle = getNextNodeTitle(baseTitle, titles);
        titles.add(finalTitle);

        // Initialize width/height from theme defaults
        const theme = (
          NODE_THEMES as Record<string, { width?: number; height?: number; autoHeight?: boolean }>
        )[(dataType as string) || 'default'];
        const width = theme && typeof theme.width === 'number' ? theme.width : 280;
        const height = theme && typeof theme.height === 'number' ? theme.height : 120;

        const finalData = {
          ...(item.data as WorkflowNodeData),
          title: finalTitle,
        } as WorkflowNodeData;
        // If parent is a container, ensure initial position respects padding (Header 40 + Pad 12 = 52)
        let finalPos = { x, y };
        if (parentId) {
          finalPos = {
            x: Math.max(x, 12),
            y: Math.max(y, 52),
          };
        }

        const node: WorkflowNode = {
          id,
          type:
            dataType === 'note' ||
            dataType === 'approval' ||
            dataType === 'announcement' ||
            dataType === 'question-answer'
              ? dataType
              : 'custom',
          position: finalPos,
          width,
          height: theme && theme.autoHeight ? undefined : height,
          data: finalData,
          ...(parentId ? { parentId, extent: 'parent' as const } : {}),
        } as WorkflowNode;

        ids.push(id);
        toAdd.push(node);
      }

      if (toAdd.length === 0) return [];

      const { past, future } = !get().isHistoryBatching
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };
      const nextNodes = [...existingNodes, ...toAdd];
      set(
        {
          historyPast: past,
          historyFuture: future,
          nodes: nextNodes,
          nodeIdToTitle: computeNodeIdToTitle(nextNodes),
          isDirty: true,
          graphVersion: get().graphVersion + 1,
        },
        false,
        'addNodes'
      );
      // Sync cached runnable sets after adding nodes
      get().syncRunnableSets();

      return ids;
    },

    deleteNode(nodeId: string) {
      if (get().mode === 'history') return;
      const node = get().nodes.find(n => n.id === nodeId);
      if (!node) return;
      if (node.data?.type === 'start') {
        return;
      }
      // Prevent deleting container-start nodes directly
      if (isContainerStartNode((node.data as WorkflowNodeData)?.type)) {
        return;
      }

      const shouldPush = !get().isHistoryBatching;
      const { past, future } = shouldPush
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };
      const edges = get().edges;
      const nodes = get().nodes;
      // If deleting a container, cascade delete its children
      if (node && isContainerNode((node.data as WorkflowNodeData)?.type)) {
        const toDelete = new Set<string>();
        toDelete.add(nodeId);
        nodes.forEach(n => {
          const parentId = (n as unknown as { parentId?: string })?.parentId;
          if (parentId === nodeId) toDelete.add(n.id);
        });
        const updatedEdges = edges.filter(e => !toDelete.has(e.source) && !toDelete.has(e.target));
        const nextNodes = nodes.filter(n => !toDelete.has(n.id));
        set({
          historyPast: past,
          historyFuture: future,
          nodes: nextNodes,
          nodeIdToTitle: computeNodeIdToTitle(nextNodes),
          edges: updatedEdges,
          isDirty: true,
          // Clear selection if the deleted node was selected
          graphVersion: get().graphVersion + 1,
        });
        // Sync cached runnable sets after deleting container node
        get().syncRunnableSets();
        return;
      }

      const updatedEdges = edges.filter(edge => edge.source !== nodeId && edge.target !== nodeId);
      const nextNodes = nodes.filter(n => n.id !== nodeId);
      set({
        historyPast: past,
        historyFuture: future,
        nodes: nextNodes,
        nodeIdToTitle: computeNodeIdToTitle(nextNodes),
        edges: updatedEdges,
        isDirty: true,
        selectedNodeId: null,
        graphVersion: get().graphVersion + 1,
      });
      // Sync cached runnable sets after deleting node
      get().syncRunnableSets();
    },

    deleteNodes(nodeIds: string[]) {
      if (get().mode === 'history') return;
      const nodes = get().nodes;
      const edges = get().edges;
      if (!Array.isArray(nodeIds) || nodeIds.length === 0) return;

      // Build deletion set, skipping protected 'start' nodes
      const toDelete = new Set<string>();
      const iterationIds = new Set<string>();
      nodeIds.forEach(id => {
        const node = nodes.find(n => n.id === id);
        if (!node) return;
        if ((node.data as WorkflowNodeData)?.type === 'start') return;
        // Skip container-start nodes (they are deleted only with their parent)
        if (isContainerStartNode((node.data as WorkflowNodeData)?.type)) return;
        toDelete.add(id);
        if (isContainerNode((node.data as WorkflowNodeData)?.type)) {
          iterationIds.add(id);
        }
      });

      if (toDelete.size === 0) return;

      // Cascade: delete direct children of any iteration node
      if (iterationIds.size > 0) {
        nodes.forEach(n => {
          const parentId = (n as unknown as { parentId?: string })?.parentId;
          if (parentId && iterationIds.has(parentId)) {
            toDelete.add(n.id);
          }
        });
      }

      const { past, future } = histPushHistory(get().historyPast, get().nodes, get().edges);
      const updatedEdges = edges.filter(e => !toDelete.has(e.source) && !toDelete.has(e.target));
      const nextNodes = nodes.filter(n => !toDelete.has(n.id));
      set(
        {
          historyPast: past,
          historyFuture: future,
          nodes: nextNodes,
          nodeIdToTitle: computeNodeIdToTitle(nextNodes),
          edges: updatedEdges,
          isDirty: true,
          graphVersion: get().graphVersion + 1,
        },
        false,
        'deleteNodes'
      );
      // Sync cached runnable sets after deleting nodes
      get().syncRunnableSets();
    },

    updateNode(nodeId: string, updates: Partial<WorkflowNode>) {
      if (get().mode === 'history') return;

      const uiOnlyKeys = new Set<keyof WorkflowNode>(['position', 'selected', 'width', 'height']);
      const updateKeys = Object.keys(updates) as Array<keyof WorkflowNode>;
      const isUIOnly = updateKeys.length > 0 && updateKeys.every(k => uiOnlyKeys.has(k));
      const markLayout =
        isUIOnly && updateKeys.some(k => k === 'position' || k === 'width' || k === 'height');

      const nextNodes = get().nodes.map(node =>
        node.id === nodeId ? { ...node, ...updates } : node
      );
      const shouldPush = !get().isHistoryBatching && !isUIOnly;
      const { past, future } = shouldPush
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };

      set(
        {
          historyPast: past,
          historyFuture: future,
          nodes: nextNodes,
          isDirty: isUIOnly ? get().isDirty : true,
          hasLayoutChanges: markLayout ? true : get().hasLayoutChanges,
          graphVersion: isUIOnly ? get().graphVersion : get().graphVersion + 1,
        },
        false,
        'updateNode'
      );
    },

    updateNodeData(nodeId: string, data: Partial<WorkflowNodeData>) {
      if (get().mode === 'history') return;

      let didChange = false;
      const nextNodes = get().nodes.map(node => {
        if (node.id !== nodeId) return node;

        const currentData = node.data as WorkflowNodeData;
        // Partial update check: see if any key in 'data' differs from 'currentData'
        const keys = Object.keys(data) as Array<keyof WorkflowNodeData>;
        const changed = keys.some(key => {
          const newVal = data[key];
          const oldVal = currentData[key];
          // Simple O(1) check for primitives; fallback to JSON for complexes if needed
          // but avoid full data stringification here.
          if (newVal !== oldVal) {
            if (typeof newVal === 'object' && newVal !== null) {
              return JSON.stringify(newVal) !== JSON.stringify(oldVal);
            }
            return true;
          }
          return false;
        });

        if (!changed) return node;

        didChange = true;
        return { ...node, data: { ...currentData, ...data } } as WorkflowNode;
      });

      if (!didChange) return;
      const { past, future } = !get().isHistoryBatching
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };
      set(
        {
          historyPast: past,
          historyFuture: future,
          nodes: nextNodes,
          nodeIdToTitle: computeNodeIdToTitle(nextNodes),
          isDirty: true,
          graphVersion: get().graphVersion + 1,
        },
        false,
        'updateNodeData'
      );

      // Reset run-time visual statuses for this node and all downstream nodes
      try {
        const downstream = getDescendants(get().edges, nodeId);
        const affected = [nodeId, ...downstream];
        get().resetRunStatus(affected);
        get().resetActiveOutputHandles(affected);
        const cur = get().currentRunningNodeId;
        if (cur && affected.includes(cur)) {
          get().setCurrentRunningNodeId(null);
        }
      } catch {
        // Runtime reset is best-effort; data updates should still apply.
      }

      // Clean up orphan edges when handles are removed (e.g., if-else case deletion)
      try {
        const updatedNode = nextNodes.find(n => n.id === nodeId);
        const updatedData = updatedNode?.data as WorkflowNodeData;
        if (updatedData && hasDynamicHandles(updatedData.type)) {
          const validSourceHandles = getValidSourceHandles(updatedData);
          const validTargetHandles = getValidTargetHandles(updatedData);
          const currentEdges = get().edges;
          const cleanedEdges = currentEdges.filter(e => {
            if (e.source === nodeId && validSourceHandles) {
              const handleId = e.sourceHandle || 'source';
              if (!validSourceHandles.has(handleId)) return false;
            }
            if (e.target === nodeId && validTargetHandles) {
              const handleId = e.targetHandle || 'target';
              if (!validTargetHandles.has(handleId)) return false;
            }
            return true;
          });
          if (cleanedEdges.length !== currentEdges.length) {
            set({ edges: cleanedEdges }, false, 'updateNodeData:cleanOrphanEdges');
          }
        }
      } catch {
        // Edge cleanup is best-effort and should not block node data updates.
      }

      // Sync cached runnable sets and re-validate
      get().syncRunnableSetsDebounced();
    },
    updateEdgeData(edgeId: string, data: Partial<WorkflowEdge['data']>) {
      if (get().mode === 'history') return;
      if (!data) return;

      let didChange = false;
      const nextEdges = get().edges.map(edge => {
        if (edge.id !== edgeId) return edge;

        const currentData = (edge.data || {}) as Record<string, unknown>;
        const keys = Object.keys(data) as Array<keyof WorkflowEdge['data']>;
        const changed = keys.some(key => {
          const newVal = data[key];
          const oldVal = currentData[key as string];
          if (newVal !== oldVal) {
            if (typeof newVal === 'object' && newVal !== null) {
              return JSON.stringify(newVal) !== JSON.stringify(oldVal);
            }
            return true;
          }
          return false;
        });

        if (!changed) return edge;

        didChange = true;
        return { ...edge, data: { ...currentData, ...data } } as WorkflowEdge;
      });

      if (!didChange) return;
      const { past, future } = !get().isHistoryBatching
        ? histPushHistory(get().historyPast, get().nodes, get().edges)
        : { past: get().historyPast, future: get().historyFuture };
      set(
        {
          historyPast: past,
          historyFuture: future,
          edges: nextEdges,
          isDirty: true,
          graphVersion: get().graphVersion + 1,
        },
        false,
        'updateEdgeData'
      );

      // Sync cached runnable sets and re-validate
      get().syncRunnableSetsDebounced();
    },

    updateNodesData(
      updates: Array<{ nodeId: string; data: Partial<WorkflowNodeData> }>,
      options?: { markDirty?: boolean; pushHistory?: boolean }
    ) {
      if (get().mode === 'history') return;
      if (!updates.length) return;
      const markDirty = options?.markDirty ?? true;
      const pushHistory = options?.pushHistory ?? markDirty;

      let didChange = false;
      const updatesMap = new Map(updates.map(u => [u.nodeId, u.data]));

      const nextNodes = get().nodes.map(node => {
        const update = updatesMap.get(node.id);
        if (!update) return node;

        const currentData = node.data as WorkflowNodeData;
        const changed = (Object.keys(update) as Array<keyof WorkflowNodeData>).some(key => {
          const newVal = update[key];
          const oldVal = currentData[key];
          if (newVal !== oldVal) {
            if (typeof newVal === 'object' && newVal !== null) {
              return JSON.stringify(newVal) !== JSON.stringify(oldVal);
            }
            return true;
          }
          return false;
        });

        if (!changed) return node;

        didChange = true;
        return { ...node, data: { ...currentData, ...update } } as WorkflowNode;
      });

      if (!didChange) return;

      const { past, future } =
        pushHistory && !get().isHistoryBatching
          ? histPushHistory(get().historyPast, get().nodes, get().edges)
          : { past: get().historyPast, future: get().historyFuture };

      set(
        {
          historyPast: past,
          historyFuture: future,
          nodes: nextNodes,
          nodeIdToTitle: computeNodeIdToTitle(nextNodes),
          isDirty: markDirty ? true : get().isDirty,
          ...(markDirty
            ? {
                suppressNextLayoutDirty: false,
                suppressNextViewportDirty: false,
              }
            : {}),
          graphVersion: get().graphVersion + 1,
        },
        false,
        'updateNodesData'
      );

      // Reset run-time visual statuses for all affected nodes and their downstream
      try {
        const allAffected = new Set<string>();
        for (let i = 0; i < updates.length; i++) {
          const uId = updates[i].nodeId;
          allAffected.add(uId);
          const downstream = getDescendants(get().edges, uId);
          for (let j = 0; j < downstream.length; j++) allAffected.add(downstream[j]);
        }
        const affectedArr = Array.from(allAffected);
        get().resetRunStatus(affectedArr);
        get().resetActiveOutputHandles(affectedArr);
      } catch {
        // Runtime reset is best-effort; batched data updates should still apply.
      }

      // Sync cached runnable sets and re-validate
      get().syncRunnableSetsDebounced();
    },

    generateNodeId() {
      return nodeGenerateNodeId();
    },

    canAddNode(nodeType: string) {
      return nodeCanAddNode(get().nodes, nodeType);
    },

    getUniqueNodeTypes() {
      return nodeGetUniqueNodeTypes(get().nodes);
    },

    getIncomingSources(nodeId: string) {
      return graphGetIncomingSources(get().edges, nodeId);
    },

    getAncestors(nodeId: string) {
      const { graphVersion, _analysisCache, nodes, edges, mode, selectedRunId, historySnapshots } =
        get();

      // Determine effective graph
      let targetNodes = nodes;
      let targetEdges = edges;
      if (mode === 'history' && selectedRunId) {
        const snap = historySnapshots[selectedRunId];
        if (snap) {
          targetNodes = snap.nodes;
          targetEdges = snap.edges;
        }
      }

      // Check cache version and runId (switching runs in history mode doesn't change graphVersion)
      if (_analysisCache.version !== graphVersion || _analysisCache.runId !== selectedRunId) {
        _analysisCache.version = graphVersion;
        _analysisCache.runId = selectedRunId;
        _analysisCache.upstreamVariables.clear();
        _analysisCache.ancestors.clear();
      }

      const cached = _analysisCache.ancestors.get(nodeId);
      if (cached) return cached;

      const result = graphGetAncestors(targetNodes, targetEdges, nodeId);
      _analysisCache.ancestors.set(nodeId, result);
      return result;
    },

    getUpstreamVariables(nodeId: string) {
      const {
        graphVersion,
        _analysisCache,
        nodes,
        edges,
        agentType,
        mode,
        selectedRunId,
        historySnapshots,
      } = get();

      // Determine effective graph
      let targetNodes = nodes;
      let targetEdges = edges;
      if (mode === 'history' && selectedRunId) {
        const snap = historySnapshots[selectedRunId];
        if (snap) {
          targetNodes = snap.nodes;
          targetEdges = snap.edges;
        }
      }

      if (_analysisCache.version !== graphVersion || _analysisCache.runId !== selectedRunId) {
        _analysisCache.version = graphVersion;
        _analysisCache.runId = selectedRunId;
        _analysisCache.upstreamVariables.clear();
        _analysisCache.ancestors.clear();
      }

      const cached = _analysisCache.upstreamVariables.get(nodeId);
      if (cached) return cached;

      const base = graphGetUpstreamVariables(targetNodes, targetEdges, nodeId, agentType);
      const extra: UpstreamExportItem[] = [];

      // 1) Environment variables for ALL agent types (read-only)
      try {
        const env = (
          get() as unknown as {
            workflowData?: {
              environment_variables?: Array<{
                name: string;
                type: 'string' | 'number' | 'secret';
                description?: string;
              }>;
            };
          }
        )?.workflowData?.environment_variables;
        const envItems = Array.isArray(env)
          ? env
              .filter(v => typeof v?.name === 'string' && v.name.trim().length > 0)
              .map(v => ({
                key: v.name,
                type: (v.type === 'number'
                  ? 'number'
                  : 'string') as UpstreamExportItem['variables'][number]['type'],
                description: v.description,
              }))
          : [];
        if (envItems.length > 0) {
          extra.push({
            nodeId: 'environment',
            nodeType: 'llm',
            nodeTitle: 'Environment',
            variables: envItems,
          });
        }
      } catch {
        // no-op
      }

      // 2) Conversation variables for conversational agents (writable)
      if (get().agentType === AgentTypeEnum.CONVERSATIONAL_AGENT) {
        try {
          const conv = (
            get() as unknown as {
              workflowData?: {
                conversation_variables?: Array<{
                  name: string;
                  type: string;
                  description?: string;
                }>;
              };
            }
          )?.workflowData?.conversation_variables;
          const convItems = Array.isArray(conv)
            ? conv
                .filter(v => typeof v?.name === 'string' && v.name.trim().length > 0)
                .map(v => ({
                  key: v.name,
                  type: v.type as UpstreamExportItem['variables'][number]['type'],
                  writable: true,
                  description: v.description,
                }))
            : [];
          if (convItems.length > 0) {
            extra.push({
              nodeId: 'conversation',
              nodeType: 'llm',
              nodeTitle: 'Conversation',
              variables: convItems,
            });
          }
        } catch {
          // no-op
        }
      }

      // 3) Combined result
      const finalResult = [...extra, ...base];
      _analysisCache.upstreamVariables.set(nodeId, finalResult);
      return finalResult;
    },

    getUpstreamWritableVariables(nodeId: string) {
      const { nodes, edges, mode, selectedRunId, historySnapshots, agentType } = get();

      // Determine effective graph
      let targetNodes = nodes;
      let targetEdges = edges;
      if (mode === 'history' && selectedRunId) {
        const snap = historySnapshots[selectedRunId];
        if (snap) {
          targetNodes = snap.nodes;
          targetEdges = snap.edges;
        }
      }

      const groups = graphGetUpstreamWritableVariables(targetNodes, targetEdges, nodeId, agentType);
      if (get().agentType === AgentTypeEnum.CONVERSATIONAL_AGENT) {
        try {
          const conv = (
            get() as unknown as {
              workflowData?: {
                conversation_variables?: Array<{
                  name: string;
                  type: string;
                  description?: string;
                }>;
              };
            }
          )?.workflowData?.conversation_variables;
          const convItems = Array.isArray(conv)
            ? conv
                .filter(v => typeof v?.name === 'string' && v.name.trim().length > 0)
                .map(v => ({
                  key: v.name,
                  type: v.type as UpstreamExportItem['variables'][number]['type'],
                  writable: true,
                  description: v.description,
                }))
            : [];
          if (convItems.length > 0) {
            const group: UpstreamExportItem = {
              nodeId: 'conversation',
              nodeType: 'llm',
              nodeTitle: 'Conversation',
              variables: convItems,
            };
            return [group, ...groups];
          }
        } catch {
          // no-op
        }
      }
      return groups;
    },
  };
}
