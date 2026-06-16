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
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type {
  AIChatAssetOperationAuditRecord,
  AIChatToolGovernanceAssetRef,
} from '@/services/types/aichat';

interface AIChatAssetAuditButtonProps {
  conversationId?: string | null;
  enabled?: boolean;
  refreshKey?: string;
}

type BadgeVariant = ComponentProps<typeof Badge>['variant'];

const AUDIT_PAGE_SIZE = 50;

export function AIChatAssetAuditButton({
  conversationId,
  enabled = true,
  refreshKey,
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
        className="relative size-8 text-muted-foreground"
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
      <SheetContent side="right" className="flex w-full flex-col p-0 sm:max-w-md">
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
  const status = record.approval_status || record.governance_status || record.status || '';
  const time = formatAuditTime(record.resolved_at ?? record.created_at ?? record.message_created_at);
  const assets = record.assets ?? [];
  const assetCount = record.asset_count ?? assets.length;
  const toolLabel = formatToolLabel(record, t);
  const effect = record.effect ? formatEffect(record.effect, t) : null;
  const risk = record.risk_level ? formatRisk(record.risk_level, t) : null;
  const action = record.action ? formatAuditAction(record.action, t) : null;
  const resolvedBy = stringValue(record.resolved_by);
  const grantScope = formatGrantScope(record.session_grant ?? record.approved_grant, t);
  const resolutionItems = [
    action ? t('consoleChat.governance.audit.resolutionAction', { action }) : null,
    resolvedBy ? t('consoleChat.governance.audit.resolvedBy', { account: resolvedBy }) : null,
    record.remember_for_session ? t('consoleChat.governance.audit.rememberedSession') : null,
    grantScope ? t('consoleChat.governance.audit.sessionGrant', { scope: grantScope }) : null,
  ].filter((item): item is string => Boolean(item));

  return (
    <article className="rounded-lg border bg-background p-3 shadow-sm">
      <div className="flex items-start gap-3">
        <div className={cn('mt-0.5 rounded-full p-1.5', statusToneClass(status))}>
          {statusIcon(status)}
        </div>
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-1.5">
            <h3 className="min-w-0 truncate text-sm font-medium text-foreground">{toolLabel}</h3>
            <Badge variant={statusBadgeVariant(status)}>{formatStatus(status, t)}</Badge>
            {risk ? <Badge variant={riskBadgeVariant(record.risk_level)}>{risk}</Badge> : null}
          </div>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            {effect ? <span>{effect}</span> : null}
            <span>{t('consoleChat.governance.audit.assetCount', { count: assetCount })}</span>
            {time ? (
              <span>{t('consoleChat.governance.audit.recordTime', { time })}</span>
            ) : null}
          </div>
          {assets.length > 0 ? <AuditAssets assets={assets} /> : null}
          {resolutionItems.length > 0 ? (
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
              {resolutionItems.map(item => (
                <span key={item}>{item}</span>
              ))}
            </div>
          ) : null}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
            <span>{t('consoleChat.governance.audit.source', { source: record.source })}</span>
            <span className="font-mono">{record.correlation_id}</span>
          </div>
          {record.reason ? (
            <p className="text-xs text-muted-foreground">
              <span className="font-medium text-foreground">
                {t('consoleChat.governance.fields.reason')}
              </span>
              <span className="ml-1">{record.reason}</span>
            </p>
          ) : null}
        </div>
      </div>
    </article>
  );
}

function AuditAssets({ assets }: { assets: AIChatToolGovernanceAssetRef[] }) {
  const t = useT('webapp');
  const visibleAssets = assets.slice(0, 3);
  const hiddenCount = Math.max(assets.length - visibleAssets.length, 0);

  return (
    <div className="space-y-1 rounded-md bg-muted/40 p-2">
      {visibleAssets.map((asset, index) => (
        <div key={`${asset.id ?? asset.name ?? asset.filename ?? index}`} className="min-w-0">
          <p className="truncate text-xs font-medium text-foreground">
            {assetDisplayName(asset, t)}
          </p>
          <p className="truncate text-[11px] text-muted-foreground">{assetMeta(asset, t)}</p>
        </div>
      ))}
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
  t: ReturnType<typeof useT<'webapp'>>
): string {
  const skill = record.skill_id || t('consoleChat.skills.trace.unknownSkill');
  const tool = record.tool_name || record.tool_id || t('consoleChat.governance.audit.unknownTool');
  return t('consoleChat.governance.toolLabel', { skill, tool });
}

function formatStatus(status: string, t: ReturnType<typeof useT<'webapp'>>): string {
  switch (status) {
    case 'approved':
      return t('consoleChat.governance.approved');
    case 'rejected':
      return t('consoleChat.governance.rejected');
    case 'pending':
      return t('consoleChat.governance.audit.pending');
    case 'needs_approval':
      return t('consoleChat.governance.needsApproval');
    case 'denied':
      return t('consoleChat.governance.denied');
    case 'blocked':
      return t('consoleChat.governance.blocked');
    case 'needs_resolution':
      return t('consoleChat.governance.needsResolution');
    case 'allowed':
    case 'success':
      return t('consoleChat.governance.allowed');
    case 'error':
      return t('consoleChat.skills.trace.error');
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
  const toolID = stringValue(grant.tool_id);
  const effect = stringValue(grant.effect);
  const assetType = stringValue(grant.asset_type);
  const riskLevel = stringValue(grant.risk_level);
  const parts = [
    toolID,
    effect ? formatEffect(effect, t) : null,
    assetType,
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
  if (assetType === 'file') return t('consoleChat.governance.assetTypes.file');
  return id || t('consoleChat.governance.audit.unknownAsset');
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
  const parts = [
    stringValue(asset.type),
    asset.workspace_id
      ? `${t('consoleChat.governance.fields.workspace')} ${asset.workspace_id}`
      : null,
    asset.source ? `${t('consoleChat.governance.fields.assetSource')} ${asset.source}` : null,
  ].filter(Boolean);
  return parts.join(' / ');
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
