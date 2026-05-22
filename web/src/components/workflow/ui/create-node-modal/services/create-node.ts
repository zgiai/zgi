import { useWorkflowStore } from '../../../store';
import type { ToolNodeData } from '../../../nodes/tool/config';

export function createNodeByTypeFactory(ops: {
  addStartNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addKnowledgeRetrievalNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addLLMNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addHttpRequestNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addCallDatabaseNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addSqlGeneratorNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addToolNode?: (
    pos: { x: number; y: number },
    parentId?: string,
    initialData?: Partial<ToolNodeData>
  ) => string | null;
  addCreateScheduledTaskNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addNotificationSMSNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addEndNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addLoopEndNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addIfElseNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addCodeNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addIterationNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addLoopNode: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addAssignerNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addAnswerNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addDocumentExtractorNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addVariableAggregatorNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addParameterExtractorNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addJsonParserNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addImageGenNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addApprovalNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addAnnouncementNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
  addQuestionAnswerNode?: (pos: { x: number; y: number }, parentId?: string) => string | null;
}) {
  return (
    type: string,
    pos: { x: number; y: number },
    parentId?: string,
    initialData?: Partial<ToolNodeData>
  ): string | null => {
    switch (type) {
      case 'start':
        return ops.addStartNode(pos, parentId);
      case 'knowledge-retrieval':
        return ops.addKnowledgeRetrievalNode(pos, parentId);
      case 'llm':
        return ops.addLLMNode(pos, parentId);
      case 'http-request':
        return ops.addHttpRequestNode(pos, parentId);
      case 'call-database':
        return ops.addCallDatabaseNode ? ops.addCallDatabaseNode(pos, parentId) : null;
      case 'sql-generator':
        return ops.addSqlGeneratorNode ? ops.addSqlGeneratorNode(pos, parentId) : null;
      case 'tool':
        return ops.addToolNode ? ops.addToolNode(pos, parentId, initialData) : null;
      case 'create-scheduled-task':
        return ops.addCreateScheduledTaskNode
          ? ops.addCreateScheduledTaskNode(pos, parentId)
          : null;
      case 'notification-sms':
        return ops.addNotificationSMSNode ? ops.addNotificationSMSNode(pos, parentId) : null;
      case 'if-else':
        return ops.addIfElseNode(pos, parentId);
      case 'code':
        return ops.addCodeNode(pos, parentId);
      case 'assigner':
        return ops.addAssignerNode ? ops.addAssignerNode(pos, parentId) : null;
      case 'iteration': {
        // Delegate to ops.addIterationNode, which handles creation of iteration-start child
        return ops.addIterationNode(pos, parentId);
      }
      case 'loop': {
        return ops.addLoopNode(pos, parentId);
      }
      case 'end':
        return ops.addEndNode(pos, parentId);
      case 'loop-end':
        return ops.addLoopEndNode ? ops.addLoopEndNode(pos, parentId) : null;
      case 'answer':
        return ops.addAnswerNode ? ops.addAnswerNode(pos, parentId) : null;
      case 'document-extractor':
        return ops.addDocumentExtractorNode ? ops.addDocumentExtractorNode(pos, parentId) : null;
      case 'variable-aggregator':
        return ops.addVariableAggregatorNode ? ops.addVariableAggregatorNode(pos, parentId) : null;
      case 'parameter-extractor':
        return ops.addParameterExtractorNode ? ops.addParameterExtractorNode(pos, parentId) : null;
      case 'json-parser':
        return ops.addJsonParserNode ? ops.addJsonParserNode(pos, parentId) : null;
      case 'image-gen':
        return ops.addImageGenNode ? ops.addImageGenNode(pos, parentId) : null;
      case 'approval':
        return ops.addApprovalNode ? ops.addApprovalNode(pos, parentId) : null;
      case 'announcement':
        return ops.addAnnouncementNode ? ops.addAnnouncementNode(pos, parentId) : null;
      case 'question-answer':
        return ops.addQuestionAnswerNode ? ops.addQuestionAnswerNode(pos, parentId) : null;
      default:
        return null;
    }
  };
}

export function autoConnectFromHandle(params: {
  edges: ReturnType<typeof useWorkflowStore.getState>['edges'];
  setEdges: (edges: ReturnType<typeof useWorkflowStore.getState>['edges']) => void;
  sourceId: string;
  targetId: string;
  sourceHandle?: string;
  targetHandle?: string;
  inLoop?: boolean;
}) {
  let srcH = params.sourceHandle ?? 'source';
  if (!params.sourceHandle) {
    const nodes = useWorkflowStore.getState().nodes;
    const srcNode = nodes.find(n => n.id === params.sourceId);
    if ((srcNode?.data as { type?: string } | undefined)?.type === 'if-else') {
      const branches = (srcNode?.data as { targetBranches?: Array<{ id: string }> } | undefined)
        ?.targetBranches;
      srcH = branches && branches.length > 0 ? branches[0].id : 'true';
    }
  }
  const tgtH = params.targetHandle ?? 'target';
  const hasEdge = params.edges.some(
    e =>
      e.source === params.sourceId &&
      (e.sourceHandle ?? 'source') === srcH &&
      e.target === params.targetId &&
      (e.targetHandle ?? 'target') === tgtH
  );
  if (hasEdge) return;
  const next = params.edges.concat({
    id: `${params.sourceId}-${srcH}-${params.targetId}-${tgtH}`,
    source: params.sourceId,
    target: params.targetId,
    sourceHandle: srcH,
    targetHandle: tgtH,
    type: 'custom',
    data: { sourceType: 'default', targetType: 'default', isInLoop: Boolean(params.inLoop) },
  });
  params.setEdges(next);
}

export function reconnectForInsert(params: {
  edges: ReturnType<typeof useWorkflowStore.getState>['edges'];
  setEdges: (edges: ReturnType<typeof useWorkflowStore.getState>['edges']) => void;
  edge: {
    edgeId: string;
    sourceId: string;
    targetId: string;
    sourceHandle?: string;
    targetHandle?: string;
  };
  newNodeId: string;
}) {
  const { edges, setEdges, edge, newNodeId } = params;
  const next = edges.filter(e => e.id !== edge.edgeId);
  // Recover original edge from store to get accurate handles if missing
  const orig = edges.find(e => e.id === edge.edgeId);
  const recoverFromId = (edgeId?: string, srcId?: string, tgtId?: string) => {
    if (!edgeId || !srcId || !tgtId) {
      return { src: undefined as string | undefined, tgt: undefined as string | undefined };
    }
    const prefix = `${srcId}-`;
    const middle = `-${tgtId}-`;
    const p = edgeId.indexOf(prefix);
    const q = edgeId.indexOf(middle, p + prefix.length);
    if (p >= 0 && q > p) {
      const src = edgeId.substring(p + prefix.length, q);
      const tgt = edgeId.substring(q + middle.length);
      return { src, tgt };
    }
    return { src: undefined, tgt: undefined };
  };
  const recovered = recoverFromId(orig?.id ?? edge.edgeId, edge.sourceId, edge.targetId);
  let aSrcH = edge.sourceHandle ?? orig?.sourceHandle ?? recovered.src ?? 'source';
  if (!(edge.sourceHandle ?? orig?.sourceHandle ?? recovered.src)) {
    const nodes = useWorkflowStore.getState().nodes;
    const srcNode = nodes.find(n => n.id === edge.sourceId);
    if ((srcNode?.data as { type?: string } | undefined)?.type === 'if-else') {
      const branches = (srcNode?.data as { targetBranches?: Array<{ id: string }> } | undefined)
        ?.targetBranches;
      aSrcH = branches && branches.length > 0 ? branches[0].id : 'true';
    }
  }
  const aTgtH = 'target';
  const existsA = next.some(
    e =>
      e.source === edge.sourceId &&
      (e.sourceHandle ?? 'source') === aSrcH &&
      e.target === newNodeId &&
      (e.targetHandle ?? 'target') === aTgtH
  );
  if (!existsA) {
    next.push({
      id: `${edge.sourceId}-${aSrcH}-${newNodeId}-${aTgtH}`,
      source: edge.sourceId,
      target: newNodeId,
      sourceHandle: aSrcH,
      targetHandle: aTgtH,
      type: 'custom',
      data: { sourceType: 'default', targetType: 'default', isInLoop: false },
    });
  }
  let bSrcH = 'source';
  const nodes = useWorkflowStore.getState().nodes;
  const newNode = nodes.find(n => n.id === newNodeId);
  if ((newNode?.data as { type?: string } | undefined)?.type === 'if-else') {
    const branches = (newNode?.data as { targetBranches?: Array<{ id: string }> } | undefined)
      ?.targetBranches;
    const branchIds = Array.isArray(branches) ? branches.map(b => b.id) : [];
    // Prefer preserving the original upstream branch id if it exists on the new if-else
    const desired = (edge.sourceHandle ?? orig?.sourceHandle ?? 'true') as string;
    if (branchIds.includes(desired)) {
      bSrcH = desired;
    } else if (branchIds.includes('true')) {
      bSrcH = 'true';
    } else if (branchIds.length > 0) {
      bSrcH = branchIds[0];
    } else {
      bSrcH = 'true';
    }
  }
  const bTgtH = edge.targetHandle ?? orig?.targetHandle ?? recovered.tgt ?? 'target';
  const existsB = next.some(
    e =>
      e.source === newNodeId &&
      (e.sourceHandle ?? 'source') === bSrcH &&
      e.target === edge.targetId &&
      (e.targetHandle ?? 'target') === bTgtH
  );
  if (!existsB) {
    next.push({
      id: `${newNodeId}-${bSrcH}-${edge.targetId}-${bTgtH}`,
      source: newNodeId,
      target: edge.targetId,
      sourceHandle: bSrcH,
      targetHandle: bTgtH,
      type: 'custom',
      data: { sourceType: 'default', targetType: 'default', isInLoop: false },
    });
  }
  setEdges(next);
}
