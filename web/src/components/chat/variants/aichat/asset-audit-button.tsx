'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import type { ComponentProps } from 'react';
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  History,
  Loader2,
  RefreshCw,
  ShieldCheck,
  XCircle,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { useAIChatAssetOperationAudits } from '@/hooks/aichat/use-aichat-asset-operation-audits';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type {
  AIChatAssetOperationAuditRecord,
  AIChatToolGovernanceAssetRef,
} from '@/services/types/aichat';
import {
  getAIChatSkillToolDisplayName,
  getFallbackAIChatSkillDisplayInfo,
} from '@/components/chat/variants/aichat/skill-display';

interface AIChatAssetAuditButtonProps {
  conversationId?: string | null;
  enabled?: boolean;
  refreshKey?: string;
  className?: string;
}

type BadgeVariant = ComponentProps<typeof Badge>['variant'];

const AUDIT_PAGE_SIZE = 50;
const AGENT_BINDING_AUDIT_TOOL_NAMES = new Set([
  'replace_agent_knowledge_bindings',
  'replace_agent_database_bindings',
  'replace_agent_workflow_bindings',
  'agent.replace_knowledge_bindings',
  'agent.replace_database_bindings',
  'agent.replace_workflow_bindings',
]);

export function AIChatAssetAuditButton({
  conversationId,
  enabled = true,
  refreshKey,
  className,
}: AIChatAssetAuditButtonProps) {
  const t = useT('webapp');
  const [open, setOpen] = useState(false);
  const canLoad = enabled && Boolean(conversationId);
  const auditQuery = useAIChatAssetOperationAudits(
    conversationId,
    { page: 1, limit: AUDIT_PAGE_SIZE },
    { enabled: open && canLoad }
  );
  const lastRefreshKeyRef = useRef<string | undefined>(refreshKey);
  const auditData = auditQuery.data;
  const { refetch } = auditQuery;
  const records = useMemo(() => auditData?.data ?? [], [auditData?.data]);

  useEffect(() => {
    if (!open || !canLoad) {
      lastRefreshKeyRef.current = refreshKey;
      return;
    }
    if (lastRefreshKeyRef.current === refreshKey) return;
    lastRefreshKeyRef.current = refreshKey;
    void refetch();
  }, [canLoad, open, refetch, refreshKey]);

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <Button
        type="button"
        variant="ghost"
        isIcon
        className={cn('relative size-8 text-muted-foreground', className)}
        disabled={!canLoad}
        onClick={() => setOpen(true)}
        aria-label={
          canLoad
            ? t('consoleChat.governance.audit.action')
            : t('consoleChat.governance.audit.noConversation')
        }
        title={
          canLoad
            ? t('consoleChat.governance.audit.action')
            : t('consoleChat.governance.audit.noConversation')
        }
      >
        <History className="size-4" />
        {records.length > 0 ? (
          <span className="absolute right-1 top-1 size-1.5 rounded-full bg-primary" />
        ) : null}
      </Button>
      <SheetContent side="right" className="flex w-full flex-col p-0 sm:max-w-xl">
        <SheetHeader className="border-b px-4 py-4 pr-11 text-left">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <SheetTitle className="text-base">
                {t('consoleChat.governance.audit.title')}
              </SheetTitle>
              <SheetDescription className="mt-1">
                {t('consoleChat.governance.audit.description')}
              </SheetDescription>
            </div>
            <Button
              type="button"
              variant="ghost"
              isIcon
              className="size-8 shrink-0 text-muted-foreground"
              onClick={() => void refetch()}
              disabled={!canLoad || auditQuery.isFetching}
              aria-label={t('consoleChat.governance.audit.refresh')}
              title={t('consoleChat.governance.audit.refresh')}
            >
              <RefreshCw className={cn('size-4', auditQuery.isFetching && 'animate-spin')} />
            </Button>
          </div>
        </SheetHeader>

        <ScrollArea className="min-h-0 flex-1">
          <div className="space-y-3 p-4">
            {auditQuery.isLoading ? <AuditLoading label={t('consoleChat.governance.audit.loading')} /> : null}
            {auditQuery.isError ? (
              <AuditEmptyState
                tone="error"
                title={t('consoleChat.governance.audit.loadFailed')}
                description={
                  auditQuery.error instanceof Error
                    ? auditQuery.error.message
                    : t('consoleChat.governance.audit.loadFailed')
                }
              />
            ) : null}
            {!auditQuery.isLoading && !auditQuery.isError && records.length === 0 ? (
              <AuditEmptyState
                title={t('consoleChat.governance.audit.emptyTitle')}
                description={t('consoleChat.governance.audit.emptyDescription')}
              />
            ) : null}
            {!auditQuery.isError
              ? records.map(record => <AuditRecordCard key={record.id} record={record} />)
              : null}
            {auditData?.has_more ? (
              <p className="px-1 text-xs text-muted-foreground">
                {t('consoleChat.governance.audit.firstPageOnly', {
                  count: records.length,
                  total: auditData.total,
                })}
              </p>
            ) : null}
          </div>
        </ScrollArea>
      </SheetContent>
    </Sheet>
  );
}

function AuditRecordCard({ record }: { record: AIChatAssetOperationAuditRecord }) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const status = record.approval_status || record.governance_status || record.status || '';
  const time = formatAuditTime(record.resolved_at ?? record.created_at ?? record.message_created_at);
  const rawAssets = record.assets ?? [];
  const assets = auditDisplayAssets(record, rawAssets);
  const assetCount = auditAssetCount(record, rawAssets);
  const toolLabel = formatToolLabel(record, t, locale);
  const effect = record.effect ? formatEffect(record.effect, t) : null;
  const risk = record.risk_level ? formatRisk(record.risk_level, t) : null;
  const action = record.action ? formatAuditAction(record.action, t) : null;
  const resolvedBy = readableAuditActor(record.resolved_by);
  const grantScope = formatGrantScope(record.session_grant ?? record.approved_grant, t);
  const source = formatAuditSource(record.source, t);
  const reason = record.reason ? formatAuditReason(record.reason, t) : null;
  const resolutionItems = [
    action ? t('consoleChat.governance.audit.resolutionAction', { action }) : null,
    resolvedBy ? t('consoleChat.governance.audit.resolvedBy', { account: resolvedBy }) : null,
    record.remember_for_session ? t('consoleChat.governance.audit.rememberedSession') : null,
    grantScope ? t('consoleChat.governance.audit.sessionGrant', { scope: grantScope }) : null,
  ].filter((item): item is string => Boolean(item));

  return (
    <article className="overflow-hidden rounded-lg border bg-background p-3 shadow-sm">
      <div className="flex items-start gap-3">
        <div className={cn('mt-0.5 rounded-full p-1.5', statusToneClass(status))}>
          {statusIcon(status)}
        </div>
        <div className="min-w-0 flex-1 space-y-2 overflow-hidden">
          <div className="flex min-w-0 flex-col gap-1.5 sm:flex-row sm:items-start sm:justify-between">
            <h3 className="min-w-0 break-words text-sm font-medium text-foreground">{toolLabel}</h3>
            <div className="flex shrink-0 flex-wrap gap-1.5">
              <Badge variant={statusBadgeVariant(status)}>{formatStatus(status, t)}</Badge>
              {risk ? <Badge variant={riskBadgeVariant(record.risk_level)}>{risk}</Badge> : null}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {effect ? <span>{effect}</span> : null}
            <span>{t('consoleChat.governance.audit.assetCount', { count: assetCount })}</span>
            {time ? (
              <span>{t('consoleChat.governance.audit.recordTime', { time })}</span>
            ) : null}
          </div>
          {assets.length > 0 ? <AuditAssets assets={assets} /> : null}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
            <span>{t('consoleChat.governance.audit.source', { source })}</span>
          </div>
          {resolutionItems.length > 0 ? (
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
              {resolutionItems.map(item => (
                <span key={item}>{item}</span>
              ))}
            </div>
          ) : null}
          {reason ? (
            <p className="text-xs text-muted-foreground">
              <span className="font-medium text-foreground">
                {t('consoleChat.governance.fields.reason')}
              </span>
              <span className="ml-1">{reason}</span>
            </p>
          ) : null}
        </div>
      </div>
    </article>
  );
}

function normalizeAuditToken(value: unknown): string {
  return stringValue(value)?.toLowerCase().replace(/-/g, '_') ?? '';
}

function isAgentBindingAuditRecord(record: AIChatAssetOperationAuditRecord): boolean {
  const skillId = normalizeAuditToken(record.skill_id);
  const toolName = normalizeAuditToken(record.tool_name ?? record.tool_id);
  return (
    (skillId === 'agent_management' || toolName.startsWith('agent.')) &&
    AGENT_BINDING_AUDIT_TOOL_NAMES.has(toolName)
  );
}

function isAuditBindingOwnerAsset(asset: AIChatToolGovernanceAssetRef): boolean {
  const metadata =
    asset.metadata && typeof asset.metadata === 'object' && !Array.isArray(asset.metadata)
      ? asset.metadata
      : {};
  return (
    normalizeAuditToken(asset.type) === 'agent' ||
    metadata.binding_owner === true ||
    asset.binding_owner === true
  );
}

function auditDisplayAssets(
  record: AIChatAssetOperationAuditRecord,
  assets: AIChatToolGovernanceAssetRef[]
): AIChatToolGovernanceAssetRef[] {
  if (!isAgentBindingAuditRecord(record)) return assets;
  const targetType = normalizeAuditToken(record.asset_type);
  const targets = assets.filter(asset => {
    if (isAuditBindingOwnerAsset(asset)) return false;
    const assetType = normalizeAuditToken(asset.type);
    return !targetType || assetType === targetType;
  });
  return targets.length > 0 ? targets : assets.filter(asset => !isAuditBindingOwnerAsset(asset));
}

function auditAssetCount(
  record: AIChatAssetOperationAuditRecord,
  assets: AIChatToolGovernanceAssetRef[]
): number {
  const displayAssets = auditDisplayAssets(record, assets);
  if (displayAssets.length > 0) return displayAssets.length;
  const rawCount = record.asset_count ?? assets.length;
  return isAgentBindingAuditRecord(record) ? Math.max(0, rawCount - 1) : rawCount;
}

function AuditAssets({ assets }: { assets: AIChatToolGovernanceAssetRef[] }) {
  const t = useT('webapp');
  const visibleAssets = assets.slice(0, 3);
  const hiddenCount = Math.max(assets.length - visibleAssets.length, 0);

  return (
    <div className="space-y-1 rounded-md bg-muted/40 p-2">
      {visibleAssets.map((asset, index) => {
        const meta = assetMeta(asset, t);
        return (
          <div key={`${asset.id ?? asset.name ?? asset.filename ?? index}`} className="min-w-0">
            <p className="break-words text-xs font-medium text-foreground">
              {assetDisplayName(asset, t)}
            </p>
            {meta ? <p className="break-words text-[11px] text-muted-foreground">{meta}</p> : null}
          </div>
        );
      })}
      {hiddenCount > 0 ? (
        <p className="text-[11px] text-muted-foreground">
          {t('consoleChat.governance.audit.moreAssets', { count: hiddenCount })}
        </p>
      ) : null}
    </div>
  );
}

function AuditLoading({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 rounded-lg border bg-muted/20 p-3 text-sm text-muted-foreground">
      <Loader2 className="size-4 animate-spin" />
      {label}
    </div>
  );
}

function AuditEmptyState({
  title,
  description,
  tone = 'empty',
}: {
  title: string;
  description: string;
  tone?: 'empty' | 'error';
}) {
  return (
    <div className="rounded-lg border bg-muted/20 p-4 text-sm">
      <div className="flex items-center gap-2 font-medium text-foreground">
        {tone === 'error' ? (
          <AlertTriangle className="size-4 text-warning" />
        ) : (
          <Clock3 className="size-4 text-muted-foreground" />
        )}
        {title}
      </div>
      <p className="mt-1 text-xs text-muted-foreground">{description}</p>
    </div>
  );
}

function formatToolLabel(
  record: AIChatAssetOperationAuditRecord,
  t: ReturnType<typeof useT<'webapp'>>,
  locale: string
): string {
  const skillId = record.skill_id?.trim() ?? '';
  const toolName = (record.tool_name || record.tool_id || '').trim();
  const skill = formatSkillLabel(skillId, locale, t);
  const tool =
    localizedToolName(skillId, toolName, locale) ||
    formatAuditTokenLabel(toolName) ||
    t('consoleChat.governance.audit.unknownTool');
  return t('consoleChat.governance.audit.toolSummary', { skill, tool });
}

function formatSkillLabel(
  skillId: string,
  locale: string,
  t: ReturnType<typeof useT<'webapp'>>
): string {
  if (!skillId || looksLikeOpaqueAuditAssetID(skillId)) {
    return t('consoleChat.skills.trace.unknownSkill');
  }
  const display = getFallbackAIChatSkillDisplayInfo(skillId, locale).label;
  return display && display !== skillId ? display : formatAuditTokenLabel(skillId);
}

function formatStatus(status: string, t: ReturnType<typeof useT<'webapp'>>): string {
  switch (status) {
    case 'approved':
      return t('consoleChat.governance.audit.statuses.approved');
    case 'rejected':
      return t('consoleChat.governance.audit.statuses.rejected');
    case 'pending':
      return t('consoleChat.governance.audit.statuses.pending');
    case 'needs_approval':
      return t('consoleChat.governance.audit.statuses.needsApproval');
    case 'denied':
      return t('consoleChat.governance.audit.statuses.denied');
    case 'blocked':
      return t('consoleChat.governance.audit.statuses.blocked');
    case 'needs_resolution':
      return t('consoleChat.governance.audit.statuses.needsResolution');
    case 'allowed':
    case 'success':
      return t('consoleChat.governance.audit.statuses.allowed');
    case 'error':
      return t('consoleChat.governance.audit.statuses.error');
    default:
      return status || t('consoleChat.governance.values.unknown');
  }
}

function formatEffect(effect: string, t: ReturnType<typeof useT<'webapp'>>): string {
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

function formatRisk(risk: string, t: ReturnType<typeof useT<'webapp'>>): string {
  switch (risk) {
    case 'low':
      return t('consoleChat.governance.risks.low');
    case 'medium':
      return t('consoleChat.governance.risks.medium');
    case 'high':
      return t('consoleChat.governance.risks.high');
    case 'critical':
      return t('consoleChat.governance.risks.critical');
    default:
      return risk;
  }
}

function formatAuditAction(action: string, t: ReturnType<typeof useT<'webapp'>>): string {
  switch (action) {
    case 'approve':
    case 'approved':
      return t('consoleChat.governance.approve');
    case 'reject':
    case 'rejected':
      return t('consoleChat.governance.reject');
    default:
      return action;
  }
}

function formatGrantScope(
  grant: Record<string, unknown> | undefined,
  t: ReturnType<typeof useT<'webapp'>>
): string | null {
  if (!grant) return null;
  const effect = stringValue(grant.effect);
  const assetType = stringValue(grant.asset_type);
  const riskLevel = stringValue(grant.risk_level);
  const parts = [
    effect ? formatEffect(effect, t) : null,
    assetType ? formatAssetType(assetType, t) : null,
    riskLevel ? formatRisk(riskLevel, t) : null,
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(' / ') : null;
}

function statusBadgeVariant(status: string): BadgeVariant {
  if (status === 'approved' || status === 'allowed' || status === 'success') return 'success';
  if (status === 'rejected' || status === 'denied' || status === 'blocked' || status === 'error') {
    return 'destructive';
  }
  if (status === 'pending' || status === 'needs_approval' || status === 'needs_resolution') {
    return 'warning';
  }
  return 'secondary';
}

function riskBadgeVariant(risk: string | undefined): BadgeVariant {
  if (risk === 'critical' || risk === 'high') return 'destructive';
  if (risk === 'medium') return 'warning';
  if (risk === 'low') return 'success';
  return 'secondary';
}

function statusToneClass(status: string): string {
  if (status === 'approved' || status === 'allowed' || status === 'success') {
    return 'bg-success/15 text-success';
  }
  if (status === 'rejected' || status === 'denied' || status === 'blocked' || status === 'error') {
    return 'bg-destructive/10 text-destructive';
  }
  return 'bg-warning/15 text-warning';
}

function statusIcon(status: string) {
  if (status === 'approved' || status === 'allowed' || status === 'success') {
    return <CheckCircle2 className="size-4" />;
  }
  if (status === 'rejected' || status === 'denied' || status === 'blocked' || status === 'error') {
    return <XCircle className="size-4" />;
  }
  return <ShieldCheck className="size-4" />;
}

function assetDisplayName(asset: AIChatToolGovernanceAssetRef, t: ReturnType<typeof useT<'webapp'>>) {
  const id = stringValue(asset.id);
  const assetType = stringValue(asset.type)?.toLowerCase();
  const fileName = stringValue(asset.filename) || stringValue(asset.file_name);
  if (fileName) return fileName;
  const displayName = stringValue(asset.name) || stringValue(asset.title) || stringValue(asset.label);
  if (displayName && displayName !== id && !looksLikeOpaqueAuditAssetID(displayName)) {
    return displayName;
  }
  if (assetType) return formatAssetType(assetType, t);
  return id || t('consoleChat.governance.audit.unknownAsset');
}

function formatAssetType(assetType: string, t: ReturnType<typeof useT<'webapp'>>): string {
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
    case 'model':
    case 'llm_model':
    case 'llm-model':
      return t('consoleChat.governance.assetTypes.model');
    default:
      return formatAuditTokenLabel(assetType);
  }
}

function looksLikeOpaqueAuditAssetID(value: string): boolean {
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

function assetMeta(asset: AIChatToolGovernanceAssetRef, t: ReturnType<typeof useT<'webapp'>>) {
  const assetType = stringValue(asset.type);
  const parts = [
    assetType ? formatAssetType(assetType, t) : null,
    asset.source ? formatAuditSource(asset.source, t) : null,
  ].filter(Boolean);
  return parts.join(' / ');
}

function localizedToolName(skillId: string, toolName: string, locale: string): string {
  if (!toolName) return '';
  const direct = getAIChatSkillToolDisplayName(skillId, toolName, locale);
  if (direct && direct !== toolName) return direct;
  const normalizedToolName = toolName.startsWith('agent.')
    ? toolName.slice('agent.'.length)
    : toolName;
  const normalized = getAIChatSkillToolDisplayName(skillId, normalizedToolName, locale);
  return normalized && normalized !== normalizedToolName ? normalized : '';
}

function formatAuditSource(source: unknown, t: ReturnType<typeof useT<'webapp'>>): string {
  switch (normalizeAuditToken(source)) {
    case 'tool_governance_decision':
      return t('consoleChat.governance.audit.sources.toolGovernanceDecision');
    case 'skill_invocation':
      return t('consoleChat.governance.audit.sources.skillInvocation');
    case 'tool_arguments':
      return t('consoleChat.governance.audit.sources.toolArguments');
    case 'runtime_asset':
    case 'page_context':
    case 'context':
      return t('consoleChat.governance.audit.sources.pageContext');
    default: {
      const value = stringValue(source);
      return value ? formatAuditTokenLabel(value) : t('consoleChat.governance.values.unknown');
    }
  }
}

function formatAuditReason(reason: string, t: ReturnType<typeof useT<'webapp'>>): string {
  const normalized = reason.trim().toLowerCase().replace(/\s+/g, ' ');
  switch (normalized) {
    case 'allowed by basic permission tier':
      return t('consoleChat.governance.audit.reasons.allowedBasicTier');
    case 'allowed by manifest approval policy':
      return t('consoleChat.governance.audit.reasons.allowedManifestPolicy');
    case 'tool manifest requires approval':
      return t('consoleChat.governance.audit.reasons.manifestRequiresApproval');
    case 'permission tier requires user approval for this tool':
      return t('consoleChat.governance.audit.reasons.permissionTierRequiresApproval');
    case 'matched approved session grant':
    case 'allowed by session grant':
      return t('consoleChat.governance.audit.reasons.matchedSessionGrant');
    default:
      return reason;
  }
}

function readableAuditActor(value: unknown): string | null {
  const actor = stringValue(value);
  if (!actor || looksLikeOpaqueAuditAssetID(actor)) return null;
  return actor;
}

function formatAuditTokenLabel(value: string): string {
  return value
    .trim()
    .replace(/^[a-z]+\./i, '')
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function stringValue(value: unknown): string | null {
  return typeof value === 'string' && value.trim() ? value.trim() : null;
}

function formatAuditTime(timestamp: number | string | undefined): string | null {
  if (!timestamp) return null;
  const date =
    typeof timestamp === 'string'
      ? new Date(timestamp)
      : new Date(timestamp > 1_000_000_000_000 ? timestamp : timestamp * 1000);
  if (Number.isNaN(date.getTime())) return null;
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}
