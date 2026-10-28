'use client';

import { useMemo, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { createTextIconValue, type IconValue } from '@/components/common/icon-input/types';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Loader2 } from 'lucide-react';
import {
  useDigitalHumanMutations,
  useDigitalHumanSkillCandidates,
  useDigitalHumanTemplates,
} from '@/hooks/digital-human/use-digital-humans';
import { useLocale } from '@/hooks/use-locale';
import { useAvailableModels } from '@/hooks/model/use-model';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import { digitalHumanService } from '@/services/digital-human.service';
import type {
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
} from '@/services/types/digital-human';
import { composeCreateBindings } from './create-digital-human-bindings';
import {
  bindingDisplayNames,
  createDigitalHumanPayload,
  templateDefaultSkillIds,
} from './create-digital-human-payload';
import {
  createConnectorSelectionBindings,
  createConnectorSelectionCandidates,
  createConnectorSelectionsDirty,
  type DigitalHumanCreateConnectorSelection,
} from './digital-human-connector-create-model';
import { DigitalHumanCreateSuccessDialog } from './digital-human-create-success-dialog';
import {
  DigitalHumanTemplateForm,
  type DigitalHumanTemplateFormValue,
} from './digital-human-template-form';
import { DigitalHumanTemplateGallery } from './digital-human-template-gallery';
import {
  CUSTOM_TEMPLATE_ID,
  findDigitalHumanTemplate,
  isCustomTemplateId,
  templateRequirementGap,
  templateStartingName,
  type DigitalHumanTemplate,
} from './digital-human-templates';

const CREATE_PATH = '/console/digital-humans/create';
const LIST_PATH = '/console/digital-humans';

function initials(value: string): string {
  return value.trim().slice(0, 2).toUpperCase();
}

function iconEquals(a: IconValue, b: IconValue): boolean {
  if (a.type === 'image' || b.type === 'image') {
    if (a.type !== 'image' || b.type !== 'image') return false;
    return a.iconUrl === b.iconUrl && a.imageId === b.imageId;
  }
  return a.icon === b.icon && a.iconBackground === b.iconBackground;
}

/**
 * Creation entry point.
 *
 * The URL drives which step is shown (`?template=<id>` = configure, no parameter
 * = pick a template), so refresh, share, and browser back all behave.
 */
export function DigitalHumanCreateFlow() {
  const t = useT('digitalHumans');
  const router = useRouter();
  const searchParams = useSearchParams();
  const templateParam = searchParams.get('template');
  const { locale } = useLocale();
  const catalogQuery = useDigitalHumanTemplates({ locale });
  const catalog = catalogQuery.data?.data;
  const templates = catalog?.templates ?? [];
  const sharedRules = catalog?.shared_rules ?? [];
  const template = findDigitalHumanTemplate(templates, templateParam);
  const isCustom = isCustomTemplateId(templateParam);
  const invalidTemplate = Boolean(templateParam) && Boolean(catalog) && !template && !isCustom;

  const selectTemplate = (templateId: string) => {
    router.push(`${CREATE_PATH}?template=${encodeURIComponent(templateId)}`);
  };

  const backToGallery = () => {
    router.push(CREATE_PATH);
  };

  // The form starts from the template, and the blank entry from the shared
  // rules, so it waits for the catalog.
  if ((template || isCustom) && catalog) {
    return (
      <DigitalHumanConfigureStep
        // Remount per template so the form always starts from that template.
        key={templateParam ?? CUSTOM_TEMPLATE_ID}
        template={template}
        sharedRules={sharedRules}
        onBack={backToGallery}
      />
    );
  }
  if (templateParam && !catalog && !catalogQuery.isError) {
    return (
      <div className="flex min-h-72 items-center justify-center">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-[1440px] space-y-4 p-4 md:p-6 xl:p-8">
      {invalidTemplate ? (
        <p
          role="status"
          className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive"
        >
          {t('createFlow.invalidRole')}
        </p>
      ) : null}
      <DigitalHumanTemplateGallery
        templates={templates}
        loading={catalogQuery.isLoading}
        error={catalogQuery.isError}
        onRetry={() => void catalogQuery.refetch()}
        onSelectTemplate={selectTemplate}
      />
    </div>
  );
}

interface CreatedState {
  id: string;
  published: boolean;
  /** Set when creation succeeded but a later step failed. */
  notice: string | null;
  boundCapabilityNames: string[];
}

/**
 * Configure step for one template (or the custom entry).
 *
 * It owns the form state and the whole creation sequence: create the digital
 * human, then replace the draft bindings once with the selected capabilities
 * (the template's skills) resolved against the workspace's real candidates, then
 * publish version 1. The form only records that choice; nothing is bound while it
 * renders. Being remounted per template via `key` keeps the seeding out of an
 * effect, so a late model list can never wipe what the user typed.
 */
function DigitalHumanConfigureStep({
  template,
  sharedRules,
  onBack,
}: {
  template: DigitalHumanTemplate | null;
  sharedRules: readonly string[];
  onBack: () => void;
}) {
  const t = useT('digitalHumans');
  const router = useRouter();
  const queryClient = useQueryClient();
  const availableModels = useAvailableModels({ use_case: 'agent-runtime' });
  const skillCandidatesQuery = useDigitalHumanSkillCandidates();
  const mutations = useDigitalHumanMutations();

  const templateTitle = template ? template.title : t('createFlow.gallery.customEntry');
  const tags = useMemo(() => template?.tags ?? [], [template]);
  const skillCandidates: DigitalHumanResourceCandidate[] = useMemo(
    () => skillCandidatesQuery.data?.data?.items ?? [],
    [skillCandidatesQuery.data?.data?.items]
  );
  // A template seeds the skill selection; the user owns it from there. `null`
  // means "untouched", so a late candidate response can still seed the default
  // without ever overwriting a deliberate choice.
  const [skillSelection, setSkillSelection] = useState<string[] | null>(null);
  const defaultSkillIds = useMemo(
    () => templateDefaultSkillIds(template, skillCandidates),
    [template, skillCandidates]
  );
  const selectedSkillIds = skillSelection ?? defaultSkillIds;
  const changeSkillSelection = (resourceIds: string[]) => setSkillSelection(resourceIds);

  // The Connector selection lives with the form: the picker edits it and
  // creation submits it, while the credentials themselves never leave the
  // server-side connection in Connector Center.
  const [connectorSelections, setConnectorSelections] = useState<
    DigitalHumanCreateConnectorSelection[]
  >([]);

  const [initialValue] = useState<DigitalHumanTemplateFormValue>(() => ({
    name: template ? templateStartingName(template) : '',
    responsibility: template ? template.responsibility : '',
    icon: template
      ? createTextIconValue(
          initials(templateStartingName(template)),
          template.icon_background || ICON_BG
        )
      : createTextIconValue('', ICON_BG),
    model: '',
  }));
  const [value, setValue] = useState<DigitalHumanTemplateFormValue>(initialValue);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [created, setCreated] = useState<CreatedState | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [discardOpen, setDiscardOpen] = useState(false);
  const autoIconText = useRef(initials(initialValue.name));

  const defaultModel = availableModels.models[0]?.model ?? '';
  const effectiveModel = value.model || defaultModel;
  const skillsDirty =
    skillSelection !== null &&
    (skillSelection.length !== defaultSkillIds.length ||
      skillSelection.some(id => !defaultSkillIds.includes(id)));
  // Only an explicit skill edit counts; leaving the template's seed untouched
  // keeps the form in its clean state.
  const dirty =
    value.name !== initialValue.name ||
    value.responsibility !== initialValue.responsibility ||
    value.model !== initialValue.model ||
    !iconEquals(value.icon, initialValue.icon) ||
    skillsDirty ||
    createConnectorSelectionsDirty(connectorSelections);

  const handleChange = (patch: Partial<DigitalHumanTemplateFormValue>) => {
    setValue(current => {
      if (patch.name === undefined || current.icon.type !== 'text') {
        return { ...current, ...patch };
      }
      const nextAuto = initials(patch.name);
      const followsName = !current.icon.icon || current.icon.icon === autoIconText.current;
      autoIconText.current = nextAuto;
      return {
        ...current,
        ...patch,
        icon: followsName
          ? createTextIconValue(nextAuto, current.icon.iconBackground)
          : current.icon,
      };
    });
  };

  // A template that needs a skill the workspace lacks, or a connection, would
  // create a digital human that cannot do its work (ADR 0022).
  const requirementGap = skillCandidatesQuery.isLoading
    ? null
    : templateRequirementGap(
        template,
        skillCandidates,
        selectedSkillIds,
        connectorSelections.length
      );
  const requirementNotice = requirementGap
    ? [
        requirementGap.missingSkillIds.length > 0
          ? t('createFlow.form.requiredSkillsMissing', {
              skills: requirementGap.missingSkillIds.join(', '),
            })
          : '',
        requirementGap.connectorMissing ? t('createFlow.form.connectorRequired') : '',
      ]
        .filter(Boolean)
        .join(' ')
    : null;

  const requestBack = () => {
    if (dirty && !created) {
      setDiscardOpen(true);
      return;
    }
    onBack();
  };

  /**
   * Compose the single replacement list creation will send.
   *
   * Skills and Connector connections are resolved against the workspace's real
   * candidates; the Connector side uses the exact snapshots the user selected.
   * A stale, unavailable, widened, or empty selection fails here, so creation
   * can stop before it writes anything.
   */
  const composeSelectedBindings = () =>
    composeCreateBindings(
      selectedSkillIds,
      skillCandidates,
      createConnectorSelectionBindings(connectorSelections),
      createConnectorSelectionCandidates(connectorSelections)
    );

  /**
   * Bind exactly the composed list, once.
   *
   * Nothing selected means nothing to bind; the create request itself never
   * carries a credential or capability claim.
   */
  const bindSelectedCapabilities = async (
    id: string,
    lockVersion: number,
    items: DigitalHumanResourceBindingInput[]
  ) => {
    if (items.length === 0) return [];
    const bound = await digitalHumanService.replaceDraftBindings(id, lockVersion, items);
    return bindingDisplayNames(bound.data?.items ?? items);
  };

  const submit = async () => {
    if (submitting) return;
    // The digital human already exists for this form: never create a second one.
    if (created) {
      router.push(`${LIST_PATH}/${created.id}/configuration`);
      return;
    }
    setSubmitting(true);
    setSubmitError(null);
    let items: DigitalHumanResourceBindingInput[];
    try {
      items = composeSelectedBindings();
    } catch {
      // An invalid selection must block creation with a visible error rather
      // than create a draft that cannot carry the chosen authority.
      setSubmitError(t('createFlow.form.invalidSelection'));
      setSubmitting(false);
      return;
    }
    try {
      const payload = createDigitalHumanPayload({
        name: value.name,
        responsibility: value.responsibility,
        icon: value.icon,
        model: effectiveModel,
        template,
        sharedRules,
      });
      const response = await mutations.create.mutateAsync(payload);
      const detail = response.data;
      const id = detail?.definition.id;
      if (!id) throw new Error('missing digital human id');

      let notice: string | null = null;
      let boundCapabilityNames: string[] = [];

      try {
        boundCapabilityNames = await bindSelectedCapabilities(
          id,
          detail?.draft?.lock_version ?? 1,
          items
        );
      } catch {
        notice = t('createFlow.form.bindingFailed');
      }

      // Something the template requires is still missing: the digital human
      // stays a draft, and its setup checklist leads the way (ADR 0023).
      if (!notice && requirementGap) {
        await queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.detail(id) });
        router.push(`${LIST_PATH}/${id}/configuration?setup=1`);
        return;
      }

      if (!notice) {
        try {
          const fresh = await digitalHumanService.get(id);
          await digitalHumanService.publish(id, {
            expected_revision: fresh.data?.definition.revision ?? detail?.definition.revision ?? 0,
            expected_draft_lock_version: fresh.data?.draft?.lock_version ?? 0,
          });
        } catch {
          notice = t('createFlow.form.publishFailed');
        }
      }

      if (!notice) {
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.detail(id) }),
          queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.draftBindings(id) }),
        ]);
      }

      setCreated({ id, published: !notice, notice, boundCapabilityNames });
      if (!notice) setDialogOpen(true);
    } catch {
      // Keep every field so the user can retry.
      setSubmitError(t('createFlow.form.createFailed'));
    } finally {
      setSubmitting(false);
    }
  };

  const draftBase = created ? `${LIST_PATH}/${created.id}` : LIST_PATH;
  const notice = created?.notice ?? null;

  return (
    <>
      <DigitalHumanTemplateForm
        templateTitle={templateTitle}
        tags={tags}
        skillCandidates={skillCandidates}
        selectedSkillIds={selectedSkillIds}
        onChangeSkillSelection={changeSkillSelection}
        skillsLoading={skillCandidatesQuery.isLoading}
        skillsError={skillCandidatesQuery.isError}
        onRetrySkills={() => void skillCandidatesQuery.refetch()}
        connectorSelections={connectorSelections}
        onChangeConnectorSelections={setConnectorSelections}
        value={{ ...value, model: effectiveModel }}
        onChange={handleChange}
        models={availableModels.models}
        modelsLoading={availableModels.isLoading}
        modelsError={availableModels.error}
        onRetryModels={() => void availableModels.refetch()}
        submitError={submitError}
        submitting={submitting}
        onSubmit={() => void submit()}
        onBack={requestBack}
        notice={notice ?? (created ? t('createFlow.form.draftCreatedNotice') : requirementNotice)}
        submitLabel={
          created
            ? t('createFlow.success.continueSetup')
            : requirementGap
              ? t('createFlow.form.createDraft')
              : undefined
        }
      />

      <DigitalHumanCreateSuccessDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        name={value.name}
        templateTitle={templateTitle}
        responsibility={value.responsibility}
        tags={tags}
        boundCapabilityNames={created?.boundCapabilityNames ?? []}
        icon={value.icon}
        onContinueSetup={() => router.push(`${draftBase}/configuration`)}
        onChannels={() => router.push(`${draftBase}/channels`)}
        onAutonomy={() => router.push(`${draftBase}/configuration`)}
        onBackToList={() => router.push(LIST_PATH)}
      />

      <ConfirmDialog
        open={discardOpen}
        onOpenChange={setDiscardOpen}
        variant="warning"
        title={t('createFlow.form.discardTitle')}
        description={t('createFlow.form.discardDescription')}
        cancelText={t('createFlow.form.discardCancel')}
        confirmText={t('createFlow.form.discardConfirm')}
        onConfirm={() => {
          setValue(initialValue);
          setConnectorSelections([]);
          onBack();
        }}
      />
    </>
  );
}
