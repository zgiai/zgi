'use client';

import { useEffect, useMemo, useState } from 'react';
import { AppWindow, Plus, Save } from 'lucide-react';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import {
  useBuiltInWorkflowRuntimeSurfaces,
  useUpdateBuiltInWorkflowRuntimeSurfaces,
} from '@/hooks/workflow/use-built-in-workflow-runtime-surfaces';
import { cn } from '@/lib/utils';
import type {
  PublishedRuntimeGrantSubject,
  UpdatePublishedRuntimeSurfaceGrant,
} from '@/services/types/workflow';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  RuntimeGrantSubjectRow,
  type RuntimeGrantSubjectLabels,
} from '@/components/runtime-auth/runtime-grant-subject-row';

const BUILT_IN_SCENARIOS = ['global_chat', 'bi_chat', 'imagegen_chat'] as const;
const EDITABLE_GRANT_SUBJECTS = ['organization', 'department', 'account'] as const;

type EditableGrantSubject = (typeof EDITABLE_GRANT_SUBJECTS)[number];

interface EditableGrant {
  subject_type: EditableGrantSubject;
  subject_id: string;
}

const SCENARIO_LABEL_KEYS: Record<
  string,
  'scenarios.globalChat' | 'scenarios.biChat' | 'scenarios.imageGenChat'
> = {
  global_chat: 'scenarios.globalChat',
  bi_chat: 'scenarios.biChat',
  imagegen_chat: 'scenarios.imageGenChat',
};

function isEditableGrantSubject(
  subject: PublishedRuntimeGrantSubject
): subject is EditableGrantSubject {
  return EDITABLE_GRANT_SUBJECTS.includes(subject as EditableGrantSubject);
}

function normalizeSubjectId(grant: EditableGrant): string | null {
  return grant.subject_type === 'organization' ? null : grant.subject_id.trim();
}

export function BuiltInWorkflowRuntimeSection() {
  const t = useT('dashboard.organization.permissions.builtInRuntime');
  const [scenario, setScenario] = useState<(typeof BUILT_IN_SCENARIOS)[number]>('global_chat');
  const [enabled, setEnabled] = useState(true);
  const [grants, setGrants] = useState<EditableGrant[]>([
    { subject_type: 'organization', subject_id: '' },
  ]);

  const { data, error, isLoading, isFetching } = useBuiltInWorkflowRuntimeSurfaces(scenario);
  const updateMutation = useUpdateBuiltInWorkflowRuntimeSurfaces();

  const runtimeData = data?.data;
  const builtinSurface = useMemo(
    () => runtimeData?.surfaces.find(surface => surface.surface === 'builtin_app') ?? null,
    [runtimeData]
  );
  const internalSurface = useMemo(
    () => runtimeData?.surfaces.find(surface => surface.surface === 'internal') ?? null,
    [runtimeData]
  );

  useEffect(() => {
    if (!builtinSurface) return;
    const editableGrants: EditableGrant[] = [];
    for (const grant of builtinSurface.grants) {
      if (grant.enabled && isEditableGrantSubject(grant.subject_type)) {
        editableGrants.push({
          subject_type: grant.subject_type,
          subject_id: grant.subject_id ?? '',
        });
      }
    }

    setEnabled(builtinSurface.enabled);
    setGrants(
      editableGrants.length > 0
        ? editableGrants
        : [{ subject_type: 'organization', subject_id: '' }]
    );
  }, [builtinSurface]);

  const scenarioLabel = (value: string) =>
    SCENARIO_LABEL_KEYS[value]
      ? t(SCENARIO_LABEL_KEYS[value])
      : t('scenarios.unknown', { scenario: value });

  const updateGrant = (index: number, next: EditableGrant) => {
    setGrants(current => current.map((grant, i) => (i === index ? next : grant)));
  };

  const addGrant = () => {
    setGrants(current => [...current, { subject_type: 'department', subject_id: '' }]);
  };

  const removeGrant = (index: number) => {
    setGrants(current => current.filter((_, i) => i !== index));
  };

  const buildPayloadGrants = (): UpdatePublishedRuntimeSurfaceGrant[] | null => {
    const normalized = grants.map(grant => ({
      subject_type: grant.subject_type,
      subject_id: normalizeSubjectId(grant),
      enabled: true,
    }));

    const missingSubjectId = normalized.some(
      grant => grant.subject_type !== 'organization' && !grant.subject_id
    );
    if (missingSubjectId) {
      toast.error(t('validation.subjectIdRequired'));
      return null;
    }

    const dedupeKeys = new Set<string>();
    for (const grant of normalized) {
      const key = `${grant.subject_type}:${grant.subject_id ?? ''}`;
      if (dedupeKeys.has(key)) {
        toast.error(t('validation.duplicateGrant'));
        return null;
      }
      dedupeKeys.add(key);
    }

    if (normalized.length === 0) {
      toast.error(t('validation.grantRequired'));
      return null;
    }

    return normalized;
  };

  const handleSave = async () => {
    const nextGrants = enabled ? buildPayloadGrants() : [];
    if (nextGrants === null) return;

    await updateMutation.mutateAsync({
      scenario,
      payload: {
        surfaces: [
          {
            surface: 'builtin_app',
            enabled,
            grants: nextGrants,
          },
        ],
      },
    });
  };

  const renderStatusBadge = (isEnabled: boolean) => (
    <Badge variant={isEnabled ? 'success' : 'subtle'}>
      {isEnabled ? t('status.enabled') : t('status.disabled')}
    </Badge>
  );

  const showLoading = isLoading || (isFetching && !runtimeData);
  const errorMessage = error ? getErrorMessage(error) || t('loadError') : null;
  const grantSubjectLabels: RuntimeGrantSubjectLabels = {
    subjectLabels: {
      organization: t('grantSubjects.organization'),
      department: t('grantSubjects.department'),
      account: t('grantSubjects.account'),
    },
    organizationWide: t('grants.organizationWide'),
    departmentPlaceholder: t('grants.departmentPlaceholder'),
    accountPlaceholder: t('grants.accountPlaceholder'),
    searchMembersPlaceholder: t('grants.searchMembersPlaceholder'),
    noMembers: t('grants.noMembers'),
    loadingMembers: t('grants.loadingMembers'),
    resolvingAccount: t('grants.resolvingAccount'),
    selectionRequired: t('grants.selectionRequired'),
    accountLookupFailed: t('grants.accountLookupFailed'),
    departmentLookupFailed: t('grants.departmentLookupFailed'),
    unresolvedAccount: t('grants.unresolvedAccount'),
    unresolvedDepartment: t('grants.unresolvedDepartment'),
    removeGrant: t('actions.removeGrant'),
  };

  return (
    <section className="rounded-lg border border-border/80 bg-background p-4 shadow-sm">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-primary/15 bg-primary/10 text-primary">
            <AppWindow className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-text-primary">{t('title')}</h2>
            <p className="mt-1 max-w-3xl text-sm leading-6 text-text-secondary">{t('subtitle')}</p>
          </div>
        </div>
        <div className="grid w-full gap-2 sm:grid-cols-[minmax(0,220px)_auto] lg:w-auto">
          <Select value={scenario} onValueChange={value => setScenario(value as typeof scenario)}>
            <SelectTrigger className="h-9 rounded-md">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {BUILT_IN_SCENARIOS.map(value => (
                <SelectItem key={value} value={value}>
                  {scenarioLabel(value)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            className="h-9 rounded-md"
            disabled={showLoading}
            loading={updateMutation.isPending}
            onClick={() => void handleSave()}
          >
            <Save className="h-4 w-4" />
            {t('actions.save')}
          </Button>
        </div>
      </div>

      {showLoading ? (
        <div className="mt-5 grid gap-3 md:grid-cols-3">
          <Skeleton className="h-20 rounded-lg" />
          <Skeleton className="h-20 rounded-lg" />
          <Skeleton className="h-20 rounded-lg" />
        </div>
      ) : (
        <div className="mt-5 space-y-4">
          {errorMessage && (
            <div className="rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive">
              {errorMessage}
            </div>
          )}

          <div className="grid gap-3 md:grid-cols-2">
            <div className="rounded-md border border-border/70 p-3">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <Label className="text-sm font-semibold text-text-primary">
                    {t('surfaces.builtinApp')}
                  </Label>
                  <p className="mt-1 text-xs text-text-secondary">{t('surfaces.builtinAppHint')}</p>
                </div>
                <Switch checked={enabled} onCheckedChange={setEnabled} />
              </div>
              <div className="mt-3 flex items-center gap-2">{renderStatusBadge(enabled)}</div>
            </div>

            <div className="rounded-md border border-border/70 p-3">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <Label className="text-sm font-semibold text-text-primary">
                    {t('surfaces.internal')}
                  </Label>
                  <p className="mt-1 text-xs text-text-secondary">{t('surfaces.internalHint')}</p>
                </div>
                {renderStatusBadge(internalSurface?.enabled ?? true)}
              </div>
            </div>
          </div>

          <div
            className={cn(
              'rounded-md border border-border/70 p-3 transition-opacity',
              !enabled && 'opacity-60'
            )}
          >
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <Label className="text-sm font-semibold text-text-primary">
                  {t('grants.title')}
                </Label>
                <p className="mt-1 text-xs text-text-secondary">{t('grants.subtitle')}</p>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-8 rounded-md"
                disabled={!enabled}
                onClick={addGrant}
              >
                <Plus className="h-3.5 w-3.5" />
                {t('actions.addGrant')}
              </Button>
            </div>

            {grants.length === 0 ? (
              <div className="rounded-md border border-dashed border-border/70 py-6 text-center text-sm text-text-secondary">
                {t('grants.empty')}
              </div>
            ) : (
              <div className="space-y-2">
                {grants.map((grant, index) => (
                  <RuntimeGrantSubjectRow
                    key={`${grant.subject_type}-${grant.subject_id}-${index}`}
                    subjectType={grant.subject_type}
                    subjectId={grant.subject_id}
                    disabled={!enabled}
                    canRemove={grants.length > 1}
                    labels={grantSubjectLabels}
                    onChange={next => updateGrant(index, next)}
                    onRemove={() => removeGrant(index)}
                  />
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
