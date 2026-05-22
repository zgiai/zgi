import { NODE_TYPES } from '../../nodes';
import type {
  WorkflowNode,
  WorkflowEdge,
  StoreValidationResults,
  StoreValidationError,
  CodeNodeData,
  AssignerNodeData,
  KnowledgeRetrievalNodeData,
  LLMNodeData,
  IfElseNodeData,
  HttpRequestNodeData,
  CallDatabaseNodeData,
  ToolNodeData,
  CreateScheduledTaskNodeData,
  NotificationSMSNodeData,
  EndNodeData,
  AnswerNodeData,
  StartNodeData,
  IterationNodeData,
  ParameterExtractorNodeData,
  SqlGeneratorNodeData,
  DocumentExtractorNodeData,
  ApprovalNodeData,
  AnnouncementNodeData,
  QuestionAnswerNodeData,
} from '../type';
import { isContainerStartNode } from '../type';
import { AgentType } from '@/services/types/agent';
import type { SystemFeatures } from '@/services/types/auth';
import { checkValid as codeCheck } from '../../nodes/code/config';
import { checkValid as assignerCheck } from '../../nodes/assigner/config';
import { checkValid as krCheck } from '../../nodes/knowledge-retrieval/config';
import { checkValid as llmCheck } from '../../nodes/llm/config';
import { checkValid as ifElseCheck } from '../../nodes/if-else/config';
import { checkValid as httpCheck } from '../../nodes/http-request/config';
import { checkValid as toolCheck } from '../../nodes/tool/config';
import { checkValid as createScheduledTaskCheck } from '../../nodes/create-scheduled-task/config';
import { checkValid as notificationSMSCheck } from '../../nodes/notification-sms/config';
import { checkValid as endCheck } from '../../nodes/end/config';
import { checkValid as answerCheck } from '../../nodes/answer/config';
import { checkValid as startCheck } from '../../nodes/start/config';
import { checkValid as iterationCheck } from '../../nodes/iteration/config';
import { checkValid as callDbCheck } from '../../nodes/call-database/config';
import {
  type VariableAggregatorNodeData,
  checkValid as variableAggregatorCheck,
} from '../../nodes/variable-aggregator/config';
import { checkValid as parameterExtractorCheck } from '../../nodes/parameter-extractor/config';
import { checkValid as deCheck } from '../../nodes/document-extractor/config';
import { checkValid as sqlCheck } from '../../nodes/sql-generator/config';
import {
  checkValid as jsonParserCheck,
  type JsonParserNodeData,
} from '../../nodes/json-parser/config';
import { checkValid as approvalCheck } from '../../nodes/approval/config';
import { checkValid as announcementCheck } from '../../nodes/announcement/config';
import { checkValid as questionAnswerCheck } from '../../nodes/question-answer/config';
import type { RunnableSets } from '../store';
import type { ValidationResult } from '../../nodes/common/validation';
import { getNotificationSMSTemplates } from '@/lib/features/notification-sms';
import type { BuiltinToolProvider, ToolParameter } from '@/services/types/tool';
import type { Locale } from '@/lib/i18n';
import { pickLocale } from '@/utils/tool-helpers';
import type { ToolRequiredField } from '../../nodes/tool/config';

function getToolRequiredFields(
  toolProviders: BuiltinToolProvider[] | null | undefined,
  providerId: string,
  toolName: string,
  locale: Locale
): {
  requiredParams: ToolRequiredField[];
  requiredConfigurations: ToolRequiredField[];
} {
  if (!Array.isArray(toolProviders)) {
    return { requiredParams: [], requiredConfigurations: [] };
  }

  const provider = toolProviders.find(
    provider => provider.id === providerId || provider.name === providerId
  );
  const tool = provider?.tools?.find(tool => tool.name === toolName);
  if (!tool) {
    return { requiredParams: [], requiredConfigurations: [] };
  }

  const mapField = (param: ToolParameter): ToolRequiredField => ({
    name: param.name,
    label: param.label ? pickLocale(param.label, locale, param.name) : param.name,
    type: param.type,
  });

  return {
    requiredParams: (tool.parameters || []).filter(param => param.required).map(mapField),
    requiredConfigurations: (tool.config_parameters?.parameters || [])
      .filter(param => param.required)
      .map(mapField),
  };
}

/**
 * Pure validation engine for workflow graphs.
 * Returns structured error codes to be translated by the UI layer.
 */
export function validateWorkflow(
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  agentType: AgentType,
  runnableSets: RunnableSets,
  systemFeatures?: SystemFeatures | null,
  toolProviders?: BuiltinToolProvider[] | null,
  locale: Locale = 'zh-Hans'
): StoreValidationResults {
  const errors: StoreValidationError[] = [];
  const warnings: StoreValidationError[] = [];
  const smsTemplates = getNotificationSMSTemplates(systemFeatures);

  // 1. Build lookup maps once (O(N + E))
  const nodesMap = new Map(nodes.map(n => [n.id, n]));
  const adj = new Map<string, string[]>();
  const revAdj = new Map<string, string[]>();
  const incomingEdgesMap = new Map<string, WorkflowEdge[]>();

  for (let i = 0; i < edges.length; i++) {
    const e = edges[i];
    const list = adj.get(e.source) || [];
    list.push(e.target);
    adj.set(e.source, list);

    const rList = revAdj.get(e.target) || [];
    rList.push(e.source);
    revAdj.set(e.target, rList);

    const incoming = incomingEdgesMap.get(e.target) || [];
    incoming.push(e);
    incomingEdgesMap.set(e.target, incoming);
  }

  // Check if workflow has start node
  const startNodes = nodes.filter(node => node.data.type === NODE_TYPES.START);
  if (startNodes.length !== 1) {
    errors.push({
      type: 'error',
      code: 'workflow.validation.startExactlyOne',
    });
  }

  // Answer node rules
  if (agentType === AgentType.WORKFLOW) {
    for (const node of nodes) {
      if (node.data.type === 'answer') {
        errors.push({
          type: 'error',
          code: 'workflow.validation.answerNotAllowedInWorkflow',
          nodeId: node.id,
          nodeTitle: node.data.title,
        });
      }
    }
  }

  // End node rules
  if (agentType === AgentType.WORKFLOW) {
    const hasEnd = nodes.some(node => node.data.type === NODE_TYPES.END);
    if (!hasEnd) {
      warnings.push({
        type: 'warning',
        code: 'workflow.validation.endRecommendedAtLeastOne',
      });
    }
  }

  // 2. Pre-calculate Runnable States (O(V+E))
  const { mainRunnable, iterRunnableMap, commentSet } = runnableSets;
  const allRunnableSet = new Set<string>();
  startNodes.forEach(n => allRunnableSet.add(n.id));
  mainRunnable.forEach(id => allRunnableSet.add(id));
  for (const set of iterRunnableMap.values()) {
    set.forEach(id => allRunnableSet.add(id));
  }

  // 3. Pre-calculate Sink Reachability (O(V+E))
  const canReachSink = new Set<string>();
  const canReachRelaxedOutput = new Set<string>();
  const allowedOutputs = new Set<string>([
    NODE_TYPES.END,
    ...(agentType === AgentType.WORKFLOW ? [] : [NODE_TYPES.ANSWER]),
  ]);
  const relaxedOutputNodeTypes = new Set<string>([
    ...Array.from(allowedOutputs),
    NODE_TYPES.TOOL,
    NODE_TYPES.HTTP_REQUEST,
    NODE_TYPES.CALL_DATABASE,
    NODE_TYPES.SQL_GENERATOR,
  ]);

  const sinkQueue: string[] = [];
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    if (allowedOutputs.has(n.data.type)) {
      canReachSink.add(n.id);
      sinkQueue.push(n.id);
    }
  }

  let head = 0;
  while (head < sinkQueue.length) {
    const cur = sinkQueue[head++];
    const upstreams = revAdj.get(cur) || [];
    for (let i = 0; i < upstreams.length; i++) {
      const u = upstreams[i];
      if (!canReachSink.has(u)) {
        canReachSink.add(u);
        sinkQueue.push(u);
      }
    }
  }

  const relaxedQueue: string[] = [];
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    if (relaxedOutputNodeTypes.has(n.data.type)) {
      canReachRelaxedOutput.add(n.id);
      relaxedQueue.push(n.id);
    }
  }

  let relaxedHead = 0;
  while (relaxedHead < relaxedQueue.length) {
    const cur = relaxedQueue[relaxedHead++];
    const upstreams = revAdj.get(cur) || [];
    for (let i = 0; i < upstreams.length; i++) {
      const u = upstreams[i];
      if (!canReachRelaxedOutput.has(u)) {
        canReachRelaxedOutput.add(u);
        relaxedQueue.push(u);
      }
    }
  }

  const getStringArray = (value: unknown): string[] => {
    if (!Array.isArray(value)) return [];
    return value.filter((v): v is string => typeof v === 'string');
  };

  const hasContainerInternalRelaxedOutput = (containerNode: WorkflowNode): boolean => {
    if (
      containerNode.data.type !== NODE_TYPES.LOOP &&
      containerNode.data.type !== NODE_TYPES.ITERATION
    ) {
      return false;
    }
    const data = containerNode.data as unknown as Record<string, unknown>;
    const childrenFromData = getStringArray(data['_children']);
    const childrenFromParent = nodes
      .filter(n => (n as unknown as { parentId?: string }).parentId === containerNode.id)
      .map(n => n.id);
    const childIds = new Set<string>([...childrenFromData, ...childrenFromParent]);
    for (const childId of childIds) {
      const child = nodesMap.get(childId);
      if (child && relaxedOutputNodeTypes.has(child.data.type)) return true;
    }
    return false;
  };

  // 4. Main node validation loop
  for (let i = 0; i < nodes.length; i++) {
    const node = nodes[i];
    const nodeType = node.data.type as string;
    const isActuallyRunnable = allRunnableSet.has(node.id);

    // Node-specific validator logic
    if (!commentSet.has(node.id)) {
      const pushNodeValidation = (r: ValidationResult) => {
        if (!r.isValid) {
          r.errors.forEach(err =>
            errors.push({
              type: 'error',
              code: err.code,
              params: err.params,
              nodeId: node.id,
              nodeTitle: node.data.title,
            })
          );
        }
        r.warnings.forEach(err =>
          warnings.push({
            type: 'warning',
            code: err.code,
            params: err.params,
            nodeId: node.id,
            nodeTitle: node.data.title,
          })
        );
      };

      switch (nodeType) {
        case NODE_TYPES.CODE:
          pushNodeValidation(codeCheck(node.data as CodeNodeData));
          break;
        case NODE_TYPES.ASSIGNER:
          pushNodeValidation(assignerCheck(node.data as AssignerNodeData));
          break;
        case NODE_TYPES.KNOWLEDGE_RETRIEVAL:
          pushNodeValidation(krCheck(node.data as KnowledgeRetrievalNodeData));
          break;
        case NODE_TYPES.LLM:
          pushNodeValidation(llmCheck(node.data as LLMNodeData));
          break;
        case NODE_TYPES.IF_ELSE:
          pushNodeValidation(ifElseCheck(node.data as IfElseNodeData));
          break;
        case NODE_TYPES.HTTP_REQUEST:
          pushNodeValidation(httpCheck(node.data as HttpRequestNodeData));
          break;
        case NODE_TYPES.CALL_DATABASE:
          pushNodeValidation(callDbCheck(node.data as CallDatabaseNodeData));
          break;
        case NODE_TYPES.TOOL: {
          const toolData = node.data as ToolNodeData;
          const toolFields = getToolRequiredFields(
            toolProviders,
            toolData.provider_id,
            toolData.tool_name,
            locale
          );
          pushNodeValidation(
            toolCheck(toolData, {
              nodes,
              requiredParams: toolFields.requiredParams,
              requiredConfigurations: toolFields.requiredConfigurations,
            })
          );
          break;
        }
        case NODE_TYPES.END:
          pushNodeValidation(endCheck(node.data as EndNodeData));
          break;
        case NODE_TYPES.VARIABLE_AGGREGATOR:
          pushNodeValidation(variableAggregatorCheck(node.data as VariableAggregatorNodeData));
          break;
        case NODE_TYPES.ANSWER:
          pushNodeValidation(answerCheck(node.data as AnswerNodeData));
          break;
        case NODE_TYPES.START:
          pushNodeValidation(startCheck(node.data as StartNodeData));
          break;
        case NODE_TYPES.ITERATION:
          pushNodeValidation(iterationCheck(node.data as IterationNodeData));
          break;
        case NODE_TYPES.PARAMETER_EXTRACTOR:
          pushNodeValidation(parameterExtractorCheck(node.data as ParameterExtractorNodeData));
          break;
        case NODE_TYPES.DOCUMENT_EXTRACTOR:
          pushNodeValidation(deCheck(node.data as DocumentExtractorNodeData));
          break;
        case NODE_TYPES.SQL_GENERATOR:
          pushNodeValidation(sqlCheck(node.data as SqlGeneratorNodeData));
          break;
        case NODE_TYPES.CREATE_SCHEDULED_TASK:
          pushNodeValidation(
            createScheduledTaskCheck(node.data as CreateScheduledTaskNodeData, smsTemplates)
          );
          break;
        case NODE_TYPES.NOTIFICATION_SMS:
          pushNodeValidation(
            notificationSMSCheck(node.data as NotificationSMSNodeData, smsTemplates)
          );
          break;
        case NODE_TYPES.JSON_PARSER:
          pushNodeValidation(jsonParserCheck(node.data as JsonParserNodeData));
          break;
        case NODE_TYPES.APPROVAL:
          pushNodeValidation(approvalCheck(node.data as ApprovalNodeData));
          break;
        case NODE_TYPES.ANNOUNCEMENT:
          pushNodeValidation(announcementCheck(node.data as AnnouncementNodeData));
          break;
        case NODE_TYPES.QUESTION_ANSWER:
          pushNodeValidation(questionAnswerCheck(node.data as QuestionAnswerNodeData));
          break;
      }
    }

    if (!isActuallyRunnable) continue;

    const incoming = incomingEdgesMap.get(node.id) || [];
    const hasOutgoing = (adj.get(node.id)?.length || 0) > 0;
    const hasIncoming = incoming.length > 0;

    switch (nodeType) {
      case NODE_TYPES.START:
        if (!hasOutgoing) {
          warnings.push({
            type: 'warning',
            code: 'workflow.validation.nodeNoOutgoing',
            params: { title: node.data.title || '' },
            nodeId: node.id,
            nodeTitle: node.data.title,
          });
        }
        break;
      case NODE_TYPES.END:
      case NODE_TYPES.ANSWER:
        if (!hasIncoming) {
          warnings.push({
            type: 'warning',
            code: 'workflow.validation.nodeNoIncoming',
            params: { title: node.data.title || '' },
            nodeId: node.id,
            nodeTitle: node.data.title,
          });
        }
        break;
      default: {
        if (isContainerStartNode(nodeType)) break;
        const isNodeInContainer = (node as unknown as { parentId?: string })?.parentId;
        if (!isNodeInContainer) {
          const hasRelaxedDownstream = canReachRelaxedOutput.has(node.id);
          const hasContainerInternalOutput = hasContainerInternalRelaxedOutput(node);
          if (!canReachSink.has(node.id) && !hasRelaxedDownstream && !hasContainerInternalOutput) {
            warnings.push({
              type: 'warning',
              code: 'workflow.validation.runnableNoOutput',
              nodeId: node.id,
              nodeTitle: node.data.title,
            });
          }
        }
      }
    }
  }

  // 5. Optimized Circular Dependency Check
  const state = new Map<string, number>();
  const hasCycle = (u: string): boolean => {
    state.set(u, 1);
    const neighbors = adj.get(u) || [];
    for (let i = 0; i < neighbors.length; i++) {
      const v = neighbors[i];
      const s = state.get(v) || 0;
      if (s === 1) return true;
      if (s === 0 && hasCycle(v)) return true;
    }
    state.set(u, 2);
    return false;
  };

  for (let i = 0; i < nodes.length; i++) {
    if ((state.get(nodes[i].id) || 0) === 0) {
      if (hasCycle(nodes[i].id)) {
        errors.push({
          type: 'error',
          code: 'workflow.validation.circularDependencyDetected',
        });
        break;
      }
    }
  }

  // 6. Optimized Comment Connections Check
  const reportedComments = new Set<string>();
  for (let i = 0; i < edges.length; i++) {
    const edge = edges[i];
    if (
      commentSet.has(edge.source) &&
      allRunnableSet.has(edge.target) &&
      !commentSet.has(edge.target) &&
      !reportedComments.has(edge.source)
    ) {
      reportedComments.add(edge.source);
      const sNode = nodesMap.get(edge.source);
      errors.push({
        type: 'error',
        code: 'workflow.validation.commentConnectedToRunnable',
        nodeId: edge.source,
        nodeTitle: sNode?.data?.title ?? '',
      });
    }
  }

  // 7. Pre-index results for O(1) lookup
  const errorMap = new Map<string, StoreValidationError[]>();
  const warningMap = new Map<string, StoreValidationError[]>();

  errors.forEach(err => {
    if (!err.nodeId) return;
    const list = errorMap.get(err.nodeId) || [];
    list.push(err);
    errorMap.set(err.nodeId, list);
  });
  warnings.forEach(wrn => {
    if (!wrn.nodeId) return;
    const list = warningMap.get(wrn.nodeId) || [];
    list.push(wrn);
    warningMap.set(wrn.nodeId, list);
  });

  return { errors, warnings, errorMap, warningMap };
}
