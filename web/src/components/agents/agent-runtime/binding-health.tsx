'use client';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useT } from '@/i18n';
import type {
  AgentBindingHealth,
  AgentBindingHealthItem,
  AgentBindingStatus,
  AgentBindingType,
} from '@/services/types/agent';

function statusLabel(
  t: ReturnType<typeof useT<'agents.agentRuntime'>>,
  status: AgentBindingStatus
) {
  if (status === 'active') return t('bindingHealth.status.active');
  if (status === 'suspended') return t('bindingHealth.status.suspended');
  return t('bindingHealth.status.unavailable');
}

function typeLabel(t: ReturnType<typeof useT<'agents.agentRuntime'>>, type: AgentBindingType) {
  if (type === 'skill') return t('bindingHealth.types.skill');
  if (type === 'knowledge_dataset') return t('bindingHealth.types.knowledge_dataset');
  if (type === 'database') return t('bindingHealth.types.database');
  if (type === 'database_table') return t('bindingHealth.types.database_table');
  return t('bindingHealth.types.workflow');
}

function accessModeLabel(
  t: ReturnType<typeof useT<'agents.agentRuntime'>>,
  mode: NonNullable<AgentBindingHealthItem['access_mode']>
) {
  if (mode === 'read') return t('bindingHealth.accessModes.read');
  if (mode === 'write') return t('bindingHealth.accessModes.write');
  return t('bindingHealth.accessModes.execute');
}

function reasonLabel(t: ReturnType<typeof useT<'agents.agentRuntime'>>, reason: string) {
  if (reason === 'organization_skill_suspended') {
    return t('bindingHealth.reasons.organizationSkillSuspended');
  }
  if (reason === 'resource_deleted_or_missing') {
    return t('bindingHealth.reasons.resourceDeletedOrMissing');
  }
  if (reason === 'resource_moved_workspace') {
    return t('bindingHealth.reasons.resourceMovedWorkspace');
  }
  if (reason === 'authorization_revoked') {
    return t('bindingHealth.reasons.authorizationRevoked');
  }
  if (reason === 'resolution_failed') {
    return t('bindingHealth.reasons.resolutionFailed');
  }
  return reason;
}

function suggestionLabel(t: ReturnType<typeof useT<'agents.agentRuntime'>>, suggestion: string) {
  if (suggestion === 'remove_or_replace_binding') {
    return t('bindingHealth.suggestions.removeOrReplace');
  }
  if (suggestion === 'restore_access_or_remove_binding') {
    return t('bindingHealth.suggestions.restoreOrRemove');
  }
  return suggestion;
}

export function AgentBindingHealthBadge({ item }: { item?: AgentBindingHealthItem }) {
  const t = useT('agents.agentRuntime');
  if (!item || item.status === 'active') return null;

  return (
    <Badge variant={item.status === 'unavailable' ? 'destructive' : 'warning'}>
      {reasonLabel(t, item.reason)}
    </Badge>
  );
}

export function AgentBindingHealthItemRow({ item }: { item: AgentBindingHealthItem }) {
  const t = useT('agents.agentRuntime');
  return (
    <div className="rounded-md border bg-background p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate text-sm font-medium">
            {item.display_name || item.resource_id}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <Badge variant="outline">{typeLabel(t, item.binding_type)}</Badge>
            {item.access_mode ? (
              <Badge variant="subtle">{accessModeLabel(t, item.access_mode)}</Badge>
            ) : null}
          </div>
        </div>
        <Badge
          variant={
            item.status === 'unavailable'
              ? 'destructive'
              : item.status === 'suspended'
                ? 'warning'
                : 'success'
          }
        >
          {statusLabel(t, item.status)}
        </Badge>
      </div>
      <p className="mt-2 text-xs leading-5 text-muted-foreground">{reasonLabel(t, item.reason)}</p>
      {item.suggestion ? (
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          {t('bindingHealth.suggestion', { suggestion: suggestionLabel(t, item.suggestion) })}
        </p>
      ) : null}
      <p className="mt-1 truncate text-[11px] text-muted-foreground/70">
        {t('bindingHealth.resourceId', { id: item.resource_id })}
      </p>
    </div>
  );
}

export function AgentSuspendedBindingsDialog({
  open,
  health,
  isPublishing,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  health?: AgentBindingHealth;
  isPublishing: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  const t = useT('agents.agentRuntime');
  const suspendedItems = health?.items.filter(item => item.status === 'suspended') ?? [];
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{t('bindingHealth.publishSuspendedTitle')}</DialogTitle>
          <DialogDescription>
            {t('bindingHealth.publishSuspendedDescription', { count: suspendedItems.length })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[420px] space-y-2">
          {suspendedItems.map((item, index) => (
            <AgentBindingHealthItemRow
              key={`${item.binding_type}:${item.parent_resource_id ?? ''}:${item.resource_id}:${index}`}
              item={item}
            />
          ))}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t('bindingHealth.cancel')}
          </Button>
          <Button type="button" onClick={onConfirm} disabled={isPublishing}>
            {isPublishing ? t('bindingHealth.publishing') : t('bindingHealth.publishAnyway')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
