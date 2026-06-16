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
type WebappTranslator = ScopedTranslations<'webapp'>;

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
  workspace: 'consoleChat.governance.fields.workspace',
  reversible: 'consoleChat.governance.fields.reversible',
  bulkSensitive: 'consoleChat.governance.fields.bulkSensitive',
  externalSideEffect: 'consoleChat.governance.fields.externalSideEffect',
  permissionTier: 'consoleChat.governance.fields.permissionTier',
  decision: 'consoleChat.governance.fields.decision',
  riskLevel: 'consoleChat.governance.fields.riskLevel',
  effect: 'consoleChat.governance.fields.effect',
  assetType: 'consoleChat.governance.fields.assetType',
  correlationId: 'consoleChat.governance.fields.correlationId',
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
  'prose prose-sm min-w-0 max-w-full dark:prose-invert sm:pr-4 md:pr-6 lg:pr-8 xl:pr-9';

const TRANSIENT_PROGRESS_TEXT_KEYS = [
  'consoleChat.skills.agentic.thinking',
  'consoleChat.skills.agentic.organizing',
  'consoleChat.skills.agentic.preparing',
  'consoleChat.skills.agentic.checkingTools',
] as const;

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

function formatDebugValue(value: unknown): string | null {
  if (value === undefined || value === null || value === '') return null;
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
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
          {event.detail ? (
            <div className="mb-2 whitespace-pre-wrap break-words text-muted-foreground">
              {event.detail}
            </div>
          ) : null}
          <AIChatSkillResultSummary result={event.item.invocation.result} className="mb-2" />
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
  return String(item.event.reason ?? item.event.governance?.reason ?? '').trim();
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

function governanceAssetCount(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[]
): number {
  if (assets.length > 0) return assets.length;
  const modelFeedback = governanceModelFeedback(item);
  const assetOperationAudit = governanceAssetOperationAudit(item);
  for (const source of [assetOperationAudit, modelFeedback, item.event.governance, item.event]) {
    const count = governanceRecordNumber(source, ['asset_count', 'assetCount']);
    if (count !== null) return count;
  }
  return 0;
}

function governanceAssetDisplayName(asset: AIChatToolGovernanceAssetRef): string {
  const id = governanceStringValue(asset.id);
  const assetType = governanceStringValue(asset.type)?.toLowerCase();
  const fileName =
    governanceRecordString(asset, ['filename', 'file_name']) ??
    governanceRecordString(asset.metadata, ['filename', 'file_name']);
  if (fileName) return fileName;
  const displayName =
    governanceRecordString(asset, ['name', 'title', 'label']) ??
    governanceRecordString(asset.metadata, ['name', 'title', 'label']);
  if (displayName && displayName !== id && !looksLikeOpaqueAssetID(displayName)) {
    return displayName;
  }
  if (assetType === 'file') return 'file';
  return id ?? 'asset';
}

function looksLikeOpaqueAssetID(value: string): boolean {
  const normalized = value.trim();
  if (!normalized) return false;
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(normalized)) {
    return true;
  }
  if (/^(file|upload_file|asset)[_-][a-z0-9_-]{8,}$/i.test(normalized)) {
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
    if (!part || seen.has(part)) continue;
    seen.add(part);
    out.push(part);
  }
  return out;
}

function governanceAssetMeta(asset: AIChatToolGovernanceAssetRef, t: WebappTranslator): string {
  const workspaceID =
    governanceStringValue(asset.workspace_id) ??
    governanceRecordString(asset.metadata, ['workspace_id', 'workspaceId']);
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
    workspaceID ? `${t('consoleChat.governance.fields.workspace')} ${workspaceID}` : null,
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

function governanceAssetWorkspaceIDs(assets: AIChatToolGovernanceAssetRef[]): string[] {
  const seen = new Set<string>();
  const values: string[] = [];
  for (const asset of assets) {
    const workspaceID =
      governanceStringValue(asset.workspace_id) ??
      governanceRecordString(asset.metadata, ['workspace_id', 'workspaceId']);
    if (!workspaceID || seen.has(workspaceID)) continue;
    seen.add(workspaceID);
    values.push(workspaceID);
  }
  return values;
}

function governanceWorkspaceIDs(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[]
): string[] {
  const seen = new Set<string>();
  const values: string[] = [];
  const append = (workspaceID: string | null) => {
    if (!workspaceID || seen.has(workspaceID)) return;
    seen.add(workspaceID);
    values.push(workspaceID);
  };

  for (const workspaceID of governanceAssetWorkspaceIDs(assets)) {
    append(workspaceID);
  }
  for (const source of [
    governanceAssetOperationAudit(item),
    item.event,
    item.event.governance,
    item.event.governance?.approval_event,
    governanceApprovalEvent(item),
  ]) {
    append(governanceRecordString(source, ['workspace_id', 'workspaceId']));
  }
  return values;
}

function governanceEventString(
  item: GovernanceTimelineItem,
  keys: readonly string[]
): string | null {
  const approvalEvent = governanceApprovalEvent(item);
  const modelFeedback = governanceModelFeedback(item);
  for (const source of [
    item.event,
    item.event.governance,
    item.event.governance?.manifest,
    approvalEvent,
    modelFeedback,
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
  if (explicitIntent) return explicitIntent;
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
  switch (assetType) {
    case 'file':
      return t('consoleChat.governance.assetTypes.file');
    default:
      return assetType;
  }
}

function governanceSummaryRows(
  item: GovernanceTimelineItem,
  assets: AIChatToolGovernanceAssetRef[],
  t: WebappTranslator
) {
  const workspaces = governanceWorkspaceIDs(item, assets);
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
  const workspaceValue =
    workspaces.length > 0 ? workspaces.join(', ') : shouldSurfaceUnknowns ? unknown : null;
  const reversible = governanceBooleanLabel(governanceEventBoolean(item, ['reversible']), t);
  return [
    ['intent', governanceIntentText(item, assets, assetCount, t)],
    ['assetCount', assetCount > 0 ? String(assetCount) : null],
    ['workspace', workspaceValue],
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
    [
      'correlationId',
      item.event.correlation_id ??
        item.event.governance?.correlation_id ??
        approvalEvent?.correlation_id,
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
  t: WebappTranslator
): string {
  const effect = governanceEventString(item, ['effect'])?.toLowerCase();
  const assetType = governanceEventString(item, ['asset_type'])?.toLowerCase();
  const count = Math.max(assetCount, assets.length, 1);
  const singleAssetName = assets.length === 1 ? governanceAssetDisplayName(assets[0]) : null;

  if (effect === 'delete' && assetType === 'file') {
    return singleAssetName
      ? t('consoleChat.governance.approvalPanel.fileDeleteOne', { name: singleAssetName })
      : t('consoleChat.governance.approvalPanel.fileDeleteMany', { count });
  }

  if (effect && assetType && singleAssetName) {
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
  const assetCount = governanceAssetCount(item, approvalAssets);
  const notice = governanceNoticeText(item, assetCount, t);
  const summaryRows: ToolGovernanceDisplayRow[] = governanceSummaryRows(
    item,
    approvalAssets,
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
  const assets: ToolGovernanceDisplayAsset[] = approvalAssets.map((asset, index) => {
    const key = `${governanceStringValue(asset.id) ?? governanceAssetDisplayName(asset)}-${index}`;
    return {
      key,
      name: governanceAssetDisplayName(asset),
      meta: governanceAssetMeta(asset, t) || undefined,
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

  const actionSentence = governanceActionSentence(item, approvalAssets, assetCount, t);

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

  const events = useMemo(
    () =>
      timeline
        .filter(
          item =>
            !isGovernedSkillEvent(item, governanceCorrelationIds) &&
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
          const skill =
            skillDisplayById[skillId] ?? getFallbackAIChatSkillDisplayInfo(skillId, locale);
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
        }),
    [enableToolGovernanceApprovals, governanceCorrelationIds, locale, skillDisplayById, t, timeline]
  );

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
        <div className="space-y-2">
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
                    className="md-viewer break-words"
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
                    className="md-viewer break-words"
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
