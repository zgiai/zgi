'use client';

import { useState, type ReactNode } from 'react';
import { AlertTriangle, Check, CheckCircle2, Loader2, Plus, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { DigitalHumanResourceBindings } from '@/components/digital-human/digital-human-resource-bindings';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import {
  useDigitalHuman,
  useDigitalHumanDraftBindings,
  useDigitalHumanDraftRoles,
  useDigitalHumanMutations,
  useDigitalHumanRoleTemplates,
} from '@/hooks/digital-human/use-digital-humans';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import type { DigitalHumanRoleAssignmentInput } from '@/services/types/digital-human';

export function DigitalHumanSettings({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const detailQuery = useDigitalHuman(id);
  const detail = detailQuery.data?.data;
  if (detailQuery.isLoading || !detail) {
    return <DigitalHumanConfigurationLoading />;
  }
  return (
    <div className="mx-auto max-w-5xl space-y-6 px-4 py-6 md:px-8">
      <PageIntroduction title={t('settings.title')} description={t('settings.description')} />
      <IdentityCard id={id} key={`${detail.definition.revision}:${detail.draft.lock_version}`} />
    </div>
  );
}

export function DigitalHumanCapabilities({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const detailQuery = useDigitalHuman(id);
  if (detailQuery.isLoading || !detailQuery.data?.data) {
    return <DigitalHumanConfigurationLoading />;
  }
  return (
    <div className="mx-auto max-w-6xl space-y-6 px-4 py-6 md:px-8">
      <PageIntroduction
        title={t('capabilities.title')}
        description={t('capabilities.description')}
      />
      <div className="grid gap-6 lg:grid-cols-[minmax(300px,0.75fr)_minmax(0,1.25fr)]">
        <RolesCard id={id} />
        <DigitalHumanResourceBindings id={id} />
      </div>
    </div>
  );
}

function DigitalHumanConfigurationLoading() {
  return (
    <div className="mx-auto max-w-5xl p-8">
      <div className="h-72 animate-pulse rounded-xl bg-muted/40" />
    </div>
  );
}

function PageIntroduction({ title, description }: { title: string; description: string }) {
  return (
    <div>
      <h2 className="text-xl font-semibold tracking-tight">{title}</h2>
      <p className="mt-1 text-sm text-muted-foreground">{description}</p>
    </div>
  );
}

function IdentityCard({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const detail = useDigitalHuman(id).data?.data;
  const mutations = useDigitalHumanMutations(id);
  const availableModels = useAvailableModels({ use_case: 'agent-runtime' });
  const [name, setName] = useState(detail?.definition.name ?? '');
  const [description, setDescription] = useState(detail?.definition.description ?? '');
  const [persona, setPersona] = useState(detail?.draft.persona ?? '');
  const [rules, setRules] = useState((detail?.draft.rules ?? []).join('\n'));
  const [model, setModel] = useState(detail?.draft.default_model ?? '');
  const [intermediateMessagesEnabled, setIntermediateMessagesEnabled] = useState(
    detail?.draft.trigger_policy?.channel_response?.intermediate_messages_enabled ?? true
  );
  const [acknowledgementEnabled, setAcknowledgementEnabled] = useState(
    detail?.draft.trigger_policy?.channel_response?.acknowledgement_enabled ?? true
  );
  // `useAvailableModels` returns a compact, already-filtered response whose
  // entries do not include the full model catalog's `callable` projection.
  const models = availableModels.models;
  const canUpdate = permissions.hasPermission('digital_human.update');

  if (!detail) return null;
  const normalizedRules = rules
    .split('\n')
    .map(item => item.trim())
    .filter(Boolean);
  const dirty =
    name.trim() !== detail.definition.name ||
    description.trim() !== (detail.definition.description ?? '') ||
    persona.trim() !== detail.draft.persona ||
    normalizedRules.join('\n') !== (detail.draft.rules ?? []).join('\n') ||
    model !== detail.draft.default_model ||
    intermediateMessagesEnabled !==
      (detail.draft.trigger_policy?.channel_response?.intermediate_messages_enabled ?? true) ||
    acknowledgementEnabled !==
      (detail.draft.trigger_policy?.channel_response?.acknowledgement_enabled ?? true);
  const save = async () => {
    if (!name.trim() || !persona.trim() || !model.trim()) {
      toast.error(t('errors.required'));
      return;
    }
    await mutations.update.mutateAsync({
      digitalHumanId: id,
      request: {
        expected_revision: detail.definition.revision,
        name: name.trim(),
        description: description.trim(),
      },
    });
    await mutations.updateDraft.mutateAsync({
      digitalHumanId: id,
      request: {
        expected_lock_version: detail.draft.lock_version,
        persona: persona.trim(),
        rules: normalizedRules,
        default_model: model,
        memory_policy: detail.draft.memory_policy ?? {
          enabled: false,
          ownership: 'digital_human',
        },
        trigger_policy: {
          ...(detail.draft.trigger_policy ?? {}),
          manual: detail.draft.trigger_policy?.manual ?? true,
          channel_response: {
            ...(detail.draft.trigger_policy?.channel_response ?? {}),
            intermediate_messages_enabled: intermediateMessagesEnabled,
            acknowledgement_enabled: acknowledgementEnabled,
          },
        },
      },
    });
    toast.success(t('configuration.saved'));
  };
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{t('settings.identityTitle')}</CardTitle>
          <CardDescription>{t('settings.identityDescription')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field label={t('fields.name')}>
            <Input
              value={name}
              onChange={event => setName(event.target.value)}
              disabled={!canUpdate}
            />
          </Field>
          <Field label={t('fields.description')}>
            <Textarea
              value={description}
              onChange={event => setDescription(event.target.value)}
              disabled={!canUpdate}
            />
          </Field>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('settings.behaviorTitle')}</CardTitle>
          <CardDescription>{t('settings.behaviorDescription')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Field label={t('fields.persona')}>
            <Textarea
              rows={7}
              value={persona}
              onChange={event => setPersona(event.target.value)}
              disabled={!canUpdate}
              placeholder={t('settings.personaPlaceholder')}
            />
          </Field>
          <Field label={t('fields.rules')}>
            <Textarea
              rows={6}
              value={rules}
              onChange={event => setRules(event.target.value)}
              disabled={!canUpdate}
              placeholder={t('settings.rulesPlaceholder')}
            />
          </Field>
          <Field label={t('fields.model')}>
            <select
              className="h-10 w-full rounded-md border bg-background px-3 text-sm"
              value={model}
              onChange={event => setModel(event.target.value)}
              disabled={!canUpdate}
            >
              {model && !models.some(item => item.model === model) ? (
                <option value={model}>{model}</option>
              ) : null}
              <option value="">—</option>
              {models.map(item => (
                <option key={`${item.provider}:${item.model}`} value={item.model}>
                  {item.model_name || item.model}
                </option>
              ))}
            </select>
          </Field>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('settings.channelResponseTitle')}</CardTitle>
          <CardDescription>{t('settings.channelResponseDescription')}</CardDescription>
        </CardHeader>
        <CardContent className="divide-y rounded-lg border px-4">
          <div className="flex items-start justify-between gap-6 py-4">
            <div className="space-y-1">
              <div className="text-sm font-medium">{t('settings.intermediateMessages')}</div>
              <p className="text-xs leading-5 text-muted-foreground">
                {t('settings.intermediateMessagesDescription')}
              </p>
            </div>
            <Switch
              checked={intermediateMessagesEnabled}
              onCheckedChange={setIntermediateMessagesEnabled}
              disabled={!canUpdate}
              aria-label={t('settings.intermediateMessages')}
            />
          </div>
          <div className="flex items-start justify-between gap-6 py-4">
            <div className="space-y-1">
              <div className="text-sm font-medium">{t('settings.acknowledgement')}</div>
              <p className="text-xs leading-5 text-muted-foreground">
                {t('settings.acknowledgementDescription')}
              </p>
            </div>
            <Switch
              checked={acknowledgementEnabled}
              onCheckedChange={setAcknowledgementEnabled}
              disabled={!canUpdate}
              aria-label={t('settings.acknowledgement')}
            />
          </div>
        </CardContent>
      </Card>
      {canUpdate ? (
        <div className="sticky bottom-4 z-10 flex flex-col gap-3 rounded-xl border bg-background/95 p-4 shadow-lg backdrop-blur sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="text-sm font-medium">
              {dirty ? t('settings.unsaved') : t('settings.saved')}
            </div>
            <p className="text-xs text-muted-foreground">{t('settings.saveHint')}</p>
          </div>
          <Button
            onClick={() => void save()}
            disabled={!dirty || mutations.update.isPending || mutations.updateDraft.isPending}
          >
            {mutations.update.isPending || mutations.updateDraft.isPending
              ? t('configuration.saving')
              : t('configuration.save')}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

export function DigitalHumanPublishPanel({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const detail = useDigitalHuman(id).data?.data;
  const rolesQuery = useDigitalHumanDraftRoles(id);
  const bindingsQuery = useDigitalHumanDraftBindings(id);
  const mutations = useDigitalHumanMutations(id);
  if (!detail) return null;
  const rules = detail.draft.rules ?? [];
  const checks = [
    {
      label: t('configuration.preflight.identity'),
      ready: Boolean(detail.definition.name.trim() && detail.draft.persona.trim()),
      blocking: true,
    },
    {
      label: t('configuration.preflight.rules'),
      ready: rules.some(rule => rule.trim()),
      blocking: true,
    },
    {
      label: t('configuration.preflight.model'),
      ready: Boolean(detail.draft.default_model.trim()),
      blocking: true,
    },
    {
      label: t('configuration.preflight.roles'),
      ready: Boolean(rolesQuery.data?.data?.items.length),
      blocking: false,
    },
    {
      label: t('configuration.preflight.bindings'),
      ready: Boolean(bindingsQuery.data?.data?.items.length),
      blocking: false,
    },
  ];
  const preflightLoading = rolesQuery.isLoading || bindingsQuery.isLoading;
  const blockingIssues = checks.filter(check => check.blocking && !check.ready);
  const warningCount = checks.filter(check => !check.blocking && !check.ready).length;
  const publish = async () => {
    await mutations.publish.mutateAsync({
      digitalHumanId: id,
      request: {
        expected_revision: detail.definition.revision,
        expected_draft_lock_version: detail.draft.lock_version,
      },
    });
    toast.success(t('configuration.publishedSuccess'));
  };
  return (
    <Card id="publish">
      <CardHeader>
        <CardTitle>{t('configuration.publish')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2 rounded-lg border p-3">
          <div className="text-sm font-medium">{t('configuration.preflight.title')}</div>
          {preflightLoading ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 className="size-3.5 animate-spin" />
              {t('configuration.preflight.loading')}
            </div>
          ) : (
            checks.map(check => (
              <div key={check.label} className="flex items-center gap-2 text-xs">
                {check.ready ? (
                  <CheckCircle2 className="size-4 text-emerald-600" />
                ) : (
                  <AlertTriangle
                    className={check.blocking ? 'size-4 text-destructive' : 'size-4 text-amber-500'}
                  />
                )}
                <span className={check.ready ? 'text-foreground' : 'text-muted-foreground'}>
                  {check.label}
                </span>
              </div>
            ))
          )}
          {!preflightLoading && warningCount > 0 && blockingIssues.length === 0 ? (
            <p className="text-xs text-amber-600 dark:text-amber-400">
              {t('configuration.preflight.warning', { count: warningCount })}
            </p>
          ) : null}
        </div>
        {detail.published ? (
          <div className="rounded-lg border bg-muted/20 p-3 text-xs text-muted-foreground">
            <div className="flex items-center gap-2 text-foreground">
              <Check className="size-4 text-emerald-600" />
              {t('configuration.publishedVersion', {
                version: detail.published.version_number ?? 1,
              })}
            </div>
            <details className="mt-2">
              <summary className="cursor-pointer text-xs">{t('common.technicalDetails')}</summary>
              <code className="mt-2 block break-all">{detail.published.revision}</code>
            </details>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('configuration.notPublished')}</p>
        )}
        {permissions.hasPermission('digital_human.publish') ? (
          <Button
            className="w-full"
            onClick={() => void publish()}
            disabled={mutations.publish.isPending || preflightLoading || blockingIssues.length > 0}
          >
            {mutations.publish.isPending
              ? t('configuration.publishing')
              : detail.published
                ? t('configuration.publishChanges')
                : t('configuration.publish')}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  );
}

function RolesCard({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const templatesQuery = useDigitalHumanRoleTemplates();
  const rolesQuery = useDigitalHumanDraftRoles(id);
  const mutations = useDigitalHumanMutations(id);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selected, setSelected] = useState('');
  const assignments = rolesQuery.data?.data?.items ?? [];
  const templates = templatesQuery.data?.data?.items ?? [];
  const publishedTemplates = templates.filter(item => item.published_version_id);
  const assignedIDs = new Set(assignments.map(item => item.role_template_version_id));
  const canUpdate = permissions.hasPermission('digital_human.update');

  const assign = async () => {
    if (!selected || !rolesQuery.data?.data) {
      return;
    }
    const items: DigitalHumanRoleAssignmentInput[] = [
      ...assignments.map(item => ({
        role_template_version_id: item.role_template_version_id,
        parameters: item.parameters ?? {},
        override_policy: item.override_policy ?? {},
      })),
      { role_template_version_id: selected, parameters: {}, override_policy: {} },
    ];
    await mutations.replaceRoles.mutateAsync({
      digitalHumanId: id,
      expectedDraftLockVersion: rolesQuery.data.data.draft_lock_version,
      items,
    });
    setSelected('');
    toast.success(t('roles.saved'));
  };
  const remove = async (roleTemplateVersionID: string) => {
    if (!rolesQuery.data?.data) {
      return;
    }
    await mutations.replaceRoles.mutateAsync({
      digitalHumanId: id,
      expectedDraftLockVersion: rolesQuery.data.data.draft_lock_version,
      items: assignments
        .filter(item => item.role_template_version_id !== roleTemplateVersionID)
        .map(item => ({
          role_template_version_id: item.role_template_version_id,
          parameters: item.parameters ?? {},
          override_policy: item.override_policy ?? {},
        })),
    });
    toast.success(t('roles.saved'));
  };
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('roles.title')}</CardTitle>
        <CardDescription>{t('roles.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <div className="text-sm font-medium">{t('roles.assigned')}</div>
          {assignments.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('roles.empty')}</p>
          ) : (
            assignments.map(item => {
              const template = templates.find(
                candidate => candidate.published_version_id === item.role_template_version_id
              );
              return (
                <div
                  key={item.id}
                  className="flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm"
                >
                  <span className="min-w-0 truncate">
                    {template?.name ?? item.role_template_version_id}
                  </span>
                  {canUpdate ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="size-8 p-0"
                      aria-label={t('roles.remove')}
                      disabled={mutations.replaceRoles.isPending}
                      onClick={() => void remove(item.role_template_version_id)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  ) : null}
                </div>
              );
            })
          )}
        </div>
        {canUpdate ? (
          <div className="flex gap-2">
            <select
              className="h-10 min-w-0 flex-1 rounded-md border bg-background px-3 text-sm"
              value={selected}
              onChange={event => setSelected(event.target.value)}
            >
              <option value="">{t('roles.assign')}</option>
              {publishedTemplates
                .filter(
                  item => item.published_version_id && !assignedIDs.has(item.published_version_id)
                )
                .map(item => (
                  <option key={item.id} value={item.published_version_id ?? ''}>
                    {item.name}
                  </option>
                ))}
            </select>
            <Button
              variant="outline"
              onClick={() => void assign()}
              disabled={!selected || mutations.replaceRoles.isPending}
            >
              {t('roles.assign')}
            </Button>
          </div>
        ) : null}
        {canUpdate ? (
          <Button variant="secondary" className="w-full" onClick={() => setDialogOpen(true)}>
            <Plus className="mr-2 size-4" />
            {t('roles.create')}
          </Button>
        ) : null}
        <CreateRoleDialog open={dialogOpen} onOpenChange={setDialogOpen} />
      </CardContent>
    </Card>
  );
}

function CreateRoleDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT('digitalHumans');
  const mutations = useDigitalHumanMutations();
  const [name, setName] = useState('');
  const [goals, setGoals] = useState('');
  const [responsibilities, setResponsibilities] = useState('');
  const create = async () => {
    if (!name.trim() || !goals.trim() || !responsibilities.trim()) {
      toast.error(t('errors.required'));
      return;
    }
    const created = await mutations.createRole.mutateAsync({
      name: name.trim(),
      description: '',
      goals: splitLines(goals),
      responsibilities: splitLines(responsibilities),
      capabilities: [],
      sop_refs: [],
      knowledge_refs: [],
      policy: {},
      evaluation_refs: [],
    });
    const role = created.data;
    if (role) {
      await mutations.publishRole.mutateAsync({
        roleId: role.template.id,
        request: {
          expected_revision: role.template.revision,
          expected_draft_lock_version: role.draft.lock_version,
        },
      });
    }
    onOpenChange(false);
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('roles.create')}</DialogTitle>
          <DialogDescription>{t('roles.createDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <Field label={t('fields.roleName')}>
            <Input value={name} onChange={event => setName(event.target.value)} />
          </Field>
          <Field label={t('fields.goals')}>
            <Textarea rows={4} value={goals} onChange={event => setGoals(event.target.value)} />
          </Field>
          <Field label={t('fields.responsibilities')}>
            <Textarea
              rows={4}
              value={responsibilities}
              onChange={event => setResponsibilities(event.target.value)}
            />
          </Field>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={() => void create()}
            disabled={mutations.createRole.isPending || mutations.publishRole.isPending}
          >
            {mutations.createRole.isPending || mutations.publishRole.isPending
              ? t('roles.creating')
              : t('roles.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function splitLines(value: string) {
  return value
    .split('\n')
    .map(item => item.trim())
    .filter(Boolean);
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block space-y-1.5 text-sm font-medium">
      <span>{label}</span>
      {children}
    </label>
  );
}
