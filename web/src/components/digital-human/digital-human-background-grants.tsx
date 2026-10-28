'use client';

import { useMemo, useState } from 'react';
import { Clock3, Plus, RefreshCw, ShieldCheck, Trash2, X } from 'lucide-react';
import { toast } from 'sonner';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  connectorCatalogItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useDigitalHumanBackgroundGrants } from '@/hooks/digital-human/use-digital-human-background-grants';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import type {
  ConnectorBackgroundGrant,
  ConnectorCapabilitySummary,
  ConnectorTargetBinding,
  PersonalConnection,
} from '@/services/types/connector';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  backgroundGrantOperationAllowed,
  buildBackgroundGrantIssueRequest,
  grantsForEmployee,
  type BackgroundGrantDestinationRow,
  type BackgroundGrantDraftError,
} from './digital-human-background-grants-model';

interface DigitalHumanBackgroundGrantsProps {
  employeeId: string;
  versionId: string;
  bindings: ConnectorTargetBinding[];
  canView: boolean;
  canCreate: boolean;
}

interface DestinationEditorRow extends BackgroundGrantDestinationRow {
  id: string;
}

function operationLabel(
  grant: ConnectorBackgroundGrant,
  operationId: string,
  connections: PersonalConnection[],
  capabilitiesByConnector: Map<string, ConnectorCapabilitySummary[]>
) {
  const connectorId = connections.find(item => item.id === grant.connection_id)?.connector_id;
  return connectorId
    ? capabilitiesByConnector.get(connectorId)?.find(item => item.id === operationId)?.name ||
        operationId
    : operationId;
}

export function DigitalHumanBackgroundGrants({
  employeeId,
  versionId,
  bindings,
  canView,
  canCreate,
}: DigitalHumanBackgroundGrantsProps) {
  const t = useT('connectors');
  const { locale } = useLocale();
  const account = useAuthStore.use.user();
  const organization = useOrganizationStore.use.currentOrganization();
  const workspace = useWorkspaceStore.use.currentWorkspace();
  const scope =
    account?.id && organization?.id && workspace?.id && employeeId && versionId
      ? {
          organizationId: organization.id,
          workspaceId: workspace.id,
          accountId: account.id,
          employeeId,
          versionId,
        }
      : undefined;
  const grants = useDigitalHumanBackgroundGrants(scope, canView);
  const catalogQuery = useConnectorCatalog(undefined, canView);
  const connectionsQuery = usePersonalConnections(undefined, canView);
  const catalog = useMemo(
    () =>
      connectorCatalogItems(catalogQuery.data?.data).map(item =>
        localizeConnectorCatalogItem(item, locale)
      ),
    [catalogQuery.data?.data, locale]
  );
  const connections = personalConnectionItems(connectionsQuery.data?.data);
  const currentBindings = bindings.filter(
    binding =>
      binding.status === 'active' &&
      binding.target.type === 'digital_human' &&
      binding.target.id === employeeId &&
      binding.target.version_id === versionId
  );
  const rows = scope
    ? grantsForEmployee(grants.query.data?.data.items ?? [], scope, bindings, connections)
    : [];
  const capabilitiesByConnector = new Map(
    catalog.map(item => [item.id, item.capabilities] as const)
  );
  const date = new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' });
  const [open, setOpen] = useState(false);
  const [bindingId, setBindingId] = useState('');
  const [operations, setOperations] = useState<string[]>([]);
  const [durationDays, setDurationDays] = useState(7);
  const [maxInvocations, setMaxInvocations] = useState(30);
  const [destinationRows, setDestinationRows] = useState<DestinationEditorRow[]>([]);
  const [formError, setFormError] = useState<BackgroundGrantDraftError | 'binding' | 'request'>();
  const selectedBinding = currentBindings.find(binding => binding.id === bindingId);
  const selectedConnection = connections.find(
    connection => connection.id === selectedBinding?.connection_id
  );
  const selectedConnector = catalog.find(
    connector => connector.id === selectedBinding?.connector_id
  );
  const usableOperations = (selectedConnector?.capabilities ?? []).filter(
    operation =>
      operation.usable &&
      selectedConnection?.usable_operation_ids?.includes(operation.id) &&
      backgroundGrantOperationAllowed(selectedConnector?.id ?? '', operation.id)
  );
  const pending = grants.issue.isPending || grants.revoke.isPending;

  if (!canView || !scope) return null;

  const resetForm = () => {
    setBindingId('');
    setOperations([]);
    setDurationDays(7);
    setMaxInvocations(30);
    setDestinationRows([]);
    setFormError(undefined);
  };
  const changeOpen = (next: boolean) => {
    if (!next && grants.issue.isPending) return;
    setOpen(next);
    if (!next) resetForm();
  };
  const submit = async () => {
    if (!selectedBinding) {
      setFormError('binding');
      return;
    }
    const result = buildBackgroundGrantIssueRequest({
      operations,
      destinationRows,
      durationDays,
      maxInvocations,
    });
    if ('error' in result) {
      setFormError(result.error);
      return;
    }
    setFormError(undefined);
    try {
      await grants.issue.mutateAsync({ bindingId: selectedBinding.id, request: result.request });
      toast.success(t('backgroundGrants.created'));
      changeOpen(false);
    } catch (error) {
      setFormError('request');
      toast.error(getErrorMessage(error) || t('backgroundGrants.createFailed'));
    }
  };

  return (
    <div className="mt-4 border-t pt-4" data-testid="digital-human-background-grants">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <ShieldCheck className="size-4 text-primary" />
            <h4 className="text-sm font-medium">{t('backgroundGrants.title')}</h4>
          </div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('backgroundGrants.description')}
          </p>
        </div>
        {canCreate ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={pending || currentBindings.length === 0}
            onClick={() => setOpen(true)}
          >
            <Plus className="size-4" />
            {t('backgroundGrants.create')}
          </Button>
        ) : null}
      </div>
      <p className="mt-2 rounded-md bg-muted/40 px-3 py-2 text-xs leading-5 text-muted-foreground">
        {t('backgroundGrants.regrantNotice')}
      </p>

      {grants.query.isLoading || catalogQuery.isLoading || connectionsQuery.isLoading ? (
        <div className="mt-3 h-16 animate-pulse rounded-lg bg-muted" />
      ) : grants.query.isError || catalogQuery.isError || connectionsQuery.isError ? (
        <div
          role="alert"
          className="mt-3 flex items-center justify-between gap-3 rounded-lg border p-3 text-sm"
        >
          <span>{t('backgroundGrants.loadFailed')}</span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() =>
              void Promise.all([
                grants.query.refetch(),
                catalogQuery.refetch(),
                connectionsQuery.refetch(),
              ])
            }
          >
            <RefreshCw className="size-4" />
            {t('actions.retry')}
          </Button>
        </div>
      ) : rows.length === 0 ? (
        <p className="mt-3 rounded-lg border border-dashed p-4 text-center text-xs text-muted-foreground">
          {t('backgroundGrants.empty')}
        </p>
      ) : (
        <div className="mt-3 space-y-2">
          {rows.map(({ grant, stale, expired, revoked }) => {
            const connection = connections.find(item => item.id === grant.connection_id);
            const connector = catalog.find(item => item.id === connection?.connector_id);
            return (
              <div key={grant.id} className="rounded-lg border bg-background p-3">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">
                      {connection?.name || connector?.name || t('backgroundGrants.unknownAccount')}
                    </p>
                    <div className="mt-1 flex flex-wrap gap-1.5">
                      {revoked ? (
                        <Badge variant="subtle">{t('backgroundGrants.status.revoked')}</Badge>
                      ) : null}
                      {expired ? (
                        <Badge variant="warning">{t('backgroundGrants.status.expired')}</Badge>
                      ) : null}
                      {stale ? (
                        <Badge variant="warning">{t('backgroundGrants.status.stale')}</Badge>
                      ) : null}
                      {!revoked && !expired && !stale ? (
                        <Badge variant="success">{t('backgroundGrants.status.active')}</Badge>
                      ) : null}
                    </div>
                  </div>
                  {!revoked ? (
                    <ConfirmDialog
                      title={t('backgroundGrants.revokeTitle')}
                      description={t('backgroundGrants.revokeDescription')}
                      confirmText={t('backgroundGrants.revoke')}
                      cancelText={t('backgroundGrants.cancel')}
                      variant="warning"
                      loading={grants.revoke.isPending}
                      onConfirm={() => {
                        void grants.revoke
                          .mutateAsync(grant.id)
                          .then(() => toast.success(t('backgroundGrants.revoked')))
                          .catch(error =>
                            toast.error(
                              getErrorMessage(error) || t('backgroundGrants.revokeFailed')
                            )
                          );
                      }}
                      trigger={
                        <Button type="button" variant="ghost" size="sm" isIcon disabled={pending}>
                          <Trash2 className="size-4" />
                          <span className="sr-only">{t('backgroundGrants.revoke')}</span>
                        </Button>
                      }
                    />
                  ) : null}
                </div>
                <div className="mt-3 grid gap-2 text-xs text-muted-foreground sm:grid-cols-2">
                  <span className="flex items-center gap-1.5">
                    <Clock3 className="size-3.5" />
                    {t('backgroundGrants.expires', {
                      date: date.format(new Date(grant.expires_at)),
                    })}
                  </span>
                  <span>
                    {t('backgroundGrants.usage', {
                      used: grant.used_invocations,
                      total: grant.max_invocations,
                    })}
                  </span>
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {grant.operations.map(operationId => (
                    <Badge key={operationId} variant="outline">
                      {operationLabel(grant, operationId, connections, capabilitiesByConnector)}
                    </Badge>
                  ))}
                </div>
                {Object.keys(grant.destinations).length > 0 ? (
                  <dl className="mt-2 grid gap-1 text-xs text-muted-foreground">
                    {Object.entries(grant.destinations).map(([field, value]) => (
                      <div key={field} className="flex min-w-0 gap-1">
                        <dt className="font-medium text-foreground">{field}:</dt>
                        <dd className="truncate">{value}</dd>
                      </div>
                    ))}
                  </dl>
                ) : null}
              </div>
            );
          })}
        </div>
      )}

      <Dialog open={open} onOpenChange={changeOpen}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{t('backgroundGrants.dialogTitle')}</DialogTitle>
            <DialogDescription>{t('backgroundGrants.dialogDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-5">
            <div className="space-y-2">
              <Label>{t('backgroundGrants.account')}</Label>
              <Select
                value={bindingId}
                disabled={pending}
                onValueChange={value => {
                  setBindingId(value);
                  setOperations([]);
                  setFormError(undefined);
                }}
              >
                <SelectTrigger aria-invalid={formError === 'binding'}>
                  <SelectValue placeholder={t('backgroundGrants.accountPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {currentBindings.map(binding => {
                    const connection = connections.find(item => item.id === binding.connection_id);
                    const connector = catalog.find(item => item.id === binding.connector_id);
                    return (
                      <SelectItem key={binding.id} value={binding.id}>
                        {connection?.name || t('backgroundGrants.unknownAccount')} ·{' '}
                        {connector?.name || binding.connector_id}
                      </SelectItem>
                    );
                  })}
                </SelectContent>
              </Select>
            </div>

            <fieldset className="space-y-2" disabled={!selectedBinding || pending}>
              <legend className="text-sm font-medium">{t('backgroundGrants.operations')}</legend>
              <p className="text-xs text-muted-foreground">
                {t('backgroundGrants.operationsDescription')}
              </p>
              {selectedBinding && usableOperations.length === 0 ? (
                <p role="alert" className="text-sm text-destructive">
                  {t('backgroundGrants.noOperations')}
                </p>
              ) : (
                <div className="grid gap-2 sm:grid-cols-2">
                  {usableOperations.map(operation => (
                    <label
                      key={operation.id}
                      className="flex items-start gap-2 rounded-lg border p-3"
                    >
                      <Checkbox
                        checked={operations.includes(operation.id)}
                        onCheckedChange={checked =>
                          setOperations(current =>
                            checked === true
                              ? [...current, operation.id]
                              : current.filter(id => id !== operation.id)
                          )
                        }
                      />
                      <span className="min-w-0 text-sm">
                        <span className="block font-medium">{operation.name}</span>
                        <span className="mt-1 block text-xs leading-5 text-muted-foreground">
                          {operation.description}
                        </span>
                      </span>
                    </label>
                  ))}
                </div>
              )}
            </fieldset>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="background-grant-duration">{t('backgroundGrants.duration')}</Label>
                <Input
                  id="background-grant-duration"
                  type="number"
                  min={1}
                  max={90}
                  step={1}
                  value={durationDays}
                  disabled={pending}
                  error={formError === 'duration'}
                  onChange={event => setDurationDays(Number(event.target.value))}
                />
                <p className="text-xs text-muted-foreground">
                  {t('backgroundGrants.durationHint')}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="background-grant-limit">{t('backgroundGrants.maxCalls')}</Label>
                <Input
                  id="background-grant-limit"
                  type="number"
                  min={1}
                  max={100000}
                  step={1}
                  value={maxInvocations}
                  disabled={pending}
                  error={formError === 'maxInvocations'}
                  onChange={event => setMaxInvocations(Number(event.target.value))}
                />
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <Label>{t('backgroundGrants.destinations')}</Label>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('backgroundGrants.destinationsDescription')}
                  </p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={pending || destinationRows.length >= 16}
                  onClick={() =>
                    setDestinationRows(current => [
                      ...current,
                      { id: crypto.randomUUID(), field: '', value: '' },
                    ])
                  }
                >
                  <Plus className="size-4" />
                  {t('backgroundGrants.addDestination')}
                </Button>
              </div>
              {destinationRows.map(row => (
                <div
                  key={row.id}
                  className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto] gap-2"
                >
                  <Input
                    aria-label={t('backgroundGrants.destinationField')}
                    placeholder={t('backgroundGrants.destinationFieldPlaceholder')}
                    value={row.field}
                    maxLength={64}
                    disabled={pending}
                    onChange={event =>
                      setDestinationRows(current =>
                        current.map(item =>
                          item.id === row.id ? { ...item, field: event.target.value } : item
                        )
                      )
                    }
                  />
                  <Input
                    aria-label={t('backgroundGrants.destinationValue')}
                    placeholder={t('backgroundGrants.destinationValuePlaceholder')}
                    value={row.value}
                    maxLength={512}
                    disabled={pending}
                    onChange={event =>
                      setDestinationRows(current =>
                        current.map(item =>
                          item.id === row.id ? { ...item, value: event.target.value } : item
                        )
                      )
                    }
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    isIcon
                    disabled={pending}
                    onClick={() =>
                      setDestinationRows(current => current.filter(item => item.id !== row.id))
                    }
                  >
                    <X className="size-4" />
                    <span className="sr-only">{t('backgroundGrants.removeDestination')}</span>
                  </Button>
                </div>
              ))}
            </div>

            {formError ? (
              <p role="alert" className="text-sm text-destructive">
                {t(`backgroundGrants.errors.${formError}`)}
              </p>
            ) : null}
          </DialogBody>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              disabled={pending}
              onClick={() => changeOpen(false)}
            >
              {t('backgroundGrants.cancel')}
            </Button>
            <Button
              type="button"
              disabled={pending || usableOperations.length === 0}
              onClick={() => void submit()}
            >
              {grants.issue.isPending
                ? t('backgroundGrants.creating')
                : t('backgroundGrants.create')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
