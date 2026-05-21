'use client';

import React from 'react';

import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { AlertCircle, Play } from 'lucide-react';
import { useWorkflowStore } from '../store';
import type { WorkflowDragPreview, WorkflowNodeData } from '../store/type';
import NodeCard from '../nodes/custom/node-card';
import { NODE_THEMES, type NodeTheme } from '../nodes/custom/config';
import OutputVariablesView, { type OutputVariableViewItem } from '../common/output-variables-view';
import StartContent from '../nodes/start';
import KnowledgeRetrievalContent from '../nodes/knowledge-retrieval';
import LLMContent from '../nodes/llm';
import EndContent from '../nodes/end';
import LoopEndContent from '../nodes/loop-end';
import HttpRequestContent from '../nodes/http-request';
import CreateScheduledTaskContent from '../nodes/create-scheduled-task';
import NotificationSMSContent from '../nodes/notification-sms';
import CallDatabaseContent from '../nodes/call-database';
import SqlGeneratorContent from '../nodes/sql-generator';
import { checkValid as checkCodeValid, type CodeNodeData } from '../nodes/code/config';
import AssignerContent from '../nodes/assigner';
import AnswerContent from '../nodes/answer';
import DocumentExtractorContent from '../nodes/document-extractor';
import type { VariableAggregatorNodeData } from '../nodes/variable-aggregator/config';
import ParameterExtractorContent from '../nodes/parameter-extractor';
import JsonParserContent from '../nodes/json-parser';
import ImageGenContent from '../nodes/image-gen';
import type { ApprovalNodeData } from '../nodes/approval/config';
import { normalizeApprovalNodeData } from '../nodes/approval/config';
import type { AnnouncementNodeData } from '../nodes/announcement/config';
import { normalizeAnnouncementNodeData } from '../nodes/announcement/config';
import QuestionAnswerContent from '../nodes/question-answer';

const DEFAULT_PREVIEW_WIDTH = 280;
const DEFAULT_PREVIEW_HEIGHT = 48;
const PREVIEW_NODE_ID = '__workflow-drag-preview__';
const CONTAINER_START_LEFT = 12;
const CONTAINER_START_TOP_IN_BODY = 12;
const CONTAINER_START_SIZE = 40;
const CONTAINER_GUIDE_TOP = 13;
const CONTAINER_GUIDE_LEFT = CONTAINER_START_LEFT + CONTAINER_START_SIZE;
const CONTAINER_GUIDE_WIDTH = 80;

function getThemeKey(type: string): keyof typeof NODE_THEMES {
  return (type === 'tool' ? 'tools' : type) as keyof typeof NODE_THEMES;
}

function resolveTheme(type: string): NodeTheme {
  return NODE_THEMES[getThemeKey(type) as WorkflowNodeData['type']] ?? NODE_THEMES.default;
}

function resolvePreviewMeta(preview: WorkflowDragPreview) {
  const type = preview.data?.type ?? preview.type;
  const theme = resolveTheme(type);
  const config = theme.preview;
  const kind = config?.kind ?? 'card';
  const fixedHeight = kind === 'container' || kind === 'branch' || theme.autoHeight === false;
  const height = config?.height ?? theme.height ?? DEFAULT_PREVIEW_HEIGHT;
  return {
    theme,
    kind,
    width: config?.width ?? theme.width ?? DEFAULT_PREVIEW_WIDTH,
    height,
    fixedHeight,
  };
}

function ContainerStartPreview({ type }: { type?: WorkflowNodeData['type'] }) {
  const isLoop = type === 'loop';

  return (
    <div
      className="absolute flex size-10 items-center justify-center rounded-xl border border-slate-200/50 bg-white/70 shadow-sm backdrop-blur-md transition-all dark:border-slate-800/50 dark:bg-slate-900/70"
      style={{ left: CONTAINER_START_LEFT, top: CONTAINER_START_TOP_IN_BODY }}
    >
      <div
        className={cn(
          'flex size-6 items-center justify-center rounded-lg shadow-[0_0_12px_rgba(99,102,241,0.4)]',
          isLoop ? 'bg-blue-500/90' : 'bg-indigo-500/90'
        )}
      >
        <Play className="size-4 fill-white text-white" />
      </div>
      <span className="absolute right-0 top-1/2 h-3.5 w-1 translate-x-[80%] -translate-y-1/2 rounded-[1px] bg-blue-400 dark:bg-blue-300" />
    </div>
  );
}

function ContainerPreviewBody({
  type,
  addNodeLabel,
}: {
  type?: WorkflowNodeData['type'];
  addNodeLabel: string;
}) {
  return (
    <div className="relative h-full overflow-hidden rounded-b-[11px] border-t border-slate-200/60 bg-slate-50/50 shadow-[inset_0_2px_4px_rgba(0,0,0,0.02)] dark:border-slate-800/60 dark:bg-slate-900/50">
      <div className="absolute inset-0 bg-[linear-gradient(to_right,#e5e7eb_1px,transparent_1px),linear-gradient(to_bottom,#e5e7eb_1px,transparent_1px)] bg-[size:20px_20px] dark:opacity-20" />
      <ContainerStartPreview type={type} />
      <div
        className="absolute flex items-center overflow-visible"
        style={{ left: CONTAINER_GUIDE_LEFT, top: CONTAINER_GUIDE_TOP }}
      >
        <svg className="h-0.5 shrink-0 overflow-visible" style={{ width: CONTAINER_GUIDE_WIDTH }}>
          <path
            d="M 2 1 L 80 1"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeDasharray="5,5"
            className="text-[var(--edge-default)] opacity-60 dark:opacity-40"
          />
        </svg>
        <div className="relative ml-[-2px] flex h-9 items-center gap-2 rounded-full border-transparent bg-indigo-600 px-4 text-sm font-semibold tracking-wide text-white shadow-[0_4px_12px_-2px_rgba(79,70,229,0.4)]">
          <span className="flex size-5 items-center justify-center rounded-full bg-white/20 text-base leading-none">
            +
          </span>
          <span>{addNodeLabel}</span>
        </div>
      </div>
    </div>
  );
}

function BranchPreviewBody({
  data,
  unconfiguredLabel,
}: {
  data?: Extract<WorkflowNodeData, { type: 'if-else' }>;
  unconfiguredLabel: string;
}) {
  const cases = Array.isArray(data?.cases) ? data.cases : [];

  return (
    <div className="mt-1">
      {cases.map((caseItem, index) => (
        <div key={caseItem.case_id || index} className="mt-2 border-t pt-2 first:mt-0">
          <div className="flex items-center justify-between text-xs font-medium text-primary">
            <span>{cases.length > 1 ? `CASE ${index + 1}` : ''}</span>
            <span className="text-[10px] font-bold tracking-wider text-slate-400">
              {index === 0 ? 'IF' : 'ELIF'}
            </span>
          </div>
          <div className="mt-1 text-xs text-muted-foreground">{unconfiguredLabel}</div>
        </div>
      ))}
      <div className="mt-2 border-t border-slate-200/50 pt-2 dark:border-slate-800/50">
        <div className="text-right text-[10px] font-bold tracking-wider text-slate-400">ELSE</div>
      </div>
    </div>
  );
}

function ApprovalPreviewBody({ data }: { data: ApprovalNodeData }) {
  const t = useT('nodes');
  const normalized = normalizeApprovalNodeData(data);
  const content = normalized.approval.content.trim();

  return (
    <div className="mt-1 space-y-2">
      <div className="max-h-[160px] min-h-8 overflow-hidden rounded-md bg-muted/70 p-1.5 text-xs leading-relaxed text-secondary-foreground break-words whitespace-pre-wrap">
        {content || (
          <span className="text-muted-foreground">{t('approval.preview.emptyContent')}</span>
        )}
      </div>
      <div>
        {normalized.approval.actions.map((action, index) => (
          <div key={`${action.id}-${index}`} className="border-t py-2 text-xs">
            {action.id ? (
              <div className="truncate text-end font-mono font-semibold">{action.id}</div>
            ) : null}
            <div className="truncate font-medium">
              {action.label.trim() ? action.label : t('approval.preview.emptyAction')}
            </div>
          </div>
        ))}
        <div className="flex items-center justify-end border-t py-2 text-xs">
          <span className="font-mono font-semibold">{t('approval.preview.timeout')}</span>
        </div>
      </div>
    </div>
  );
}

function AnnouncementPreviewBody({ data }: { data: AnnouncementNodeData }) {
  const t = useT('nodes');
  const normalized = normalizeAnnouncementNodeData(data);
  const title = normalized.announcement.title.trim();
  const content = normalized.announcement.content.trim();
  const expiration = t('announcement.preview.expirationValue', {
    duration: normalized.timeout.duration,
    unit: t(`announcement.timeout.${normalized.timeout.unit}`),
  });

  return (
    <div className="mt-1 space-y-2">
      <div className="space-y-1 rounded-md border bg-background/80 px-2 py-1.5">
        <div className="text-[10px] font-medium uppercase text-muted-foreground">
          {t('announcement.preview.title')}
        </div>
        <div className="truncate text-xs font-medium">
          {title || t('announcement.preview.emptyTitle')}
        </div>
      </div>
      <div className="space-y-1 rounded-md border bg-background/80 px-2 py-1.5">
        <div className="text-[10px] font-medium uppercase text-muted-foreground">
          {t('announcement.preview.content')}
        </div>
        <div className="line-clamp-2 text-xs leading-relaxed text-foreground break-words whitespace-pre-wrap">
          {content || t('announcement.preview.emptyContent')}
        </div>
      </div>
      <div className="space-y-1 rounded-md border bg-background/80 px-2 py-1.5">
        <div className="text-[10px] font-medium uppercase text-muted-foreground">
          {t('announcement.preview.expiration')}
        </div>
        <div className="truncate text-xs font-medium text-muted-foreground">{expiration}</div>
      </div>
    </div>
  );
}

function CodePreviewBody({ data }: { data: CodeNodeData }) {
  const t = useT('nodes');
  const outputKeys = data.outputKeyOrders?.length
    ? data.outputKeyOrders
    : Object.keys(data.outputs || {});
  const outputVariables = outputKeys.reduce<OutputVariableViewItem[]>((items, key) => {
    const output = data.outputs?.[key];
    if (!output) return items;

    items.push({
      name: key,
      type: output.type,
      description: '',
    });
    return items;
  }, []);
  const validation = checkCodeValid(data, { nodes: [] });
  const firstError = validation.errors[0];

  return (
    <>
      <OutputVariablesView
        variant="compact"
        title={t('common.outputVariables')}
        variables={outputVariables}
        maxItems={3}
      />

      {!validation.isValid && firstError ? (
        <div className="mt-2 space-y-1">
          <div className="flex items-center gap-1 text-xs text-red-600">
            <AlertCircle className="size-3" />
            {t(`${firstError.code}` as never, firstError.params as never)}
          </div>
        </div>
      ) : null}
    </>
  );
}

function VariableAggregatorPreviewBody({ data }: { data: VariableAggregatorNodeData }) {
  const t = useT('nodes');
  const groupEnabled = Boolean(data.advanced_settings?.group_enabled);
  const variables = Array.isArray(data.variables) ? data.variables : [];
  const groups = Array.isArray(data.advanced_settings?.groups) ? data.advanced_settings.groups : [];
  const visibleVariables = variables.slice(0, 3);
  const visibleGroups = groups.slice(0, 2);
  const outputVariables = groupEnabled
    ? groups
        .filter(group => group.group_name?.trim())
        .map(group => ({
          name: group.group_name,
          type: 'object',
          description: '',
        }))
    : [
        {
          name: 'output',
          type: data.output_type || 'string',
          description: '',
        },
      ];

  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-primary">
        {groupEnabled
          ? t('variableAggregator.content.groupTitle')
          : t('variableAggregator.content.normalTitle')}
      </div>

      {!groupEnabled ? (
        visibleVariables.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {visibleVariables.map((selector, index) => (
              <span
                key={`${selector[0]}::${selector[1]}::${index}`}
                className="rounded-md border bg-background/70 px-2 py-1 text-xs text-muted-foreground"
              >
                {selector.join('.')}
              </span>
            ))}
            {variables.length > visibleVariables.length ? (
              <span className="text-[10px] text-muted-foreground">
                +{variables.length - visibleVariables.length}
              </span>
            ) : null}
          </div>
        ) : (
          <div className="text-xs text-muted-foreground">
            {t('variableAggregator.manager.noVariables')}
          </div>
        )
      ) : (
        <div className="space-y-1">
          {visibleGroups.map(group => {
            const variableCount = Array.isArray(group.variables) ? group.variables.length : 0;
            return (
              <div
                key={group.groupId}
                className="flex items-center justify-between gap-2 rounded-md border bg-background/70 px-2 py-1 text-xs"
              >
                <span className="truncate font-medium">
                  {group.group_name || t('variableAggregator.content.groupTitle')}
                </span>
                <span className="shrink-0 text-muted-foreground">{variableCount}</span>
              </div>
            );
          })}
          {groups.length > visibleGroups.length ? (
            <div className="text-[11px] text-muted-foreground">
              +{groups.length - visibleGroups.length}
            </div>
          ) : null}
        </div>
      )}

      <OutputVariablesView variant="compact" variables={outputVariables} maxItems={2} />
    </div>
  );
}

function InitialDataPreviewContent({
  data,
  kind,
  addNodeLabel,
  unconfiguredLabel,
}: {
  data?: WorkflowNodeData;
  kind: 'card' | 'container' | 'branch';
  addNodeLabel: string;
  unconfiguredLabel: string;
}) {
  if (!data) {
    return kind === 'container' ? <ContainerPreviewBody addNodeLabel={addNodeLabel} /> : null;
  }

  switch (data.type) {
    case 'start':
      if (!data.variables?.length) return null;
      return <StartContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'knowledge-retrieval':
      return <KnowledgeRetrievalContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'llm':
      return <LLMContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'http-request':
      return <HttpRequestContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'create-scheduled-task':
      return <CreateScheduledTaskContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'notification-sms':
      return <NotificationSMSContent data={data} />;
    case 'call-database':
      return <CallDatabaseContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'sql-generator':
      return <SqlGeneratorContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'tools':
      return null;
    case 'end':
      return <EndContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'loop-end':
      return <LoopEndContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'if-else':
      return <BranchPreviewBody data={data} unconfiguredLabel={unconfiguredLabel} />;
    case 'code':
      return <CodePreviewBody data={data} />;
    case 'assigner':
      return <AssignerContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'iteration':
    case 'loop':
      return <ContainerPreviewBody type={data.type} addNodeLabel={addNodeLabel} />;
    case 'answer':
      return <AnswerContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'document-extractor':
      if (!Array.isArray(data.variable_selector) || data.variable_selector.length !== 2) {
        return null;
      }
      return <DocumentExtractorContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'variable-aggregator':
      return <VariableAggregatorPreviewBody data={data} />;
    case 'parameter-extractor':
      return <ParameterExtractorContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'json-parser':
      return <JsonParserContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'image-gen':
      return <ImageGenContent nodeId={PREVIEW_NODE_ID} data={data} />;
    case 'approval':
      return <ApprovalPreviewBody data={data} />;
    case 'announcement':
      return <AnnouncementPreviewBody data={data} />;
    case 'question-answer':
      return <QuestionAnswerContent nodeId={PREVIEW_NODE_ID} data={data} />;
    default:
      return null;
  }
}

function hasInitialDataPreviewBody(
  data: WorkflowNodeData | undefined,
  kind: 'card' | 'container' | 'branch'
) {
  if (kind === 'container') return true;
  if (!data) return false;

  if (data.type === 'start') return Boolean(data.variables?.length);
  if (data.type === 'tools') return false;
  if (data.type === 'document-extractor') {
    return Array.isArray(data.variable_selector) && data.variable_selector.length === 2;
  }
  if (data.type === 'end' || data.type === 'loop-end') return false;
  return true;
}

/**
 * @component WorkflowNodeDragPreview
 * @category Feature
 * @status Stable
 * @description Fixed-position workflow node preview rendered during left-panel drag creation.
 * @usage Render once inside the workflow canvas host to mirror the eventual dropped node.
 */
export function WorkflowNodeDragPreview() {
  const t = useT();
  const tn = useT('nodes');
  const preview = useWorkflowStore.use.draggingNodePreview();
  const zoom = useWorkflowStore.use.viewport().zoom;

  if (!preview) return null;

  const { theme, kind, width, height, fixedHeight } = resolvePreviewMeta(preview);
  const data = preview.data;
  const left = preview.client.x - preview.anchor.x * zoom;
  const top = preview.client.y - preview.anchor.y * zoom;
  const previewType = data?.type ?? preview.type;
  const hasBody = hasInitialDataPreviewBody(data, kind);

  return (
    <div
      aria-hidden="true"
      className="pointer-events-none fixed z-[1000] opacity-90 drop-shadow-2xl"
      style={{
        left,
        top,
        width,
        ...(fixedHeight ? { height } : { minHeight: height }),
        transform: `scale(${zoom})`,
        transformOrigin: 'top left',
      }}
    >
      <NodeCard
        icon={theme.icon}
        iconUrl={preview.iconUrl}
        title={data?.title || preview.title}
        desc={data?.desc}
        badge={
          previewType === 'tools' || preview.type === 'tool'
            ? undefined
            : {
                text: t(`nodes.catalog.${String(previewType)}.label` as Parameters<typeof t>[0]),
                className: theme.classNames.badge,
              }
        }
        className={cn(theme.classNames.wrapper, fixedHeight && 'h-full')}
        cardClassName={cn(
          theme.classNames.card,
          fixedHeight && 'h-full',
          'border-primary/25 bg-white/90 shadow-xl ring-1 ring-primary/20 dark:bg-slate-900/90'
        )}
        titleClassName={theme.classNames.title}
        iconBgClassName={theme.classNames.iconBg}
        descClassName={theme.classNames.desc}
        contentClassName={cn(
          theme.classNames.content,
          kind === 'container' && 'grow p-0',
          fixedHeight && kind !== 'container' && 'grow'
        )}
      >
        {hasBody ? (
          <InitialDataPreviewContent
            data={data}
            kind={kind}
            addNodeLabel={t('agents.workflow.addNode')}
            unconfiguredLabel={tn('ifElse.fields.unconfigured')}
          />
        ) : null}
      </NodeCard>
    </div>
  );
}
