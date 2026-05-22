import type { ToolNodeData, WorkflowNodeData } from '../../store/type';
import { getNextNodeTitle } from '../../store/helpers/titles';
import {
  DEFAULT_DATABASE_EXECUTION,
  DEFAULT_DATABASE_SOURCE,
  DEFAULT_CALL_DATABASE_NODE_DATA,
} from '../../nodes/call-database/config';
import { DEFAULT_KNOWLEDGE_RETRIEVAL_NODE } from '../../nodes/knowledge-retrieval/config';
import { DEFAULT_JSON_PARSER_NODE } from '../../nodes/json-parser/config';
import { DEFAULT_IMAGE_GEN_NODE_DATA } from '../../nodes/image-gen/config';
import { createApprovalActionId, DEFAULT_APPROVAL_NODE_DATA } from '../../nodes/approval/config';
import { DEFAULT_ANNOUNCEMENT_NODE_DATA } from '../../nodes/announcement/config';
import { DEFAULT_QUESTION_ANSWER_NODE_DATA } from '../../nodes/question-answer/config';
import { DEFAULT_VARIABLE_AGGREGATOR_NODE_DATA } from '../../nodes/variable-aggregator/config';
import { DEFAULT_SQL_GENERATOR_NODE_DATA } from '../../nodes/sql-generator/config';
import { DEFAULT_LLM_NODE_DATA } from '../../nodes/llm/config';
import { DEFAULT_HTTP_REQUEST_NODE_DATA } from '../../nodes/http-request/config';
import { DEFAULT_TOOL_NODE_DATA } from '../../nodes/tool/config';
import { createDefaultCreateScheduledTaskNodeData } from '../../nodes/create-scheduled-task/config';
import { DEFAULT_NOTIFICATION_SMS_NODE_DATA } from '../../nodes/notification-sms/config';
import { DEFAULT_END_NODE_DATA } from '../../nodes/end/config';
import { DEFAULT_LOOP_END_NODE_DATA } from '../../nodes/loop-end/config';
import { DEFAULT_ANSWER_NODE_DATA } from '../../nodes/answer/config';
import { DEFAULT_CODE_NODE_DATA } from '../../nodes/code/config';
import { DEFAULT_ASSIGNER_NODE_DATA } from '../../nodes/assigner/config';
import { DEFAULT_DOCUMENT_EXTRACTOR_NODE_DATA } from '../../nodes/document-extractor/config';
import { DEFAULT_PARAMETER_EXTRACTOR_NODE } from '../../nodes/parameter-extractor/config';
import { DEFAULT_ITERATION_NODE_DATA } from '../../nodes/iteration/config';
import { DEFAULT_LOOP_NODE } from '../../nodes/loop/config';

const DEFAULT_CODE_TEMPLATE =
  `# Entry function. Return a dict with your outputs\n` +
  `def main(var1, var2):\n` +
  `    return { 'result': var1 + var2 }\n`;

interface ApprovalPreviewLabels {
  content: string;
  approveLabel: string;
  rejectLabel: string;
  emailSubject: string;
  emailBody: string;
}

interface CreateDragPreviewNodeDataParams {
  type: string;
  title: string;
  defaultLlmProvider?: string;
  defaultLlmName?: string;
  approvalLabels?: ApprovalPreviewLabels;
}

interface CreateToolDragPreviewNodeDataParams {
  title: string;
  providerId: string;
  toolName: string;
}

const DRAG_PREVIEW_TITLE_KEYS: Record<string, string> = {
  start: 'catalog.start.title',
  'knowledge-retrieval': 'catalog.knowledge-retrieval.title',
  'parameter-extractor': 'catalog.parameter-extractor.title',
  'json-parser': 'catalog.json-parser.title',
  'image-gen': 'catalog.image-gen.title',
  approval: 'catalog.approval.title',
  announcement: 'catalog.announcement.title',
  'question-answer': 'catalog.question-answer.title',
  'variable-aggregator': 'catalog.variable-aggregator.title',
  'sql-generator': 'catalog.sql-generator.title',
  llm: 'catalog.llm.title',
  'http-request': 'catalog.http-request.title',
  'call-database': 'catalog.call-database.title',
  'create-scheduled-task': 'catalog.create-scheduled-task.title',
  'notification-sms': 'catalog.notification-sms.title',
  end: 'catalog.end.title',
  'loop-end': 'catalog.loop-end.title',
  answer: 'catalog.answer.title',
  code: 'catalog.code.title',
  iteration: 'catalog.iteration.title',
  loop: 'catalog.loop.title',
  assigner: 'catalog.assigner.title',
  'if-else': 'catalog.if-else.title',
  'document-extractor': 'catalog.document-extractor.title',
};

/**
 * @util resolveDragPreviewNodeTitle
 * @description Resolves the node title through the same catalog keys used by node creation.
 */
export function resolveDragPreviewNodeTitle<T extends (key: never) => string>(
  type: string,
  t: T,
  fallback: string,
  existingTitles?: Set<string>
): string {
  const key = DRAG_PREVIEW_TITLE_KEYS[type];
  if (!key) return existingTitles ? getNextNodeTitle(fallback, existingTitles) : fallback;

  try {
    const baseTitle = t(key as Parameters<T>[0]) || fallback;
    return existingTitles ? getNextNodeTitle(baseTitle, existingTitles) : baseTitle;
  } catch (_error) {
    return existingTitles ? getNextNodeTitle(fallback, existingTitles) : fallback;
  }
}

/**
 * @util createDragPreviewNodeData
 * @description Builds the same initial node data shape used when dragging a workflow node onto the canvas.
 */
export function createDragPreviewNodeData({
  type,
  title,
  defaultLlmProvider = '',
  defaultLlmName = '',
  approvalLabels,
}: CreateDragPreviewNodeDataParams): WorkflowNodeData | null {
  switch (type) {
    case 'start':
      return {
        type: 'start',
        title,
        desc: '',
        variables: [],
        unique: true,
      };
    case 'knowledge-retrieval':
      return {
        ...DEFAULT_KNOWLEDGE_RETRIEVAL_NODE,
        type: 'knowledge-retrieval',
        title,
        desc: '',
        query_variable_selector: [],
        dataset_ids: [],
        retrieval_mode: 'multiple',
        multiple_retrieval_config: {
          top_k: 4,
          reranking_enable: false,
        },
      };
    case 'parameter-extractor':
      return {
        ...DEFAULT_PARAMETER_EXTRACTOR_NODE,
        title,
        model: {
          ...DEFAULT_PARAMETER_EXTRACTOR_NODE.model,
          provider: defaultLlmProvider,
          name: defaultLlmName,
        },
      };
    case 'json-parser':
      return {
        ...DEFAULT_JSON_PARSER_NODE,
        type: 'json-parser',
        title,
        desc: '',
        input_selector: [],
        is_flatten_output: false,
        outputs: {},
        error_strategy: 'none',
      };
    case 'image-gen':
      return {
        ...DEFAULT_IMAGE_GEN_NODE_DATA,
        title,
        desc: '',
      };
    case 'approval': {
      const approveActionId = createApprovalActionId([], 'approve');
      const rejectActionId = createApprovalActionId([approveActionId], 'reject');
      return {
        ...DEFAULT_APPROVAL_NODE_DATA,
        type: 'approval',
        title,
        desc: '',
        approval: {
          ...DEFAULT_APPROVAL_NODE_DATA.approval,
          content: approvalLabels?.content ?? DEFAULT_APPROVAL_NODE_DATA.approval.content,
          fields: [],
          actions: [
            {
              ...DEFAULT_APPROVAL_NODE_DATA.approval.actions[0],
              id: approveActionId,
              label:
                approvalLabels?.approveLabel ??
                DEFAULT_APPROVAL_NODE_DATA.approval.actions[0].label,
            },
            {
              ...DEFAULT_APPROVAL_NODE_DATA.approval.actions[1],
              id: rejectActionId,
              label:
                approvalLabels?.rejectLabel ?? DEFAULT_APPROVAL_NODE_DATA.approval.actions[1].label,
            },
          ],
        },
        submit_methods: {
          ...DEFAULT_APPROVAL_NODE_DATA.submit_methods,
          email: {
            ...DEFAULT_APPROVAL_NODE_DATA.submit_methods.email,
            subject: approvalLabels?.emailSubject ?? '',
            body: approvalLabels?.emailBody
              ? `${approvalLabels.emailBody} {{#url#}}`
              : DEFAULT_APPROVAL_NODE_DATA.submit_methods.email.body,
          },
        },
      };
    }
    case 'announcement':
      return {
        ...DEFAULT_ANNOUNCEMENT_NODE_DATA,
        type: 'announcement',
        title,
        desc: '',
        announcement: {
          ...DEFAULT_ANNOUNCEMENT_NODE_DATA.announcement,
          content: DEFAULT_ANNOUNCEMENT_NODE_DATA.announcement.content,
        },
      };
    case 'question-answer':
      return {
        ...DEFAULT_QUESTION_ANSWER_NODE_DATA,
        type: 'question-answer',
        title,
        desc: '',
        model: {
          ...DEFAULT_QUESTION_ANSWER_NODE_DATA.model,
          provider: defaultLlmProvider,
          name: defaultLlmName,
        },
        model_config: {
          ...DEFAULT_QUESTION_ANSWER_NODE_DATA.model_config,
          provider: defaultLlmProvider,
          name: defaultLlmName,
        },
      };
    case 'variable-aggregator':
      return {
        ...DEFAULT_VARIABLE_AGGREGATOR_NODE_DATA,
        type: 'variable-aggregator',
        title,
        desc: '',
        variables: [],
        output_type: undefined,
        advanced_settings: { group_enabled: false, groups: [] },
      };
    case 'sql-generator':
      return {
        ...DEFAULT_SQL_GENERATOR_NODE_DATA,
        type: 'sql-generator',
        title,
        desc: '',
        data: {
          model: {
            provider: defaultLlmProvider,
            name: defaultLlmName,
            mode: 'chat',
            completion_params: {},
          },
          data_source: {
            source: {
              id: '',
              name: '',
              schema: 'public',
              type: 'postgres',
            },
            tables: [],
          },
          prompt: '',
          execution: { ...DEFAULT_DATABASE_EXECUTION },
        },
      };
    case 'llm':
      return {
        ...DEFAULT_LLM_NODE_DATA,
        type: 'llm',
        title,
        desc: '',
        variables: [],
        model: {
          provider: defaultLlmProvider,
          name: defaultLlmName,
          mode: 'chat',
          completion_params: {},
        },
        prompt_template: [
          {
            id: 'system',
            role: 'system',
            text: '',
          },
          {
            id: 'current-user',
            role: 'user',
            text: '{{#sys.query#}}',
            group_id: 'current-user',
            group_kind: 'current_user',
          },
        ],
        prompt_layout: {
          version: 1,
          items: [
            { type: 'history', id: 'conversation_history' },
            { type: 'group', group_id: 'current-user' },
          ],
        },
        prompt_config: {
          jinja2_variables: [],
        },
        vision: {
          enabled: false,
        },
        structured_output_enabled: false,
      } as WorkflowNodeData;
    case 'http-request':
      return {
        ...DEFAULT_HTTP_REQUEST_NODE_DATA,
        type: 'http-request',
        title,
        desc: '',
        method: 'GET',
        url: '',
        params: [],
        headers: [],
        body: { type: 'none', data: [] },
        timeout: {
          max_connect_timeout: 0,
          max_read_timeout: 0,
          max_write_timeout: 0,
          connect: 10,
          read: 60,
          write: 10,
        },
        retry_config: {
          retry_enabled: false,
          max_retries: 3,
          retry_interval: 1000,
        },
      };
    case 'call-database':
      return {
        ...DEFAULT_CALL_DATABASE_NODE_DATA,
        type: 'call-database',
        title,
        desc: '',
        data: {
          data_source: { ...DEFAULT_DATABASE_SOURCE },
          table_selection: [],
          manual_sql: '',
          execution: { ...DEFAULT_DATABASE_EXECUTION },
        },
      };
    case 'create-scheduled-task':
      return {
        ...createDefaultCreateScheduledTaskNodeData(),
        type: 'create-scheduled-task',
        title,
        desc: '',
      };
    case 'notification-sms':
      return {
        ...DEFAULT_NOTIFICATION_SMS_NODE_DATA,
        type: 'notification-sms',
        title,
        desc: '',
      };
    case 'end':
      return {
        ...DEFAULT_END_NODE_DATA,
        type: 'end',
        title,
        desc: '',
        outputs: [],
      };
    case 'loop-end':
      return {
        ...DEFAULT_LOOP_END_NODE_DATA,
        type: 'loop-end',
        title,
        desc: '',
      };
    case 'answer':
      return {
        ...DEFAULT_ANSWER_NODE_DATA,
        type: 'answer',
        title,
        desc: '',
        variables: [],
        answer: '',
      };
    case 'code':
      return {
        ...DEFAULT_CODE_NODE_DATA,
        type: 'code',
        title,
        desc: '',
        code: DEFAULT_CODE_TEMPLATE,
        code_language: 'python3',
        variables: [
          { variable: 'var1', value_selector: [], value_type: 'string' },
          { variable: 'var2', value_selector: [], value_type: 'string' },
        ],
        outputs: { result: { type: 'string', children: null } },
        outputKeyOrders: ['result'],
      };
    case 'iteration':
      return {
        ...DEFAULT_ITERATION_NODE_DATA,
        type: 'iteration',
        title,
        desc: '',
        start_node_id: '',
        _children: [],
      };
    case 'loop':
      return {
        ...DEFAULT_LOOP_NODE,
        type: 'loop',
        title,
        desc: '',
        start_node_id: '',
        _children: [],
      };
    case 'assigner':
      return {
        ...DEFAULT_ASSIGNER_NODE_DATA,
        type: 'assigner',
        title,
        desc: '',
        version: '2',
        items: [],
      };
    case 'if-else':
      return {
        type: 'if-else',
        title,
        desc: '',
        cases: [
          {
            case_id: 'true',
            logical_operator: 'and',
            conditions: [],
          },
        ],
        targetBranches: [
          { id: 'true', name: 'IF' },
          { id: 'false', name: 'ELSE' },
        ],
      };
    case 'document-extractor':
      return {
        ...DEFAULT_DOCUMENT_EXTRACTOR_NODE_DATA,
        type: 'document-extractor',
        title,
        desc: '',
        variable_selector: [],
        is_array_file: false,
      };
    default:
      return null;
  }
}

/**
 * @util createToolDragPreviewNodeData
 * @description Builds initial preview data for a builtin tool node.
 */
export function createToolDragPreviewNodeData({
  title,
  providerId,
  toolName,
}: CreateToolDragPreviewNodeDataParams): ToolNodeData {
  return {
    ...DEFAULT_TOOL_NODE_DATA,
    type: 'tools',
    title,
    desc: '',
    provider_type: 'builtin',
    provider_id: providerId,
    tool_name: toolName,
    tool_parameters: {},
  };
}
