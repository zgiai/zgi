'use client';

import { useEffect, useMemo, useState } from 'react';
import { ExternalLink, KeyRound, UserRound } from 'lucide-react';
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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { RadioCard, RadioCardGroup } from '@/components/ui/radio-card';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useIntegrationOAuthFlow } from '@/hooks';
import { useT } from '@/i18n';
import type {
  CreateIntegrationConnectionRequest,
  IntegrationAuthDefinition,
  IntegrationAuthType,
  IntegrationCatalogItem,
  IntegrationConnection,
  IntegrationCredentialField,
  IntegrationCredentialSource,
  StartIntegrationOAuthFlowRequest,
  UpdateIntegrationConnectionRequest,
} from '@/services/types/integration';
import { actionIDsForAuthMethod } from './action-auth-compatibility';
import { AuthSetupGuide } from './auth-setup-guide';
import { safeOptionalIntegrationDisplayText } from './display-utils';
import {
  integrationAuthCredentialSource,
  integrationCatalogID,
  resolveCredentialFields,
  resolveIntegrationAuthDefinitions,
} from './integration-utils';
import { useIntegrationMetadata } from './metadata-i18n';
import { IntegrationOAuthFlowDialog } from './oauth-flow-dialog';

interface IntegrationConnectionDialogProps {
  context?: 'personal' | 'shared';
  open: boolean;
  catalog: IntegrationCatalogItem[];
  connection?: IntegrationConnection | null;
  isSubmitting?: boolean;
  allowedCredentialSources?: IntegrationCredentialSource[];
  availableAuthMethodsOnly?: boolean;
  lockedAuthMethodId?: string;
  onOpenChange: (open: boolean) => void;
  onCreate: (data: CreateIntegrationConnectionRequest) => Promise<void>;
  onUpdate: (id: string, data: UpdateIntegrationConnectionRequest) => Promise<void>;
  onOAuthCompleted?: (connectionId: string) => void | Promise<void>;
}

type CredentialValue = string | boolean;

function initialFieldValues(
  fields: IntegrationCredentialField[],
  connection?: IntegrationConnection | null
): Record<string, CredentialValue> {
  const values: Record<string, CredentialValue> = {};
  for (const field of fields) {
    const configuredValue =
      field.storage === 'config' ? connection?.config?.[field.name] : undefined;
    if (typeof configuredValue === 'boolean' || typeof configuredValue === 'string') {
      values[field.name] = configuredValue;
    } else if (typeof configuredValue === 'number') {
      values[field.name] = String(configuredValue);
    } else if (typeof field.default_value === 'boolean') {
      values[field.name] = field.default_value;
    } else {
      values[field.name] = field.default_value ?? '';
    }
  }
  return values;
}

function fieldHasValue(value: CredentialValue | undefined): boolean {
  return typeof value === 'boolean' || (typeof value === 'string' && Boolean(value.trim()));
}

export function IntegrationConnectionDialog({
  context = 'shared',
  open,
  catalog,
  connection,
  isSubmitting = false,
  allowedCredentialSources,
  availableAuthMethodsOnly = false,
  lockedAuthMethodId,
  onOpenChange,
  onCreate,
  onUpdate,
  onOAuthCompleted,
}: IntegrationConnectionDialogProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const editing = Boolean(connection);
  const personal = context === 'personal';
  const [integrationId, setIntegrationId] = useState('');
  const [name, setName] = useState('');
  const [credentialSource, setCredentialSource] =
    useState<IntegrationCredentialSource>('organization');
  const [authType, setAuthType] = useState<IntegrationAuthType>('api_key');
  const [authMethodId, setAuthMethodId] = useState('');
  const [fieldValues, setFieldValues] = useState<Record<string, CredentialValue>>({});
  const [validationError, setValidationError] = useState(false);
  const [lastOAuthRequest, setLastOAuthRequest] = useState<StartIntegrationOAuthFlowRequest | null>(
    null
  );
  const [oauthProviderName, setOAuthProviderName] = useState('');
  const [oauthConnectionName, setOAuthConnectionName] = useState('');
  const oauthFlow = useIntegrationOAuthFlow();

  const selectedIntegration = useMemo(
    () => catalog.find(item => integrationCatalogID(item) === integrationId),
    [catalog, integrationId]
  );
  const authDefinitions = useMemo(
    () =>
      resolveIntegrationAuthDefinitions(selectedIntegration).filter(
        definition =>
          (!availableAuthMethodsOnly || definition.available) &&
          (!lockedAuthMethodId || definition.id === lockedAuthMethodId) &&
          (!allowedCredentialSources?.length ||
            allowedCredentialSources.includes(integrationAuthCredentialSource(definition)))
      ),
    [allowedCredentialSources, availableAuthMethodsOnly, lockedAuthMethodId, selectedIntegration]
  );
  const organizationAuth = authDefinitions.filter(
    definition => integrationAuthCredentialSource(definition) === 'organization'
  );
  const accountAuth = authDefinitions.filter(
    definition => integrationAuthCredentialSource(definition) === 'account'
  );
  const selectedAuth = authDefinitions.find(
    definition =>
      definition.id === authMethodId ||
      (definition.type === authType &&
        integrationAuthCredentialSource(definition) === credentialSource)
  );
  const fields = useMemo(
    () => resolveCredentialFields(selectedIntegration, selectedAuth),
    [selectedAuth, selectedIntegration]
  );
  const organizationAvailable = organizationAuth.some(definition => definition.available);
  const accountAvailable = accountAuth.some(definition => definition.available);

  useEffect(() => {
    if (!open) return;
    const initialId =
      connection?.integration_id || (catalog.length === 1 ? integrationCatalogID(catalog[0]) : '');
    const item = catalog.find(candidate => integrationCatalogID(candidate) === initialId);
    const definitions = resolveIntegrationAuthDefinitions(item).filter(
      definition =>
        (!availableAuthMethodsOnly || definition.available) &&
        (!lockedAuthMethodId || definition.id === lockedAuthMethodId) &&
        (!allowedCredentialSources?.length ||
          allowedCredentialSources.includes(integrationAuthCredentialSource(definition)))
    );
    const initialSource =
      connection?.credential_source ??
      (personal &&
      definitions.some(definition => integrationAuthCredentialSource(definition) === 'account')
        ? 'account'
        : definitions.some(
              definition => integrationAuthCredentialSource(definition) === 'organization'
            )
          ? 'organization'
          : 'account');
    const definition =
      definitions.find(candidate => candidate.id === connection?.auth_method_id) ??
      definitions.find(
        candidate =>
          candidate.type === connection?.auth_type &&
          integrationAuthCredentialSource(candidate) === initialSource
      ) ??
      definitions.find(
        candidate =>
          integrationAuthCredentialSource(candidate) === initialSource && candidate.available
      );
    const initialAuthType = connection?.auth_type ?? definition?.type ?? 'api_key';
    const initialFields = resolveCredentialFields(item, definition);

    setIntegrationId(initialId);
    setName(safeOptionalIntegrationDisplayText(connection?.name) ?? '');
    setCredentialSource(initialSource);
    setAuthType(initialAuthType);
    setAuthMethodId(definition?.id ?? connection?.auth_method_id ?? '');
    setFieldValues(initialFieldValues(initialFields, connection));
    setValidationError(false);
  }, [
    allowedCredentialSources,
    availableAuthMethodsOnly,
    catalog,
    connection,
    lockedAuthMethodId,
    open,
    personal,
  ]);

  const applyAuthDefinition = (
    source: IntegrationCredentialSource,
    definition: IntegrationAuthDefinition | undefined
  ) => {
    setCredentialSource(source);
    setAuthType(definition?.type ?? 'api_key');
    setAuthMethodId(definition?.id ?? '');
    setFieldValues(
      initialFieldValues(resolveCredentialFields(selectedIntegration, definition), connection)
    );
    setValidationError(false);
  };

  const handleIntegrationChange = (nextIntegrationId: string) => {
    const item = catalog.find(candidate => integrationCatalogID(candidate) === nextIntegrationId);
    const definitions = resolveIntegrationAuthDefinitions(item).filter(
      candidate =>
        (!availableAuthMethodsOnly || candidate.available) &&
        (!lockedAuthMethodId || candidate.id === lockedAuthMethodId) &&
        (!allowedCredentialSources?.length ||
          allowedCredentialSources.includes(integrationAuthCredentialSource(candidate)))
    );
    const definition = personal
      ? definitions.find(
          candidate =>
            integrationAuthCredentialSource(candidate) === 'account' && candidate.available
        )
      : (definitions.find(
          candidate =>
            integrationAuthCredentialSource(candidate) === 'organization' && candidate.available
        ) ?? definitions.find(candidate => candidate.available));
    const source = definition ? integrationAuthCredentialSource(definition) : 'organization';
    setIntegrationId(nextIntegrationId);
    setCredentialSource(source);
    setAuthType(definition?.type ?? 'api_key');
    setAuthMethodId(definition?.id ?? '');
    setFieldValues(initialFieldValues(resolveCredentialFields(item, definition)));
    setValidationError(false);
  };

  const missingRequiredField = fields.some(field => {
    if (!field.required) return false;
    if (editing && field.secret && connection?.credential_configured !== false) return false;
    return !fieldHasValue(fieldValues[field.name]);
  });
  const oauthSelected = ['oauth', 'oauth2'].includes(selectedAuth?.type ?? '');
  const oauthConnectReady =
    editing ||
    !oauthSelected ||
    (selectedAuth?.oauth
      ? selectedAuth.oauth.connect_enabled && selectedAuth.oauth.client_configured
      : Boolean(selectedAuth?.available));
  const dynamicAuthUnsupported =
    Boolean(selectedAuth) && (!selectedAuth?.available || !oauthConnectReady);

  const clearSecretState = () => {
    const secretNames = new Set(fields.filter(field => field.secret).map(field => field.name));
    setFieldValues(current =>
      Object.fromEntries(
        Object.entries(current).map(([key, value]) => [key, secretNames.has(key) ? '' : value])
      )
    );
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) clearSecretState();
    onOpenChange(nextOpen);
  };

  const submit = async () => {
    const trimmedName = name.trim();
    if (
      !integrationId ||
      !trimmedName ||
      !selectedAuth ||
      dynamicAuthUnsupported ||
      missingRequiredField
    ) {
      setValidationError(true);
      return;
    }

    if (oauthSelected && !editing && selectedAuth && selectedIntegration) {
      const defaultActionIDs = selectedAuth.oauth?.default_action_ids;
      const requestedActionIDs =
        defaultActionIDs == null
          ? undefined
          : actionIDsForAuthMethod(selectedIntegration.actions, selectedAuth.id, defaultActionIDs);
      const oauthRequest: StartIntegrationOAuthFlowRequest = {
        integration_id: integrationId,
        auth_method_id: selectedAuth.id,
        credential_source: credentialSource === 'organization' ? 'organization' : 'account',
        intent: 'connect',
        connection_name: trimmedName,
        requested_action_ids: requestedActionIDs,
        return_path: '/console/integrations?view=connected',
      };
      setLastOAuthRequest(oauthRequest);
      setOAuthProviderName(metadata.providerName(selectedIntegration));
      setOAuthConnectionName(trimmedName);
      handleOpenChange(false);
      await oauthFlow.begin(oauthRequest);
      return;
    }

    const credentials: Record<string, string> = {};
    const config: Record<string, unknown> = editing ? { ...(connection?.config ?? {}) } : {};
    let hasConfigFields = false;
    for (const field of fields) {
      const value = fieldValues[field.name];
      if (field.storage === 'config') {
        hasConfigFields = true;
        if (typeof value === 'boolean') config[field.name] = value;
        else if (typeof value === 'string' && value.trim()) config[field.name] = value.trim();
        else delete config[field.name];
        continue;
      }
      if (typeof value === 'boolean') credentials[field.name] = String(value);
      else if (typeof value === 'string' && value.trim()) credentials[field.name] = value.trim();
    }

    try {
      if (connection) {
        await onUpdate(connection.id, {
          revision: connection.revision ?? 1,
          name: trimmedName,
          credentials: Object.keys(credentials).length > 0 ? credentials : undefined,
          config: hasConfigFields ? config : undefined,
        });
      } else {
        await onCreate({
          integration_id: integrationId,
          driver_id: selectedIntegration?.driver_id ?? '',
          name: trimmedName,
          credential_source: credentialSource,
          auth_type: selectedAuth.type,
          auth_method_id: selectedAuth.id,
          credentials: Object.keys(credentials).length > 0 ? credentials : undefined,
          config: hasConfigFields ? config : undefined,
        });
      }
      onOpenChange(false);
    } catch {
      // Mutation hooks already surface the server error. Secrets are still
      // cleared below so a failed request cannot remain in component state.
    } finally {
      for (const key of Object.keys(credentials)) {
        credentials[key] = '';
        delete credentials[key];
      }
      clearSecretState();
    }
  };

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent size="md" className="p-0">
          <DialogHeader>
            <DialogTitle>
              {t(
                personal
                  ? editing
                    ? 'dialog.editPersonalTitle'
                    : 'dialog.createPersonalTitle'
                  : editing
                    ? 'dialog.editSharedTitle'
                    : 'dialog.createSharedTitle'
              )}
            </DialogTitle>
            <DialogDescription>
              {t(personal ? 'dialog.personalDescription' : 'dialog.sharedDescription')}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-5 pb-4">
            <div className="space-y-2">
              <Label htmlFor="integration-id">{t('dialog.integration')}</Label>
              <Select
                value={integrationId}
                disabled={editing}
                onValueChange={handleIntegrationChange}
              >
                <SelectTrigger id="integration-id">
                  <SelectValue placeholder={t('dialog.integrationPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {catalog
                    .filter(item => item.enabled)
                    .map(item => (
                      <SelectItem
                        key={integrationCatalogID(item)}
                        value={integrationCatalogID(item)}
                      >
                        {metadata.providerName(item)}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="connection-name">{t('dialog.name')}</Label>
              <Input
                id="connection-name"
                value={name}
                maxLength={120}
                placeholder={t(
                  personal ? 'dialog.personalNamePlaceholder' : 'dialog.sharedNamePlaceholder'
                )}
                onChange={event => {
                  setName(event.target.value);
                  setValidationError(false);
                }}
              />
            </div>

            {!personal && !lockedAuthMethodId ? (
              <div className="space-y-2">
                <Label>{t('dialog.credentialSource')}</Label>
                <RadioCardGroup
                  value={credentialSource}
                  onValueChange={value => {
                    const source = value as IntegrationCredentialSource;
                    const definition = authDefinitions.find(
                      candidate =>
                        integrationAuthCredentialSource(candidate) === source && candidate.available
                    );
                    applyAuthDefinition(source, definition);
                  }}
                  className={
                    allowedCredentialSources?.length === 1
                      ? 'grid-cols-1'
                      : 'grid-cols-1 sm:grid-cols-2'
                  }
                >
                  {(!allowedCredentialSources ||
                    allowedCredentialSources.includes('organization')) && (
                    <RadioCard
                      value="organization"
                      title={t('dialog.organizationCredential')}
                      icon={<KeyRound className="size-5" />}
                      disabled={editing || !organizationAvailable}
                    />
                  )}
                  {(!allowedCredentialSources || allowedCredentialSources.includes('account')) && (
                    <RadioCard
                      value="account"
                      title={t('dialog.accountCredential')}
                      icon={<UserRound className="size-5" />}
                      disabled={editing || !accountAvailable}
                    />
                  )}
                </RadioCardGroup>
              </div>
            ) : null}

            {lockedAuthMethodId && selectedAuth ? (
              <div className="rounded-lg border bg-muted/20 px-3 py-2.5">
                <p className="text-xs font-medium text-muted-foreground">
                  {t('dialog.authMethod')}
                </p>
                <p className="mt-1 text-sm font-medium">
                  {metadata.authMethodLabel(integrationId, selectedAuth)}
                </p>
                {metadata.authMethodDescription(integrationId, selectedAuth) ? (
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {metadata.authMethodDescription(integrationId, selectedAuth)}
                  </p>
                ) : null}
              </div>
            ) : null}

            {!lockedAuthMethodId &&
            authDefinitions.filter(
              definition => integrationAuthCredentialSource(definition) === credentialSource
            ).length > 1 ? (
              <div className="space-y-2">
                <Label htmlFor="integration-auth-type">{t('dialog.authMethod')}</Label>
                <Select
                  value={authMethodId}
                  disabled={editing}
                  onValueChange={value => {
                    const definition = authDefinitions.find(candidate => candidate.id === value);
                    setAuthMethodId(value);
                    setAuthType(definition?.type ?? 'api_key');
                    setFieldValues(
                      initialFieldValues(
                        resolveCredentialFields(selectedIntegration, definition),
                        connection
                      )
                    );
                    setValidationError(false);
                  }}
                >
                  <SelectTrigger id="integration-auth-type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {authDefinitions
                      .filter(
                        definition =>
                          integrationAuthCredentialSource(definition) === credentialSource
                      )
                      .map(definition => (
                        <SelectItem
                          key={definition.id}
                          value={definition.id}
                          disabled={!definition.available}
                        >
                          {metadata.authMethodLabel(integrationId, definition)}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
                {selectedAuth && metadata.authMethodDescription(integrationId, selectedAuth) ? (
                  <p className="text-xs text-muted-foreground">
                    {metadata.authMethodDescription(integrationId, selectedAuth)}
                  </p>
                ) : null}
              </div>
            ) : null}

            {!oauthSelected && selectedAuth?.setup_guide ? (
              <AuthSetupGuide
                key={`${integrationId}:${selectedAuth.id}:${editing ? 'edit' : 'create'}`}
                providerName={
                  selectedAuth
                    ? metadata.authMethodLabel(integrationId, selectedAuth)
                    : selectedIntegration
                      ? metadata.providerName(selectedIntegration)
                      : t('common.unknownExternalApp')
                }
                guide={selectedAuth.setup_guide}
              />
            ) : null}

            {fields.map(field => {
              const fieldId = `integration-credential-${field.name}`;
              const value = fieldValues[field.name];
              const label = metadata.credentialFieldLabel(integrationId, field);
              const description = metadata.credentialFieldDescription(integrationId, field);
              const placeholder = metadata.credentialFieldPlaceholder(integrationId, field);
              if (field.type === 'boolean') {
                return (
                  <div
                    key={field.name}
                    className="flex items-start justify-between gap-4 rounded-lg border p-3"
                  >
                    <div>
                      <Label htmlFor={fieldId}>{label}</Label>
                      {description ? (
                        <p className="mt-1 text-xs text-muted-foreground">{description}</p>
                      ) : null}
                    </div>
                    <Switch
                      id={fieldId}
                      checked={Boolean(value)}
                      onCheckedChange={checked => {
                        setFieldValues(current => ({ ...current, [field.name]: checked }));
                        setValidationError(false);
                      }}
                    />
                  </div>
                );
              }

              return (
                <div key={field.name} className="space-y-2">
                  <Label htmlFor={fieldId}>
                    {label}
                    {field.required ? ' *' : ''}
                  </Label>
                  {field.type === 'select' ? (
                    <Select
                      value={typeof value === 'string' ? value : ''}
                      onValueChange={nextValue => {
                        setFieldValues(current => ({ ...current, [field.name]: nextValue }));
                        setValidationError(false);
                      }}
                    >
                      <SelectTrigger id={fieldId}>
                        <SelectValue placeholder={placeholder || t('dialog.selectValue')} />
                      </SelectTrigger>
                      <SelectContent>
                        {(field.options ?? []).map(option => (
                          <SelectItem key={option.value} value={option.value}>
                            {metadata.optionLabel(option)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : field.type === 'textarea' ? (
                    <Textarea
                      id={fieldId}
                      value={typeof value === 'string' ? value : ''}
                      placeholder={placeholder ?? undefined}
                      onChange={event => {
                        setFieldValues(current => ({
                          ...current,
                          [field.name]: event.target.value,
                        }));
                        setValidationError(false);
                      }}
                    />
                  ) : (
                    <Input
                      id={fieldId}
                      type={field.secret ? 'password' : 'text'}
                      autoComplete={field.secret ? 'new-password' : 'off'}
                      value={typeof value === 'string' ? value : ''}
                      placeholder={
                        editing && field.secret
                          ? t('dialog.keepExistingSecret')
                          : placeholder ||
                            (field.name === 'api_key' ? t('dialog.apiKeyPlaceholder') : undefined)
                      }
                      onChange={event => {
                        setFieldValues(current => ({
                          ...current,
                          [field.name]: event.target.value,
                        }));
                        setValidationError(false);
                      }}
                    />
                  )}
                  {description ? (
                    <p className="text-xs text-muted-foreground">{description}</p>
                  ) : null}
                  {editing && field.secret ? (
                    <p className="text-xs text-muted-foreground">{t('dialog.replaceSecretHint')}</p>
                  ) : null}
                </div>
              );
            })}

            {dynamicAuthUnsupported ? (
              <p className="text-sm text-warning">
                {oauthSelected && selectedAuth?.oauth && !selectedAuth.oauth.client_configured
                  ? t('oauth.clientConfig.adminSetupRequired')
                  : t('dialog.authUnavailable')}
              </p>
            ) : null}
            {validationError ? (
              <p className="text-sm text-destructive">{t('dialog.required')}</p>
            ) : null}
          </DialogBody>
          <DialogFooter className="border-t bg-muted/30">
            {!oauthSelected ? (
              <p className="mr-auto max-w-sm text-left text-xs leading-5 text-muted-foreground">
                {t('dialog.testAfterSaveNotice')}
              </p>
            ) : null}
            <Button variant="ghost" onClick={() => handleOpenChange(false)} disabled={isSubmitting}>
              {t('dialog.cancel')}
            </Button>
            <Button onClick={() => void submit()} disabled={isSubmitting || dynamicAuthUnsupported}>
              {oauthSelected && !editing ? <ExternalLink className="size-4" /> : null}
              {oauthSelected && !editing
                ? t('oauth.flow.continueToProvider', {
                    provider: selectedIntegration
                      ? metadata.providerName(selectedIntegration)
                      : t('common.unknownExternalApp'),
                  })
                : t(
                    editing
                      ? 'dialog.saveAndTest'
                      : personal
                        ? 'dialog.createAndTestPersonal'
                        : 'dialog.createAndTestShared'
                  )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <IntegrationOAuthFlowDialog
        state={oauthFlow.state}
        providerName={oauthProviderName}
        connectionName={oauthConnectionName}
        onCancel={oauthFlow.cancel}
        onDone={oauthFlow.reset}
        onContinue={
          oauthFlow.state.flow?.completed_connection_id && onOAuthCompleted
            ? () => {
                const connectionId = oauthFlow.state.flow?.completed_connection_id;
                oauthFlow.reset();
                if (connectionId) void onOAuthCompleted(connectionId);
              }
            : undefined
        }
        onRefresh={() => void oauthFlow.refresh()}
        onOpenFullPage={oauthFlow.openFullPage}
        onReopenPopup={oauthFlow.reopenPopup}
        onRetry={() => {
          if (lastOAuthRequest) void oauthFlow.begin(lastOAuthRequest);
        }}
      />
    </>
  );
}
