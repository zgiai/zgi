'use client';

import Link from 'next/link';
import { AlertTriangle, CheckCircle2, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
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
import {
  usePersonalConnectionRemovalImpact,
  useRemovePersonalConnection,
} from '@/hooks/connectors/use-connectors';
import { useT } from '@/i18n';
import { useChatConnectorPreferences } from '@/hooks/connectors/use-chat-connector-preferences';
import type { ConnectorTargetBinding, ConnectorTargetType } from '@/services/types/connector';
import {
  connectorBindingTargetHref,
  getConnectionRemovalMode,
  sortConnectionBindings,
} from './connection-management-model';

interface ConnectionRemovalDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  connectionId: string;
  onRemoved: () => void;
}

export function ConnectionRemovalDialog({
  open,
  onOpenChange,
  connectionId,
  onRemoved,
}: ConnectionRemovalDialogProps) {
  const t = useT('connectors');
  const impactQuery = usePersonalConnectionRemovalImpact(connectionId, open);
  const removeMutation = useRemovePersonalConnection(connectionId);
  const preferencesQuery = useChatConnectorPreferences(open);
  const isDefault = Object.values(preferencesQuery.data?.data.defaults ?? {}).some(
    value => value.mode === 'selected' && value.connection_id === connectionId
  );
  const impact = impactQuery.isError ? undefined : impactQuery.data?.data;
  const mode = impact ? getConnectionRemovalMode(impact) : 'blocked';
  const affectedTargets = impact ? sortConnectionBindings(impact.affected_targets) : [];

  const surfaceLabel = (type: ConnectorTargetType) => {
    if (type === 'chat') return t('usage.surfaces.chat');
    if (type === 'agent') return t('usage.surfaces.agent');
    return t('usage.surfaces.workflow');
  };

  const statusLabel = (status: ConnectorTargetBinding['status']) => {
    if (status === 'active') return t('usage.status.active');
    if (status === 'disabled') return t('usage.status.disabled');
    return t('usage.status.needsReplacement');
  };

  const remove = async () => {
    if (!impact || mode === 'blocked') return;
    try {
      await removeMutation.mutateAsync(
        mode === 'force' ? { force: true, impactToken: impact.impact_token } : { force: false }
      );
      toast.success(t('feedback.removed'));
      onOpenChange(false);
      onRemoved();
    } catch {
      toast.error(t('feedback.removeFailed'));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('removal.title')}</DialogTitle>
          <DialogDescription>{t('removal.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {isDefault ? (
            <p className="rounded-lg border border-warning/30 bg-warning/5 p-3 text-sm">
              {t('defaults.deletionNotice')}
            </p>
          ) : null}
          {impactQuery.isLoading ? (
            <div className="rounded-xl border px-5 py-10 text-center" role="status">
              <Loader2 aria-hidden="true" className="mx-auto size-7 animate-spin text-primary" />
              <p className="mt-3 text-sm font-medium">{t('removal.loading')}</p>
            </div>
          ) : null}

          {impactQuery.isError ? (
            <Alert variant="destructive">
              <AlertTriangle aria-hidden="true" className="size-4" />
              <AlertTitle>{t('removal.loadFailedTitle')}</AlertTitle>
              <AlertDescription className="space-y-3">
                <p>{t('removal.loadFailedDescription')}</p>
                <Button size="sm" variant="outline" onClick={() => void impactQuery.refetch()}>
                  {t('actions.retry')}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}

          {impact && affectedTargets.length === 0 ? (
            <Alert className="border-success/25 bg-success/5 text-success">
              <CheckCircle2 aria-hidden="true" className="size-4" />
              <AlertTitle>{t('removal.safeTitle')}</AlertTitle>
              <AlertDescription className="text-muted-foreground">
                {t('removal.safeDescription')}
              </AlertDescription>
            </Alert>
          ) : null}

          {impact && affectedTargets.length > 0 ? (
            <>
              <Alert variant="destructive">
                <AlertTriangle aria-hidden="true" className="size-4" />
                <AlertTitle>{t('removal.forceTitle')}</AlertTitle>
                <AlertDescription>{t('removal.forceDescription')}</AlertDescription>
              </Alert>
              <section aria-labelledby="connector-removal-impact-title">
                <h3 id="connector-removal-impact-title" className="font-semibold">
                  {t('removal.affectedTitle', { count: affectedTargets.length })}
                </h3>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t('removal.affectedDescription')}
                </p>
                <div className="mt-3 max-h-72 overflow-y-auto rounded-lg border">
                  {affectedTargets.map((binding, index) => {
                    const surface = surfaceLabel(binding.target.type);
                    return (
                      <div
                        key={binding.id}
                        className={`grid gap-2 px-3 py-3 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-center ${
                          index > 0 ? 'border-t' : ''
                        }`}
                      >
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium">
                            {binding.target_name?.trim() || t('usage.unnamedTarget', { surface })}
                          </p>
                          <p className="mt-0.5 text-xs text-muted-foreground">
                            {statusLabel(binding.status)}
                          </p>
                        </div>
                        <Badge variant="outline">{surface}</Badge>
                        <Button asChild size="sm" variant="ghost">
                          <Link href={connectorBindingTargetHref(binding)}>{t('usage.view')}</Link>
                        </Button>
                      </div>
                    );
                  })}
                </div>
              </section>
              {mode === 'blocked' ? (
                <p className="text-sm font-medium text-destructive" role="alert">
                  {t('removal.tokenUnavailable')}
                </p>
              ) : null}
            </>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('removal.cancel')}
          </Button>
          <Button
            variant="destructive"
            loading={removeMutation.isPending}
            disabled={!impact || impactQuery.isError || mode === 'blocked'}
            onClick={() => void remove()}
          >
            {mode === 'force' ? t('removal.forceAction') : t('removal.normalAction')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
