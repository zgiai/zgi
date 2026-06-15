import { useCallback, useMemo } from 'react';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { useWorkflowStore } from '../store';
import type { WorkflowEdge, WorkflowNode, WorkflowNodeData, ToolNodeData } from '../store/type';
import {
  isContainerNode,
  isContainerStartNode,
  getContainerStartType,
  getContainerCustomStartType,
} from '../store/type';
import { NODE_TYPES } from '../nodes';

import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { AgentType } from '@/services/types/agent';
import { useCombinedWorkflowSave } from './use-combined-workflow-save';
import { useWorkflowEditor } from './use-workflow-editor';
import { default as useResetWorkflow } from './use-reset-workflow';

import { DEFAULT_PARAMETER_EXTRACTOR_NODE } from '../nodes/parameter-extractor/config';
import {
  DEFAULT_DATABASE_SOURCE,
  DEFAULT_DATABASE_EXECUTION,
  DEFAULT_CALL_DATABASE_NODE_DATA,
} from '../nodes/call-database/config';
import { DEFAULT_KNOWLEDGE_RETRIEVAL_NODE } from '../nodes/knowledge-retrieval/config';
import { DEFAULT_JSON_PARSER_NODE } from '../nodes/json-parser/config';
import { DEFAULT_IMAGE_GEN_NODE_DATA } from '../nodes/image-gen/config';
import { createApprovalActionId, DEFAULT_APPROVAL_NODE_DATA } from '../nodes/approval/config';
import { DEFAULT_ANNOUNCEMENT_NODE_DATA } from '../nodes/announcement/config';
import { DEFAULT_QUESTION_ANSWER_NODE_DATA } from '../nodes/question-answer/config';
import { DEFAULT_VARIABLE_AGGREGATOR_NODE_DATA } from '../nodes/variable-aggregator/config';
import { DEFAULT_SQL_GENERATOR_NODE_DATA } from '../nodes/sql-generator/config';
import { DEFAULT_LLM_NODE_DATA } from '../nodes/llm/config';
import { getDefaultWorkflowPrompts } from '../nodes/llm/default-prompts';
import { DEFAULT_HTTP_REQUEST_NODE_DATA } from '../nodes/http-request/config';
import { DEFAULT_TOOL_NODE_DATA } from '../nodes/tool/config';
import { createDefaultCreateScheduledTaskNodeData } from '../nodes/create-scheduled-task/config';
import { DEFAULT_NOTIFICATION_SMS_NODE_DATA } from '../nodes/notification-sms/config';
import { DEFAULT_END_NODE_DATA } from '../nodes/end/config';
import { DEFAULT_LOOP_END_NODE_DATA } from '../nodes/loop-end/config';
import { DEFAULT_ANSWER_NODE_DATA } from '../nodes/answer/config';
import { DEFAULT_CODE_NODE_DATA } from '../nodes/code/config';
import { DEFAULT_ASSIGNER_NODE_DATA } from '../nodes/assigner/config';
import { DEFAULT_DOCUMENT_EXTRACTOR_NODE_DATA } from '../nodes/document-extractor/config';

/**
 * Hook for workflow operations like adding nodes, copying, etc.
 */
const useWorkflowOperations = () => {
  const t = useT('nodes');
  const { locale } = useLocale();
  // Fine-grained selectors for minimize re-renders
  const addNode = useWorkflowStore.use.addNode();
  const addNodes = useWorkflowStore.use.addNodes();
  const deleteNode = useWorkflowStore.use.deleteNode();
  const deleteNodes = useWorkflowStore.use.deleteNodes();
  const updateNode = useWorkflowStore.use.updateNode();
  const updateNodeData = useWorkflowStore.use.updateNodeData();
  const canAddNode = useWorkflowStore.use.canAddNode();
  const agentType = useWorkflowStore.use.agentType();
  const setClipboardNodeData = useWorkflowStore.use.setClipboardNodeData();
  const setClipboardNodes = useWorkflowStore.use.setClipboardNodes();
  const beginHistoryBatch = useWorkflowStore.use.beginHistoryBatch();
  const endHistoryBatch = useWorkflowStore.use.endHistoryBatch();
  const selectedNodeId = useWorkflowStore.use.selectedNodeId();

  const isLoading = useWorkflowStore.use.isLoading();
  const isDirty = useWorkflowStore.use.isDirty();

  // Editor context
  const { agentId } = useWorkflowEditor();

  const { handleCombinedSave } = useCombinedWorkflowSave(agentId);
  const { resetWithConfirm } = useResetWorkflow();

  // Fetch default LLM model for initializing new LLM nodes
  const { value: defaultLlm } = useDefaultModelByUseCase('text-chat');
  const defaultLlmProvider = defaultLlm?.provider ?? '';
  const defaultLlmName = defaultLlm?.model ?? '';
  const { conversational: defaultConversationalPrompt, standard: defaultStandardPrompt } =
    useMemo(() => getDefaultWorkflowPrompts(locale), [locale]);

  // side effects moved to useWorkflowInitializer

  // Add different types of nodes
  const addNodeWithContainerCheck = useCallback(
    (
      data: WorkflowNodeData,
      position: { x: number; y: number },
      parentId?: string
    ): string | null => {
      let finalData = { ...data };
      if (parentId) {
        const parentNode = useWorkflowStore.getState().nodes.find(n => n.id === parentId);
        const parentType = (parentNode?.data as WorkflowNodeData)?.type;
        if (parentType === 'iteration') {
          finalData = { ...finalData, isInIteration: true, isInLoop: false };
        } else if (parentType === 'loop') {
          finalData = { ...finalData, isInLoop: true, isInIteration: false };
        }
      }
      return addNode(finalData, position, parentId);
    },
    [addNode]
  );

  const addStartNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      if (!canAddNode(NODE_TYPES.START)) {
        return null;
      }

      const id = addNodeWithContainerCheck(
        {
          type: 'start',
          title: (() => {
            try {
              return t('catalog.start.title');
            } catch (_e) {
              return '开始';
            }
          })(),
          desc: '',
          variables: [],
          unique: true,
        },
        position,
        parentId
      );
      return id;
    },
    [addNode, canAddNode, t]
  );

  const addKnowledgeRetrievalNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_KNOWLEDGE_RETRIEVAL_NODE,
          type: 'knowledge-retrieval',
          title: (() => {
            try {
              return t('catalog.knowledge-retrieval.title');
            } catch (_e) {
              return '知识检索';
            }
          })(),
          desc: '',
          // New schema: a pair [sourceNodeId, variableKey]; empty when unset
          query_variable_selector: [],
          dataset_ids: [],
          retrieval_mode: 'multiple',
          multiple_retrieval_config: {
            top_k: 4,
            reranking_enable: false,
          },
        },
        position,
        parentId
      );
      return id;
    },
    [addNode, t]
  );

  const addParameterExtractorNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_PARAMETER_EXTRACTOR_NODE,
          title: (() => {
            try {
              return t('catalog.parameter-extractor.title');
            } catch (_e) {
              return '参数提取器';
            }
          })(),
          model: {
            ...DEFAULT_PARAMETER_EXTRACTOR_NODE.model,
            provider: defaultLlmProvider,
            name: defaultLlmName,
          },
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, defaultLlmName, defaultLlmProvider, t]
  );

  const addJsonParserNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_JSON_PARSER_NODE,
          type: 'json-parser',
          title: (() => {
            try {
              return t('catalog.json-parser.title');
            } catch (_e) {
              return 'JSON Parser';
            }
          })(),
          desc: '',
          input_selector: [],
          is_flatten_output: false,
          outputs: {},
          error_strategy: 'none',
        },
        position,
        parentId
      );
      return id;
    },
    [addNode, t]
  );

  const addImageGenNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_IMAGE_GEN_NODE_DATA,
          title: (() => {
            try {
              return t('catalog.image-gen.title');
            } catch (_e) {
              return 'Image Generation';
            }
          })(),
          desc: '',
        },
        position,
        parentId
      );
      return id;
    },
    [addNode, t]
  );

  const addApprovalNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      if (parentId) {
        return null;
      }

      const approvalDefaults = DEFAULT_APPROVAL_NODE_DATA;
      const approveActionId = createApprovalActionId([], 'approve');
      const rejectActionId = createApprovalActionId([approveActionId], 'reject');
      const id = addNodeWithContainerCheck(
        {
          ...approvalDefaults,
          type: 'approval',
          title: (() => {
            try {
              return t('catalog.approval.title');
            } catch (_e) {
              return 'Approval';
            }
          })(),
          desc: '',
          approval: {
            ...approvalDefaults.approval,
            content: t('approval.defaults.content'),
            fields: [],
            actions: [
              {
                ...approvalDefaults.approval.actions[0],
                id: approveActionId,
                label: t('approval.defaults.approveLabel'),
              },
              {
                ...approvalDefaults.approval.actions[1],
                id: rejectActionId,
                label: t('approval.defaults.rejectLabel'),
              },
            ],
          },
          submit_methods: {
            ...approvalDefaults.submit_methods,
            email: {
              ...approvalDefaults.submit_methods.email,
              subject: t('approval.defaults.emailSubject'),
              body: `${t('approval.defaults.emailBody')} {{#url#}}`,
            },
          },
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  const addAnnouncementNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      return addNodeWithContainerCheck(
        {
          ...DEFAULT_ANNOUNCEMENT_NODE_DATA,
          type: 'announcement',
          title: (() => {
            try {
              return t('catalog.announcement.title');
            } catch (_e) {
              return 'Announcement';
            }
          })(),
          desc: '',
          announcement: {
            ...DEFAULT_ANNOUNCEMENT_NODE_DATA.announcement,
            title: t('catalog.announcement.title'),
            content: t('announcement.defaults.content'),
          },
        },
        position,
        parentId
      );
    },
    [addNodeWithContainerCheck, t]
  );

  const addQuestionAnswerNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      if (parentId) {
        return null;
      }

      return addNodeWithContainerCheck(
        {
          ...DEFAULT_QUESTION_ANSWER_NODE_DATA,
          type: 'question-answer',
          title: (() => {
            try {
              return t('catalog.question-answer.title');
            } catch (_e) {
              return 'Question Answer';
            }
          })(),
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
        },
        position,
        parentId
      );
    },
    [addNodeWithContainerCheck, defaultLlmName, defaultLlmProvider, t]
  );

  const addVariableAggregatorNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_VARIABLE_AGGREGATOR_NODE_DATA,
          type: 'variable-aggregator',
          title: (() => {
            try {
              return t('catalog.variable-aggregator.title');
            } catch (_e) {
              return '变量聚合器';
            }
          })(),
          desc: '',
          variables: [],
          output_type: undefined,
          advanced_settings: { group_enabled: false, groups: [] },
        },
        position,
        parentId
      );
      return id;
    },
    [addNode, t]
  );

  const addSqlGeneratorNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_SQL_GENERATOR_NODE_DATA,
          type: 'sql-generator',
          title: (() => {
            try {
              return t('catalog.sql-generator.title');
            } catch (_e) {
              return 'SQL Generator';
            }
          })(),
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
        },
        position,
        parentId
      );
      return id;
    },
    [addNode, defaultLlmName, defaultLlmProvider, t]
  );

  const addLLMNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const isConversational = agentType === AgentType.CONVERSATIONAL_AGENT;
      const defaultPrompt = isConversational
        ? defaultConversationalPrompt
        : defaultStandardPrompt;
      const promptTemplate = [
        {
          id: 'system',
          role: 'system' as const,
          text: defaultPrompt.text,
        },
        ...(isConversational
          ? [
              {
                id: 'current-user',
                role: 'user' as const,
                text: '{{#sys.query#}}',
                group_id: 'current-user',
                group_kind: 'current_user' as const,
              },
            ]
          : [
              {
                id: 'task-input',
                role: 'user' as const,
                text: '',
                group_id: 'task-input',
              },
            ]),
      ];
      const promptLayoutItems = isConversational
        ? [{ type: 'group' as const, group_id: 'current-user' }]
        : [{ type: 'group' as const, group_id: 'task-input' }];

      const baseData = {
        ...DEFAULT_LLM_NODE_DATA,
        type: 'llm' as const,
        title: (() => {
          try {
            return t('catalog.llm.title');
          } catch (_e) {
            return 'LLM';
          }
        })(),
        desc: '',
        variables: [],
        model: {
          provider: defaultLlmProvider,
          name: defaultLlmName,
          mode: 'chat' as const,
          completion_params: {},
        },
        prompt_template: promptTemplate,
        prompt_layout: {
          version: 1 as const,
          items: promptLayoutItems,
        },
        prompt_config: {
          jinja2_variables: [],
        },
        conversation_history: {
          enabled: isConversational,
          history_window_size: 3,
        },
        vision: {
          enabled: false,
        },
        structured_output_enabled: false,
      };

      const id = addNodeWithContainerCheck(baseData, position, parentId);
      return id;
    },
    [
      addNodeWithContainerCheck,
      agentType,
      defaultConversationalPrompt,
      defaultLlmName,
      defaultLlmProvider,
      defaultStandardPrompt,
      t,
    ]
  );

  const addHttpRequestNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_HTTP_REQUEST_NODE_DATA,
          type: 'http-request',
          title: (() => {
            try {
              return t('catalog.http-request.title');
            } catch (_e) {
              return 'HTTP 请求';
            }
          })(),
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
        },
        position,
        parentId
      );
      return id;
    },
    [addNode, t]
  );

  const addCallDatabaseNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_CALL_DATABASE_NODE_DATA,
          type: 'call-database',
          title: (() => {
            try {
              return t('catalog.call-database.title');
            } catch (_e) {
              return '调用数据表';
            }
          })(),
          desc: '',
          data: {
            data_source: { ...DEFAULT_DATABASE_SOURCE },
            table_selection: [],
            manual_sql: '',
            execution: { ...DEFAULT_DATABASE_EXECUTION },
          },
        },
        position,
        parentId
      );
      return id;
    },
    [addNode, t]
  );

  const addToolNode = useCallback(
    (
      position: { x: number; y: number },
      parentId?: string,
      initialData?: Partial<ToolNodeData>
    ): string | null => {
      // Use provided title from initialData, or try translating generic tool title as a last resort fallback.
      // We check for initialData.title FIRST to avoid calling useTranslations if not needed,
      // preventing "Missing message" warnings in the console.
      const nodeTitle =
        initialData?.title ||
        (() => {
          try {
            return t('catalog.tool.title');
          } catch (_e) {
            return 'Tool';
          }
        })();

      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_TOOL_NODE_DATA,
          type: 'tools',
          title: nodeTitle,
          desc: '',
          provider_type: 'builtin',
          provider_id: '',
          tool_name: '',
          tool_parameters: {},
          ...initialData,
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  const addCreateScheduledTaskNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const defaultNodeData = createDefaultCreateScheduledTaskNodeData();
      const id = addNodeWithContainerCheck(
        {
          ...defaultNodeData,
          type: 'create-scheduled-task',
          title: t('catalog.create-scheduled-task.title'),
          desc: '',
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  const addNotificationSMSNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_NOTIFICATION_SMS_NODE_DATA,
          type: 'notification-sms',
          title: t('catalog.notification-sms.title'),
          desc: '',
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  const addEndNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      // Disallow end node in conversational agent workflows
      if (agentType === AgentType.CONVERSATIONAL_AGENT) {
        return null;
      }

      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_END_NODE_DATA,
          type: 'end',
          title: (() => {
            try {
              return t('catalog.end.title');
            } catch (_e) {
              return '结束';
            }
          })(),
          desc: '',
          outputs: [],
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, agentType, t]
  );

  const addLoopEndNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_LOOP_END_NODE_DATA,
          type: 'loop-end',
          title: (() => {
            try {
              return t('catalog.loop-end.title');
            } catch (_e) {
              return '退出循环';
            }
          })(),
          desc: '',
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  const addAnswerNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      if (agentType !== AgentType.CONVERSATIONAL_AGENT) {
        return null;
      }

      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_ANSWER_NODE_DATA,
          type: 'answer',
          title: (() => {
            try {
              return t('catalog.answer.title');
            } catch (_e) {
              return '直接回复';
            }
          })(),
          desc: '',
          variables: [],
          answer: '',
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, agentType, t]
  );

  const addCodeNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      // Default to python3 template and consistent IO with sample
      const defaultPy =
        `# Entry function. Return a dict with your outputs\n` +
        `def main(var1, var2):\n` +
        `    return { 'result': var1 + var2 }\n`;

      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_CODE_NODE_DATA,
          type: 'code',
          title: (() => {
            try {
              return t('catalog.code.title');
            } catch (_e) {
              return '代码执行';
            }
          })(),
          desc: '',
          code: defaultPy,
          code_language: 'python3',
          variables: [
            { variable: 'var1', value_selector: [], value_type: 'string' },
            { variable: 'var2', value_selector: [], value_type: 'string' },
          ],
          outputs: { result: { type: 'string', children: null } },
          outputKeyOrders: ['result'],
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  const addIterationNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      // Ensure creation is a single undo step
      const alreadyBatching = useWorkflowStore.getState().isHistoryBatching;
      if (!alreadyBatching) beginHistoryBatch();
      try {
        const iterationId = addNodeWithContainerCheck(
          {
            type: 'iteration',
            title: (() => {
              try {
                return t('catalog.iteration.title');
              } catch (_e) {
                return '迭代';
              }
            })(),
            desc: '',
            start_node_id: '',
            iterator_selector: [],
            iterator_input_type: 'array[string]',
            output_selector: [],
            output_type: 'array[string]',
            is_parallel: false,
            parallel_nums: 10,
            error_handle_mode: 'terminated',
            _children: [],
          },
          position,
          parentId
        );
        if (!iterationId) return null;

        // Create companion iteration-start child and enforce id = parentId + 'start'
        const rawStartId = addNode(
          { type: 'iteration-start', isInIteration: true, isInLoop: false, title: '', desc: '' },
          position,
          iterationId // parentId updated from parentId to iterationId to avoid conflict with outer parentId
        );
        if (rawStartId) {
          const enforcedStartId = `${iterationId}start`;
          updateNode(rawStartId, {
            id: enforcedStartId,
            type: 'custom-iteration-start',
            parentId: iterationId as unknown as never,
            extent: 'parent' as unknown as never,
            position: { x: 12, y: 52 },
            selectable: false,
            draggable: false,
            width: 40,
            height: 40,
          } as unknown as WorkflowNode);
          updateNodeData(iterationId, {
            start_node_id: enforcedStartId,
            _children: [enforcedStartId],
          });
          const parentNode = useWorkflowStore.getState().nodes.find(n => n.id === iterationId);
          const parentTitle = ((parentNode?.data as unknown as { title?: string })?.title ||
            '迭代') as string;
          updateNodeData(enforcedStartId, { title: parentTitle });
        }
        return iterationId;
      } finally {
        if (!alreadyBatching) endHistoryBatch();
      }
    },
    [
      addNodeWithContainerCheck,
      addNode,
      updateNode,
      updateNodeData,
      t,
      beginHistoryBatch,
      endHistoryBatch,
    ]
  );

  const addLoopNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      // Ensure creation is a single undo step
      const alreadyBatching = useWorkflowStore.getState().isHistoryBatching;
      if (!alreadyBatching) beginHistoryBatch();
      try {
        const loopId = addNodeWithContainerCheck(
          {
            type: 'loop',
            title: (() => {
              try {
                return t('catalog.loop.title');
              } catch (_e) {
                return '循环';
              }
            })(),
            desc: '',
            start_node_id: '',
            loop_count: 10,
            break_conditions: [],
            logical_operator: 'and',
            loop_variables: [],
            _children: [],
          },
          position,
          parentId
        );
        if (!loopId) return null;

        // Create companion loop-start child and enforce id = parentId + 'start'
        const rawStartId = addNode(
          { type: 'loop-start', isInLoop: true, isInIteration: false, title: '', desc: '' },
          position,
          loopId // parentId updated from parentId to loopId
        );
        if (rawStartId) {
          const enforcedStartId = `${loopId}start`;
          updateNode(rawStartId, {
            id: enforcedStartId,
            type: 'custom-loop-start',
            parentId: loopId as unknown as never,
            extent: 'parent' as unknown as never,
            position: { x: 12, y: 52 },
            selectable: false,
            draggable: false,
            width: 40,
            height: 40,
          } as unknown as WorkflowNode);
          updateNodeData(loopId, {
            start_node_id: enforcedStartId,
            _children: [enforcedStartId],
          });
          const parentNode = useWorkflowStore.getState().nodes.find(n => n.id === loopId);
          const parentTitle = ((parentNode?.data as unknown as { title?: string })?.title ||
            '循环') as string;
          updateNodeData(enforcedStartId, { title: parentTitle });
        }
        return loopId;
      } finally {
        if (!alreadyBatching) endHistoryBatch();
      }
    },
    [
      addNodeWithContainerCheck,
      addNode,
      updateNode,
      updateNodeData,
      t,
      beginHistoryBatch,
      endHistoryBatch,
    ]
  );

  const addAssignerNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_ASSIGNER_NODE_DATA,
          type: 'assigner',
          title: (() => {
            try {
              return t('catalog.assigner.title');
            } catch (_e) {
              return '变量赋值';
            }
          })(),
          desc: '',
          version: '2',
          items: [],
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  const addIfElseNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          type: 'if-else',
          title: (() => {
            try {
              return t('catalog.if-else.title');
            } catch (_e) {
              return '条件分支';
            }
          })(),
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
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  const addDocumentExtractorNode = useCallback(
    (position: { x: number; y: number }, parentId?: string): string | null => {
      const id = addNodeWithContainerCheck(
        {
          ...DEFAULT_DOCUMENT_EXTRACTOR_NODE_DATA,
          type: 'document-extractor',
          title: (() => {
            try {
              return t('catalog.document-extractor.title');
            } catch (_e) {
              return '文档提取器';
            }
          })(),
          desc: '',
          variable_selector: [],
          is_array_file: false,
        },
        position,
        parentId
      );
      return id;
    },
    [addNodeWithContainerCheck, t]
  );

  // Delete node with guard against deleting start node
  const deleteNodeSafe = useCallback(
    (nodeId: string): boolean => {
      const nodes = useWorkflowStore.getState().nodes;
      const node = nodes.find(n => n.id === nodeId);
      if (!node) return false;
      if (node.data.type === NODE_TYPES.START) {
        return false;
      }
      deleteNode(nodeId);
      return true;
    },
    [deleteNode]
  );

  // Delete selected node
  const deleteSelectedNode = useCallback(() => {
    const { selectedNodeId } = useWorkflowStore.getState();
    if (selectedNodeId) {
      deleteNodeSafe(selectedNodeId);
    }
  }, [deleteNodeSafe]);

  // Delete all currently selected nodes (box/multi-select)
  const deleteSelectedNodes = useCallback(() => {
    const { nodes } = useWorkflowStore.getState();
    const selectedIds = nodes.filter(n => n.selected).map(n => n.id);
    if (selectedIds.length === 0) return;
    deleteNodes(selectedIds);
  }, [deleteNodes]);

  // Copy selected node (to internal clipboard only)
  const copySelectedNode = useCallback(() => {
    const { nodes, selectedNodeId } = useWorkflowStore.getState();
    // Multi-select: when multiple nodes are selected in pointer mode, copy all
    const selectedNodesAll = nodes.filter(n => n.selected);
    // Ignore Start nodes during copy
    const selectedNodes = selectedNodesAll.filter(
      n => (n.data as { type?: string })?.type !== NODE_TYPES.START
    );
    if (selectedNodes.length > 1) {
      // Use the first selected node as origin to compute relative offsets
      const first = selectedNodes[0];
      const origin = first ? first.position : { x: 0, y: 0 };
      const iterParentIds = new Set(
        selectedNodes
          .filter(n => isContainerNode((n.data as { type?: string })?.type))
          .map(n => n.id)
      );
      const items: Array<{
        data: WorkflowNode['data'];
        offset: { x: number; y: number };
        parentRef?: string;
        refId?: string;
      }> = [];
      // Add independent selected nodes (exclude iteration children under selected iteration parents)
      selectedNodes.forEach(n => {
        const parentId = (n as unknown as { parentId?: string }).parentId;
        const isChildOfSelectedIter = parentId && iterParentIds.has(parentId);
        if (!isChildOfSelectedIter) {
          items.push({
            data: n.data,
            offset: { x: n.position.x - origin.x, y: n.position.y - origin.y },
            refId: isContainerNode((n.data as { type?: string })?.type) ? n.id : undefined,
          });
        }
      });
      // For each selected iteration parent, include its children (including iteration-start)
      iterParentIds.forEach(pid => {
        const children = nodes.filter(
          n => (n as unknown as { parentId?: string }).parentId === pid
        );
        children.forEach(c => {
          items.push({
            data: c.data,
            // children offset relative to parent
            offset: { x: c.position.x, y: c.position.y },
            parentRef: pid,
            refId: c.id,
          });
        });
      });
      setClipboardNodes(items);
      setClipboardNodeData(null);
      return;
    }
    // Single selection fallback
    if (!selectedNodeId) return;
    const selectedNode = nodes.find(node => node.id === selectedNodeId);
    if (!selectedNode) return;
    const selType = (selectedNode.data as { type?: string })?.type;
    // Ignore Start node when single-copy
    if (selType === NODE_TYPES.START) {
      setClipboardNodeData(null);
      setClipboardNodes([]);
      return;
    }
    // If copying a container node, bundle its children (including container-start)
    if (isContainerNode(selType)) {
      const children = nodes.filter(
        n => (n as unknown as { parentId?: string }).parentId === selectedNodeId
      );
      const items = [
        { data: selectedNode.data, offset: { x: 0, y: 0 }, refId: selectedNodeId },
        ...children.map(c => ({
          data: c.data,
          // children position is already relative to parent in iteration extent
          offset: { x: c.position.x, y: c.position.y },
          parentRef: selectedNodeId,
          refId: c.id,
        })),
      ];
      setClipboardNodes(items);
      setClipboardNodeData(null);
      return;
    }
    setClipboardNodeData(selectedNode.data);
    setClipboardNodes([]);
  }, [setClipboardNodes, setClipboardNodeData]);

  // Paste clipboard at current pointer position (flow coordinates)
  const pasteClipboardAtPointer = useCallback(() => {
    const { clipboardNodeData, clipboardNodes, viewport, selectedNodeId } =
      useWorkflowStore.getState();
    const lastMouseClient = useWorkflowStore.getState().lastMouseClient;
    if (!clipboardNodeData && clipboardNodes.length === 0) return;
    const container = document.querySelector('.react-flow') as HTMLElement | null;
    let position = { x: 0, y: 0 };
    if (container && lastMouseClient) {
      const rect = container.getBoundingClientRect();
      const flowX = (lastMouseClient.x - rect.left - viewport.x) / viewport.zoom;
      const flowY = (lastMouseClient.y - rect.top - viewport.y) / viewport.zoom;
      position = { x: flowX, y: flowY };
    } else {
      // Fallback: if a node is selected, paste near it; else at origin
      const nodes = useWorkflowStore.getState().nodes;
      const selectedNode = selectedNodeId ? nodes.find(n => n.id === selectedNodeId) : undefined;
      position = selectedNode
        ? { x: selectedNode.position.x + 60, y: selectedNode.position.y + 60 }
        : { x: 0, y: 0 };
    }
    if (clipboardNodes.length > 0) {
      beginHistoryBatch();
      try {
        // Create all container parents first(container-start will be created in next step)
        const iterParentItems = clipboardNodes.filter(item =>
          isContainerNode((item.data as { type?: string })?.type)
        );
        const newParentIdByRefId: Record<string, string> = {};
        const parentChildrenIdsByNewId: Record<string, string[]> = {};
        const iterationStartByNewParent: Record<string, string | null> = {};
        const oldToNewByParent: Record<string, Record<string, string>> = {};

        iterParentItems.forEach(parentItem => {
          const offset = parentItem.offset || { x: 0, y: 0 };
          const posAbs = { x: position.x + offset.x, y: position.y + offset.y };
          const newParentId = addNode(parentItem.data, posAbs);
          if (!newParentId) return;
          const refId = parentItem.refId || '';
          if (refId) newParentIdByRefId[refId] = newParentId;
          parentChildrenIdsByNewId[newParentId] = [];
          iterationStartByNewParent[newParentId] = null;
          oldToNewByParent[newParentId] = {};
        });

        // Create and mount all child nodes with parentRef to their new parent nodes
        clipboardNodes
          .filter(item => !!item.parentRef)
          .forEach(item => {
            const refParentId = item.parentRef!;
            const newParentId = newParentIdByRefId[refParentId];
            if (!newParentId) return;
            const childId = addNode(item.data, { x: position.x, y: position.y });
            if (!childId) return;
            updateNode(childId, {
              parentId: newParentId as unknown as never,
              extent: 'parent' as unknown as never,
              position: { x: item.offset.x, y: item.offset.y },
            } as unknown as WorkflowNode);
            const t = (item.data as { type?: string })?.type;
            if (isContainerStartNode(t)) {
              const enforcedStartId = `${newParentId}start`;
              const newParentType =
                (
                  useWorkflowStore.getState().nodes.find(n => n.id === newParentId)
                    ?.data as WorkflowNodeData
                )?.type || 'iteration';
              updateNode(childId, {
                id: enforcedStartId,
                type: getContainerCustomStartType(newParentType),
                selectable: false,
                draggable: false,
                width: 48,
                height: 48,
              } as unknown as WorkflowNode);
              iterationStartByNewParent[newParentId] = enforcedStartId;
            }
            parentChildrenIdsByNewId[newParentId].push(
              t === 'iteration-start' || t === 'loop-start' ? `${newParentId}start` : childId
            );
            const oldChildId = item.refId || '';
            if (oldChildId) {
              (oldToNewByParent[newParentId] ||= {})[oldChildId] =
                t === 'iteration-start' || t === 'loop-start' ? `${newParentId}start` : childId;
            }
          });

        // Paste independent nodes (without parentRef, not iteration parent, not Start)
        const independentItems = clipboardNodes.filter(
          item =>
            !item.parentRef &&
            !isContainerNode((item.data as { type?: string })?.type) &&
            (item.data as { type?: string })?.type !== NODE_TYPES.START
        );
        if (independentItems.length > 0) {
          addNodes(
            independentItems.map(it => ({
              data: it.data,
              position: { x: position.x + it.offset.x, y: position.y + it.offset.y },
            }))
          );
        }

        // Add container-start nodes for parents without them (fallback) and update parent data links
        Object.entries(newParentIdByRefId).forEach(([refId, newParentId]) => {
          if (!iterationStartByNewParent[newParentId]) {
            const newParentType =
              (
                useWorkflowStore.getState().nodes.find(n => n.id === newParentId)
                  ?.data as WorkflowNodeData
              )?.type || 'iteration';
            const startChildId = addNode(
              {
                type: getContainerStartType(newParentType),
                isInIteration: true,
              } as unknown as WorkflowNode['data'],
              { x: position.x, y: position.y }
            );
            if (startChildId) {
              const enforcedStartId = `${newParentId}start`;
              updateNode(startChildId, {
                id: enforcedStartId,
                parentId: newParentId as unknown as never,
                extent: 'parent' as unknown as never,
                position: { x: 24, y: 56 },
                type: getContainerCustomStartType(newParentType),
                selectable: false,
                draggable: false,
                width: 48,
                height: 48,
              } as unknown as WorkflowNode);
              const parentItem2 = iterParentItems.find(it => (it.refId || '') === refId);
              const pTitle = ((parentItem2?.data as unknown as { title?: string })?.title ||
                '迭代') as string;
              updateNodeData(enforcedStartId, { title: pTitle });
              iterationStartByNewParent[newParentId] = enforcedStartId;
              (parentChildrenIdsByNewId[newParentId] ||= []).push(enforcedStartId);
            }
          }
          const parentItem = iterParentItems.find(it => (it.refId || '') === refId);
          const baseData = parentItem?.data ?? ({} as WorkflowNode['data']);
          updateNode(newParentId, {
            data: {
              ...baseData,
              start_node_id: iterationStartByNewParent[newParentId] ?? '',
              _children: parentChildrenIdsByNewId[newParentId] ?? [],
            } as unknown as WorkflowNode['data'],
          });
        });

        // Duplicate edges among children inside each pasted iteration
        const newEdgesToAdd: WorkflowEdge[] = [];
        Object.entries(newParentIdByRefId).forEach(([_oldParentId, newParentId]) => {
          const mapping = oldToNewByParent[newParentId] || {};
          const oldChildIdsSet = new Set(Object.keys(mapping));
          if (oldChildIdsSet.size === 0) return;
          const edges = useWorkflowStore.getState().edges;
          edges.forEach(e => {
            if (oldChildIdsSet.has(e.source) && oldChildIdsSet.has(e.target)) {
              const srcNew = mapping[e.source];
              const tgtNew = mapping[e.target];
              if (!srcNew || !tgtNew) return;
              newEdgesToAdd.push({
                id: `${srcNew}-${tgtNew}`,
                source: srcNew,
                target: tgtNew,
                sourceHandle: e.sourceHandle ?? 'source',
                targetHandle: e.targetHandle ?? 'target',
                type: e.type || 'custom',
                data: { ...(e.data || {}) } as WorkflowEdge['data'],
              });
            }
          });
        });
        if (newEdgesToAdd.length > 0) {
          useWorkflowStore
            .getState()
            .setEdges([...useWorkflowStore.getState().edges, ...newEdgesToAdd]);
        }
      } finally {
        endHistoryBatch();
      }
      return;
    } else if (clipboardNodeData) {
      // Ignore Start node on single paste
      if ((clipboardNodeData as { type?: string })?.type === NODE_TYPES.START) return;
      beginHistoryBatch();
      try {
        addNode(clipboardNodeData, position);
      } finally {
        endHistoryBatch();
      }
      return;
    }
    // Paste plain nodes (not iteration parent, not Start) with relative offsets
    if (clipboardNodes.length > 0) {
      const items = clipboardNodes
        .filter(item => (item.data as { type?: string })?.type !== NODE_TYPES.START)
        .map(item => ({
          data: item.data,
          position: { x: position.x + item.offset.x, y: position.y + item.offset.y },
        }));
      if (items.length > 0) {
        addNodes(items);
      }
    }
  }, [addNode, addNodes, updateNode, updateNodeData, beginHistoryBatch, endHistoryBatch]);

  // Duplicate node with special handling for iteration parent and its children
  const duplicateNode = useCallback(
    (nodeId: string) => {
      const nodes = useWorkflowStore.getState().nodes;
      const selectedNode = nodes.find(node => node.id === nodeId);
      if (!selectedNode) return;
      const newPosition = {
        x: selectedNode.position.x + 50,
        y: selectedNode.position.y + 50,
      };
      beginHistoryBatch();
      try {
        const selType = (selectedNode.data as { type?: string })?.type;
        if (isContainerNode(selType)) {
          // Create new container parent
          const parentId = addNode(selectedNode.data, newPosition);
          if (!parentId) return;
          const parentType = selType;
          // Create container-start child
          let startId: string | null = null;
          const startChildId = addNode(
            {
              type: getContainerStartType(parentType),
              isInIteration: parentType === 'iteration' ? true : undefined,
              isInLoop: parentType === 'loop' ? true : undefined,
              title: '',
              desc: '',
            } as unknown as WorkflowNode['data'],
            newPosition
          );
          if (startChildId) {
            const enforcedStartId = `${parentId}start`;
            updateNode(startChildId, {
              id: enforcedStartId,
              type: getContainerCustomStartType(parentType),
              position: { x: 24, y: 56 },
              parentId: parentId as unknown as never,
              extent: 'parent' as unknown as never,
              selectable: false,
              draggable: false,
              width: 48,
              height: 48,
            } as unknown as WorkflowNode);
            const newParentNode = useWorkflowStore.getState().nodes.find(n => n.id === parentId);
            const parentTitle2 = ((newParentNode?.data as unknown as { title?: string })?.title ||
              '迭代') as string;
            updateNodeData(enforcedStartId, { title: parentTitle2 });
            startId = enforcedStartId;
          }
          // Duplicate other children of original container (excluding container-start)
          const origChildren = nodes.filter(
            n =>
              (n as unknown as { parentId?: string }).parentId === nodeId &&
              !isContainerStartNode((n.data as { type?: string })?.type)
          );
          const newChildIds: string[] = [];
          const oldToNewChild: Record<string, string> = {};
          const oldStartId =
            (selectedNode.data as unknown as { start_node_id?: string })?.start_node_id || '';
          if (startId && oldStartId) {
            oldToNewChild[oldStartId] = startId;
          }
          if (startId) newChildIds.push(startId);
          origChildren.forEach(c => {
            const childNewId = addNode(c.data, newPosition);
            if (!childNewId) return;
            updateNode(childNewId, {
              parentId: parentId as unknown as never,
              extent: 'parent' as unknown as never,
              position: { x: c.position.x, y: c.position.y },
            } as unknown as WorkflowNode);
            newChildIds.push(childNewId);
            oldToNewChild[c.id] = childNewId;
          });
          // Link parent data to new children
          updateNode(parentId, {
            data: {
              ...selectedNode.data,
              start_node_id: startId ?? '',
              _children: newChildIds,
            } as unknown as WorkflowNode['data'],
          });
          // Duplicate edges among original iteration children into the new iteration children
          const oldChildSet = new Set(Object.keys(oldToNewChild));
          const edgesToAdd: WorkflowEdge[] = [];
          const edges = useWorkflowStore.getState().edges;
          edges.forEach(e => {
            if (oldChildSet.has(e.source) && oldChildSet.has(e.target)) {
              const srcNew = oldToNewChild[e.source];
              const tgtNew = oldToNewChild[e.target];
              if (!srcNew || !tgtNew) return;
              edgesToAdd.push({
                id: `${srcNew}-${tgtNew}`,
                source: srcNew,
                target: tgtNew,
                sourceHandle: e.sourceHandle ?? 'source',
                targetHandle: e.targetHandle ?? 'target',
                type: e.type || 'custom',
                data: { ...(e.data || {}) } as WorkflowEdge['data'],
              });
            }
          });
          if (edgesToAdd.length > 0) {
            useWorkflowStore
              .getState()
              .setEdges([...useWorkflowStore.getState().edges, ...edgesToAdd]);
          }
          return;
        }
        // Default: non-iteration node duplicate
        addNode(selectedNode.data, newPosition);
      } finally {
        endHistoryBatch();
      }
    },
    [addNode, updateNode, updateNodeData, beginHistoryBatch, endHistoryBatch]
  );

  // Reset workflow with confirmation via dedicated hook
  const handleResetWorkflow = useCallback(() => {
    resetWithConfirm();
  }, [resetWithConfirm]);

  // Get workflow statistics
  const getWorkflowStats = useCallback(() => {
    const { nodes, edges } = useWorkflowStore.getState();
    return {
      nodeCount: nodes.length,
      edgeCount: edges.length,
      hasStartNode: nodes.some(node => node.data.type === NODE_TYPES.START),
      hasEndNode: nodes.some(node => node.data.type === NODE_TYPES.END),
    };
  }, []);

  // Explicitly memoize return object to avoid downstream re-render cascades
  return useMemo(
    () => ({
      addStartNode,
      addKnowledgeRetrievalNode,
      addParameterExtractorNode,
      addVariableAggregatorNode,
      addSqlGeneratorNode,
      addLLMNode,
      addHttpRequestNode,
      addCallDatabaseNode,
      addToolNode,
      addCreateScheduledTaskNode,
      addNotificationSMSNode,
      addEndNode,
      addLoopEndNode,
      addAnswerNode,
      addCodeNode,
      addIterationNode,
      addLoopNode,
      addAssignerNode,
      addIfElseNode,
      addDocumentExtractorNode,
      addJsonParserNode,
      addImageGenNode,
      addApprovalNode,
      addAnnouncementNode,
      addQuestionAnswerNode,
      deleteNodeSafe,
      deleteSelectedNode,
      deleteSelectedNodes,
      copySelectedNode,
      pasteClipboardAtPointer,
      updateNode,
      updateNodeData,
      deleteNode: deleteNodeSafe,
      duplicateNode,
      handleCombinedSave,
      handleResetWorkflow,
      getWorkflowStats,
      isLoading,
      isDirty,
      selectedNodeId,
    }),
    [
      addStartNode,
      addKnowledgeRetrievalNode,
      addParameterExtractorNode,
      addVariableAggregatorNode,
      addSqlGeneratorNode,
      addLLMNode,
      addHttpRequestNode,
      addCallDatabaseNode,
      addToolNode,
      addCreateScheduledTaskNode,
      addNotificationSMSNode,
      addEndNode,
      addLoopEndNode,
      addAnswerNode,
      addCodeNode,
      addIterationNode,
      addLoopNode,
      addAssignerNode,
      addIfElseNode,
      addDocumentExtractorNode,
      addJsonParserNode,
      addImageGenNode,
      addApprovalNode,
      addAnnouncementNode,
      addQuestionAnswerNode,
      deleteNodeSafe,
      deleteSelectedNode,
      deleteSelectedNodes,
      copySelectedNode,
      pasteClipboardAtPointer,
      updateNode,
      updateNodeData,
      duplicateNode,
      handleCombinedSave,
      handleResetWorkflow,
      getWorkflowStats,
      isLoading,
      isDirty,
      selectedNodeId,
    ]
  );
};

export default useWorkflowOperations;
