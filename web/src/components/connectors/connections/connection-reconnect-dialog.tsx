'use client';

import Link from 'next/link';
import { useState } from 'react';
import { AlertCircle, AlertTriangle, CheckCircle2, Loader2, UserRoundPlus } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
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
import { ConnectorKeyForm } from '@/components/connectors/setup/connector-key-form';
import { ConnectorStatusQueryError } from '@/components/connectors/setup/connector-setup-status';
import { useConnectorReconnect } from '@/hooks/connectors/use-connector-reconnect';
import { useT, type ScopedTranslations } from '@/i18n';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';
import { supportsMailSetup, connectionMailUsage } from '../setup/mail-setup-state';
import { supportsServerOpsSSHConnectorSetup } from '../setup/app-setup-state';
import { ConnectorMailSetupPanel } from '../setup/connector-mail-setup-panel';
import { ConnectorSocialOAuthPanel } from '../setup/connector-social-oauth-panel';
import { supportsSocialOAuth } from '../setup/social-oauth-state';
import { ServerOpsSSHSetupPanel } from '../setup/serverops-ssh-setup-panel';

function errorText(t: ScopedTranslations<'connectors'>, code: string | undefined): string {
  if (
    code === 'connector_key_invalid' ||
    code === 'invalid_key' ||
    code === 'key_invalid' ||
    code === 'key_revoked'
  ) {
    return t('setup.errors.invalidKey');
  }
  if (
    code === 'connector_key_insufficient' ||
    code === 'insufficient_key' ||
    code === 'insufficient_access'
  ) {
    return t('setup.errors.insufficientKey');
  }
  if (code === 'connector_provider_unavailable' || code === 'provider_unavailable') {
    return t('setup.errors.providerUnavailable');
  }
  return t('setup.errors.requestFailed');
}

interface ConnectionReconnectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  connectionId: string;
  connector: ConnectorCatalogItem;
  connection?: PersonalConnection;
}

export function ConnectionReconnectDialog(props: ConnectionReconnectDialogProps) {
  const t = useT('connectors');
  const [socialManual, setSocialManual] = useState(false);
  if (supportsSocialOAuth(props.connector) && socialManual) {
    return (
      <KeyReconnectDialog
        {...props}
        onOpenChange={open => {
          props.onOpenChange(open);
          if (!open) setSocialManual(false);
        }}
      />
    );
  }
  if (supportsSocialOAuth(props.connector)) {
    return (
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent size="md">
          <DialogHeader>
            <DialogTitle>{t('oauth.reconnect')}</DialogTitle>
            <DialogDescription>{t('oauth.reconnectDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            {props.open && (
              <ConnectorSocialOAuthPanel
                connector={props.connector}
                reconnectId={props.connectionId}
                embedded
                manualFallback={null}
                onManual={() => setSocialManual(true)}
              />
            )}
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => props.onOpenChange(false)}>
              {t('reconnect.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }
  if (supportsServerOpsSSHConnectorSetup(props.connector)) {
    return <ServerOpsSSHReconnectDialog {...props} />;
  }
  if (supportsMailSetup(props.connector)) {
    return (
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent size="md">
          <DialogHeader>
            <DialogTitle>
              {t(
                props.connection && connectionMailUsage(props.connection) !== 'read_send'
                  ? 'mail.upgradeReconnect'
                  : 'mail.reconnect'
              )}
            </DialogTitle>
            <DialogDescription>{props.connection?.external_account_id}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            {props.open && props.connection && (
              <ConnectorMailSetupPanel
                connector={props.connector}
                updateConnection={props.connection}
                embedded
              />
            )}
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => props.onOpenChange(false)}>
              {t('reconnect.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    );
  }
  return <KeyReconnectDialog {...props} />;
}

/**
 * Pinned-host-key SSH reconnect. The panel reuses the connection id, so a
 * matching username@host:port identity updates this connection in place, while a
 * different verified identity is reported as a separate connection rather than
 * silently replaced. Alias management stays in Connection Center.
 */
function ServerOpsSSHReconnectDialog({
  open,
  onOpenChange,
  connector,
  connection,
}: ConnectionReconnectDialogProps) {
  const t = useT('connectors');
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{t('serveropsSsh.reconnectTitle')}</DialogTitle>
          <DialogDescription>{t('serveropsSsh.reconnectDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody>
          {open && connection ? (
            <ServerOpsSSHSetupPanel connector={connector} updateConnection={connection} embedded />
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('reconnect.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function KeyReconnectDialog({
  open,
  onOpenChange,
  connectionId,
  connector,
}: ConnectionReconnectDialogProps) {
  const t = useT('connectors');
  const reconnect = useConnectorReconnect(connectionId, connector.key_fields, open);
  const state = reconnect.state;
  const terminal = state.status === 'ready' || state.status === 'new_connection';

  const setOpen = (nextOpen: boolean) => {
    if (!nextOpen) {
      if (state.status === 'verifying') void reconnect.cancel();
      else reconnect.reset();
    }
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{t('reconnect.title')}</DialogTitle>
          <DialogDescription>{t('reconnect.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {state.status === 'idle' ? (
            <div className="space-y-4">
              {state.confirmUnstableIdentity ? (
                <Alert className="border-warning/30 bg-warning/10">
                  <AlertTriangle aria-hidden="true" className="size-4 text-warning" />
                  <AlertTitle>{t('reconnect.confirmedTitle')}</AlertTitle>
                  <AlertDescription>{t('reconnect.confirmedDescription')}</AlertDescription>
                </Alert>
              ) : null}
              <ConnectorKeyForm
                fields={connector.key_fields}
                values={state.values}
                canSubmit={reconnect.canSubmit}
                onValueChange={reconnect.updateValue}
                onSubmit={() => void reconnect.submit()}
                submitLabel={t('reconnect.submit')}
              />
            </div>
          ) : null}

          {state.status === 'confirmation_required' ? (
            <div className="space-y-3">
              <Alert className="border-warning/30 bg-warning/10">
                <AlertTriangle aria-hidden="true" className="size-4 text-warning" />
                <AlertTitle>{t('reconnect.confirmIdentityTitle')}</AlertTitle>
                <AlertDescription>{t('reconnect.confirmIdentityDescription')}</AlertDescription>
              </Alert>
              <Button className="w-full" onClick={reconnect.confirmIdentityReplacement}>
                {t('reconnect.confirmIdentityAction')}
              </Button>
            </div>
          ) : null}

          {state.status === 'verifying' ? (
            reconnect.statusError && state.attemptId ? (
              <ConnectorStatusQueryError
                onRetry={reconnect.retryStatus}
                isRetrying={reconnect.statusRefreshing}
              />
            ) : (
              <div className="rounded-xl border bg-muted/20 px-5 py-8 text-center" role="status">
                <Loader2 aria-hidden="true" className="mx-auto size-7 animate-spin text-primary" />
                <p className="mt-4 font-medium">{t('reconnect.verifyingTitle')}</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t('reconnect.verifyingDescription')}
                </p>
              </div>
            )
          ) : null}

          {state.status === 'failed' || state.status === 'unknown' ? (
            <div className="space-y-3">
              <Alert variant="destructive">
                <AlertCircle aria-hidden="true" className="size-4" />
                <AlertTitle>
                  {t(state.status === 'unknown' ? 'setup.unknownTitle' : 'reconnect.failedTitle')}
                </AlertTitle>
                <AlertDescription>
                  {state.status === 'unknown'
                    ? t('setup.errors.resultUnknown')
                    : errorText(t, state.errorCode)}
                </AlertDescription>
              </Alert>
              {state.status === 'unknown' ? (
                <Button asChild variant="outline" className="w-full">
                  <Link
                    href="/console/connectors/connections"
                    onClick={reconnect.refreshConnections}
                  >
                    {t('setup.checkConnections')}
                  </Link>
                </Button>
              ) : null}
              <Button className="w-full" variant="outline" onClick={reconnect.reset}>
                {t('reconnect.retry')}
              </Button>
            </div>
          ) : null}

          {state.status === 'ready' ? (
            <Alert className="border-success/25 bg-success/5 text-success">
              <CheckCircle2 aria-hidden="true" className="size-4" />
              <AlertTitle>{t('reconnect.readyTitle')}</AlertTitle>
              <AlertDescription className="text-muted-foreground">
                {t('reconnect.readyDescription')}
              </AlertDescription>
            </Alert>
          ) : null}

          {state.status === 'new_connection' ? (
            <Alert className="border-primary/20 bg-primary/5">
              <UserRoundPlus aria-hidden="true" className="size-4 text-primary" />
              <AlertTitle>{t('reconnect.newConnectionTitle')}</AlertTitle>
              <AlertDescription>{t('reconnect.newConnectionDescription')}</AlertDescription>
            </Alert>
          ) : null}
        </DialogBody>
        <DialogFooter>
          {state.status === 'new_connection' && state.connection ? (
            <Button asChild>
              <Link
                href={`/console/connectors/connections/${encodeURIComponent(state.connection.id)}`}
              >
                {t('reconnect.viewNewConnection')}
              </Link>
            </Button>
          ) : null}
          <Button variant={terminal ? 'default' : 'outline'} onClick={() => setOpen(false)}>
            {terminal ? t('reconnect.close') : t('reconnect.cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
