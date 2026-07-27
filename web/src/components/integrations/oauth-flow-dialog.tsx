'use client';

import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  Loader2,
  RefreshCw,
  ShieldCheck,
  UserRoundCheck,
} from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
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
import type { IntegrationOAuthUIState } from './oauth-flow-state';
import { useIntegrationMetadata } from './metadata-i18n';

interface IntegrationOAuthFlowDialogProps {
  state: IntegrationOAuthUIState;
  providerName: string;
  connectionName: string;
  onCancel: () => void | Promise<void>;
  onDone: () => void;
  onRetry: () => void;
  onRefresh: () => void;
  onOpenFullPage: () => void;
  onReopenPopup: () => boolean;
}

function isWaiting(status: IntegrationOAuthUIState['status']) {
  return ['starting', 'waiting', 'exchanging', 'popup_blocked', 'popup_closed'].includes(status);
}

export function IntegrationOAuthFlowDialog({
  state,
  providerName,
  connectionName,
  onCancel,
  onDone,
  onRetry,
  onRefresh,
  onOpenFullPage,
  onReopenPopup,
}: IntegrationOAuthFlowDialogProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const open = state.status !== 'idle';
  const waiting = isWaiting(state.status);
  const succeeded = state.status === 'succeeded';
  const retryable = ['failed', 'expired', 'timed_out'].includes(state.status);
  const popupNeedsAttention = state.popupBlocked || state.popupClosed;

  return (
    <Dialog
      open={open}
      onOpenChange={nextOpen => {
        if (!nextOpen && waiting) void onCancel();
        else if (!nextOpen) onDone();
      }}
    >
      <DialogContent size="sm" className="p-0">
        <DialogHeader>
          <DialogTitle>
            {t(
              state.flow?.intent === 'scope_upgrade'
                ? 'oauth.flow.upgradeTitle'
                : state.flow?.intent === 'reconnect'
                  ? 'oauth.flow.reconnectTitle'
                  : 'oauth.flow.connectTitle',
              { provider: providerName }
            )}
          </DialogTitle>
          <DialogDescription>
            {t('oauth.flow.description', { connection: connectionName, provider: providerName })}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4 pb-5">
          <div
            role="status"
            aria-live="polite"
            aria-busy={waiting}
            className="flex min-h-32 flex-col items-center justify-center rounded-xl border bg-muted/20 px-5 py-7 text-center"
          >
            {waiting ? (
              <Loader2 aria-hidden="true" className="size-8 animate-spin text-primary" />
            ) : null}
            {succeeded ? <CheckCircle2 aria-hidden="true" className="size-9 text-success" /> : null}
            {!waiting && !succeeded ? (
              <AlertCircle aria-hidden="true" className="size-9 text-destructive" />
            ) : null}
            <p className="mt-3 font-medium">{t(`oauth.flow.status.${state.status}`)}</p>
            <p className="mt-1 max-w-sm text-sm leading-5 text-muted-foreground">
              {state.flow?.error_code && !waiting && !succeeded
                ? metadata.error(state.flow.error_code)
                : t(`oauth.flow.statusDescription.${state.status}`)}
            </p>
          </div>

          {popupNeedsAttention && state.flow?.authorization_url ? (
            <Alert>
              <ExternalLink className="size-4" />
              <AlertDescription className="space-y-3">
                <p>
                  {t(
                    state.popupBlocked
                      ? 'oauth.flow.popupBlockedHint'
                      : 'oauth.flow.popupClosedHint'
                  )}
                </p>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      if (!onReopenPopup()) onOpenFullPage();
                    }}
                  >
                    {t('oauth.flow.openPopup')}
                  </Button>
                  <Button type="button" size="sm" variant="ghost" onClick={onOpenFullPage}>
                    {t('oauth.flow.continueFullPage')}
                  </Button>
                </div>
              </AlertDescription>
            </Alert>
          ) : null}

          {succeeded ? (
            <div className="grid gap-3 sm:grid-cols-3">
              <OAuthSuccessStep
                icon={<UserRoundCheck className="size-4" />}
                title={t('oauth.flow.success.verified')}
                description={t('oauth.flow.success.verifiedDescription')}
                complete
              />
              <OAuthSuccessStep
                icon={<ShieldCheck className="size-4" />}
                title={
                  state.flow?.usage_rules_required
                    ? t('oauth.flow.success.rulesRequired')
                    : t('oauth.flow.success.rulesReady')
                }
                description={
                  state.flow?.usage_rules_required
                    ? t('oauth.flow.success.rulesRequiredDescription')
                    : t('oauth.flow.success.rulesReadyDescription')
                }
                complete={!state.flow?.usage_rules_required}
              />
              <OAuthSuccessStep
                icon={<CheckCircle2 className="size-4" />}
                title={
                  state.flow?.ai_chat_available
                    ? t('oauth.flow.success.aiChatAvailable')
                    : t('oauth.flow.success.aiChatPending')
                }
                description={
                  state.flow?.ai_chat_available
                    ? t('oauth.flow.success.aiChatAvailableDescription')
                    : t('oauth.flow.success.aiChatPendingDescription')
                }
                complete={Boolean(state.flow?.ai_chat_available)}
              />
            </div>
          ) : null}
        </DialogBody>

        <DialogFooter className="border-t bg-muted/30">
          {waiting ? (
            <>
              <Button variant="ghost" onClick={() => void onCancel()}>
                {t('oauth.flow.cancel')}
              </Button>
              <Button variant="outline" onClick={onRefresh}>
                <RefreshCw className="size-4" />
                {t('oauth.flow.checkStatus')}
              </Button>
            </>
          ) : retryable ? (
            <>
              <Button variant="ghost" onClick={onDone}>
                {t('oauth.flow.close')}
              </Button>
              <Button onClick={onRetry}>
                <RefreshCw className="size-4" />
                {t('oauth.flow.tryAgain')}
              </Button>
            </>
          ) : (
            <Button onClick={onDone}>{t('oauth.flow.done')}</Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function OAuthSuccessStep({
  icon,
  title,
  description,
  complete,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
  complete: boolean;
}) {
  return (
    <div className="rounded-lg border p-3">
      <div
        className={
          complete
            ? 'flex items-center gap-2 text-sm font-medium text-success'
            : 'flex items-center gap-2 text-sm font-medium text-warning'
        }
      >
        {icon}
        <span>{title}</span>
      </div>
      <p className="mt-1.5 text-xs leading-5 text-muted-foreground">{description}</p>
    </div>
  );
}
