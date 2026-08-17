'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Bot,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Circle,
  Loader2,
  MessageSquare,
  Pencil,
  ShieldCheck,
  TestTube2,
  Workflow,
} from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
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
import { Switch } from '@/components/ui/switch';
import {
  integrationConnectionGrantItems,
  integrationConnectionItems,
  useIntegrationConnectionGrants,
  useTestIntegrationConnection,
  useTestMyIntegrationConnection,
} from '@/hooks';
import { AICHAT_KEYS, INTEGRATION_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { integrationService } from '@/services/integration.service';
import type { ApiResponseData } from '@/services/types/common';
import type {
  AIChatIntegrationPreferenceInput,
  IntegrationCatalogItem,
  IntegrationConnection,
  IntegrationConnectionHealthEvent,
} from '@/services/types/integration';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { actionsForAuthMethod } from './action-auth-compatibility';
import {
  IntegrationConnectionActionSetup,
  type ConnectionActionSetupHandle,
  type ConnectionActionSetupState,
} from './connection-action-setup';
import { IntegrationConnectionGrantsPanel } from './connection-grants-panel';
import { IntegrationConnectionHealthBadge } from './health-badge';
import {
  resolveAutomaticConnectionVerification,
  resolveConnectionSetupInitialization,
} from './connection-setup-lifecycle';
import { IntegrationConnectionPermissionSummary } from './connection-permission-summary';
import { ProviderDiagnosticsDetails } from './provider-diagnostics-details';
import { useIntegrationMetadata } from './metadata-i18n';

interface IntegrationConnectionSetupDialogProps {
  open: boolean;
  connection: IntegrationConnection | null;
  provider?: IntegrationCatalogItem;
  canManageShared?: boolean;
  initialStep?: number;
  onOpenChange: (open: boolean) => void;
  onCompleted: (connection: IntegrationConnection) => void;
  onEdit?: (connection: IntegrationConnection) => void;
}

type ConnectionVerificationState = 'idle' | 'testing' | 'passed' | 'failed';

function updateAIChatPreference(
  existing: AIChatIntegrationPreferenceInput[],
  connection: IntegrationConnection,
  enabled: boolean
): AIChatIntegrationPreferenceInput[] {
  const next = existing.map(item => ({
    integration_id: item.integration_id,
    selected_connection_ids: [...item.selected_connection_ids],
    preferred_connection_id: item.preferred_connection_id,
  }));
  const item = next.find(candidate => candidate.integration_id === connection.integration_id);
  if (!enabled) {
    if (!item) return next;
    item.selected_connection_ids = item.selected_connection_ids.filter(id => id !== connection.id);
    if (item.selected_connection_ids.length === 0) {
      return next.filter(candidate => candidate !== item);
    }
    if (item.preferred_connection_id === connection.id) {
      item.preferred_connection_id = item.selected_connection_ids[0];
    }
    return next;
  }
  if (item) {
    item.selected_connection_ids = [...new Set([...item.selected_connection_ids, connection.id])];
    item.preferred_connection_id = connection.id;
  } else {
    next.push({
      integration_id: connection.integration_id,
      selected_connection_ids: [connection.id],
      preferred_connection_id: connection.id,
    });
  }
  return next;
}

export function IntegrationConnectionSetupDialog({
  open,
  connection,
  provider,
  canManageShared = false,
  initialStep = 0,
  onOpenChange,
  onCompleted,
  onEdit,
}: IntegrationConnectionSetupDialogProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const currentWorkspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const initializedConnectionIDRef = useRef<string | null>(null);
  const initializedAIChatConnectionIDRef = useRef<string | null>(null);
  const automaticallyVerifiedConnectionIDRef = useRef<string | null>(null);
  const actionSetupRef = useRef<ConnectionActionSetupHandle | null>(null);
  const [item, setItem] = useState<IntegrationConnection | null>(connection);
  const [step, setStep] = useState(0);
  const [completed, setCompleted] = useState(false);
  const [verificationState, setVerificationState] = useState<ConnectionVerificationState>('idle');
  const [verificationDiagnostics, setVerificationDiagnostics] =
    useState<IntegrationConnectionHealthEvent | null>(null);
  const [aiChatEnabled, setAIChatEnabled] = useState(false);
  const [aiChatTouched, setAIChatTouched] = useState(false);
  const [actionSetupState, setActionSetupState] = useState<ConnectionActionSetupState>({
    ready: false,
    loading: true,
    dirty: false,
    saving: false,
    enabledActionIDs: [],
  });
  const personal = item?.credential_source === 'account';
  const actions = actionsForAuthMethod(provider?.actions ?? [], item?.auth_method_id);
  const grantsQuery = useIntegrationConnectionGrants(
    item?.id ?? '',
    open && Boolean(item) && !personal
  );
  const preferencesQuery = useQuery({
    queryKey: AICHAT_KEYS.integrationPreferences(),
    queryFn: () => integrationService.getAIChatPreferences(),
    enabled: open && Boolean(item),
    staleTime: 0,
    retry: false,
  });
  const availableConnectionsQuery = useQuery({
    queryKey: INTEGRATION_KEYS.availableConnections({ all: true }),
    queryFn: () => integrationService.getAllAvailableConnections(),
    enabled: open && Boolean(item),
    staleTime: 0,
    retry: false,
  });
  const testShared = useTestIntegrationConnection();
  const testPersonal = useTestMyIntegrationConnection();

  const test = useCallback(async () => {
    if (!item) return false;
    setVerificationState('testing');
    setVerificationDiagnostics(null);
    try {
      const response =
        item.credential_source === 'account'
          ? await testPersonal.mutateAsync(item.id)
          : await testShared.mutateAsync(item.id);
      setItem(response.data.connection);
      setVerificationState('passed');
      setStep(current => (current === 0 ? 1 : current));
      return true;
    } catch {
      setVerificationState('failed');
      if (item.credential_source !== 'account') {
        try {
          const healthResponse = await integrationService.getConnectionHealthEvents(item.id, {
            page: 1,
            limit: 1,
          });
          setVerificationDiagnostics(healthResponse.data.items?.[0] ?? null);
        } catch {
          // Diagnostics are supplemental. Keep the actionable retry/edit path
          // available even when health history cannot be loaded.
        }
      }
      return false;
    }
  }, [item, testPersonal, testShared]);

  useEffect(() => {
    const connectionID = connection?.id ?? null;
    const initialization = resolveConnectionSetupInitialization(
      initializedConnectionIDRef.current,
      connectionID,
      open
    );
    initializedConnectionIDRef.current = initialization.initializedConnectionID;
    if (!initialization.shouldReset) return;
    setItem(connection);
    const usageTargetStep = connection?.credential_source === 'account' ? 3 : 4;
    setStep(
      initialStep === 3 ? usageTargetStep : Math.min(Math.max(initialStep, 0), usageTargetStep)
    );
    setCompleted(false);
    setVerificationState(initialStep === 0 ? 'idle' : 'passed');
    setVerificationDiagnostics(null);
    setAIChatEnabled(false);
    setAIChatTouched(false);
    setActionSetupState({
      ready: false,
      loading: true,
      dirty: false,
      saving: false,
      enabledActionIDs: [],
    });
    initializedAIChatConnectionIDRef.current = null;
  }, [connection, initialStep, open]);

  useEffect(() => {
    if (!open) {
      automaticallyVerifiedConnectionIDRef.current = null;
      return;
    }
    if (!item || item.id !== connection?.id || step !== 0 || initialStep !== 0) return;
    if (automaticallyVerifiedConnectionIDRef.current === item.id) return;

    automaticallyVerifiedConnectionIDRef.current = item.id;
    const decision = resolveAutomaticConnectionVerification({
      open,
      initialStep,
      verified:
        item.status === 'active' &&
        item.auth_status === 'valid' &&
        item.health_status !== 'unhealthy',
      lastTestedAt: item.last_tested_at,
    });
    if (decision === 'reuse') {
      setVerificationState('passed');
      setStep(1);
      return;
    }
    if (decision === 'run') void test();
  }, [connection?.id, initialStep, item, open, step, test]);

  useEffect(() => {
    if (
      !open ||
      !item ||
      preferencesQuery.isLoading ||
      initializedAIChatConnectionIDRef.current === item.id
    ) {
      return;
    }
    const selected = (preferencesQuery.data?.data.items ?? []).some(preference =>
      preference.selected_connection_ids?.includes(item.id)
    );
    setAIChatEnabled(selected);
    initializedAIChatConnectionIDRef.current = item.id;
  }, [item, open, preferencesQuery.data?.data.items, preferencesQuery.isLoading]);

  const usableCapabilities = useMemo(
    () =>
      item?.permission_summary?.adapted_capabilities?.filter(
        capability => capability.scope_satisfied
      ) ?? [],
    [item?.permission_summary?.adapted_capabilities]
  );
  const validActionIDs = useMemo(
    () => new Set(usableCapabilities.map(action => action.action_id)),
    [usableCapabilities]
  );
  const grants = integrationConnectionGrantItems(grantsQuery.data?.data);
  const hasUsageRule =
    personal ||
    grants.some(
      grant =>
        grant.principal_state !== 'missing' &&
        grant.allowed_action_ids.some(actionID => validActionIDs.has(actionID))
    );
  const grantedActionIDs = useMemo(
    () =>
      new Set(
        grants.flatMap(grant =>
          grant.principal_state === 'missing' ? [] : grant.allowed_action_ids
        )
      ),
    [grants]
  );
  const hasExecutableAction = usableCapabilities.some(
    capability =>
      actionSetupState.enabledActionIDs.includes(capability.action_id) &&
      (personal || grantedActionIDs.has(capability.action_id))
  );
  const verified = Boolean(
    item?.status === 'active' && item.auth_status === 'valid' && item.health_status !== 'unhealthy'
  );
  const aiChatAvailable = integrationConnectionItems(availableConnectionsQuery.data?.data).some(
    connection => connection.id === item?.id
  );
  const supportedCallers = useMemo(
    () => new Set(actions.flatMap(action => action.supported_callers ?? [])),
    [actions]
  );
  const agentAvailable = !personal && supportedCallers.has('agent');
  const executionRulesStep = 2;
  const usageRulesStep = personal ? -1 : 3;
  const usageTargetsStep = personal ? 3 : 4;
  const usageTargetsFocused = initialStep === 3 && step === usageTargetsStep && !completed;

  const steps = personal
    ? [
        t('setup.steps.verify'),
        t('setup.steps.capabilities'),
        t('setup.steps.executionRules'),
        t('setup.steps.usageTargets'),
      ]
    : [
        t('setup.steps.verify'),
        t('setup.steps.capabilities'),
        t('setup.steps.executionRules'),
        t('setup.steps.usageRules'),
        t('setup.steps.usageTargets'),
      ];
  const stepReady = personal
    ? [
        verified && verificationState === 'passed',
        usableCapabilities.length > 0,
        actionSetupState.ready && !actionSetupState.loading && !actionSetupState.saving,
        true,
      ]
    : [
        verified && verificationState === 'passed',
        usableCapabilities.length > 0,
        actionSetupState.ready && !actionSetupState.loading && !actionSetupState.saving,
        hasUsageRule && hasExecutableAction,
        true,
      ];

  const completeMutation = useMutation<ApiResponseData<IntegrationConnection>, Error, void>({
    mutationFn: async () => {
      if (!item) throw new Error('connection setup is incomplete');
      if (aiChatTouched) {
        // Reload immediately before the replace-style write. Using a stale or
        // partially loaded preference snapshot could otherwise remove another
        // app selected in this workspace.
        const preferenceResponse = await integrationService.getAIChatPreferences();
        const existing = (preferenceResponse.data.items ?? []).map(preference => ({
          integration_id: preference.integration_id,
          selected_connection_ids: preference.selected_connection_ids ?? [],
          preferred_connection_id: preference.preferred_connection_id,
        }));
        await integrationService.replaceAIChatPreferences({
          items: updateAIChatPreference(existing, item, aiChatEnabled),
        });
      }
      return personal
        ? integrationService.completeMyConnectionSetup(item.id, {})
        : integrationService.completeConnectionSetup(item.id, {});
    },
    onSuccess: async response => {
      setItem(response.data);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.connections() }),
        queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.myConnectionLists() }),
        queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.availableConnectionLists() }),
        queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.catalog() }),
        queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.capabilityLists() }),
        queryClient.invalidateQueries({ queryKey: AICHAT_KEYS.integrationPreferences() }),
      ]);
      setCompleted(true);
      onCompleted(response.data);
    },
  });

  const handleNext = async () => {
    if (step < steps.length - 1) {
      if (step === executionRulesStep) {
        try {
          await actionSetupRef.current?.save();
        } catch {
          return;
        }
      }
      if (step === usageRulesStep) {
        // A shared connection often becomes available only after its first
        // usage rule is created on this step. Refresh before rendering the
        // usage-target step so its AIChat option reflects the rules that were
        // just saved. Other targets keep their own configuration boundaries.
        await Promise.all([availableConnectionsQuery.refetch(), preferencesQuery.refetch()]);
      }
      setStep(current => current + 1);
      return;
    }
    try {
      await completeMutation.mutateAsync();
    } catch {
      // Keep the current step open so the user can correct it and retry.
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={nextOpen => {
        if (!nextOpen && completeMutation.isPending) return;
        onOpenChange(nextOpen);
      }}
    >
      <DialogContent size="lg" className="p-0">
        <DialogHeader>
          <DialogTitle>
            {completed
              ? t(initialStep === 3 ? 'setup.usageTargetsSavedTitle' : 'setup.completedTitle')
              : t(usageTargetsFocused ? 'setup.usageTargets.dialogTitle' : 'setup.title')}
          </DialogTitle>
          <DialogDescription>
            {completed
              ? t(
                  initialStep === 3
                    ? 'setup.usageTargetsSavedDescription'
                    : 'setup.completedDescription',
                  { connection: item?.name ?? '' }
                )
              : usageTargetsFocused
                ? t('setup.usageTargets.dialogDescription', {
                    connection: item?.name ?? '',
                    provider: provider ? metadata.providerName(provider) : '',
                  })
                : t('setup.description', {
                    connection: item?.name ?? '',
                    provider: provider ? metadata.providerName(provider) : '',
                  })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5 pb-5">
          {completed ? (
            <div className="rounded-xl border border-success/30 bg-success/5 p-8 text-center">
              <CheckCircle2 className="mx-auto size-10 text-success" />
              <h3 className="mt-3 font-semibold">{t('setup.summary.ready')}</h3>
              <p className="mx-auto mt-1 max-w-lg text-sm leading-6 text-muted-foreground">
                {t(aiChatEnabled ? 'setup.summary.withAIChat' : 'setup.summary.withoutUsageTarget')}
              </p>
            </div>
          ) : (
            <>
              <ol
                className={cn(
                  'grid grid-cols-2 gap-2',
                  steps.length === 5 ? 'sm:grid-cols-5' : 'sm:grid-cols-4'
                )}
                aria-label={t('setup.progress')}
              >
                {steps.map((label, index) => (
                  <li
                    key={label}
                    className={cn(
                      'rounded-lg border px-3 py-2 text-xs',
                      index === step && 'border-primary bg-primary/5 text-primary',
                      index < step && 'border-success/30 bg-success/5 text-success'
                    )}
                  >
                    <span className="flex items-center gap-2 font-medium">
                      {index < step ? (
                        <CheckCircle2 className="size-3.5" />
                      ) : (
                        <Circle className="size-3.5" />
                      )}
                      {label}
                    </span>
                  </li>
                ))}
              </ol>

              {step === 0 && item ? (
                <section className="space-y-4" aria-live="polite">
                  <div
                    className={cn(
                      'flex items-center justify-between gap-4 rounded-xl border p-4',
                      verificationState === 'testing' && 'border-primary/30 bg-primary/5',
                      verificationState === 'failed' && 'border-destructive/30 bg-destructive/5'
                    )}
                  >
                    <div className="flex min-w-0 items-start gap-3">
                      <div
                        className={cn(
                          'flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground',
                          verificationState === 'testing' && 'bg-primary/10 text-primary',
                          verificationState === 'failed' && 'bg-destructive/10 text-destructive'
                        )}
                      >
                        {verificationState === 'testing' ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : verificationState === 'passed' ? (
                          <CheckCircle2 className="size-4 text-success" />
                        ) : (
                          <TestTube2 className="size-4" />
                        )}
                      </div>
                      <div className="min-w-0">
                        <h3 className="font-medium">
                          {t(
                            verificationState === 'testing'
                              ? 'setup.verify.testingTitle'
                              : verificationState === 'failed'
                                ? 'setup.verify.failedTitle'
                                : verificationState === 'passed'
                                  ? 'setup.verify.passedTitle'
                                  : 'setup.verify.title'
                          )}
                        </h3>
                        <p className="mt-1 text-sm leading-6 text-muted-foreground">
                          {t(
                            verificationState === 'testing'
                              ? 'setup.verify.testingDescription'
                              : verificationState === 'failed'
                                ? 'setup.verify.failedDescription'
                                : verificationState === 'passed'
                                  ? 'setup.verify.passedDescription'
                                  : 'setup.verify.description'
                          )}
                        </p>
                      </div>
                    </div>
                    {verificationState === 'testing' ? (
                      <Badge variant="info">{t('setup.verify.testing')}</Badge>
                    ) : verificationState === 'failed' ? (
                      <Badge variant="destructive">{t('setup.verify.failed')}</Badge>
                    ) : (
                      <IntegrationConnectionHealthBadge connection={item} />
                    )}
                  </div>
                  {verificationState === 'failed' ? (
                    <div className="rounded-xl border border-destructive/20 bg-destructive/[0.03] p-4">
                      <p className="text-sm leading-6 text-muted-foreground">
                        {t(
                          item.integration_id === 'wecom' &&
                            verificationDiagnostics?.provider_error_code === '60020'
                            ? 'setup.verify.wecomTrustedIPHint'
                            : item.integration_id === 'dingtalk'
                              ? 'setup.verify.dingtalkConfigurationHint'
                              : 'setup.verify.retryHint'
                        )}
                      </p>
                      <ProviderDiagnosticsDetails
                        className="mt-3"
                        providerErrorCode={verificationDiagnostics?.provider_error_code}
                        providerRequestId={verificationDiagnostics?.provider_request_id}
                        providerHTTPStatus={verificationDiagnostics?.provider_http_status}
                        retryAfterAt={verificationDiagnostics?.retry_after_at}
                      />
                      <div className="mt-4 flex flex-wrap justify-end gap-2">
                        {onEdit ? (
                          <Button type="button" variant="outline" onClick={() => onEdit(item)}>
                            <Pencil className="size-4" />
                            {t('setup.verify.editConnection')}
                          </Button>
                        ) : null}
                        <Button type="button" variant="outline" onClick={() => void test()}>
                          <TestTube2 className="size-4" />
                          {t('setup.verify.retry')}
                        </Button>
                      </div>
                    </div>
                  ) : verificationState === 'passed' ? (
                    <Button type="button" variant="ghost" size="sm" onClick={() => void test()}>
                      <TestTube2 className="size-4" />
                      {t('setup.verify.testAgain')}
                    </Button>
                  ) : null}
                </section>
              ) : null}

              {step === 1 && item ? (
                <section className="space-y-4">
                  <div className="flex items-center justify-between">
                    <div>
                      <h3 className="font-medium">{t('setup.capabilities.title')}</h3>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {t('setup.capabilities.description')}
                      </p>
                    </div>
                    <Badge variant={usableCapabilities.length > 0 ? 'success' : 'warning'}>
                      {t('setup.capabilities.available', { count: usableCapabilities.length })}
                    </Badge>
                  </div>
                  <IntegrationConnectionPermissionSummary
                    summary={item.permission_summary}
                    grantedScopes={item.granted_scopes}
                    provider={provider}
                  />
                </section>
              ) : null}

              {step === executionRulesStep && item && provider ? (
                <section className="space-y-5">
                  <div>
                    <h3 className="font-medium">{t('setup.executionRules.title')}</h3>
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">
                      {t('setup.executionRules.description')}
                    </p>
                  </div>

                  <IntegrationConnectionActionSetup
                    ref={actionSetupRef}
                    connection={item}
                    provider={provider}
                    canManageShared={canManageShared}
                    enabled={open && step === executionRulesStep}
                    onStateChange={setActionSetupState}
                  />

                  {!actionSetupState.loading && !actionSetupState.ready ? (
                    <Alert className="border-warning/40 bg-warning/5">
                      <ShieldCheck className="size-4" />
                      <AlertDescription>{t('setup.executionRules.required')}</AlertDescription>
                    </Alert>
                  ) : null}

                  {personal ? (
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t('setup.personal.description')}
                    </p>
                  ) : null}
                </section>
              ) : null}

              {step === usageRulesStep && item ? (
                <section className="space-y-4">
                  <div>
                    <h3 className="font-medium">{t('setup.rules.title')}</h3>
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">
                      {t('setup.rules.description')}
                    </p>
                  </div>
                  <Alert className={hasUsageRule ? undefined : 'border-warning/40 bg-warning/5'}>
                    <ShieldCheck className="size-4" />
                    <AlertDescription>
                      {t(hasUsageRule ? 'setup.rules.ready' : 'setup.rules.required')}
                    </AlertDescription>
                  </Alert>
                  <IntegrationConnectionGrantsPanel
                    connectionId={item.id}
                    actions={actions}
                    enabled={open && step === usageRulesStep}
                  />
                </section>
              ) : null}

              {step === usageTargetsStep ? (
                <section className="space-y-4">
                  <div>
                    <h3 className="font-medium">{t('setup.usageTargets.title')}</h3>
                    <p className="mt-1 text-sm leading-6 text-muted-foreground">
                      {t('setup.usageTargets.description')}
                    </p>
                  </div>
                  <div className="grid gap-3">
                    <div className="flex items-start gap-4 rounded-xl border p-4">
                      <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/8 text-primary">
                        <MessageSquare className="size-5" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <div>
                            <h4 className="font-medium">{t('setup.usageTargets.aiChat.title')}</h4>
                            <p className="mt-1 text-sm text-muted-foreground">
                              {t('setup.usageTargets.aiChat.workspace', {
                                workspace:
                                  currentWorkspace?.name ?? t('setup.usageTargets.noWorkspace'),
                              })}
                            </p>
                          </div>
                          <Switch
                            checked={aiChatEnabled}
                            onCheckedChange={checked => {
                              setAIChatEnabled(checked);
                              setAIChatTouched(true);
                            }}
                            disabled={
                              availableConnectionsQuery.isLoading ||
                              (!aiChatAvailable && !aiChatEnabled) ||
                              !currentWorkspace
                            }
                            aria-label={t('setup.usageTargets.aiChat.toggle')}
                          />
                        </div>
                        <p className="mt-2 text-xs leading-5 text-muted-foreground">
                          {t('setup.usageTargets.aiChat.description')}
                        </p>
                      </div>
                    </div>

                    <div className="grid gap-3 sm:grid-cols-2">
                      <div className="rounded-xl border p-4">
                        <div className="flex items-start gap-3">
                          <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                            <Bot className="size-4" />
                          </div>
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <h4 className="font-medium">{t('setup.usageTargets.agent.title')}</h4>
                              <Badge variant={agentAvailable ? 'info' : 'subtle'}>
                                {t(
                                  personal
                                    ? 'setup.usageTargets.personalUnsupported'
                                    : agentAvailable
                                      ? 'setup.usageTargets.configureSeparately'
                                      : 'setup.usageTargets.unsupported'
                                )}
                              </Badge>
                            </div>
                            <p className="mt-2 text-xs leading-5 text-muted-foreground">
                              {t(
                                personal
                                  ? 'setup.usageTargets.agent.personalDescription'
                                  : agentAvailable
                                    ? 'setup.usageTargets.agent.description'
                                    : 'setup.usageTargets.agent.unsupportedDescription'
                              )}
                            </p>
                          </div>
                        </div>
                      </div>

                      <div className="rounded-xl border p-4">
                        <div className="flex items-start gap-3">
                          <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                            <Workflow className="size-4" />
                          </div>
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <h4 className="font-medium">
                                {t('setup.usageTargets.workflow.title')}
                              </h4>
                              <Badge variant="subtle">{t('setup.usageTargets.comingSoon')}</Badge>
                            </div>
                            <p className="mt-2 text-xs leading-5 text-muted-foreground">
                              {t('setup.usageTargets.workflow.description')}
                            </p>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                  {!availableConnectionsQuery.isLoading && !aiChatAvailable ? (
                    <Alert className="border-warning/40 bg-warning/5">
                      <AlertDescription>
                        {t('setup.usageTargets.aiChat.notAvailable')}
                      </AlertDescription>
                    </Alert>
                  ) : null}
                  {completeMutation.isError ? (
                    <Alert variant="destructive">
                      <AlertDescription>{t('setup.completeFailed')}</AlertDescription>
                    </Alert>
                  ) : null}
                </section>
              ) : null}
            </>
          )}
        </DialogBody>
        <DialogFooter className="border-t bg-muted/30">
          {completed ? (
            <Button onClick={() => onOpenChange(false)}>{t('setup.done')}</Button>
          ) : (
            <>
              {step < steps.length - 1 ? (
                <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
                  {t('setup.saveAndClose')}
                </Button>
              ) : null}
              {step > 0 ? (
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setStep(current => current - 1)}
                >
                  <ChevronLeft className="size-4" />
                  {t('setup.previous')}
                </Button>
              ) : null}
              <Button
                type="button"
                onClick={() => void handleNext()}
                disabled={
                  !stepReady[step] ||
                  completeMutation.isPending ||
                  (step === executionRulesStep && actionSetupState.saving)
                }
              >
                {completeMutation.isPending ||
                (step === executionRulesStep && actionSetupState.saving) ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : step === steps.length - 1 ? (
                  <CheckCircle2 className="size-4" />
                ) : (
                  <ChevronRight className="size-4" />
                )}
                {t(step === steps.length - 1 ? 'setup.complete' : 'setup.next')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
