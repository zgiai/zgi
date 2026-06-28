'use client';

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, CheckCircle2, ChevronDown, ExternalLink, Loader2 } from 'lucide-react';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Button } from '@/components/ui/button';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { useT } from '@/i18n/translations';
import type { ScopedTranslations } from '@/i18n/translations';
import { useLocale } from '@/hooks/use-locale';
import { cn } from '@/lib/utils';
import { formatMs } from '@/utils/format';
import type {
  AIChatMessage,
  AIChatSkillInvocation,
  AIChatToolGovernanceAssetRef,
} from '@/services/types/aichat';
import type { AIChatAgenticTimelineItem } from '@/components/chat/controllers/aichat';
import { isPendingToolGovernanceInvocation } from '@/components/chat/controllers/aichat/governance';
import {
  getAIChatSkillResultDisplay,
  getAIChatSkillToolDisplayName,
  getAIChatUserMemoryMutationTitle,
  getFallbackAIChatSkillDisplayInfo,
  type AIChatSkillDisplayInfo,
  type AIChatSkillDisplayMap,
} from '@/components/chat/variants/aichat/skill-display';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';
import { AIChatSkillResultSummary } from '@/components/chat/variants/aichat/skill-result-summary';
import {
  ToolGovernanceDecisionCard,
  publishToolGovernancePendingApproval,
  useToolGovernancePendingApprovalScope,
  type ToolGovernanceDecisionAction,
  type ToolGovernanceDisplayAsset,
  type ToolGovernanceDisplayRow,
  type ToolGovernancePendingApproval,
} from '@/components/chat/variants/aichat/tool-governance-decision-card';
import WorkflowRunMonitor from '@/components/chat/ui/workflow-run-monitor';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';

type TimelineTone = 'running' | 'success' | 'error';
type TimelineDebugLabel = keyof typeof TIMELINE_DEBUG_LABEL_KEYS;
type GovernanceFieldLabel = keyof typeof GOVERNANCE_FIELD_LABEL_KEYS;
export type WebappTranslator = ScopedTranslations<'webapp'>;

const TIMELINE_DEBUG_LABEL_KEYS = {
  kind: 'consoleChat.skills.trace.debug.kind',
  skillId: 'consoleChat.skills.trace.debug.skillId',
  toolName: 'consoleChat.skills.trace.debug.toolName',
  path: 'consoleChat.skills.trace.debug.path',
  duration: 'consoleChat.skills.trace.debug.duration',
  arguments: 'consoleChat.skills.trace.debug.arguments',
  message: 'consoleChat.skills.trace.debug.message',
  error: 'consoleChat.skills.trace.debug.error',
} as const;

const GOVERNANCE_FIELD_LABEL_KEYS = {
  intent: 'consoleChat.governance.fields.intent',
  assetCount: 'consoleChat.governance.fields.assetCount',
  reversible: 'consoleChat.governance.fields.reversible',
  bulkSensitive: 'consoleChat.governance.fields.bulkSensitive',
  externalSideEffect: 'consoleChat.governance.fields.externalSideEffect',
  permissionTier: 'consoleChat.governance.fields.permissionTier',
  decision: 'consoleChat.governance.fields.decision',
  riskLevel: 'consoleChat.governance.fields.riskLevel',
  effect: 'consoleChat.governance.fields.effect',
  assetType: 'consoleChat.governance.fields.assetType',
  executionStatus: 'consoleChat.governance.fields.executionStatus',
  executionError: 'consoleChat.governance.fields.executionError',
  executionDuration: 'consoleChat.governance.fields.executionDuration',
  matchedGrant: 'consoleChat.governance.fields.matchedGrant',
  modelFeedback: 'consoleChat.governance.fields.modelFeedback',
  approvalResult: 'consoleChat.governance.fields.approvalResult',
  sessionGrant: 'consoleChat.governance.fields.sessionGrant',
  approvalEvent: 'consoleChat.governance.fields.approvalEvent',
} as const;

const assistantMarkdownClassName =
  'prose prose-sm min-w-0 max-w-full overflow-hidden dark:prose-invert sm:pr-4 md:pr-6 lg:pr-8 xl:pr-9';

const TRANSIENT_PROGRESS_TEXT_KEYS = [
  'consoleChat.skills.agentic.thinking',
  'consoleChat.skills.agentic.organizing',
  'consoleChat.skills.agentic.preparing',
  'consoleChat.skills.agentic.checkingTools',
] as const;

const INTERNAL_DISPLAY_FIELD_KEYS = new Set([
  'id',
  'file_id',
  'file_ids',
  'upload_file_id',
  'upload_file_ids',
  'workspace_id',
  'workspace_ids',
  'organization_id',
  'organization_ids',
  'conversation_id',
  'message_id',
  'correlation_id',
  'approved_by_correlation_id',
  'source_id',
  'runtime_id',
  'deleted_count',
]);

const INTERNAL_DISPLAY_FIELD_NAME_PATTERN =
  /\b(?:file_ids?|fileIds?|upload_file_ids?|uploadFileIds?|workspace_ids?|workspaceIds?|organization_ids?|organizationIds?|conversation_id|conversationId|message_id|messageId|correlation_id|correlationId|approved_by_correlation_id|approvedByCorrelationId|source_id|sourceId|runtime_id|runtimeId|deleted_count|deletedCount)\b/i;

const UUID_DISPLAY_PATTERN =
  /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi;

const OPAQUE_INLINE_ID_PATTERN =
  /\b(?:file|upload[-_]?file|asset|workspace|ws)[-_](?=[a-z0-9_-]*\d)[a-z0-9][a-z0-9_-]*\b/gi;

interface AgentBindingDisplaySummary {
  action: string;
  kind: string;
  count: number;
  names: string[];
  agentName: string | null;
  changeCount: number;
}

interface AIChatAgenticTimelineProps {
  timeline: AIChatAgenticTimelineItem[];
  skillDisplayById: AIChatSkillDisplayMap;
  defaultOpen?: boolean;
  showMemoryKey?: boolean;
  showSkillEventDetails?: boolean;
  enableToolGovernanceApprovals?: boolean;
  messageStatus?: AIChatMessage['status'];
  onToolGovernanceDecision?: (
    payload: AIChatToolGovernanceDecisionSubmitPayload
  ) => void | Promise<void>;
}

export interface AIChatToolGovernanceDecisionSubmitPayload {
  conversationId: string;
  messageId: string;
  correlationId: string;
  action: 'approve' | 'reject';
  rememberForSession?: boolean;
  reason?: string;
}

interface SkillTimelineViewModel {
  item: Extract<AIChatAgenticTimelineItem, { type: 'skill_event' }>;
  title: string;
  detail?: string;
  skill: AIChatSkillDisplayInfo;
  tone: TimelineTone;
}

type MemoryTimelineItem = Extract<AIChatAgenticTimelineItem, { type: 'memory_event' }>;
type GovernanceTimelineItem = Extract<
  AIChatAgenticTimelineItem,
  { type: 'tool_governance_decision' }
>;
type WorkflowTimelineItem = Extract<AIChatAgenticTimelineItem, { type: 'workflow_run' }>;

interface ToolGovernanceDecisionViewModel {
  title: string;
  toolLabel: string | null;
  actionSentence: string;
  notice: string | null;
  reason: string;
  assets: ToolGovernanceDisplayAsset[];
  summaryRows: ToolGovernanceDisplayRow[];
  details: ToolGovernanceDisplayRow[];
  needsApproval: boolean;
  approvalStatus: string;
  isHighImpact: boolean;
  isAllowed: boolean;
  canSubmit: boolean;
  riskLabel: string | null;
  permissionLabel: string | null;
  pendingApprovalId: string;
  onSubmitDecision: (
    action: ToolGovernanceDecisionAction,
    rememberForSession: boolean
  ) => void | Promise<void>;
}

function getInvocationTone(invocation: AIChatSkillInvocation): TimelineTone {
  if (invocation.status === 'loading' || invocation.status === 'running') return 'running';
  if (invocation.kind === 'guardrail') return 'success';
  if (invocation.status === 'error' || invocation.status === 'blocked') return 'error';
  return 'success';
}

function getStatusIcon(tone: TimelineTone) {
  if (tone === 'running') return <Loader2 className="size-3.5 animate-spin" />;
  if (tone === 'error') return <AlertCircle className="size-3.5" />;
  return <CheckCircle2 className="size-3.5 text-emerald-600" />;
}

function getDurationText(durationMs: number | undefined): string | null {
  if (typeof durationMs !== 'number' || !Number.isFinite(durationMs)) return null;
  if (durationMs < 0) return null;
  if (durationMs === 0) return '<1ms';
  return formatMs(durationMs);
}

function normalizeDisplayFieldKey(key: string): string {
  return key
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replace(/[-\s]+/g, '_')
    .toLowerCase();
}

function isInternalDisplayFieldKey(key: string): boolean {
  const normalized = normalizeDisplayFieldKey(key);
  return (
    INTERNAL_DISPLAY_FIELD_KEYS.has(normalized) ||
    normalized.endsWith('_id') ||
    normalized.endsWith('_ids') ||
    normalized.endsWith('_uuid') ||
    normalized.endsWith('_uuids')
  );
}

function sanitizeDisplayString(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed || INTERNAL_DISPLAY_FIELD_NAME_PATTERN.test(trimmed)) return null;
  if (looksLikeOpaqueAssetID(trimmed)) return null;

  const sanitized = trimmed
    .replace(UUID_DISPLAY_PATTERN, '')
    .replace(OPAQUE_INLINE_ID_PATTERN, '')
    .replace(/\s{2,}/g, ' ')
    .trim();
  if (!sanitized) return null;
  return sanitized;
}

function sanitizeDisplayPayload(value: unknown): unknown {
  if (value === undefined || value === null || value === '') return null;
  if (typeof value === 'string') return sanitizeDisplayString(value);
  if (typeof value === 'number' || typeof value === 'boolean') return value;
  if (Array.isArray(value)) {
    const items = value
      .map(item => sanitizeDisplayPayload(item))
      .filter(item => item !== null && item !== undefined);
    return items.length > 0 ? items : null;
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>).flatMap(([key, rawValue]) => {
      if (isInternalDisplayFieldKey(key)) return [];
      const sanitized = sanitizeDisplayPayload(rawValue);
      return sanitized === null || sanitized === undefined ? [] : [[key, sanitized] as const];
    });
    return entries.length > 0 ? Object.fromEntries(entries) : null;
  }
  return String(value);
}

function formatDebugValue(value: unknown): string | null {
  const sanitized = sanitizeDisplayPayload(value);
  if (sanitized === undefined || sanitized === null || sanitized === '') return null;
  if (typeof sanitized === 'string') return sanitized;
  if (typeof sanitized === 'number' || typeof sanitized === 'boolean') return String(sanitized);
  try {
    return JSON.stringify(sanitized);
  } catch {
    return String(sanitized);
  }
}

function sanitizeTimelineResultForDisplay(
  result?: Record<string, unknown> | null
): Record<string, unknown> | null {
  const sanitized = sanitizeDisplayPayload(result);
  return governanceRecord(sanitized);
}

function stringListFromUnknown(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value
      .map(item => governanceStringValue(item))
      .filter((item): item is string => Boolean(item))
      .filter(name => !looksLikeOpaqueAssetID(name));
  }
  const single = governanceStringValue(value);
  return single && !looksLikeOpaqueAssetID(single) ? [single] : [];
}

function firstRecordFromUnknown(value: unknown): Record<string, unknown> | null {
  if (Array.isArray(value)) {
    return (
      value
        .map(item => governanceRecord(item))
        .find((item): item is Record<string, unknown> => Boolean(item)) ?? null
    );
  }
  return governanceRecord(value);
}

function agentBindingDisplaySummaryFromRecord(
  result: Record<string, unknown>,
  toolName?: string | null
): AgentBindingDisplaySummary | null {
  const normalizedTool = normalizeGovernanceToolName(toolName);
  const primaryChange =
    firstRecordFromUnknown(result.config_changes) ?? firstRecordFromUnknown(result.binding_changes);
  const hasTopLevelSummary =
    Boolean(governanceRecordString(result, ['binding_kind'])) ||
    Boolean(governanceRecordString(result, ['change_action']));
  const source = hasTopLevelSummary ? result : primaryChange ?? result;
  const kind =
    governanceRecordString(source, ['binding_kind']) ??
    agentBindingKindFromToolName(normalizedTool);
  if (!kind) return null;

  const action =
    governanceRecordString(source, ['change_action', 'action']) ??
    agentBindingActionFallbackFromToolName(normalizedTool);
  const addedCount = governanceRecordNumber(source, ['added_resource_count', 'added_count']) ?? 0;
  const removedCount =
    governanceRecordNumber(source, ['removed_resource_count', 'removed_count']) ?? 0;
  const finalCount = governanceRecordNumber(source, ['final_resource_count', 'final_count']) ?? 0;
  const explicitCount = governanceRecordNumber(source, ['resource_count', 'asset_count']);
  const resolvedAction = action || (removedCount > 0 && addedCount === 0 ? 'unbind' : 'bind');
  const names =
    resolvedAction === 'unbind'
      ? stringListFromUnknown(source.removed_resource_names)
      : resolvedAction === 'bind'
        ? stringListFromUnknown(source.added_resource_names)
        : stringListFromUnknown(source.resource_names);
  const fallbackNames = stringListFromUnknown(source.resource_names);
  const count =
    explicitCount ??
    (resolvedAction === 'unbind'
      ? removedCount
      : resolvedAction === 'bind'
        ? addedCount
        : Math.max(addedCount + removedCount, finalCount, fallbackNames.length));
  const agent = governanceRecord(result.agent);
  const agentName =
    governanceRecordString(result, ['agent_name']) ??
    governanceRecordString(agent, ['name', 'agent_name']);
  const changeCount = Array.isArray(result.config_changes)
    ? result.config_changes.length
    : Array.isArray(result.binding_changes)
      ? result.binding_changes.length
      : 1;
  return {
    action: resolvedAction,
    kind,
    count: Math.max(count, names.length, fallbackNames.length, 1),
    names: names.length > 0 ? names : fallbackNames,
    agentName,
    changeCount: Math.max(changeCount, 1),
  };
}

function agentBindingKindFromToolName(toolName: string): string | null {
  switch (toolName) {
    case 'replace_agent_skill_bindings':
    case 'agent.replace_skill_bindings':
      return 'agent_skill';
    case 'replace_agent_knowledge_bindings':
    case 'agent.replace_knowledge_bindings':
      return 'knowledge_base';
    case 'replace_agent_database_bindings':
    case 'agent.replace_database_bindings':
      return 'database_table';
    case 'replace_agent_workflow_bindings':
    case 'agent.replace_workflow_bindings':
      return 'workflow';
    default:
      return null;
  }
}

function agentBindingKindFromAssetType(assetType: string): string | null {
  switch (normalizeGovernanceAssetType(assetType)) {
    case 'agent_skill':
    case 'skill':
      return 'agent_skill';
    case 'knowledge_base':
    case 'knowledge':
    case 'dataset':
      return 'knowledge_base';
    case 'database_table':
    case 'table':
      return 'database_table';
    case 'workflow':
      return 'workflow';
    default:
      return null;
  }
}

function agentBindingActionFallbackFromToolName(toolName: string): string | null {
  if (!AGENT_BINDING_TOOL_NAMES.has(toolName)) return null;
  return 'update';
}

function formatAgentBindingNames(
  names: string[],
  count: number,
  locale: string,
  t: WebappTranslator
) {
  return names.length > 0
    ? names.slice(0, 6).join(locale === 'en-US' ? ', ' : '、')
    : t('consoleChat.governance.approvalPanel.targetCountFallback', { count });
}

function agentBindingDisplayText(
  summary: AgentBindingDisplaySummary,
  fallbackAgentName: string,
  locale: string,
  t: WebappTranslator
): string | null {
  const agent = summary.agentName ?? fallbackAgentName;
  const count = Math.max(summary.count, 1);
  const names = formatAgentBindingNames(summary.names, count, locale, t);
  if (summary.kind === 'multiple') {
    if (summary.action === 'unbind') {
      return t('consoleChat.governance.approvalPanel.agentUnbindResources', {
        agent,
        count,
      });
    }
    if (summary.action === 'bind') {
      return t('consoleChat.governance.approvalPanel.agentBindResources', {
        agent,
        count,
      });
    }
    return t('consoleChat.governance.approvalPanel.agentUpdateConfigChanges', {
      agent,
      count: summary.changeCount,
    });
  }
  const key =
    summary.action === 'unbind'
      ? agentBindingUnbindTranslationKey(summary.kind)
      : summary.action === 'bind'
        ? agentBindingBindTranslationKey(summary.kind)
        : agentBindingUpdateTranslationKey(summary.kind);
  if (!key) {
    return t('consoleChat.governance.approvalPanel.agentUpdateConfigChanges', {
      agent,
      count: summary.changeCount,
    });
  }
  return t(key, { agent, count, names });
}

function agentOwnerNameFromAssets(value: unknown): string | null {
  return (
    governanceAssetsFromUnknown(value)
      .filter(asset => isBindingOwnerAsset(asset))
      .map(asset => governanceAssetSpecificDisplayName(asset))
      .find((name): name is string => Boolean(name)) ?? null
  );
}

function agentOwnerNameFromSkillInvocation(invocation: AIChatSkillInvocation): string | null {
  const governance = invocation.governance;
  const audit = governanceRecord(invocation.asset_operation_audit ?? governance?.asset_operation_audit);
  const approvalEvent = governanceRecord(governance?.approval_event);
  return (
    agentOwnerNameFromAssets(audit?.assets) ??
    agentOwnerNameFromAssets(approvalEvent?.assets) ??
    agentOwnerNameFromAssets(governance?.assets)
  );
}

function agentBindingBindTranslationKey(kind: string) {
  switch (kind) {
    case 'agent_skill':
      return 'consoleChat.governance.approvalPanel.agentBindSkills' as const;
    case 'knowledge_base':
      return 'consoleChat.governance.approvalPanel.agentBindKnowledge' as const;
    case 'database_table':
      return 'consoleChat.governance.approvalPanel.agentBindDatabaseTables' as const;
    case 'workflow':
      return 'consoleChat.governance.approvalPanel.agentBindWorkflows' as const;
    default:
      return null;
  }
}

function agentBindingUnbindTranslationKey(kind: string) {
  switch (kind) {
    case 'agent_skill':
      return 'consoleChat.governance.approvalPanel.agentUnbindSkills' as const;
    case 'knowledge_base':
      return 'consoleChat.governance.approvalPanel.agentUnbindKnowledge' as const;
    case 'database_table':
      return 'consoleChat.governance.approvalPanel.agentUnbindDatabaseTables' as const;
    case 'workflow':
      return 'consoleChat.governance.approvalPanel.agentUnbindWorkflows' as const;
    default:
      return null;
  }
}

function agentBindingUpdateTranslationKey(kind: string) {
  switch (kind) {
    case 'agent_skill':
      return 'consoleChat.governance.approvalPanel.agentUpdateSkills' as const;
    case 'knowledge_base':
      return 'consoleChat.governance.approvalPanel.agentUpdateKnowledge' as const;
    case 'database_table':
      return 'consoleChat.governance.approvalPanel.agentUpdateDatabaseTables' as const;
    case 'workflow':
      return 'consoleChat.governance.approvalPanel.agentUpdateWorkflows' as const;
    default:
      return null;
  }
}

function timelineDebugRows(invocation: AIChatSkillInvocation, locale: string) {
  return [
    ['kind', invocation.kind],
    ['skillId', invocation.skill_id],
    ['toolName', getAIChatSkillToolDisplayName(invocation.skill_id, invocation.tool_name, locale)],
    ['path', invocation.path],
    ['duration', getDurationText(invocation.duration_ms)],
    ['arguments', invocation.arguments],
    ['message', invocation.message],
    ['error', invocation.error],
  ] as const satisfies ReadonlyArray<readonly [TimelineDebugLabel, unknown]>;
}

function buildSkillTitle(
  invocation: AIChatSkillInvocation,
  skill: AIChatSkillDisplayInfo,
  tone: TimelineTone,
  locale: string,
  t: WebappTranslator
): string {
  const routeTarget = routeNavigationDisplayTarget(invocation, locale);
  if (routeTarget) {
    const alreadyLoaded = routeNavigationAlreadyLoaded(invocation);
    if (alreadyLoaded && tone !== 'running' && tone !== 'error') {
      return locale === 'en-US' ? `Already on ${routeTarget}` : `已在 ${routeTarget}`;
    }
    if (tone === 'running') {
      return locale === 'en-US' ? `Navigating to ${routeTarget}` : `正在导航到 ${routeTarget}`;
    }
    if (tone === 'error') {
      return locale === 'en-US' ? `Failed to navigate to ${routeTarget}` : `导航到 ${routeTarget} 失败`;
    }
    return locale === 'en-US' ? `Navigated to ${routeTarget}` : `已导航到 ${routeTarget}`;
  }

  const toolName =
    getAIChatSkillToolDisplayName(invocation.skill_id, invocation.tool_name, locale) ||
    invocation.path ||
    t('consoleChat.skills.trace.unknownTool');

  if (invocation.kind === 'skill_load') {
    if (tone === 'running') {
      return t('consoleChat.skills.agentic.loadingSkill', { skill: skill.label });
    }
    if (tone === 'error') return t('consoleChat.skills.agentic.loadFailed', { skill: skill.label });
    return t('consoleChat.skills.agentic.loadedSkill', { skill: skill.label });
  }

  if (invocation.kind === 'reference_read') {
    return t('consoleChat.skills.agentic.referenceRead', {
      skill: skill.label,
      path: invocation.path || t('consoleChat.skills.trace.unknownReference'),
    });
  }

  if (invocation.kind === 'guardrail') {
    return t('consoleChat.skills.agentic.strategyAdjusted');
  }

  if (tone === 'success' && normalizeGovernanceToolName(invocation.skill_id) === 'agent-management') {
    const result = invocationRecord(invocation.result);
    const summary = agentBindingDisplaySummaryFromRecord(result, invocation.tool_name);
    const fallbackAgentName =
      agentOwnerNameFromSkillInvocation(invocation) ??
      t('consoleChat.governance.approvalPanel.currentAgent');
    const title = summary
      ? agentBindingDisplayText(
          summary,
          fallbackAgentName,
          locale,
          t
        )
      : null;
    if (title) return title;
  }

  if (tone === 'running') {
    return t('consoleChat.skills.agentic.callingTool', {
      skill: skill.label,
      tool: toolName,
    });
  }
  if (tone === 'error') {
    return t('consoleChat.skills.agentic.toolFailed', {
      skill: skill.label,
      tool: toolName,
    });
  }
  return t('consoleChat.skills.agentic.toolSucceeded', {
    skill: skill.label,
    tool: toolName,
  });
}

function SkillTimelineRow({
  event,
  showDetails,
}: {
  event: SkillTimelineViewModel;
  showDetails: boolean;
}) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const [isOpen, setIsOpen] = useState(false);
  const duration = getDurationText(event.item.invocation.duration_ms);
  const detail = event.detail ? sanitizeDisplayString(event.detail) : null;
  const displayResult = sanitizeTimelineResultForDisplay(event.item.invocation.result);
  const rowContent = (
    <>
      <span
        className={cn(
          'flex size-5 shrink-0 items-center justify-center rounded-full border bg-background',
          event.tone === 'error'
            ? 'border-destructive/40 text-destructive'
            : 'border-border text-muted-foreground'
        )}
      >
        {getStatusIcon(event.tone)}
      </span>
      <AIChatSkillIcon
        icon={event.skill.icon}
        className="size-3.5 shrink-0 text-muted-foreground"
      />
      <span className="min-w-0 flex-1 truncate text-foreground">{event.title}</span>
      {duration ? <span className="shrink-0 text-muted-foreground">{duration}</span> : null}
      {showDetails ? (
        <ChevronDown
          className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform', {
            'rotate-180': isOpen,
          })}
        />
      ) : null}
    </>
  );

  return (
    <div
      className={cn(
        'rounded-md border bg-background/80 text-xs',
        event.tone === 'error' ? 'border-destructive/30' : 'border-border'
      )}
    >
      {showDetails ? (
        <button
          type="button"
          className="flex min-h-8 w-full min-w-0 items-center gap-2 px-2.5 py-1.5 text-left"
          onClick={() => setIsOpen(open => !open)}
          aria-expanded={isOpen}
        >
          {rowContent}
        </button>
      ) : (
        <div className="flex min-h-8 w-full min-w-0 items-center gap-2 px-2.5 py-1.5 text-left">
          {rowContent}
        </div>
      )}
      {showDetails && isOpen ? (
        <div className="border-t bg-muted/20 px-2.5 py-2">
          {detail ? (
            <div className="mb-2 whitespace-pre-wrap break-words text-muted-foreground">
              {detail}
            </div>
          ) : null}
          <AIChatSkillResultSummary result={displayResult} className="mb-2" />
          <dl className="grid gap-1 rounded-md bg-background/80 p-2 text-[11px]">
            {timelineDebugRows(event.item.invocation, locale).map(([labelKey, value]) => {
              const formatted = formatDebugValue(value);
              if (!formatted) return null;

              return (
                <div key={labelKey} className="grid grid-cols-[88px_minmax(0,1fr)] gap-2">
                  <dt className="text-muted-foreground">
                    {t(TIMELINE_DEBUG_LABEL_KEYS[labelKey])}
                  </dt>
                  <dd className="min-w-0 max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-foreground/80">
                    {formatted}
                  </dd>
                </div>
              );
            })}
          </dl>
        </div>
      ) : null}
    </div>
  );
}

function memoryEventContent(item: MemoryTimelineItem): string {
  return (item.event.content ?? item.event.content_preview ?? '').trim();
}

function memoryEventTitle(
  item: MemoryTimelineItem,
  locale: string,
  showMemoryKey: boolean
): string {
  return getAIChatUserMemoryMutationTitle(item.event.action, locale, {
    content: item.event.content_preview || item.event.content,
    entryId: item.event.entry_id ?? (showMemoryKey ? item.event.key : undefined),
  });
}

function MemoryTimelineRow({
  item,
  showMemoryKey,
}: {
  item: MemoryTimelineItem;
  showMemoryKey: boolean;
}) {
  const { locale } = useLocale();
  const [isOpen, setIsOpen] = useState(false);
  const content = memoryEventContent(item);
  const canExpand = Boolean(
    content || (showMemoryKey && item.event.key) || item.event.category || item.event.memory_type
  );

  return (
    <div className="rounded-md border border-emerald-500/20 bg-emerald-500/5 text-xs text-foreground">
      <button
        type="button"
        className="flex min-h-8 w-full min-w-0 items-center gap-2 px-2.5 py-1.5 text-left"
        onClick={() => canExpand && setIsOpen(open => !open)}
        aria-expanded={isOpen}
      >
        <span className="flex size-5 shrink-0 items-center justify-center rounded-full border border-emerald-500/30 bg-background text-emerald-600">
          <CheckCircle2 className="size-3.5" />
        </span>
        <span className="min-w-0 flex-1 truncate">
          {memoryEventTitle(item, locale, showMemoryKey)}
        </span>
        {showMemoryKey && item.event.key ? (
          <span className="max-w-32 shrink-0 truncate rounded border border-emerald-500/20 bg-background/70 px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
            {item.event.key}
          </span>
        ) : null}
        {canExpand ? (
          <ChevronDown
            className={cn('size-3.5 shrink-0 text-muted-foreground transition-transform', {
              'rotate-180': isOpen,
            })}
          />
        ) : null}
      </button>
      {isOpen ? (
        <div className="space-y-2 border-t border-emerald-500/15 bg-background/70 px-2.5 py-2">
          {content ? (
            <div className="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md border bg-background p-2 leading-relaxed text-foreground/85">
              {content}
            </div>
          ) : null}
          {item.event.category || item.event.memory_type ? (
            <div className="flex flex-wrap gap-1.5 text-[11px] text-muted-foreground">
              {item.event.category ? (
                <span className="rounded border bg-background/80 px-1.5 py-0.5">
                  {item.event.category}
                </span>
              ) : null}
              {item.event.memory_type ? (
                <span className="rounded border bg-background/80 px-1.5 py-0.5">
                  {item.event.memory_type}
                </span>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function governanceDecisionStatus(item: GovernanceTimelineItem): string {
  return String(
    item.event.decision ?? item.event.governance?.status ?? item.event.status ?? ''
  ).toLowerCase();
}

function governanceApprovalStatus(item: GovernanceTimelineItem): string {
  const status = String(
    item.event.approval_status ??
      item.event.governance?.approval_status ??
      item.event.governance?.approval_result?.approval_status ??
      ''
  ).toLowerCase();
  return status === 'approved' || status === 'rejected' ? status : '';
}

function isToolGovernanceNeedsApproval(item: GovernanceTimelineItem): boolean {
  if (governanceApprovalStatus(item)) return false;
  return (
    governanceDecisionStatus(item) === 'needs_approval' ||
    item.event.requires_approval === true ||
    item.event.governance?.requires_approval === true
  );
}

function canPublishPendingGovernanceApproval(messageStatus?: AIChatMessage['status']) {
  return (
    messageStatus === 'pending' ||
    messageStatus === 'streaming' ||
    messageStatus === 'waiting_approval'
  );
}

function governanceDisplayText(value: unknown): string | null {
  return formatDebugValue(value);
}

function governanceReason(item: GovernanceTimelineItem): string {
  const reason = String(item.event.reason ?? item.event.governance?.reason ?? '').trim();
  return reason ? (sanitizeDisplayString(reason) ?? '') : '';
}

function governanceApprovalEvent(item: GovernanceTimelineItem) {
  return item.event.approval_event ?? item.event.governance?.approval_event;
}

function governanceItemCorrelationId(item: GovernanceTimelineItem): string | null {
  const assetOperationAudit = governanceAssetOperationAudit(item);
  return (
    governanceStringValue(item.event.correlation_id) ??
    governanceStringValue(item.event.governance?.correlation_id) ??
    governanceStringValue(governanceApprovalEvent(item)?.correlation_id) ??
    governanceRecordString(assetOperationAudit, ['correlation_id'])
  );
}

function governanceRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

function governanceStringValue(value: unknown): string | null {
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return null;
}

function governanceNumberValue(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value) && value >= 0) return Math.floor(value);
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number.parseInt(value.trim(), 10);
    if (Number.isFinite(parsed) && parsed >= 0) return parsed;
  }
  return null;
}

function governanceRecordString(value: unknown, keys: readonly string[]): string | null {
  const record = governanceRecord(value);
  if (!record) return null;
  for (const key of keys) {
    const text = governanceStringValue(record[key]);
    if (text) return text;
  }
  return null;
}

function governanceRecordNumber(value: unknown, keys: readonly string[]): number | null {
  const record = governanceRecord(value);
  if (!record) return null;
  for (const key of keys) {
    const number = governanceNumberValue(record[key]);
    if (number !== null) return number;
  }
  return null;
}

function governanceApprovalResult(item: GovernanceTimelineItem): Record<string, unknown> | null {
  return (
    governanceRecord(item.event.approval_result) ??
    governanceRecord(item.event.governance?.approval_result)
  );
}

function governanceExecutionResult(item: GovernanceTimelineItem): Record<string, unknown> | null {
  const approvalResult = governanceApprovalResult(item);
  return (
    governanceRecord(item.event.execution_result) ??
    governanceRecord(item.event.result) ??
    governanceRecord(item.event.governance?.execution_result) ??
    governanceRecord(approvalResult?.execution_result)
  );
}

function governanceModelFeedback(item: GovernanceTimelineItem): Record<string, unknown> | null {
  const approvalResult = governanceApprovalResult(item);
  return (
    governanceRecord(item.event.model_feedback) ??
    governanceRecord(item.event.governance?.model_feedback) ??
    governanceRecord(approvalResult?.model_feedback)
  );
}

function governanceAssetOperationAudit(
  item: GovernanceTimelineItem
): Record<string, unknown> | null {
  const modelFeedback = governanceModelFeedback(item);
  const approvalResult = governanceApprovalResult(item);
  return (
    governanceRecord(item.event.asset_operation_audit) ??
    governanceRecord(item.event.governance?.asset_operation_audit) ??
    governanceRecord(modelFeedback?.asset_operation_audit) ??
    governanceRecord(approvalResult?.asset_operation_audit)
  );
}

function governanceMatchedGrant(item: GovernanceTimelineItem): Record<string, unknown> | null {
  const feedback = governanceModelFeedback(item);
  return (
    governanceRecord(item.event.matched_grant) ??
    governanceRecord(item.event.governance?.matched_grant) ??
    governanceRecord(feedback?.matched_grant)
  );
}

function governanceSessionGrant(item: GovernanceTimelineItem): Record<string, unknown> | null {
  const approvalResult = governanceApprovalResult(item);
  return (
    governanceRecord(item.event.session_grant) ??
    governanceRecord(approvalResult?.session_grant) ??
    governanceRecord(approvalResult?.approved_grant)
  );
}

function governanceAssetsFromUnknown(value: unknown): AIChatToolGovernanceAssetRef[] {
  if (!Array.isArray(value)) return [];
  return value.filter((asset): asset is AIChatToolGovernanceAssetRef =>
    Boolean(governanceRecord(asset))
  );
}

function appendGovernanceAssets(
  out: AIChatToolGovernanceAssetRef[],
  seen: Set<string>,
  assets: AIChatToolGovernanceAssetRef[]
) {
  for (const asset of assets) {
    const key =
      governanceStringValue(asset.id) ??
      governanceStringValue(asset.name) ??
      governanceStringValue(asset.title) ??
      `${out.length}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(asset);
  }
}

function governanceApprovalAssets(item: GovernanceTimelineItem): AIChatToolGovernanceAssetRef[] {
  const approvalEvent = governanceApprovalEvent(item);
  const matchedGrant = governanceMatchedGrant(item);
  const modelFeedback = governanceModelFeedback(item);
  const sessionGrant = governanceSessionGrant(item);
  const approvalResult = governanceApprovalResult(item);
  const assetOperationAudit = governanceAssetOperationAudit(item);
  const out: AIChatToolGovernanceAssetRef[] = [];
  const seen = new Set<string>();
  for (const assets of [
    approvalEvent?.assets,
    assetOperationAudit?.assets,
    item.event.governance?.assets,
    matchedGrant?.assets,
    modelFeedback?.matched_assets,
    sessionGrant?.assets,
    governanceRecord(approvalResult?.session_grant)?.assets,
    governanceRecord(approvalResult?.approved_grant)?.assets,
  ]) {
    appendGovernanceAssets(out, seen, governanceAssetsFromUnknown(assets));
  }
  return out;
}

interface GovernanceAssetGroups {
  all: AIChatToolGovernanceAssetRef[];
  owners: AIChatToolGovernanceAssetRef[];
  targets: AIChatToolGovernanceAssetRef[];
  display: AIChatToolGovernanceAssetRef[];
}

const AGENT_BINDING_TOOL_NAMES = new Set([
  'replace_agent_skill_bindings',
  'replace_agent_knowledge_bindings',
  'replace_agent_database_bindings',
  'replace_agent_workflow_bindings',
  'agent.replace_skill_bindings',
  'agent.replace_knowledge_bindings',
  'agent.replace_database_bindings',
  'agent.replace_workflow_bindings',
]);

const AGENT_CONFIG_TOOL_NAMES = new Set(['update_agent_config', 'agent.update_config']);

const AGENT_BINDING_ASSET_TYPES = new Set([
  'agent_skill',
  'skill',
  'knowledge_base',
  'knowledge',
  'dataset',
  'database_table',
  'table',
  'workflow',
]);

function normalizeGovernanceAssetType(value: unknown): string {
  return governanceStringValue(value)?.toLowerCase().replace(/-/g, '_') ?? '';
}

function normalizeGovernanceToolName(value: unknown): string {
  return governanceStringValue(value)?.toLowerCase() ?? '';
}

function isAgentManagementGovernanceTool(skillId: string, toolName: string): boolean {
  return (
    skillId === 'agent-management' ||
    skillId === 'agent_management' ||
    toolName.startsWith('agent.')
  );
}

function isAgentBindingGovernance(item: GovernanceTimelineItem): boolean {
  const skillId = normalizeGovernanceToolName(governanceEventString(item, ['skill_id']));
  const toolName = normalizeGovernanceToolName(governanceEventString(item, ['tool_name', 'tool_id']));
  if (!isAgentManagementGovernanceTool(skillId, toolName)) return false;
  if (AGENT_BINDING_TOOL_NAMES.has(toolName)) return true;
  if (!AGENT_CONFIG_TOOL_NAMES.has(toolName)) return false;

  const executionSummary = agentBindingDisplaySummaryFromRecord(
    governanceExecutionResult(item) ?? {},
    toolName
  );
  if (executionSummary) return true;

  const assetType = normalizeGovernanceAssetType(governanceEventString(item, ['asset_type']));
  if (AGENT_BINDING_ASSET_TYPES.has(assetType)) return true;

  const assets = governanceApprovalAssets(item);
  const hasOwner = assets.some(asset => isBindingOwnerAsset(asset));
  const hasBindingTarget = assets.some(asset =>
    AGENT_BINDING_ASSET_TYPES.has(normalizeGovernanceAssetType(asset.type))
  );
  return hasOwner && hasBindingTarget;
}

function governanceAssetMetadata(
  asset: AIChatToolGovernanceAssetRef
): Record<string, unknown> | null {
  return governanceRecord(asset.metadata);
}

function isBindingOwnerAsset(asset: AIChatToolGovernanceAssetRef): boolean {
  const metadata = governanceAssetMetadata(asset);
  return (
    normalizeGovernanceAssetType(asset.type) === 'agent' ||
    governanceBooleanValue(asset.binding_owner) === true ||
    governanceBooleanValue(metadata?.binding_owner) === true
  );
}

function governanceAssetGroups(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[]
): GovernanceAssetGroups {
  if (!isAgentBindingGovernance(item)) {
    return { all: assets, owners: [], targets: assets, display: assets };
  }

  const targetType = normalizeGovernanceAssetType(governanceEventString(item, ['asset_type']));
  const owners: AIChatToolGovernanceAssetRef[] = [];
  const targets: AIChatToolGovernanceAssetRef[] = [];

  for (const asset of assets) {
    if (isBindingOwnerAsset(asset)) {
      owners.push(asset);
      continue;
    }
    const assetType = normalizeGovernanceAssetType(asset.type);
    if (!targetType || assetType === targetType) {
      targets.push(asset);
    }
  }

  return {
    all: assets,
    owners,
    targets,
    display: targets.length > 0 ? targets : assets.filter(asset => !isBindingOwnerAsset(asset)),
  };
}

function governanceDisplayAssets(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[]
): AIChatToolGovernanceAssetRef[] {
  return governanceAssetGroups(item, assets).display;
}

function governanceAssetCount(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[]
): number {
  const displayAssets = governanceDisplayAssets(item, assets);
  if (displayAssets.length > 0) return displayAssets.length;
  const modelFeedback = governanceModelFeedback(item);
  const assetOperationAudit = governanceAssetOperationAudit(item);
  for (const source of [assetOperationAudit, modelFeedback, item.event.governance, item.event]) {
    const count = governanceRecordNumber(source, ['asset_count', 'assetCount']);
    if (count !== null) return isAgentBindingGovernance(item) ? Math.max(0, count - 1) : count;
  }
  return 0;
}

function governanceAssetSpecificDisplayName(asset: AIChatToolGovernanceAssetRef): string | null {
  const id = governanceStringValue(asset.id);
  const fileName =
    governanceRecordString(asset, ['filename', 'file_name']) ??
    governanceRecordString(asset.metadata, ['filename', 'file_name']);
  if (fileName && !looksLikeOpaqueAssetID(fileName)) return fileName;
  const displayName =
    governanceRecordString(asset, ['name', 'title', 'label', 'agent_name', 'resource_name']) ??
    governanceRecordString(asset.metadata, [
      'name',
      'title',
      'label',
      'agent_name',
      'resource_name',
    ]);
  if (displayName && displayName !== id && !looksLikeOpaqueAssetID(displayName)) {
    return displayName;
  }
  return null;
}

function governanceAssetDisplayName(asset: AIChatToolGovernanceAssetRef): string {
  const assetType = governanceStringValue(asset.type)?.toLowerCase();
  const displayName = governanceAssetSpecificDisplayName(asset);
  if (displayName) return displayName;
  if (assetType === 'file') return 'file';
  if (assetType && !looksLikeOpaqueAssetID(assetType)) return assetType;
  return 'asset';
}

function looksLikeOpaqueAssetID(value: string): boolean {
  const normalized = value.trim();
  if (!normalized) return false;
  if (
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(?:\.[a-z0-9]+)?$/i.test(
      normalized
    )
  ) {
    return true;
  }
  if (
    /^(file|upload[-_]?file|asset|workspace|ws)[_-](?=[a-z0-9_-]*\d)[a-z0-9_-]+$/i.test(
      normalized
    )
  ) {
    return true;
  }
  return /^[0-9a-f]{24,}$/i.test(normalized);
}

function governanceFileSizeText(bytes: number | null): string | null {
  if (bytes === null) return null;
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(bytes >= 10 * 1024 ? 0 : 1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(bytes >= 10 * 1024 * 1024 ? 0 : 1)} MB`;
}

function uniqueGovernanceAssetMetaParts(parts: Array<string | null>): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of parts) {
    const visiblePart = part ? sanitizeDisplayString(part) : null;
    if (!visiblePart || seen.has(visiblePart)) continue;
    seen.add(visiblePart);
    out.push(visiblePart);
  }
  return out;
}

function governanceAssetMeta(asset: AIChatToolGovernanceAssetRef): string {
  const fileType =
    governanceRecordString(asset, ['file_type', 'file_type_normalized']) ??
    governanceRecordString(asset.metadata, ['file_type', 'file_type_normalized']);
  const extension =
    governanceRecordString(asset, ['extension', 'extension_normalized']) ??
    governanceRecordString(asset.metadata, ['extension', 'extension_normalized']);
  const mimeType =
    governanceRecordString(asset, ['mime_type', 'mimeType']) ??
    governanceRecordString(asset.metadata, ['mime_type', 'mimeType']);
  const size = governanceFileSizeText(
    governanceRecordNumber(asset, ['size', 'file_size', 'fileSize']) ??
      governanceRecordNumber(asset.metadata, ['size', 'file_size', 'fileSize'])
  );
  const normalizedType = governanceStringValue(asset.type);
  const normalizedExtension = extension ? extension.replace(/^\./, '') : null;
  const parts = uniqueGovernanceAssetMetaParts([
    normalizedType,
    fileType && fileType !== normalizedType ? fileType : null,
    normalizedExtension ? `.${normalizedExtension}` : null,
    size,
    mimeType,
  ]);
  return parts.join(' · ');
}

function isFileDeleteGovernance(item: GovernanceTimelineItem): boolean {
  const effect = governanceEventString(item, ['effect'])?.toLowerCase();
  const assetType = governanceEventString(item, ['asset_type'])?.toLowerCase();
  return effect === 'delete' && assetType === 'file';
}

function governanceNoticeText(
  item: GovernanceTimelineItem,
  assetCount: number,
  t: WebappTranslator
): string | null {
  if (!isFileDeleteGovernance(item)) return null;
  const count = Math.max(assetCount, 1);
  const approvalStatus = governanceApprovalStatus(item);
  if (isToolGovernanceNeedsApproval(item)) {
    return t('consoleChat.governance.notices.fileDeletePending', { count });
  }
  if (approvalStatus === 'approved') {
    return t('consoleChat.governance.notices.fileDeleteApproved', { count });
  }
  if (approvalStatus === 'rejected') {
    return t('consoleChat.governance.notices.fileDeleteRejected', { count });
  }
  return null;
}

function governanceBooleanValue(value: unknown): boolean | null {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (normalized === 'true') return true;
    if (normalized === 'false') return false;
  }
  return null;
}

function governanceBooleanLabel(value: unknown, t: WebappTranslator): string | null {
  const normalized = governanceBooleanValue(value);
  if (normalized === null) return null;
  return normalized
    ? t('consoleChat.governance.values.yes')
    : t('consoleChat.governance.values.no');
}

function governanceEventString(
  item: GovernanceTimelineItem,
  keys: readonly string[]
): string | null {
  const approvalEvent = governanceApprovalEvent(item);
  const modelFeedback = governanceModelFeedback(item);
  const executionResult = governanceExecutionResult(item);
  for (const source of [
    item.event,
    item.event.governance,
    item.event.governance?.manifest,
    approvalEvent,
    modelFeedback,
    executionResult,
  ]) {
    const value = governanceRecordString(source, keys);
    if (value) return value;
  }
  return null;
}

function governanceEventBoolean(
  item: GovernanceTimelineItem,
  keys: readonly string[]
): boolean | null {
  const approvalEvent = governanceApprovalEvent(item);
  const modelFeedback = governanceModelFeedback(item);
  for (const source of [
    item.event,
    item.event.governance,
    item.event.governance?.manifest,
    approvalEvent,
    modelFeedback,
  ]) {
    const record = governanceRecord(source);
    if (!record) continue;
    for (const key of keys) {
      const value = governanceBooleanValue(record[key]);
      if (value !== null) return value;
    }
  }
  return null;
}

function governanceIntentText(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[],
  assetCount: number,
  t: WebappTranslator
): string | null {
  const explicitIntent = governanceEventString(item, ['intent', 'model_intent', 'action_intent']);
  if (explicitIntent) {
    const visibleIntent = sanitizeDisplayString(explicitIntent);
    if (visibleIntent) return visibleIntent;
  }
  const effect = governanceEventString(item, ['effect']);
  const assetType = governanceEventString(item, ['asset_type']);
  if (!effect && !assetType && assetCount === 0) return null;
  return t('consoleChat.governance.intentFallback', {
    effect: effect ? governanceEffectLabel(effect, t) : t('consoleChat.governance.values.unknown'),
    count: assetCount || 1,
    assetType: assetType
      ? governanceAssetTypeLabel(assetType, t)
      : t('consoleChat.governance.values.asset'),
  });
}

function governanceEffectLabel(effect: string, t: WebappTranslator): string {
  switch (effect) {
    case 'none':
      return t('consoleChat.governance.effects.none');
    case 'read':
      return t('consoleChat.governance.effects.read');
    case 'create':
      return t('consoleChat.governance.effects.create');
    case 'update':
      return t('consoleChat.governance.effects.update');
    case 'delete':
      return t('consoleChat.governance.effects.delete');
    case 'publish':
      return t('consoleChat.governance.effects.publish');
    case 'invoke':
      return t('consoleChat.governance.effects.invoke');
    case 'schedule':
      return t('consoleChat.governance.effects.schedule');
    case 'external_send':
      return t('consoleChat.governance.effects.externalSend');
    default:
      return effect;
  }
}

function governanceRiskLabel(riskLevel: string, t: WebappTranslator): string {
  switch (riskLevel) {
    case 'low':
      return t('consoleChat.governance.risks.low');
    case 'medium':
      return t('consoleChat.governance.risks.medium');
    case 'high':
      return t('consoleChat.governance.risks.high');
    case 'critical':
      return t('consoleChat.governance.risks.critical');
    default:
      return riskLevel;
  }
}

function governanceAssetTypeLabel(assetType: string, t: WebappTranslator): string {
  switch (assetType.trim().toLowerCase()) {
    case 'file':
      return t('consoleChat.governance.assetTypes.file');
    case 'agent':
      return t('consoleChat.governance.assetTypes.agent');
    case 'agent_skill':
    case 'agent-skill':
    case 'skill':
      return t('consoleChat.governance.assetTypes.agentSkill');
    case 'knowledge_base':
    case 'knowledge-base':
    case 'knowledge':
      return t('consoleChat.governance.assetTypes.knowledgeBase');
    case 'database':
      return t('consoleChat.governance.assetTypes.database');
    case 'database_table':
    case 'database-table':
    case 'table':
      return t('consoleChat.governance.assetTypes.databaseTable');
    case 'workflow':
      return t('consoleChat.governance.assetTypes.workflow');
    case 'workflow_run':
    case 'workflow-run':
      return t('consoleChat.governance.assetTypes.workflowRun');
    case 'task':
    case 'scheduled_task':
    case 'scheduled-task':
      return t('consoleChat.governance.assetTypes.task');
    case 'memory':
      return t('consoleChat.governance.assetTypes.memory');
    case 'dataset':
      return t('consoleChat.governance.assetTypes.dataset');
    case 'document':
      return t('consoleChat.governance.assetTypes.document');
    case 'prompt':
      return t('consoleChat.governance.assetTypes.prompt');
    case 'workspace':
      return t('consoleChat.governance.assetTypes.workspace');
    default:
      return assetType;
  }
}

function governanceSummaryRows(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[],
  t: WebappTranslator
) {
  const effect = governanceEventString(item, ['effect']);
  const riskLevel = governanceEventString(item, ['risk_level']);
  const assetType = governanceEventString(item, ['asset_type']);
  const assetCount = governanceAssetCount(item, assets);
  const unknown = t('consoleChat.governance.values.unknown');
  const normalizedEffect = effect?.toLowerCase();
  const normalizedRiskLevel = riskLevel?.toLowerCase();
  const isHighImpact =
    normalizedEffect === 'delete' ||
    normalizedEffect === 'publish' ||
    normalizedEffect === 'external_send' ||
    normalizedRiskLevel === 'high' ||
    normalizedRiskLevel === 'critical';
  const shouldSurfaceUnknowns = isToolGovernanceNeedsApproval(item) || isHighImpact;
  const reversible = governanceBooleanLabel(governanceEventBoolean(item, ['reversible']), t);
  return [
    ['intent', governanceIntentText(item, assets, assetCount, t)],
    ['assetCount', assetCount > 0 ? String(assetCount) : null],
    ['effect', effect ? governanceEffectLabel(effect, t) : null],
    ['riskLevel', riskLevel ? governanceRiskLabel(riskLevel, t) : null],
    ['assetType', assetType ? governanceAssetTypeLabel(assetType, t) : null],
    ['reversible', reversible ?? (shouldSurfaceUnknowns ? unknown : null)],
    ['bulkSensitive', governanceBooleanLabel(governanceEventBoolean(item, ['bulk_sensitive']), t)],
    [
      'externalSideEffect',
      governanceBooleanLabel(governanceEventBoolean(item, ['external_side_effect']), t),
    ],
    ['permissionTier', governanceEventString(item, ['permission_tier'])],
  ] as const satisfies ReadonlyArray<readonly [GovernanceFieldLabel, string | null]>;
}

function governanceFieldRows(item: GovernanceTimelineItem) {
  const approvalEvent = governanceApprovalEvent(item);
  return [
    ['decision', item.event.decision ?? item.event.governance?.status ?? item.event.status],
    [
      'riskLevel',
      item.event.risk_level ??
        item.event.governance?.manifest?.risk_level ??
        approvalEvent?.risk_level,
    ],
    [
      'effect',
      item.event.effect ?? item.event.governance?.manifest?.effect ?? approvalEvent?.effect,
    ],
    [
      'assetType',
      item.event.asset_type ??
        item.event.governance?.manifest?.asset_type ??
        approvalEvent?.asset_type,
    ],
    ['executionStatus', item.event.execution_status],
    ['executionError', item.event.execution_error],
    ['executionDuration', getDurationText(item.event.execution_duration_ms)],
  ] as const satisfies ReadonlyArray<readonly [GovernanceFieldLabel, unknown]>;
}

function buildGovernanceTitle(item: GovernanceTimelineItem, t: WebappTranslator): string {
  const approvalStatus = governanceApprovalStatus(item);
  if (approvalStatus === 'approved') return t('consoleChat.governance.approved');
  if (approvalStatus === 'rejected') return t('consoleChat.governance.rejected');
  const status = governanceDecisionStatus(item);
  if (isToolGovernanceNeedsApproval(item)) {
    return t('consoleChat.governance.needsApproval');
  }
  if (status === 'denied') return t('consoleChat.governance.denied');
  if (status === 'blocked') return t('consoleChat.governance.blocked');
  if (status === 'needs_resolution') return t('consoleChat.governance.needsResolution');
  if (status === 'allowed') return t('consoleChat.governance.allowed');
  return t('consoleChat.governance.title');
}

function governanceToolLabel(
  item: GovernanceTimelineItem,
  skillDisplayById: AIChatSkillDisplayMap,
  locale: string,
  t: WebappTranslator
): string | null {
  void item;
  void skillDisplayById;
  void locale;
  void t;
  return null;
}

function governancePermissionTierLabel(permissionTier: string, t: WebappTranslator): string {
  switch (permissionTier) {
    case 'basic':
      return t('consoleChat.governance.permissionTiers.basic');
    case 'advanced':
      return t('consoleChat.governance.permissionTiers.advanced');
    case 'full':
      return t('consoleChat.governance.permissionTiers.full');
    default:
      return permissionTier;
  }
}

function governanceActionSentence(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[],
  assetCount: number,
  t: WebappTranslator,
  locale: string
): string {
  const effect = governanceEventString(item, ['effect'])?.toLowerCase();
  const assetType = governanceEventString(item, ['asset_type'])?.toLowerCase();
  const skillId = governanceEventString(item, ['skill_id'])?.toLowerCase();
  const toolName = governanceEventString(item, ['tool_name', 'tool_id'])?.toLowerCase();
  const assetGroups = governanceAssetGroups(item, assets);
  const actionAssets = assetGroups.display;
  const count = Math.max(assetCount, actionAssets.length, 1);
  const singleAssetName =
    actionAssets.length === 1 ? governanceAssetSpecificDisplayName(actionAssets[0]) : null;

  if (skillId === 'agent-management') {
    switch (toolName) {
      case 'list_agent_knowledge_candidates':
      case 'list_agent_knowledge_binding_candidates':
      case 'agent.list_knowledge_candidates':
        return t('consoleChat.governance.approvalPanel.agentInspectKnowledgeCandidates');
      case 'list_agent_database_candidates':
      case 'list_agent_database_binding_candidates':
      case 'agent.list_database_candidates':
        return t('consoleChat.governance.approvalPanel.agentInspectDatabaseCandidates');
      case 'list_agent_database_tables':
      case 'agent.list_database_tables':
        return t('consoleChat.governance.approvalPanel.agentInspectDatabaseTables');
      case 'list_agent_workflow_binding_candidates':
      case 'agent.list_workflow_binding_candidates':
        return t('consoleChat.governance.approvalPanel.agentInspectWorkflowCandidates');
      default:
        break;
    }
  }

  if (toolName && isAgentBindingGovernance(item)) {
    const resultSummary = governanceExecutionResult(item)
      ? agentBindingDisplaySummaryFromRecord(governanceExecutionResult(item) ?? {}, toolName)
      : null;
    const ownerName =
      governanceEventString(item, ['agent_name']) ??
      resultSummary?.agentName ??
      assetGroups.owners.map(asset => governanceAssetSpecificDisplayName(asset)).find(Boolean) ??
      t('consoleChat.governance.approvalPanel.currentAgent');
    if (resultSummary) {
      const actionText = agentBindingDisplayText(resultSummary, ownerName, locale, t);
      if (actionText) return actionText;
    }
    const names = actionAssets
      .map(asset => governanceAssetSpecificDisplayName(asset))
      .filter((name): name is string => Boolean(name))
      .slice(0, 6);
    const namesText =
      names.length > 0
        ? names.join(locale === 'en-US' ? ', ' : '、')
        : t('consoleChat.governance.approvalPanel.targetCountFallback', { count });
    const kind = agentBindingKindFromToolName(toolName) ?? agentBindingKindFromAssetType(assetType ?? '');
    const updateKey = kind ? agentBindingUpdateTranslationKey(kind) : null;
    if (updateKey) {
      return t(updateKey, {
        agent: ownerName,
        count,
        names: namesText,
      });
    }

    if (
      toolName === 'replace_agent_skill_bindings' ||
      toolName === 'agent.replace_skill_bindings'
    ) {
      return t('consoleChat.governance.approvalPanel.agentUpdateSkills', {
        agent: ownerName,
        count,
        names: namesText,
      });
    }
    if (
      toolName === 'replace_agent_knowledge_bindings' ||
      toolName === 'agent.replace_knowledge_bindings'
    ) {
      return t('consoleChat.governance.approvalPanel.agentUpdateKnowledge', {
        agent: ownerName,
        count,
        names: namesText,
      });
    }
    if (
      toolName === 'replace_agent_database_bindings' ||
      toolName === 'agent.replace_database_bindings'
    ) {
      return t('consoleChat.governance.approvalPanel.agentUpdateDatabaseTables', {
        agent: ownerName,
        count,
        names: namesText,
      });
    }
    if (
      toolName === 'replace_agent_workflow_bindings' ||
      toolName === 'agent.replace_workflow_bindings'
    ) {
      return t('consoleChat.governance.approvalPanel.agentUpdateWorkflows', {
        agent: ownerName,
        count,
        names: namesText,
      });
    }
    return t('consoleChat.governance.approvalPanel.agentUpdateConfigChanges', {
      agent: ownerName,
      count,
    });
  }

  if (effect === 'delete' && assetType === 'file') {
    if (singleAssetName) {
      return t('consoleChat.governance.approvalPanel.fileDeleteOne', { name: singleAssetName });
    }
    if (count === 1) {
      return t('consoleChat.governance.approvalPanel.genericMany', {
        effect: governanceEffectLabel(effect, t),
        count,
        assetType: governanceAssetTypeLabel(assetType, t),
      });
    }
    return t('consoleChat.governance.approvalPanel.fileDeleteMany', { count });
  }

  if (effect === 'delete' && assetType === 'agent' && actionAssets.length > 1) {
    const names = actionAssets
      .map(asset => governanceAssetSpecificDisplayName(asset))
      .filter(Boolean)
      .slice(0, 6);
    if (names.length > 0) {
      return t('consoleChat.governance.approvalPanel.agentDeleteManyWithNames', {
        count,
        names: names.join(', '),
      });
    }
  }

  if (effect && assetType && singleAssetName) {
    if (
      effect === 'create' &&
      assetType === 'file' &&
      skillId === 'file-manager' &&
      toolName === 'save_file_to_management'
    ) {
      return t('consoleChat.governance.approvalPanel.fileSaveOne', { name: singleAssetName });
    }
    return t('consoleChat.governance.approvalPanel.genericOne', {
      effect: governanceEffectLabel(effect, t),
      assetType: governanceAssetTypeLabel(assetType, t),
      name: singleAssetName,
    });
  }

  if (effect && assetType) {
    return t('consoleChat.governance.approvalPanel.genericMany', {
      effect: governanceEffectLabel(effect, t),
      count,
      assetType: governanceAssetTypeLabel(assetType, t),
    });
  }

  return t('consoleChat.governance.approvalPanel.generic');
}

function buildToolGovernanceDecisionViewModel(
  item: GovernanceTimelineItem,
  skillDisplayById: AIChatSkillDisplayMap,
  locale: string,
  t: WebappTranslator,
  onToolGovernanceDecision?: (
    payload: AIChatToolGovernanceDecisionSubmitPayload
  ) => void | Promise<void>
): ToolGovernanceDecisionViewModel {
  const needsApproval = isToolGovernanceNeedsApproval(item);
  const approvalStatus = governanceApprovalStatus(item);
  const title = buildGovernanceTitle(item, t);
  const toolLabel = governanceToolLabel(item, skillDisplayById, locale, t);
  const reason = governanceReason(item);
  const approvalAssets = governanceApprovalAssets(item);
  const displayAssets = governanceDisplayAssets(item, approvalAssets);
  const assetCount = governanceAssetCount(item, approvalAssets);
  const notice = governanceNoticeText(item, assetCount, t);
  const summaryRows: ToolGovernanceDisplayRow[] = governanceSummaryRows(
    item,
    displayAssets,
    t
  ).flatMap(([labelKey, value]) =>
    value
      ? [
          {
            key: labelKey,
            label: t(GOVERNANCE_FIELD_LABEL_KEYS[labelKey]),
            value,
          },
        ]
      : []
  );
  const details: ToolGovernanceDisplayRow[] = governanceFieldRows(item).flatMap(
    ([labelKey, value]) => {
      const formatted = governanceDisplayText(value);
      return formatted
        ? [
            {
              key: labelKey,
              label: t(GOVERNANCE_FIELD_LABEL_KEYS[labelKey]),
              value: formatted,
            },
          ]
        : [];
    }
  );
  const assets: ToolGovernanceDisplayAsset[] = displayAssets.map((asset, index) => {
    const key = `${governanceStringValue(asset.id) ?? governanceAssetDisplayName(asset)}-${index}`;
    return {
      key,
      name: governanceAssetDisplayName(asset),
      meta: governanceAssetMeta(asset) || undefined,
    };
  });
  const effect = governanceEventString(item, ['effect'])?.toLowerCase();
  const riskLevel = governanceEventString(item, ['risk_level'])?.toLowerCase() ?? null;
  const permissionTier = governanceEventString(item, ['permission_tier']);
  const isHighImpact =
    effect === 'delete' ||
    effect === 'publish' ||
    effect === 'external_send' ||
    riskLevel === 'high' ||
    riskLevel === 'critical';
  const isAllowed =
    !needsApproval && !approvalStatus && governanceDecisionStatus(item) === 'allowed';
  const correlationId = governanceItemCorrelationId(item) ?? '';
  const canSubmit = needsApproval && Boolean(correlationId) && Boolean(onToolGovernanceDecision);

  const submitDecision = async (
    action: ToolGovernanceDecisionAction,
    rememberForSession: boolean
  ) => {
    if (!correlationId || !onToolGovernanceDecision) {
      throw new Error(t('consoleChat.governance.submitFailed'));
    }
    await onToolGovernanceDecision({
      conversationId: item.event.conversation_id,
      messageId: item.event.message_id,
      correlationId,
      action,
      rememberForSession: action === 'approve' ? rememberForSession : false,
    });
  };

  const actionSentence = governanceActionSentence(item, approvalAssets, assetCount, t, locale);

  return {
    title: actionSentence || title,
    toolLabel,
    actionSentence,
    notice,
    reason,
    assets,
    summaryRows,
    details,
    needsApproval,
    approvalStatus,
    isHighImpact,
    isAllowed,
    canSubmit,
    riskLabel: riskLevel ? governanceRiskLabel(riskLevel, t) : null,
    permissionLabel: permissionTier ? governancePermissionTierLabel(permissionTier, t) : null,
    pendingApprovalId: `${item.event.conversation_id}:${item.event.message_id}:${
      correlationId || item.id
    }`,
    onSubmitDecision: submitDecision,
  };
}

function toPendingToolGovernanceApproval(
  view: ToolGovernanceDecisionViewModel,
  item: GovernanceTimelineItem
): ToolGovernancePendingApproval {
  return {
    id: view.pendingApprovalId,
    title: view.title,
    toolLabel: view.toolLabel,
    actionSentence: view.actionSentence,
    assets: view.assets,
    riskLabel: view.riskLabel,
    permissionLabel: view.permissionLabel,
    canSubmit: view.canSubmit,
    isHighImpact: view.isHighImpact,
    createdAt: item.created_at ?? item.event.created_at,
    onSubmitDecision: view.onSubmitDecision,
  };
}

export function resolvePendingToolGovernanceApprovalFromTimeline(
  timeline: AIChatAgenticTimelineItem[],
  skillDisplayById: AIChatSkillDisplayMap,
  locale: string,
  t: WebappTranslator,
  onToolGovernanceDecision?: (
    payload: AIChatToolGovernanceDecisionSubmitPayload
  ) => void | Promise<void>
): ToolGovernancePendingApproval | null {
  const pending = timeline
    .flatMap(item => {
      if (item.type !== 'tool_governance_decision' || !isToolGovernanceNeedsApproval(item)) {
        return [];
      }
      const view = buildToolGovernanceDecisionViewModel(
        item,
        skillDisplayById,
        locale,
        t,
        onToolGovernanceDecision
      );
      return [toPendingToolGovernanceApproval(view, item)];
    })
    .sort((left, right) => (right.createdAt ?? 0) - (left.createdAt ?? 0));

  return pending[0] ?? null;
}

function ToolGovernanceDecisionRow({
  item,
  skillDisplayById,
  enableToolGovernanceApprovals,
  onToolGovernanceDecision,
}: {
  item: GovernanceTimelineItem;
  skillDisplayById: AIChatSkillDisplayMap;
  enableToolGovernanceApprovals: boolean;
  onToolGovernanceDecision?: (
    payload: AIChatToolGovernanceDecisionSubmitPayload
  ) => void | Promise<void>;
}) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const view = buildToolGovernanceDecisionViewModel(
    item,
    skillDisplayById,
    locale,
    t,
    onToolGovernanceDecision
  );
  if (view.needsApproval && enableToolGovernanceApprovals) return null;

  const needsApproval = enableToolGovernanceApprovals ? view.needsApproval : false;
  const canSubmit = enableToolGovernanceApprovals ? view.canSubmit : false;
  const onSubmitDecision = enableToolGovernanceApprovals ? view.onSubmitDecision : undefined;

  return (
    <ToolGovernanceDecisionCard
      submissionKey={view.pendingApprovalId}
      title={view.title}
      toolLabel={view.toolLabel}
      actionSentence={view.actionSentence}
      notice={view.notice}
      reason={view.reason}
      assets={view.assets}
      summaryRows={view.summaryRows}
      details={view.details}
      needsApproval={needsApproval}
      approvalStatus={view.approvalStatus}
      isHighImpact={view.isHighImpact}
      isAllowed={view.isAllowed}
      canSubmit={canSubmit}
      compactAudit={!view.needsApproval}
      onSubmitDecision={onSubmitDecision}
    />
  );
}

function WorkflowApprovalPanel({
  approvalToken,
  approvalUrl,
  approvalFormId,
}: {
  approvalToken: string;
  approvalUrl: string;
  approvalFormId: string;
}) {
  const t = useT('webapp');

  return (
    <div className="mt-2 max-w-3xl rounded-md border bg-warning/10 px-3 py-2 text-xs text-muted-foreground">
      <div className="font-medium text-foreground">{t('consoleChat.workflow.approvalPending')}</div>
      <div className="mt-1 flex flex-wrap items-center gap-2">
        {approvalUrl ? (
          <a
            className="inline-flex items-center gap-1 text-primary underline-offset-2 hover:underline"
            href={approvalUrl}
            target="_blank"
            rel="noreferrer"
          >
            {t('consoleChat.workflow.openApproval')}
            <ExternalLink className="size-3" />
          </a>
        ) : null}
        {approvalFormId ? (
          <span title={approvalFormId}>
            {t('consoleChat.workflow.formId', { id: approvalFormId })}
          </span>
        ) : null}
        {approvalToken && !approvalUrl ? (
          <span title={approvalToken}>
            {t('consoleChat.workflow.token', { token: approvalToken })}
          </span>
        ) : null}
      </div>
      {approvalToken ? (
        <div className="mt-2 text-[11px] text-muted-foreground">
          {t('consoleChat.workflow.approvalInputLocked')}
        </div>
      ) : null}
    </div>
  );
}

function WorkflowTimelineRow({ item }: { item: WorkflowTimelineItem }) {
  const nodes: WorkflowRunNodeListItem[] = item.nodes.map((node, index) => ({
    title: node.title ?? node.nodeId ?? node.nodeType ?? '',
    nodeId: node.nodeId ?? `workflow-node-${index}`,
    executionId: node.executionId,
    createdAtMs: node.createdAtMs,
    receivedOrder: node.receivedOrder,
    nodeType: node.nodeType ?? 'custom',
    status:
      node.status === 'success' || node.status === 'partial-succeeded' ? 'succeeded' : node.status,
    nodeInput: node.data?.input,
    nodeOutput: node.data?.output,
    modelInput: node.data?.modelInput,
    elapsedTime: node.elapsedTime,
    error: node.error ?? null,
    iterationInputs: node.iterationInputs,
    iterationOutputs: node.iterationOutputs,
    iterationRounds: node.iterationRounds as WorkflowRunNodeListItem['iterationRounds'],
    loopInputs: node.loopInputs,
    loopOutputs: node.loopOutputs,
    loopRounds: node.loopRounds as WorkflowRunNodeListItem['loopRounds'],
    steps: node.steps,
  }));
  const approvalUrl =
    typeof item.approval?.approval_url === 'string' ? item.approval.approval_url : '';
  const approvalFormId =
    typeof item.approval?.approval_form_id === 'string' ? item.approval.approval_form_id : '';
  const approvalToken =
    typeof item.approval?.approval_token === 'string' ? item.approval.approval_token : '';
  const hasApproval = Boolean(approvalUrl || approvalFormId || approvalToken);
  const approvalStatus =
    typeof item.approval?.status === 'string' ? item.approval.status.toLowerCase() : '';
  const showPendingApproval =
    item.status === 'pending_approval' &&
    hasApproval &&
    !['submitted', 'approved', 'rejected', 'expired'].includes(approvalStatus);

  return (
    <div className="border-l-2 border-muted-foreground/20 pl-3">
      <WorkflowRunMonitor
        status={item.status}
        elapsedTime={item.elapsedTime}
        error={item.error}
        items={nodes}
        defaultOpen={item.status === 'running' || item.status === 'pending_approval'}
        className="max-w-3xl rounded-md bg-background"
      />
      {showPendingApproval ? (
        <WorkflowApprovalPanel
          approvalToken={approvalToken}
          approvalUrl={approvalUrl}
          approvalFormId={approvalFormId}
        />
      ) : null}
    </div>
  );
}

function isProgressTextItem(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): item is Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }> {
  return 'type' in item && item.type === 'progress_text';
}

function isIntermediateAnswerItem(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): item is Extract<AIChatAgenticTimelineItem, { type: 'intermediate_answer' }> {
  return 'type' in item && item.type === 'intermediate_answer';
}

function isMemoryEventItem(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): item is Extract<AIChatAgenticTimelineItem, { type: 'memory_event' }> {
  return 'type' in item && item.type === 'memory_event';
}

function isToolGovernanceTimelineItem(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): item is GovernanceTimelineItem {
  return 'type' in item && item.type === 'tool_governance_decision';
}

function isWorkflowTimelineItem(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): item is WorkflowTimelineItem {
  return 'type' in item && item.type === 'workflow_run';
}

function isTerminalMessageStatus(status: AIChatMessage['status'] | undefined): boolean {
  return status === 'completed' || status === 'error' || status === 'stopped';
}

function compactTerminalIntermediateAnswers(
  timeline: AIChatAgenticTimelineItem[],
  messageStatus: AIChatMessage['status'] | undefined
): AIChatAgenticTimelineItem[] {
  if (!isTerminalMessageStatus(messageStatus)) return timeline;
  const intermediateItems = timeline.filter(isIntermediateAnswerItem);
  if (intermediateItems.length <= 1) return timeline;

  const latestIntermediate = intermediateItems.reduce((latest, item) => {
    const latestAt = latest.created_at ?? 0;
    const itemAt = item.created_at ?? 0;
    if (itemAt > latestAt) return item;
    if (itemAt < latestAt) return latest;
    return item.id > latest.id ? item : latest;
  });

  return timeline.filter(
    item => !isIntermediateAnswerItem(item) || item.id === latestIntermediate.id
  );
}

function compactTerminalProgressText(
  timeline: AIChatAgenticTimelineItem[],
  messageStatus: AIChatMessage['status'] | undefined
): AIChatAgenticTimelineItem[] {
  if (messageStatus !== 'completed') return timeline;
  return timeline.filter(item => item.type !== 'progress_text' || !isTransientProgressItem(item));
}

function invocationString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function invocationRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function invocationStatusIsSuccessful(invocation: AIChatSkillInvocation): boolean {
  const status = String(invocation.status ?? '').trim().toLowerCase();
  return status === 'success' || status === 'succeeded' || status === 'allowed';
}

function invocationNavigationTarget(invocation: AIChatSkillInvocation): string {
  const result = invocationRecord(invocation.result);
  const args = invocationRecord(invocation.arguments);
  const href =
    invocationString(invocation.href) ||
    invocationString(result.href) ||
    invocationString(args.href);
  return href.replace(/\/+$/, '') || href;
}

function routeNavigationDisplayTarget(
  invocation: AIChatSkillInvocation,
  locale: string
): string | null {
  if (invocation.skill_id !== 'console-navigator' || invocation.tool_name !== 'navigate') {
    return null;
  }
  const href = invocationNavigationTarget(invocation);
  const result = invocationRecord(invocation.result);
  const rawLabel = invocationString(result.label);
  const normalizedHref = href.toLowerCase();
  const englishLabels: Record<string, string> = {
    '/console': 'Home',
    '/console/files': 'File Management',
    '/console/agents': 'Agents',
    '/console/db': 'Databases',
  };
  const chineseLabels: Record<string, string> = {
    '/console': '首页',
    '/console/files': '文件管理',
    '/console/agents': '智能体',
    '/console/db': '数据库',
  };
  if (locale === 'en-US') {
    return englishLabels[normalizedHref] || rawLabel || href;
  }
  return chineseLabels[normalizedHref] || rawLabel || href;
}

function routeNavigationAlreadyLoaded(invocation: AIChatSkillInvocation): boolean {
  const result = invocationRecord(invocation.result);
  return invocationString(result.event_type) === 'route_already_loaded';
}

function routeNavigationEventKey(
  item: AIChatAgenticTimelineItem | SkillTimelineViewModel
): string | null {
  if (!('item' in item)) return null;
  const invocation = item.item.invocation;
  if (invocation.skill_id !== 'console-navigator' || invocation.tool_name !== 'navigate') {
    return null;
  }
  const href = invocationNavigationTarget(invocation);
  return href ? href.toLowerCase() : null;
}

function compactAdjacentDuplicateRouteNavigationEvents<
  T extends AIChatAgenticTimelineItem | SkillTimelineViewModel,
>(items: T[]): T[] {
  const compacted: T[] = [];
  for (const item of items) {
    const previous = compacted[compacted.length - 1];
    const currentRouteKey = routeNavigationEventKey(item);
    if (
      previous &&
      currentRouteKey &&
      routeNavigationEventKey(previous) === currentRouteKey
    ) {
      compacted[compacted.length - 1] = item;
      continue;
    }
    compacted.push(item);
  }
  return compacted;
}

function completedClientActionKey(invocation: AIChatSkillInvocation): string | null {
  if (invocation.kind !== 'client_action') return null;
  if (invocation.action_type !== 'route_navigation') return null;
  if (!invocationStatusIsSuccessful(invocation)) return null;
  const href = invocationNavigationTarget(invocation);
  if (!href) return null;
  return [
    invocation.skill_id,
    invocation.tool_name ?? '',
    href,
  ]
    .map(value => value.trim().toLowerCase())
    .join('::');
}

function toolCallClientActionKey(invocation: AIChatSkillInvocation): string | null {
  if (invocation.kind !== 'tool_call') return null;
  if (invocation.skill_id !== 'console-navigator' || invocation.tool_name !== 'navigate') return null;
  if (!invocationStatusIsSuccessful(invocation)) return null;
  const href = invocationNavigationTarget(invocation);
  if (!href) return null;
  return [
    invocation.skill_id,
    invocation.tool_name ?? '',
    href,
  ]
    .map(value => value.trim().toLowerCase())
    .join('::');
}

function isSupersededByClientActionSkillEvent(
  item: AIChatAgenticTimelineItem,
  completedClientActionKeys: ReadonlySet<string>
): boolean {
  if (item.type !== 'skill_event') return false;
  const key = toolCallClientActionKey(item.invocation);
  return Boolean(key && completedClientActionKeys.has(key));
}

function isCompletedSuccessfulSkillLoad(
  item: AIChatAgenticTimelineItem,
  messageStatus: AIChatMessage['status'] | undefined
): boolean {
  if (
    item.type === 'skill_event' &&
    item.invocation.kind === 'skill_load' &&
    item.invocation.skill_id === 'console-navigator'
  ) {
    return true;
  }
  return (
    messageStatus === 'completed' &&
    item.type === 'skill_event' &&
    item.invocation.kind === 'skill_load' &&
    invocationStatusIsSuccessful(item.invocation)
  );
}

function governedSkillInvocationCorrelationId(invocation: AIChatSkillInvocation): string | null {
  const modelFeedback = governanceRecord(invocation.governance?.model_feedback);
  return (
    governanceStringValue(invocation.governance?.correlation_id) ??
    governanceStringValue(invocation.governance?.approval_event?.correlation_id) ??
    governanceStringValue(invocation.asset_operation_audit?.correlation_id) ??
    governanceStringValue(invocation.governance?.asset_operation_audit?.correlation_id) ??
    governanceRecordString(modelFeedback?.asset_operation_audit, ['correlation_id'])
  );
}

function isGovernedSkillEvent(
  item: AIChatAgenticTimelineItem,
  governanceCorrelationIds: ReadonlySet<string>
): boolean {
  if (item.type !== 'skill_event') return false;
  if (isPendingToolGovernanceInvocation(item.invocation)) return true;
  const correlationId = governedSkillInvocationCorrelationId(item.invocation);
  return Boolean(
    correlationId &&
      governanceCorrelationIds.has(correlationId) &&
      item.invocation.kind !== 'tool_governance'
  );
}

function normalizeGovernanceDedupePart(value: string | null): string {
  return value?.trim().toLowerCase().replace(/\s+/g, ' ') ?? '';
}

function governanceOperationDedupeKey(item: GovernanceTimelineItem): string | null {
  const assets = governanceDisplayAssets(item, governanceApprovalAssets(item));
  const assetKeys = assets
    .map(
      asset =>
        governanceStringValue(asset.id) ??
        governanceRecordString(asset, ['filename', 'file_name', 'name', 'title', 'label']) ??
        governanceRecordString(asset.metadata, ['filename', 'file_name', 'name', 'title', 'label'])
    )
    .filter((value): value is string => Boolean(value))
    .map(value => normalizeGovernanceDedupePart(value))
    .sort();
  const assetCount = governanceAssetCount(item, assets);
  const assetPart = assetKeys.length > 0 ? assetKeys.join('|') : `count:${assetCount || 1}`;
  const parts = [
    governanceEventString(item, ['skill_id']),
    governanceEventString(item, ['tool_id', 'tool_name']),
    governanceEventString(item, ['effect']),
    governanceEventString(item, ['asset_type']),
    assetPart,
  ].map(normalizeGovernanceDedupePart);

  if (!parts.some(Boolean)) return null;
  return parts.join('::');
}

function isFinalGovernanceOutcome(item: GovernanceTimelineItem): boolean {
  if (governanceApprovalStatus(item) === 'rejected') return false;
  const decisionStatus = governanceDecisionStatus(item);
  const eventStatus = String(item.event.status ?? '').toLowerCase();
  const executionStatus = String(item.event.execution_status ?? '').toLowerCase();
  return (
    decisionStatus === 'allowed' ||
    decisionStatus === 'success' ||
    eventStatus === 'success' ||
    executionStatus === 'success'
  );
}

function isResolvedApprovalGovernanceItem(item: GovernanceTimelineItem): boolean {
  return governanceApprovalStatus(item) === 'approved' && !isFinalGovernanceOutcome(item);
}

function isSupersededResolvedApprovalGovernanceItem(
  item: GovernanceTimelineItem,
  finalOperationKeys: ReadonlySet<string>
): boolean {
  if (!isResolvedApprovalGovernanceItem(item)) return false;
  const operationKey = governanceOperationDedupeKey(item);
  return Boolean(operationKey && finalOperationKeys.has(operationKey));
}

function isTransientProgressItem(
  item: Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }>
) {
  return item.transient === true || Boolean(item.phase && !item.content.trim());
}

function stableIndex(value: string, length: number): number {
  if (length <= 0) return 0;
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
  }
  return hash % length;
}

function buildProgressText(
  item: Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }>,
  skillDisplayById: AIChatSkillDisplayMap,
  locale: string,
  t: WebappTranslator
) {
  if (item.phase !== 'tool_planning') {
    if (item.phase === 'planning') {
      return t('consoleChat.skills.agentic.preparingAction');
    }
    return item.content;
  }

  const skill = item.skill_id
    ? (skillDisplayById[item.skill_id] ?? getFallbackAIChatSkillDisplayInfo(item.skill_id, locale))
    : null;
  const tool =
    item.skill_id && item.tool_name
      ? getAIChatSkillToolDisplayName(item.skill_id, item.tool_name, locale) || item.tool_name
      : item.tool_name;

  if (skill && tool) {
    return t('consoleChat.skills.agentic.preparingTool', { skill: skill.label, tool });
  }
  if (skill) {
    return t('consoleChat.skills.agentic.preparingSkill', { skill: skill.label });
  }
  return t('consoleChat.skills.agentic.preparingAction');
}

function buildTransientProgressText(
  item: Extract<AIChatAgenticTimelineItem, { type: 'progress_text' }>,
  skillDisplayById: AIChatSkillDisplayMap,
  locale: string,
  t: WebappTranslator
) {
  if (item.phase === 'tool_planning' && (item.skill_id || item.tool_name)) {
    return buildProgressText(item, skillDisplayById, locale, t);
  }
  const key =
    TRANSIENT_PROGRESS_TEXT_KEYS[
      stableIndex(item.event_id ?? item.id, TRANSIENT_PROGRESS_TEXT_KEYS.length)
    ];
  return t(key);
}

export function AIChatAgenticTimeline({
  timeline,
  skillDisplayById,
  defaultOpen = true,
  showMemoryKey = true,
  showSkillEventDetails = true,
  enableToolGovernanceApprovals = false,
  messageStatus,
  onToolGovernanceDecision,
}: AIChatAgenticTimelineProps) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const [isOpen, setIsOpen] = useState(defaultOpen);
  const pendingApprovalScopeId = useToolGovernancePendingApprovalScope();

  const pendingGovernanceApprovals = useMemo(() => {
    if (!enableToolGovernanceApprovals) return [];
    if (!canPublishPendingGovernanceApproval(messageStatus)) return [];
    return timeline.flatMap(item => {
      if (item.type !== 'tool_governance_decision' || !isToolGovernanceNeedsApproval(item)) {
        return [];
      }
      const view = buildToolGovernanceDecisionViewModel(
        item,
        skillDisplayById,
        locale,
        t,
        onToolGovernanceDecision
      );
      return [toPendingToolGovernanceApproval(view, item)];
    });
  }, [
    enableToolGovernanceApprovals,
    locale,
    messageStatus,
    onToolGovernanceDecision,
    skillDisplayById,
    t,
    timeline,
  ]);

  useEffect(() => {
    const cleanups = pendingGovernanceApprovals.map(approval =>
      publishToolGovernancePendingApproval(approval, pendingApprovalScopeId)
    );
    return () => {
      cleanups.forEach(cleanup => cleanup());
    };
  }, [pendingApprovalScopeId, pendingGovernanceApprovals]);

  const governanceCorrelationIds = useMemo(
    () =>
      new Set(
        timeline
          .flatMap(item =>
            item.type === 'tool_governance_decision' ? [governanceItemCorrelationId(item)] : []
          )
          .filter((correlationId): correlationId is string => Boolean(correlationId))
      ),
    [timeline]
  );

  const events = useMemo(() => {
    const finalGovernanceOperationKeys = new Set(
      timeline
        .filter(
          (item): item is GovernanceTimelineItem =>
            item.type === 'tool_governance_decision' && isFinalGovernanceOutcome(item)
        )
        .map(governanceOperationDedupeKey)
        .filter((key): key is string => Boolean(key))
    );
    const completedClientActionKeys = new Set(
      timeline
        .flatMap(item => {
          if (item.type !== 'skill_event') return [];
          const key = completedClientActionKey(item.invocation);
          return key ? [key] : [];
        })
    );

    return compactAdjacentDuplicateRouteNavigationEvents(compactTerminalProgressText(
      compactTerminalIntermediateAnswers(timeline, messageStatus),
      messageStatus
    )
      .filter(
        item =>
          !isGovernedSkillEvent(item, governanceCorrelationIds) &&
          !isSupersededByClientActionSkillEvent(item, completedClientActionKeys) &&
          !isCompletedSuccessfulSkillLoad(item, messageStatus) &&
          !(
            item.type === 'tool_governance_decision' &&
            isSupersededResolvedApprovalGovernanceItem(item, finalGovernanceOperationKeys)
          ) &&
          !(
            enableToolGovernanceApprovals &&
            item.type === 'tool_governance_decision' &&
            isToolGovernanceNeedsApproval(item)
          )
      )
      .map(item => {
        if (item.type === 'progress_text') return item;
        if (item.type === 'intermediate_answer') return item;
        if (item.type === 'memory_event') return item;
        if (item.type === 'tool_governance_decision') return item;
        if (item.type === 'workflow_run') return item;

        const skillId = item.invocation.skill_id || t('consoleChat.skills.trace.unknownSkill');
        const skill = skillDisplayById[skillId] ?? getFallbackAIChatSkillDisplayInfo(skillId, locale);
        const tone = getInvocationTone(item.invocation);

        return {
          item,
          skill,
          tone,
          title: buildSkillTitle(item.invocation, skill, tone, locale, t),
          detail:
            getAIChatSkillResultDisplay(item.invocation, locale) ||
            item.invocation.message ||
            item.invocation.error,
        };
      }));
  }, [enableToolGovernanceApprovals, governanceCorrelationIds, locale, messageStatus, skillDisplayById, t, timeline]);

  if (events.length === 0) return null;

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen} className="mb-3 w-full min-w-0 max-w-full">
      <div className="mb-2 flex items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 px-2 text-xs text-muted-foreground"
          asChild
        >
          <CollapsibleTrigger>
            <ChevronDown
              className={cn('size-3.5 transition-transform', { 'rotate-180': isOpen })}
            />
            {isOpen
              ? t('consoleChat.skills.agentic.hideProcess')
              : t('consoleChat.skills.agentic.showProcess')}
          </CollapsibleTrigger>
        </Button>
        <span className="text-[11px] text-muted-foreground">
          {t('consoleChat.skills.trace.eventCount', { count: events.length })}
        </span>
      </div>
      <CollapsibleContent>
        <div className="min-w-0 space-y-2">
          {events.map(item =>
            isProgressTextItem(item) ? (
              isTransientProgressItem(item) ? (
                <div
                  key={item.id}
                  className="border-l-2 border-muted-foreground/15 py-0.5 pl-3 text-xs text-muted-foreground/70 animate-pulse"
                >
                  <span>{buildTransientProgressText(item, skillDisplayById, locale, t)}</span>
                </div>
              ) : (
                <div
                  key={item.id}
                  className={cn(
                    assistantMarkdownClassName,
                    'border-l-2 border-muted-foreground/20 pl-3 text-foreground'
                  )}
                >
                  <MarkdownViewer
                    className="md-viewer min-w-0 max-w-full overflow-hidden break-words"
                    content={buildProgressText(item, skillDisplayById, locale, t)}
                    renderIdentity={item.id}
                  />
                </div>
              )
            ) : isIntermediateAnswerItem(item) ? (
              <div key={item.id} className="space-y-1.5 border-l-2 border-muted-foreground/20 pl-3">
                {item.title || item.status === 'streaming' ? (
                  <div className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                    {item.status === 'streaming' ? (
                      <Loader2 className="size-3 animate-spin" />
                    ) : null}
                    {item.title ? <span>{item.title}</span> : null}
                  </div>
                ) : null}
                <div className={assistantMarkdownClassName}>
                  <MarkdownViewer
                    className="md-viewer min-w-0 max-w-full overflow-hidden break-words"
                    content={item.content}
                    isStreaming={item.status === 'streaming'}
                    renderIdentity={item.answer_id || item.id}
                  />
                </div>
              </div>
            ) : isMemoryEventItem(item) ? (
              <MemoryTimelineRow key={item.id} item={item} showMemoryKey={showMemoryKey} />
            ) : isToolGovernanceTimelineItem(item) ? (
              <ToolGovernanceDecisionRow
                key={item.id}
                item={item}
                skillDisplayById={skillDisplayById}
                enableToolGovernanceApprovals={enableToolGovernanceApprovals}
                onToolGovernanceDecision={onToolGovernanceDecision}
              />
            ) : isWorkflowTimelineItem(item) ? (
              <WorkflowTimelineRow key={item.id} item={item} />
            ) : (
              <SkillTimelineRow
                key={item.item.id}
                event={item}
                showDetails={showSkillEventDetails}
              />
            )
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
