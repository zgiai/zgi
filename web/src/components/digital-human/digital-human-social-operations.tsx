'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  ListChecks,
  Loader2,
  Plus,
  RefreshCw,
  Rocket,
  Save,
  Trash2,
  Upload,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import {
  connectorCatalogItems,
  connectorTargetBindingItems,
  personalConnectionItems,
  useConnectorCatalog,
  useConnectorTargetBindings,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { useSocialOperations } from '@/hooks/digital-human/use-social-operations';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import type {
  ConnectorCatalogItem,
  ConnectorTargetBinding,
  PersonalConnection,
} from '@/services/types/connector';
import type {
  SocialCapability,
  SocialCapabilityMode,
  SocialOperationsCapabilityStatus,
} from '@/services/types/social-operations';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  buildSocialOperationsSettings,
  createSocialOperationsDraft,
  draftFromSocialOperationsConfiguration,
  initialVerifiedAccountId,
  isSupportedSocialOperationsHandoff,
  platformDraft,
  researchConnectionChoices,
  SOCIAL_CAPABILITIES,
  socialPlatformChoices,
  type SocialOperationsDraft,
  type SocialOperationsDraftError,
} from './digital-human-social-operations-model';

interface HandoffChoice {
  key: string;
  connectorId: string;
  connectionId: string;
  operationId: string;
  label: string;
}

const capabilityLabels = {
  account: 'socialOperations.capabilities.account',
  post: 'socialOperations.capabilities.post',
  readback: 'socialOperations.capabilities.readback',
  comment: 'socialOperations.capabilities.comment',
  inbox: 'socialOperations.capabilities.inbox',
  metrics: 'socialOperations.capabilities.metrics',
  images: 'socialOperations.capabilities.images',
} as const satisfies Record<SocialCapability, string>;

const errorLabels = {
  product: 'socialOperations.errors.product',
  audience: 'socialOperations.errors.audience',
  languages: 'socialOperations.errors.languages',
  sources: 'socialOperations.errors.sources',
  brandArtifacts: 'socialOperations.errors.brandArtifacts',
  research: 'socialOperations.errors.research',
  platforms: 'socialOperations.errors.platforms',
  account: 'socialOperations.errors.account',
  dailyLimit: 'socialOperations.errors.dailyLimit',
  timezone: 'socialOperations.errors.timezone',
  cadence: 'socialOperations.errors.cadence',
  handoff: 'socialOperations.errors.handoff',
} as const satisfies Record<SocialOperationsDraftError, string>;

const statusLabels = {
  ready: 'socialOperations.status.ready',
  missing: 'socialOperations.status.missing',
  unsupported: 'socialOperations.status.unsupported',
  optional_skipped: 'socialOperations.status.optionalSkipped',
} as const satisfies Record<SocialOperationsCapabilityStatus, string>;

function handoffChoices(
  employeeId: string,
  versionId: string,
  bindings: ConnectorTargetBinding[],
  connections: PersonalConnection[],
  catalog: ConnectorCatalogItem[]
): HandoffChoice[] {
  return bindings.flatMap(binding => {
    const connection = connections.find(item => item.id === binding.connection_id);
    const connector = catalog.find(item => item.id === binding.connector_id);
    if (
      !connection ||
      !connector ||
      connection.state !== 'ready' ||
      binding.status !== 'active' ||
      binding.target.type !== 'digital_human' ||
      binding.target.id !== employeeId ||
      binding.target.scope !== 'published' ||
      binding.target.version_id !== versionId ||
      binding.connector_id.endsWith('-social')
    ) {
      return [];
    }
    return connector.capabilities
      .filter(
        operation =>
          operation.usable &&
          operation.effect === 'external_send' &&
          isSupportedSocialOperationsHandoff(connector.id, operation.id) &&
          connection.usable_operation_ids?.includes(operation.id)
      )
      .map(operation => ({
        key: `${connection.id}:${operation.id}`,
        connectorId: connector.id,
        connectionId: connection.id,
        operationId: operation.id,
        label: `${connection.external_display_name || connection.name} · ${operation.name}`,
      }));
  });
}

export function DigitalHumanSocialOperations({ id }: { id: string }) {
  const account = useAuthStore.use.user();
  const organization = useOrganizationStore.use.currentOrganization();
  const workspace = useWorkspaceStore.use.currentWorkspace();
  const versionId = useDigitalHuman(id).data?.data.published?.id;
  const scope = [account?.id, organization?.id, workspace?.id, id, versionId].join(':');
  return <ScopedSocialOperations key={scope} id={id} versionId={versionId} />;
}

function ScopedSocialOperations({ id, versionId }: { id: string; versionId?: string }) {
  const t = useT('digitalHumans');
  const publishedTarget = versionId
    ? {
        type: 'digital_human' as const,
        id,
        scope: 'published' as const,
        version_id: versionId,
      }
    : undefined;
  const bindingsQuery = useConnectorTargetBindings(publishedTarget, Boolean(publishedTarget));
  const connectionsQuery = usePersonalConnections(undefined, Boolean(versionId));
  const catalogQuery = useConnectorCatalog(undefined, Boolean(versionId));
  const social = useSocialOperations(id, versionId, Boolean(versionId));
  const canUpdate = useAccountPermissions().hasPermission('digital_human.update');
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  const [draft, setDraft] = useState<SocialOperationsDraft>(() =>
    createSocialOperationsDraft(timezone)
  );
  const [formError, setFormError] = useState<string>();
  const [dirty, setDirty] = useState(false);
  const initialized = useRef<string>();
  const bindings = connectorTargetBindingItems(bindingsQuery.data?.data);
  const connections = personalConnectionItems(connectionsQuery.data?.data);
  const catalog = connectorCatalogItems(catalogQuery.data?.data);
  const choices = useMemo(
    () => (versionId ? socialPlatformChoices(id, versionId, bindings, connections) : []),
    [bindings, connections, id, versionId]
  );
  const researchChoices = useMemo(
    () =>
      versionId
        ? researchConnectionChoices(id, versionId, bindings, connections, catalog)
        : [],
    [bindings, catalog, connections, id, versionId]
  );
  const escalationChoices = useMemo(
    () => (versionId ? handoffChoices(id, versionId, bindings, connections, catalog) : []),
    [bindings, catalog, connections, id, versionId]
  );
  const displayedChoices = [
    ...choices,
    ...draft.platforms
      .filter(platform => !choices.some(choice => choice.connectionId === platform.connectionId))
      .map(platform => ({
        platform: platform.platform,
        connectorId: platform.connectorId,
        connectionId: platform.connectionId,
        accountId: platform.accountId,
        label: platform.label || platform.accountId,
      })),
  ];
  const response = social.query.data?.data;
  const configuration = response?.configuration;
  const record = response?.record;
  const preflight = social.preflight.data?.data;
  const provisioned = social.setup.data?.data;
  const verifiedAccountGroups = social.accounts.data?.data.items ?? [];
  const busy =
    social.save.isPending ||
    social.preflight.isPending ||
    social.setup.isPending ||
    social.uploadImage.isPending;
  const invalidResponse = Boolean(
    (configuration && !record) ||
      (!configuration && record) ||
      (configuration && record && configuration.revision !== record.revision)
  );

  useEffect(() => {
    if (!response) return;
    const marker = configuration
      ? `${configuration.employee_version_id}:${configuration.revision}`
      : 'empty';
    if (initialized.current === marker) return;
    setDraft(
      configuration
        ? draftFromSocialOperationsConfiguration(configuration)
        : createSocialOperationsDraft(timezone)
    );
    setDirty(false);
    initialized.current = marker;
  }, [configuration, response, timezone]);

  const update = <K extends keyof SocialOperationsDraft>(
    key: K,
    value: SocialOperationsDraft[K]
  ) => {
    setDraft(current => ({ ...current, [key]: value }));
    setDirty(true);
    setFormError(undefined);
  };
  const togglePlatform = (connectionId: string, checked: boolean) => {
    if (!checked) {
      update(
        'platforms',
        draft.platforms.filter(item => item.connectionId !== connectionId)
      );
      return;
    }
    const choice = choices.find(item => item.connectionId === connectionId);
    if (!choice) return;
    const verifiedAccounts =
      verifiedAccountGroups.find(
        item =>
          item.connector_id === choice.connectorId && item.connection_id === choice.connectionId
      )?.accounts ?? [];
    update('platforms', [
      ...draft.platforms.filter(item => item.connectionId !== connectionId),
      platformDraft({
        ...choice,
        accountId: initialVerifiedAccountId(choice.platform, verifiedAccounts),
      }),
    ]);
  };
  const updatePlatform = (
    connectionId: string,
    patch: Partial<SocialOperationsDraft['platforms'][number]>
  ) => {
    update(
      'platforms',
      draft.platforms.map(item =>
        item.connectionId === connectionId ? { ...item, ...patch } : item
      )
    );
  };
  const save = async () => {
    const result = buildSocialOperationsSettings(draft);
    if ('error' in result) {
      setFormError(t(errorLabels[result.error]));
      return;
    }
    setFormError(undefined);
    social.preflight.reset();
    social.setup.reset();
    try {
      const saved = await social.save.mutateAsync({
        expected_revision: record?.revision ?? 0,
        configuration: result.settings,
      });
      const savedConfiguration = saved.data.configuration;
      initialized.current = savedConfiguration
        ? `${savedConfiguration.employee_version_id}:${savedConfiguration.revision}`
        : undefined;
      if (savedConfiguration) setDraft(draftFromSocialOperationsConfiguration(savedConfiguration));
      setDirty(false);
      toast.success(t('socialOperations.saved'));
    } catch (error) {
      setFormError(getErrorMessage(error) || t('socialOperations.errors.save'));
    }
  };
  const runPreflight = async () => {
    setFormError(undefined);
    try {
      await social.preflight.mutateAsync();
    } catch (error) {
      setFormError(getErrorMessage(error) || t('socialOperations.errors.preflight'));
    }
  };
  const setup = async () => {
    if (!configuration || !record || !preflight?.ready) return;
    setFormError(undefined);
    try {
      await social.setup.mutateAsync({
        expected_configuration_revision: record.revision,
        expected_employee_version_id: configuration.employee_version_id,
      });
      toast.success(t('socialOperations.setupComplete'));
    } catch (error) {
      setFormError(getErrorMessage(error) || t('socialOperations.errors.setup'));
    }
  };
  const uploadBrandImages = async (files: FileList | null) => {
    if (!files || files.length === 0 || !canUpdate) return;
    const selected = Array.from(files);
    if (
      selected.some(
        file =>
          file.size < 1 ||
          file.size > 8 * 1024 * 1024 ||
          !['image/png', 'image/jpeg', 'image/webp'].includes(file.type)
      )
    ) {
      setFormError(t('socialOperations.errors.invalidImage'));
      return;
    }
    setFormError(undefined);
    try {
      for (const file of selected) {
        const response = await social.uploadImage.mutateAsync(file);
        const artifactId = response.data.artifact.id;
        setDraft(current => ({
          ...current,
          brandArtifactRefs: Array.from(new Set([...current.brandArtifactRefs, artifactId])),
        }));
        setDirty(true);
      }
      toast.success(t('socialOperations.imagesUploaded', { count: selected.length }));
    } catch (error) {
      setFormError(getErrorMessage(error) || t('socialOperations.errors.uploadImage'));
    }
  };

  if (!versionId) {
    return (
      <section className="mt-5 rounded-xl border bg-card p-4" data-testid="social-operations">
        <h3 className="font-medium">{t('socialOperations.title')}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{t('socialOperations.publishFirst')}</p>
      </section>
    );
  }

  const loading =
    social.query.isLoading ||
    social.accounts.isLoading ||
    bindingsQuery.isLoading ||
    connectionsQuery.isLoading ||
    catalogQuery.isLoading;
  const loadError =
    invalidResponse ||
    social.query.isError ||
    social.accounts.isError ||
    bindingsQuery.isError ||
    connectionsQuery.isError ||
    catalogQuery.isError;
  return (
    <section
      className="mt-5 overflow-hidden rounded-xl border bg-card shadow-sm"
      data-testid="social-operations"
    >
      <div className="flex flex-wrap items-start justify-between gap-3 border-b bg-muted/15 px-4 py-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <ListChecks className="size-4 text-primary" />
            <h3 className="font-medium">{t('socialOperations.title')}</h3>
            {configuration ? <Badge variant="subtle">r{configuration.revision}</Badge> : null}
          </div>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">
            {t('socialOperations.description')}
          </p>
        </div>
        <Badge variant={preflight?.ready ? 'success' : 'secondary'}>
          {preflight?.ready ? t('socialOperations.ready') : t('socialOperations.notReady')}
        </Badge>
      </div>

      {loading ? (
        <div className="m-4 h-32 animate-pulse rounded-lg bg-muted" />
      ) : loadError ? (
        <div
          role="alert"
          className="m-4 flex items-center justify-between gap-3 rounded-lg border p-3 text-sm"
        >
          <span>{t('socialOperations.errors.load')}</span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              void Promise.all([
                social.query.refetch(),
                social.accounts.refetch(),
                bindingsQuery.refetch(),
                connectionsQuery.refetch(),
                catalogQuery.refetch(),
              ])
            }
          >
            <RefreshCw className="size-4" />
            {t('common.refresh')}
          </Button>
        </div>
      ) : (
        <div className="space-y-5 p-4">
          <fieldset disabled={!canUpdate || busy} className="space-y-5 disabled:opacity-70">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label={t('socialOperations.productName')}>
                <Input
                  value={draft.productName}
                  maxLength={256}
                  onChange={event => update('productName', event.target.value)}
                />
              </Field>
              <Field label={t('socialOperations.audience')}>
                <Input
                  value={draft.audience}
                  maxLength={2000}
                  onChange={event => update('audience', event.target.value)}
                />
              </Field>
              <Field label={t('socialOperations.productDescription')} className="sm:col-span-2">
                <Textarea
                  value={draft.productDescription}
                  maxLength={4000}
                  showCharacterCount
                  onChange={event => update('productDescription', event.target.value)}
                />
              </Field>
              <Field label={t('socialOperations.publicationLanguage')}>
                <Input
                  value={draft.publicationLanguage}
                  maxLength={64}
                  onChange={event => update('publicationLanguage', event.target.value)}
                />
              </Field>
              <Field label={t('socialOperations.reviewLanguage')}>
                <Input
                  value={draft.internalReviewLanguage}
                  maxLength={64}
                  onChange={event => update('internalReviewLanguage', event.target.value)}
                />
              </Field>
            </div>

            <div className="space-y-2">
              <Label>{t('socialOperations.sources')}</Label>
              {draft.sourceUrls.map((url, index) => (
                <div key={index} className="flex gap-2">
                  <Input
                    type="url"
                    value={url}
                    placeholder="https://"
                    onChange={event =>
                      update(
                        'sourceUrls',
                        draft.sourceUrls.map((item, position) =>
                          position === index ? event.target.value : item
                        )
                      )
                    }
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    isIcon
                    aria-label={t('socialOperations.removeSource')}
                    disabled={draft.sourceUrls.length === 1}
                    onClick={() =>
                      update(
                        'sourceUrls',
                        draft.sourceUrls.filter((_, position) => position !== index)
                      )
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              ))}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => update('sourceUrls', [...draft.sourceUrls, ''])}
              >
                <Plus className="size-4" />
                {t('socialOperations.addSource')}
              </Button>
            </div>

            <div className="space-y-3 rounded-lg border p-3">
              <div>
                <h4 className="text-sm font-medium">{t('socialOperations.research')}</h4>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('socialOperations.researchHint')}
                </p>
              </div>
              <Field label={t('socialOperations.researchConnection')}>
                <Select
                  value={
                    draft.research.connectionId
                      ? `${draft.research.connectorId}:${draft.research.connectionId}`
                      : ''
                  }
                  onValueChange={value => {
                    const choice = researchChoices.find(
                      item => `${item.connectorId}:${item.connectionId}` === value
                    );
                    if (choice) {
                      update('research', {
                        connectorId: choice.connectorId,
                        connectionId: choice.connectionId,
                      });
                    }
                  }}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t('socialOperations.researchPlaceholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {draft.research.connectionId &&
                    !researchChoices.some(
                      item =>
                        item.connectorId === draft.research.connectorId &&
                        item.connectionId === draft.research.connectionId
                    ) ? (
                      <SelectItem
                        value={`${draft.research.connectorId}:${draft.research.connectionId}`}
                        disabled
                      >
                        {draft.research.connectionId} ·{' '}
                        {t('socialOperations.researchUnavailable')}
                      </SelectItem>
                    ) : null}
                    {researchChoices.map(choice => (
                      <SelectItem
                        key={`${choice.connectorId}:${choice.connectionId}`}
                        value={`${choice.connectorId}:${choice.connectionId}`}
                      >
                        {choice.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {researchChoices.length === 0 ? (
                  <p className="mt-1.5 text-xs text-amber-700 dark:text-amber-300">
                    {t('socialOperations.noResearchConnections')}
                  </p>
                ) : null}
              </Field>
            </div>

            <div className="space-y-3 rounded-lg border p-3">
              <div>
                <h4 className="text-sm font-medium">{t('socialOperations.platforms')}</h4>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('socialOperations.platformsHint')}
                </p>
              </div>
              {displayedChoices.length === 0 ? (
                <p className="rounded-md bg-muted/40 p-3 text-xs text-muted-foreground">
                  {t('socialOperations.noSocialConnections')}
                </p>
              ) : (
                displayedChoices.map(choice => {
                  const selected = draft.platforms.find(
                    item => item.connectionId === choice.connectionId
                  );
                  const available = choices.some(item => item.connectionId === choice.connectionId);
                  const verifiedGroup = verifiedAccountGroups.find(
                    item =>
                      item.connector_id === choice.connectorId &&
                      item.connection_id === choice.connectionId
                  );
                  const verifiedAccounts = verifiedGroup?.accounts ?? [];
                  const selectedAccount = selected
                    ? verifiedAccounts.find(account => account.id === selected.accountId)
                    : undefined;
                  return (
                    <div key={choice.connectionId} className="rounded-lg border bg-background p-3">
                      <label className="flex cursor-pointer items-center gap-2 text-sm font-medium">
                        <Checkbox
                          checked={Boolean(selected)}
                          onCheckedChange={checked =>
                            togglePlatform(choice.connectionId, checked === true)
                          }
                        />
                        <span>{choice.label}</span>
                        <Badge variant="outline">{choice.platform}</Badge>
                        {!available ? (
                          <Badge variant="secondary">
                            {t('socialOperations.unavailableConnection')}
                          </Badge>
                        ) : null}
                      </label>
                      {selected ? (
                        <div className="mt-3 space-y-3 border-t pt-3">
                          <div className="grid gap-3 sm:grid-cols-2">
                            <Field label={t('socialOperations.accountId')}>
                              <Select
                                value={selected.accountId}
                                disabled={verifiedAccounts.length === 0}
                                onValueChange={accountId =>
                                  updatePlatform(choice.connectionId, {
                                    accountId,
                                  })
                                }
                              >
                                <SelectTrigger>
                                  <SelectValue
                                    placeholder={t('socialOperations.accountPlaceholder')}
                                  />
                                </SelectTrigger>
                                <SelectContent>
                                  {selected.accountId && !selectedAccount ? (
                                    <SelectItem value={selected.accountId} disabled>
                                      {selected.accountId} ·{' '}
                                      {t('socialOperations.accountUnverified')}
                                    </SelectItem>
                                  ) : null}
                                  {verifiedAccounts.map(account => (
                                    <SelectItem key={account.id} value={account.id}>
                                      {account.label || account.id}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                              {verifiedAccounts.length === 0 ? (
                                <p className="mt-1.5 text-xs text-amber-700 dark:text-amber-300">
                                  {t('socialOperations.noVerifiedAccounts')}
                                </p>
                              ) : selected.accountId && !selectedAccount ? (
                                <p className="mt-1.5 text-xs text-amber-700 dark:text-amber-300">
                                  {t('socialOperations.accountUnverifiedHint')}
                                </p>
                              ) : selectedAccount ? (
                                <p className="mt-1.5 text-xs text-muted-foreground">
                                  {t('socialOperations.accountVerified')}
                                </p>
                              ) : null}
                              <details className="mt-2">
                                <summary className="cursor-pointer text-xs text-muted-foreground">
                                  {t('socialOperations.advancedAccountEntry')}
                                </summary>
                                <Input
                                  className="mt-2"
                                  value={selected.accountId}
                                  maxLength={256}
                                  placeholder={t('socialOperations.advancedAccountPlaceholder')}
                                  onChange={event =>
                                    updatePlatform(choice.connectionId, {
                                      accountId: event.target.value,
                                    })
                                  }
                                />
                                <p className="mt-1.5 text-xs text-muted-foreground">
                                  {t('socialOperations.advancedAccountHint')}
                                </p>
                              </details>
                            </Field>
                            <Field label={t('socialOperations.dailyMax')}>
                              <Input
                                type="number"
                                min={1}
                                max={100}
                                value={selected.maxDailyPublications}
                                onChange={event =>
                                  updatePlatform(choice.connectionId, {
                                    maxDailyPublications: Number(event.target.value),
                                  })
                                }
                              />
                            </Field>
                          </div>
                          <div className="grid gap-2 sm:grid-cols-2">
                            {SOCIAL_CAPABILITIES.map(capability => {
                              const linkedinUnavailable =
                                selected.platform === 'linkedin' && capability === 'inbox';
                              return (
                                <div
                                  key={capability}
                                  className="flex items-center justify-between gap-2 rounded-md bg-muted/35 px-2.5 py-2"
                                >
                                  <span className="text-xs">{t(capabilityLabels[capability])}</span>
                                  {linkedinUnavailable ? (
                                    <Badge variant="secondary">
                                      {t('socialOperations.linkedinCompanyInboxUnavailable')}
                                    </Badge>
                                  ) : (
                                    <Select
                                      value={selected.capabilities[capability]}
                                      onValueChange={mode =>
                                        updatePlatform(choice.connectionId, {
                                          capabilities: {
                                            ...selected.capabilities,
                                            [capability]: mode as SocialCapabilityMode,
                                          },
                                        })
                                      }
                                    >
                                      <SelectTrigger className="h-8 w-28">
                                        <SelectValue />
                                      </SelectTrigger>
                                      <SelectContent>
                                        <SelectItem value="required">
                                          {t('socialOperations.required')}
                                        </SelectItem>
                                        <SelectItem value="optional">
                                          {t('socialOperations.optional')}
                                        </SelectItem>
                                      </SelectContent>
                                    </Select>
                                  )}
                                </div>
                              );
                            })}
                          </div>
                        </div>
                      ) : null}
                    </div>
                  );
                })
              )}
            </div>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field label={t('socialOperations.timezone')}>
                <Input
                  value={draft.timezone}
                  onChange={event => update('timezone', event.target.value)}
                />
              </Field>
              <div />
              {(
                [
                  ['dailyContentCron', 'socialOperations.cadence.dailyContent'],
                  ['inboxCron', 'socialOperations.cadence.inbox'],
                  ['sevenDaySampleCron', 'socialOperations.cadence.sevenDay'],
                  ['weeklyReviewCron', 'socialOperations.cadence.weekly'],
                ] as const
              ).map(([key, label]) => (
                <Field key={key} label={t(label)}>
                  <Input
                    value={draft.cadence[key]}
                    onChange={event =>
                      update('cadence', { ...draft.cadence, [key]: event.target.value })
                    }
                  />
                </Field>
              ))}
            </div>

            <div className="space-y-3 rounded-lg border p-3">
              <div>
                <h4 className="text-sm font-medium">{t('socialOperations.handoff')}</h4>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('socialOperations.handoffHint')}
                </p>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label={t('socialOperations.handoffOperation')}>
                  <Select
                    value={
                      draft.handoff.operationId
                        ? `${draft.handoff.connectionId}:${draft.handoff.operationId}`
                        : ''
                    }
                    onValueChange={key => {
                      const choice = escalationChoices.find(item => item.key === key);
                      if (choice) {
                        update('handoff', {
                          connectorId: choice.connectorId,
                          connectionId: choice.connectionId,
                          operationId: choice.operationId,
                          destination: draft.handoff.destination,
                        });
                      }
                    }}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder={t('socialOperations.handoffPlaceholder')} />
                    </SelectTrigger>
                    <SelectContent>
                      {escalationChoices.map(choice => (
                        <SelectItem key={choice.key} value={choice.key}>
                          {choice.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Field label={t('socialOperations.destination')}>
                  <Input
                    value={draft.handoff.destination}
                    maxLength={512}
                    onChange={event =>
                      update('handoff', { ...draft.handoff, destination: event.target.value })
                    }
                  />
                </Field>
              </div>
            </div>

            <details className="rounded-lg border px-3 py-2">
              <summary className="flex cursor-pointer list-none items-center justify-between text-sm font-medium">
                {t('socialOperations.advanced')}
                <ChevronDown className="size-4" />
              </summary>
              <div className="mt-3">
                <Field label={t('socialOperations.brandImages')}>
                  <div className="mb-3 rounded-lg border border-dashed bg-muted/20 p-3">
                    <label className="inline-flex cursor-pointer items-center gap-2 rounded-md border bg-background px-3 py-2 text-sm font-medium shadow-xs hover:bg-muted/40">
                      {social.uploadImage.isPending ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <Upload className="size-4" />
                      )}
                      {t('socialOperations.uploadBrandImages')}
                      <input
                        className="sr-only"
                        type="file"
                        multiple
                        accept="image/png,image/jpeg,image/webp"
                        disabled={!canUpdate || busy}
                        onChange={event => {
                          const input = event.currentTarget;
                          void uploadBrandImages(input.files).finally(() => {
                            input.value = '';
                          });
                        }}
                      />
                    </label>
                    <p className="mt-2 text-xs text-muted-foreground">
                      {t('socialOperations.brandImagesHint')}
                    </p>
                  </div>
                </Field>
                <Field label={t('socialOperations.brandArtifacts')}>
                  <Textarea
                    value={draft.brandArtifactRefs.join('\n')}
                    placeholder={t('socialOperations.brandArtifactsPlaceholder')}
                    onChange={event =>
                      update('brandArtifactRefs', event.target.value.split(/\r?\n/))
                    }
                  />
                </Field>
              </div>
            </details>
          </fieldset>

          {formError ? (
            <p
              role="alert"
              className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive"
            >
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              {formError}
            </p>
          ) : null}

          <div className="flex flex-wrap gap-2 border-t pt-4">
            <Button
              type="button"
              disabled={!canUpdate || busy || invalidResponse}
              onClick={() => void save()}
            >
              {social.save.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Save className="size-4" />
              )}
              {t('socialOperations.save')}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={!canUpdate || busy || dirty || !configuration || !record}
              onClick={() => void runPreflight()}
            >
              {social.preflight.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <ListChecks className="size-4" />
              )}
              {t('socialOperations.runPreflight')}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={
                !canUpdate ||
                busy ||
                dirty ||
                !preflight?.ready ||
                preflight.configuration_revision !== configuration?.revision ||
                preflight.published_employee_version_id !== configuration?.employee_version_id ||
                record?.revision !== configuration?.revision
              }
              onClick={() => void setup()}
            >
              {social.setup.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Rocket className="size-4" />
              )}
              {t('socialOperations.startTasks')}
            </Button>
          </div>

          {preflight ? (
            <div className="rounded-lg border p-3">
              <div className="flex items-center gap-2 text-sm font-medium">
                {preflight.ready ? (
                  <CheckCircle2 className="size-4 text-emerald-600" />
                ) : (
                  <AlertCircle className="size-4 text-amber-600" />
                )}
                {preflight.ready
                  ? t('socialOperations.preflightReady')
                  : t('socialOperations.preflightBlocked')}
              </div>
              {(preflight.prerequisites ?? []).length > 0 ? (
                <div className="mt-3 space-y-2">
                  <p className="text-xs font-medium">{t('socialOperations.prerequisites')}</p>
                  {(preflight.prerequisites ?? []).map(prerequisite => (
                    <div
                      key={prerequisite.key}
                      className="flex items-start justify-between gap-3 rounded-md bg-muted/35 p-2.5 text-xs"
                    >
                      <div>
                        <p className="font-medium">
                          {prerequisite.key === 'default_model'
                            ? t('socialOperations.defaultModelPrerequisite')
                            : prerequisite.key === 'social_operations_skill'
                              ? t('socialOperations.skillPrerequisite')
                              : prerequisite.key}
                        </p>
                        <p className="mt-1 text-muted-foreground">{prerequisite.reason}</p>
                      </div>
                      <Badge variant={prerequisite.ready ? 'success' : 'secondary'}>
                        {prerequisite.ready
                          ? t('socialOperations.ready')
                          : t('socialOperations.notReady')}
                      </Badge>
                    </div>
                  ))}
                </div>
              ) : null}
              <div className="mt-3 space-y-2">
                {preflight.platforms.map(platform => (
                  <div
                    key={`${platform.connection_id}:${platform.account_id}`}
                    className="rounded-md bg-muted/35 p-2.5 text-xs"
                  >
                    <div className="flex items-center justify-between gap-2 font-medium">
                      <span>
                        {platform.platform} · {platform.account_id}
                      </span>
                      <Badge variant={platform.ready ? 'success' : 'secondary'}>
                        {platform.ready
                          ? t('socialOperations.ready')
                          : t('socialOperations.notReady')}
                      </Badge>
                    </div>
                    {platform.reason ? (
                      <p className="mt-1 text-muted-foreground">{platform.reason}</p>
                    ) : null}
                    <div className="mt-2 flex flex-wrap gap-1">
                      {platform.findings.map(finding => (
                        <Badge
                          key={finding.capability}
                          variant={finding.status === 'ready' ? 'outline' : 'secondary'}
                        >
                          {t(capabilityLabels[finding.capability])}:{' '}
                          {t(statusLabels[finding.status])}
                        </Badge>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                {t('socialOperations.handoffStatus')}:{' '}
                {preflight.handoff.reason ||
                  (preflight.handoff.ready
                    ? t('socialOperations.ready')
                    : t('socialOperations.notReady'))}
              </p>
            </div>
          ) : null}

          {provisioned ? (
            <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-3">
              <p className="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                {t('socialOperations.tasksCreated', { count: provisioned.tasks.length })}
              </p>
              <ul className="mt-2 grid gap-1 text-xs text-muted-foreground sm:grid-cols-2">
                {provisioned.tasks.map(item => (
                  <li key={item.operation_id}>{item.task.name}</li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      )}
    </section>
  );
}

function Field({
  label,
  children,
  className,
}: {
  label: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={className}>
      <Label className="mb-1.5 block">{label}</Label>
      {children}
    </div>
  );
}
