import React, { useRef } from 'react';
import { useT } from '@/i18n';
import type { NodePropsCompat, WorkflowNodeData } from '../../store';
import { cn } from '@/lib/utils';
import CustomHandle, { HandleConfigs } from '../../ui/custom-handle';
import NodeCard from './node-card';
import { NODE_THEMES, type NodeTheme } from './config';
import { NODE_CONFIG } from './config';
import { useWorkflowStore } from '../../store/store';
import NodeRuntimeLogDetails from './node-runtime-log-details';
import { AlertCircle, TriangleAlert } from 'lucide-react';

// Import content-only components per node type
import StartContent from '../start';
import KnowledgeRetrievalContent from '../knowledge-retrieval';
import LLMContent from '../llm';
import EndContent from '../end';
import HttpRequestContent from '../http-request';
import CreateScheduledTaskContent from '../create-scheduled-task';
import NotificationSMSContent from '../notification-sms';
import CallDatabaseContent from '../call-database';
import SqlGeneratorContent from '../sql-generator';
import { useUpdateNodeInternals } from '@xyflow/react';
import type { IfElseNodeData } from '../if-else/types';
import type { HttpRequestNodeData } from '../http-request/config';
import type { JsonParserNodeData } from '../json-parser/config';
// no need for DEFAULT_BRANCHES here; if-else handles rendered inline within content
import IfElseContent from '../if-else';
import CodeContent from '../code';
import AssignerContent from '../assigner';
import ContainerContent from '../container';
import useAutoDimensionsSync from '../hooks/use-auto-dimensions-sync';
import AnswerContent from '../answer';
import DocumentExtractorContent from '../document-extractor';
import VariableAggregatorContent from '../variable-aggregator';
import ParameterExtractorContent from '../parameter-extractor';
import JsonParserContent from '../json-parser';
import ImageGenContent from '../image-gen';
import ApprovalContent from '../approval';
import AnnouncementContent from '../announcement';
import QuestionAnswerContent from '../question-answer';
import type { QuestionAnswerNodeData } from '../question-answer/config';

// Wrapper + dispatcher: decide which content to render based on data.type
// It centralizes theme (icon/title/desc/badge/colors) and handle rendering.

type CustomNodeProps = NodePropsCompat<WorkflowNodeData>;
const EMPTY_RUNTIME_LOG_ITEMS: [] = [];
type StartNodeInputConfig = Extract<WorkflowNodeData, { type: 'start' }> & {
  input_config?: unknown[];
};
type NodeDataWithIconUrl = WorkflowNodeData & { iconUrl?: string };
const LOCALIZED_START_NODE_DESCRIPTIONS = new Set([
  'Workflow start node',
  '工作流开始节点',
  'Agent start node',
]);

// Resolve theme safely with a fallback
function resolveTheme(type?: WorkflowNodeData['type'] | string): NodeTheme {
  if (!type) return NODE_THEMES.default;
  const theme = (NODE_THEMES as Record<string, NodeTheme>)[type as string];
  return theme ?? NODE_THEMES.default;
}

// Resolve handle key safely with a fallback (no handles for unknown types)

type HandleKey = string;

function kebabToCamel(input: string): string {
  return input.replace(/-([a-z])/g, (_, c) => c.toUpperCase());
}

function resolveHandleKey(type?: WorkflowNodeData['type'] | string): HandleKey | null {
  if (!type) return null;
  const override = NODE_CONFIG[type as string]?.handleKey;
  if (override) return override;
  // Fallback: derive from type by normalizing kebab-case to camelCase
  return kebabToCamel(String(type));
}

const CustomNode: React.FC<CustomNodeProps> = ({ id, data, selected }) => {
  const t = useT();
  const theme = resolveTheme(data?.type as string);
  const displayDescription =
    data?.type === 'start' &&
    typeof data.desc === 'string' &&
    LOCALIZED_START_NODE_DESCRIPTIONS.has(data.desc)
      ? t('nodes.catalog.start.cardDescription')
      : data.desc;
  const handleKey = resolveHandleKey(data?.type as string);
  const cfg = handleKey
    ? (HandleConfigs as Record<string, typeof HandleConfigs.start>)[handleKey]
    : undefined;
  const status = useWorkflowStore(state => state.runStatusByNodeId[id as string]) as
    | 'running'
    | 'succeeded'
    | 'failed'
    | 'stopped'
    | 'paused'
    | undefined;
  const runtimeLogItems =
    useWorkflowStore(state => state.runtimeLogItemsByNodeId[id as string]) ??
    EMPTY_RUNTIME_LOG_ITEMS;
  const runtimeLogPopoverOpen = useWorkflowStore(
    state => state.runtimeLogPopoverOpenByNodeId[id as string] ?? false
  );
  const setRuntimeLogPopoverOpen = useWorkflowStore.use.setRuntimeLogPopoverOpen();
  const updateNodeInternals = useUpdateNodeInternals();
  // Use cached runnableSets from store with granular selector
  const isComment = useWorkflowStore(state => state.runnableSets.commentSet.has(id as string));
  const hasValidationErrors = useWorkflowStore(
    state => (state.validationResults.errorMap.get(id as string)?.length ?? 0) > 0
  );
  const hasValidationWarnings = useWorkflowStore(
    state => (state.validationResults.warningMap.get(id as string)?.length ?? 0) > 0
  );

  // Recompute handle anchors for dynamic branches of if-else
  const isIfElse = data?.type === 'if-else';
  const ifElseCases = (data as IfElseNodeData | undefined)?.cases;
  const ifElseBranches = (data as IfElseNodeData | undefined)?.targetBranches;
  React.useEffect(() => {
    if (data?.type !== 'if-else') return;
    if (!id) return;
    updateNodeInternals(id as string);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isIfElse, ifElseCases, ifElseBranches, id]);

  // Recompute handle anchors for HTTP node when error_strategy changes to/from 'fail-branch'
  const isHttpRequest = data?.type === 'http-request';
  const httpErrorStrategy = (data as HttpRequestNodeData | undefined)?.error_strategy;
  React.useEffect(() => {
    if (data?.type !== 'http-request') return;
    if (!id) return;
    updateNodeInternals(id as string);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isHttpRequest, httpErrorStrategy, id]);

  // Recompute handle anchors for JSON Parser node when error_strategy changes to/from 'fail-branch'
  const isJsonParser = data?.type === 'json-parser';
  const jsonParserErrorStrategy = (data as JsonParserNodeData | undefined)?.error_strategy;
  React.useEffect(() => {
    if (data?.type !== 'json-parser') return;
    if (!id) return;
    updateNodeInternals(id as string);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isJsonParser, jsonParserErrorStrategy, id]);

  const isApproval = data?.type === 'approval';
  const approvalActions = (data as Extract<WorkflowNodeData, { type: 'approval' }> | undefined)
    ?.approval?.actions;
  React.useEffect(() => {
    if (data?.type !== 'approval') return;
    if (!id) return;
    updateNodeInternals(id as string);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isApproval, approvalActions, id]);

  const isQuestionAnswer = data?.type === 'question-answer';
  const questionAnswerChoices = (data as QuestionAnswerNodeData | undefined)?.choices;
  const questionAnswerChoiceMode = (data as QuestionAnswerNodeData | undefined)?.choice_mode;
  const questionAnswerAnswerType = (data as QuestionAnswerNodeData | undefined)?.answer_type;
  const questionAnswerSelector = (data as QuestionAnswerNodeData | undefined)?.dynamic_choices
    ?.selector;
  React.useEffect(() => {
    if (data?.type !== 'question-answer') return;
    if (!id) return;
    updateNodeInternals(id as string);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    isQuestionAnswer,
    questionAnswerChoices,
    questionAnswerChoiceMode,
    questionAnswerAnswerType,
    questionAnswerSelector,
    id,
  ]);

  const handles = (
    <>
      {/* For if-else, handles are rendered inline within content to align with rows */}
      {data?.type !== 'if-else' && (
        <>
          {(theme.handles === 'target' || theme.handles === 'both') && cfg?.target && (
            <CustomHandle {...cfg.target} />
          )}
          {(theme.handles === 'source' || theme.handles === 'both') && cfg?.source && (
            <CustomHandle {...cfg.source} />
          )}
        </>
      )}
    </>
  );

  const renderContent = () => {
    switch (data.type) {
      case 'start':
        if (!data.variables?.length && !(data as StartNodeInputConfig).input_config?.length) {
          return null;
        }
        return (
          <StartContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'start' }>}
          />
        );
      case 'knowledge-retrieval':
        return (
          <KnowledgeRetrievalContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'knowledge-retrieval' }>}
          />
        );
      case 'llm':
        return (
          <LLMContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'llm' }>}
          />
        );
      case 'http-request':
        return (
          <HttpRequestContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'http-request' }>}
          />
        );
      case 'create-scheduled-task':
        return (
          <CreateScheduledTaskContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'create-scheduled-task' }>}
          />
        );
      case 'notification-sms':
        return (
          <NotificationSMSContent
            data={data as Extract<WorkflowNodeData, { type: 'notification-sms' }>}
          />
        );
      case 'call-database':
        return (
          <CallDatabaseContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'call-database' }>}
          />
        );
      case 'sql-generator':
        return (
          <SqlGeneratorContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'sql-generator' }>}
          />
        );
      case 'tools':
        return null;
      case 'end': {
        const endData = data as Extract<WorkflowNodeData, { type: 'end' }>;
        if (!endData.outputs?.length) return null;
        return <EndContent nodeId={id as string} data={endData} />;
      }
      case 'loop-end':
        return null;
      case 'if-else':
        return (
          <IfElseContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'if-else' }>}
          />
        );
      case 'code':
        return (
          <CodeContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'code' }>}
          />
        );
      case 'assigner':
        return (
          <AssignerContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'assigner' }>}
          />
        );
      case 'iteration':
        return (
          <ContainerContent
            id={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'iteration' }>}
          />
        );
      case 'loop':
        return (
          <ContainerContent
            id={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'loop' }>}
          />
        );
      case 'answer':
        return (
          <AnswerContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'answer' }>}
          />
        );
      case 'document-extractor':
        if (
          !Array.isArray(
            (data as Extract<WorkflowNodeData, { type: 'document-extractor' }>).variable_selector
          ) ||
          (data as Extract<WorkflowNodeData, { type: 'document-extractor' }>).variable_selector
            .length !== 2
        ) {
          return null;
        }
        return (
          <DocumentExtractorContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'document-extractor' }>}
          />
        );
      case 'variable-aggregator':
        return (
          <VariableAggregatorContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'variable-aggregator' }>}
          />
        );
      case 'parameter-extractor':
        return (
          <ParameterExtractorContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'parameter-extractor' }>}
          />
        );
      case 'json-parser':
        return (
          <JsonParserContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'json-parser' }>}
          />
        );
      case 'image-gen':
        return (
          <ImageGenContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'image-gen' }>}
          />
        );
      case 'approval':
        return (
          <ApprovalContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'approval' }>}
          />
        );
      case 'announcement':
        return (
          <AnnouncementContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'announcement' }>}
          />
        );
      case 'question-answer':
        return (
          <QuestionAnswerContent
            nodeId={id as string}
            data={data as Extract<WorkflowNodeData, { type: 'question-answer' }>}
          />
        );
      default:
        return null;
    }
  };

  const rootRef = useRef<HTMLDivElement | null>(null);
  useAutoDimensionsSync(id as string, rootRef.current);
  const setHoveredNodeId = useWorkflowStore.use.setHoveredNodeId();

  // No longer need to compute isComment here as it is selected above
  const validationBadge = hasValidationErrors ? (
    <div className="absolute -top-2 -right-2 z-20 flex h-5 w-5 items-center justify-center rounded-full border border-red-200 bg-red-50 text-red-600 shadow-sm dark:border-red-900/60 dark:bg-red-950 dark:text-red-300">
      <AlertCircle className="h-3.5 w-3.5" strokeWidth={2.5} />
    </div>
  ) : hasValidationWarnings ? (
    <div className="absolute -top-2 -right-2 z-20 flex h-5 w-5 items-center justify-center rounded-full border border-amber-200 bg-amber-50 text-amber-600 shadow-sm dark:border-amber-900/60 dark:bg-amber-950 dark:text-amber-300">
      <TriangleAlert className="h-3.5 w-3.5" strokeWidth={2.5} />
    </div>
  ) : null;

  return (
    <>
      <div
        ref={rootRef}
        className={cn(theme.resizable ? 'h-full w-full' : 'block')}
        onMouseEnter={() => setHoveredNodeId(id as string)}
        onMouseLeave={() => setHoveredNodeId(null)}
      >
        <NodeCard
          selected={selected}
          icon={theme.icon}
          iconUrl={(data as NodeDataWithIconUrl)?.iconUrl}
          title={data.title || ''}
          desc={displayDescription}
          badge={
            data?.type === 'tools'
              ? undefined
              : (() => {
                  let text = theme.badgeText;
                  text = data?.type
                    ? t(`nodes.catalog.${String(data.type)}.label` as Parameters<typeof t>[0])
                    : theme.badgeText;
                  return { text, className: theme.classNames.badge };
                })()
          }
          cardClassName={cn(
            theme.classNames.card,
            hasValidationErrors && 'border-red-300/80 dark:border-red-800/80',
            !hasValidationErrors &&
              hasValidationWarnings &&
              'border-amber-300/80 dark:border-amber-800/80',
            theme.resizable && 'h-full flex flex-col'
          )}
          className={cn(
            theme.classNames.wrapper,
            theme.resizable && 'h-full',
            status === 'running' && 'ring-2 ring-blue-500 animate-pulse',
            status === 'succeeded' && 'ring-2 ring-green-500',
            status === 'failed' && 'ring-2 ring-red-500',
            status === 'stopped' && 'ring-2 ring-gray-500',
            status === 'paused' && 'ring-2 ring-warning',
            isComment && !selected && 'opacity-70'
          )}
          titleClassName={cn(theme.classNames.title)}
          iconBgClassName={cn(
            theme.classNames.iconBg,
            status === 'paused' && 'bg-warning shadow-none'
          )}
          iconClassName={cn(status === 'paused' && 'text-white')}
          descClassName={cn(theme.classNames.desc)}
          contentClassName={cn(theme.classNames.content, theme.resizable && 'grow')}
          after={
            <>
              {handles}
              {validationBadge}
            </>
          }
          runtimeFooter={
            runtimeLogItems.length > 0 ? (
              <NodeRuntimeLogDetails
                nodeId={id as string}
                items={runtimeLogItems}
                open={runtimeLogPopoverOpen}
                onOpenChange={open => setRuntimeLogPopoverOpen(id as string, open)}
              />
            ) : null
          }
          showResizeHandle={theme.resizable}
        >
          {renderContent()}
        </NodeCard>
      </div>
    </>
  );
};

export default React.memo(CustomNode);
