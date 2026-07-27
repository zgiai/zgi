'use client';

import { useState } from 'react';
import { AlertTriangle, CheckCircle2, Clock3, ShieldAlert } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useAcknowledgeIntegrationOAuthRecovery } from '@/hooks';
import { useT } from '@/i18n';
import type {
  IntegrationOAuthRecoveryOperation,
  IntegrationOAuthRecoveryResolutionCode,
  IntegrationOAuthRecoveryStatus,
} from '@/services/types/integration';
import { useIntegrationMetadata } from './metadata-i18n';

interface IntegrationOAuthRecoveryPanelProps {
  recovery: IntegrationOAuthRecoveryStatus;
}

interface PendingAcknowledgement {
  operation: IntegrationOAuthRecoveryOperation;
  resolutionCode: IntegrationOAuthRecoveryResolutionCode;
}

export function IntegrationOAuthRecoveryPanel({ recovery }: IntegrationOAuthRecoveryPanelProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const acknowledgeMutation = useAcknowledgeIntegrationOAuthRecovery();
  const [pendingAcknowledgement, setPendingAcknowledgement] =
    useState<PendingAcknowledgement | null>(null);

  if (recovery.unresolved_dead_letters <= 0) return null;

  const pendingProviderName = pendingAcknowledgement
    ? metadata.providerName(pendingAcknowledgement.operation.integration_id)
    : '';
  const providerAccessRemoved =
    pendingAcknowledgement?.resolutionCode === 'provider_access_removed';

  return (
    <>
      <section
        aria-labelledby="oauth-recovery-title"
        className="mb-6 overflow-hidden rounded-xl border border-warning/35 bg-warning/5"
      >
        <div className="flex flex-col gap-3 border-b border-warning/20 px-4 py-3.5 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex min-w-0 gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-warning/15 text-warning">
              <ShieldAlert className="size-4" aria-hidden="true" />
            </span>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 id="oauth-recovery-title" className="font-semibold">
                  {t('connectionCenter.oauthRecovery.title')}
                </h2>
                <Badge variant="warning">
                  {t('connectionCenter.oauthRecovery.unresolvedCount', {
                    count: recovery.unresolved_dead_letters,
                  })}
                </Badge>
              </div>
              <p className="mt-1 max-w-3xl text-sm leading-5 text-muted-foreground">
                {t('connectionCenter.oauthRecovery.description')}
              </p>
            </div>
          </div>

          <div className="flex shrink-0 flex-wrap gap-2 text-xs text-muted-foreground">
            <span className="rounded-full border bg-background/80 px-2.5 py-1">
              {t('connectionCenter.oauthRecovery.pendingCount', {
                count: recovery.pending_revocations,
              })}
            </span>
            <span className="rounded-full border bg-background/80 px-2.5 py-1">
              {t('connectionCenter.oauthRecovery.manualCount', {
                count: recovery.manual_action_required,
              })}
            </span>
            <span className="rounded-full border bg-background/80 px-2.5 py-1">
              {t('connectionCenter.oauthRecovery.failedCount', {
                count: recovery.failed_revocations,
              })}
            </span>
          </div>
        </div>

        <div className="space-y-3 p-4">
          <div className="flex gap-2 rounded-lg border border-warning/20 bg-background/80 px-3 py-2.5 text-sm leading-5">
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden="true" />
            <p>{t('connectionCenter.oauthRecovery.guidance')}</p>
          </div>

          {recovery.remediation_operations.map(operation => {
            const providerName = metadata.providerName(operation.integration_id);
            return (
              <article
                key={operation.operation_ref}
                className="rounded-lg border bg-background px-3.5 py-3"
              >
                <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="font-medium">{providerName}</h3>
                      <Badge variant="outline">
                        {t('connectionCenter.oauthRecovery.manualReview')}
                      </Badge>
                    </div>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {t('connectionCenter.oauthRecovery.operationDescription', {
                        provider: providerName,
                      })}
                    </p>
                    <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      <span className="inline-flex items-center gap-1">
                        <Clock3 className="size-3.5" aria-hidden="true" />
                        {t('connectionCenter.oauthRecovery.failedAt', {
                          date: metadata.date(
                            operation.failed_at || operation.created_at,
                            t('executions.noValue')
                          ),
                        })}
                      </span>
                      <span>
                        {t('connectionCenter.oauthRecovery.attempts', {
                          count: operation.attempts,
                        })}
                      </span>
                    </div>
                  </div>

                  <div className="flex shrink-0 flex-col gap-2 sm:flex-row">
                    <Button
                      type="button"
                      size="sm"
                      onClick={() =>
                        setPendingAcknowledgement({
                          operation,
                          resolutionCode: 'provider_access_removed',
                        })
                      }
                      disabled={acknowledgeMutation.isPending}
                    >
                      <CheckCircle2 className="size-4" aria-hidden="true" />
                      {t('connectionCenter.oauthRecovery.accessRemoved')}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        setPendingAcknowledgement({
                          operation,
                          resolutionCode: 'token_confirmed_expired',
                        })
                      }
                      disabled={acknowledgeMutation.isPending}
                    >
                      {t('connectionCenter.oauthRecovery.tokenExpired')}
                    </Button>
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      </section>

      <ConfirmDialog
        open={Boolean(pendingAcknowledgement)}
        onOpenChange={open => {
          if (!open) setPendingAcknowledgement(null);
        }}
        title={t(
          providerAccessRemoved
            ? 'connectionCenter.oauthRecovery.confirmAccessRemovedTitle'
            : 'connectionCenter.oauthRecovery.confirmTokenExpiredTitle',
          { provider: pendingProviderName }
        )}
        description={t(
          providerAccessRemoved
            ? 'connectionCenter.oauthRecovery.confirmAccessRemovedDescription'
            : 'connectionCenter.oauthRecovery.confirmTokenExpiredDescription',
          { provider: pendingProviderName }
        )}
        confirmText={t(
          providerAccessRemoved
            ? 'connectionCenter.oauthRecovery.confirmAccessRemoved'
            : 'connectionCenter.oauthRecovery.confirmTokenExpired'
        )}
        cancelText={t('connectionCenter.oauthRecovery.cancel')}
        loading={acknowledgeMutation.isPending}
        onConfirm={() => {
          if (!pendingAcknowledgement) return;
          acknowledgeMutation.mutate({
            operationRef: pendingAcknowledgement.operation.operation_ref,
            data: { resolution_code: pendingAcknowledgement.resolutionCode },
          });
        }}
      />
    </>
  );
}
