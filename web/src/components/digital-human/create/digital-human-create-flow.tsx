'use client';

import { useMemo, useRef, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { createTextIconValue, type IconValue } from '@/components/common/icon-input/types';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  useDigitalHumanMutations,
  useDigitalHumanSkillCandidates,
} from '@/hooks/digital-human/use-digital-humans';
import { useAvailableModels } from '@/hooks/model/use-model';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import { digitalHumanService } from '@/services/digital-human.service';
import type { DigitalHumanResourceCandidate } from '@/services/types/digital-human';
import {
  bindingDisplayNames,
  createDigitalHumanPayload,
  selectCandidateBindings,
  templateDefaultSkillIds,
} from './create-digital-human-payload';
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
  type DigitalHumanTemplate,
} from './digital-human-templates';
import { useRoleTextResolver } from './use-role-texts';

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
  const template = findDigitalHumanTemplate(templateParam);
  const isCustom = isCustomTemplateId(templateParam);
  const invalidTemplate = Boolean(templateParam) && !template && !isCustom;

  const selectTemplate = (templateId: string) => {
    router.push(`${CREATE_PATH}?template=${encodeURIComponent(templateId)}`);
  };

  const backToGallery = () => {
    router.push(CREATE_PATH);
  };

  if (template || isCustom) {
    return (
      <DigitalHumanConfigureStep
        // Remount per template so the form always starts from that template.
        key={templateParam ?? CUSTOM_TEMPLATE_ID}
        template={template}
        onBack={backToGallery}
      />
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
      <DigitalHumanTemplateGallery onSelectTemplate={selectTemplate} />
    </div>
  );
}

interface CreatedState {
  id: string;
  published: boolean;
  /** Set when creation succeeded but a later step failed. */
  notice: string | null;
  boundSkillNames: string[];
}

/**
 * Configure step for one template (or the custom entry).
 *
 * It owns the form state and the whole creation sequence: create the digital
 * human, bind the skills the template declares against the workspace's real
 * candidates, then publish version 1. Being remounted per template via `key`
 * keeps the seeding out of an effect, so a late model list can never wipe what
 * the user typed.
 */
function DigitalHumanConfigureStep({
  template,
  onBack,
}: {
  template: DigitalHumanTemplate | null;
  onBack: () => void;
}) {
  const t = useT('digitalHumans');
  const texts = useRoleTextResolver();
  const router = useRouter();
  const queryClient = useQueryClient();
  const availableModels = useAvailableModels({ use_case: 'agent-runtime' });
  const skillCandidatesQuery = useDigitalHumanSkillCandidates();
  const mutations = useDigitalHumanMutations();

  const templateTitle = template ? texts(template.titleKey) : t('createFlow.gallery.customEntry');
  const tags = useMemo(
    () => (template ? template.tagKeys.map(key => texts(key)) : []),
    [template, texts]
  );
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

  const [initialValue] = useState<DigitalHumanTemplateFormValue>(() => ({
    name: template ? texts(template.titleKey) : '',
    responsibility: template ? texts(template.responsibilityKey) : '',
    icon: template
      ? createTextIconValue(initials(texts(template.titleKey)), template.iconBackground)
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
  const dirty =
    value.name !== initialValue.name ||
    value.responsibility !== initialValue.responsibility ||
    value.model !== initialValue.model ||
    !iconEquals(value.icon, initialValue.icon) ||
    skillsDirty;

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

  const requestBack = () => {
    if (dirty && !created) {
      setDiscardOpen(true);
      return;
    }
    onBack();
  };

  /**
   * Bind exactly the skills the user selected, resolved against the candidates
   * the workspace really offers. Nothing selected means nothing to bind.
   */
  const bindSelectedSkills = async (id: string, lockVersion: number) => {
    const items = selectCandidateBindings(selectedSkillIds, skillCandidates);
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
    try {
      const payload = createDigitalHumanPayload({
        name: value.name,
        responsibility: value.responsibility,
        icon: value.icon,
        model: effectiveModel,
        template,
        texts,
      });
      const response = await mutations.create.mutateAsync(payload);
      const detail = response.data;
      const id = detail?.definition.id;
      if (!id) throw new Error('missing digital human id');

      let notice: string | null = null;
      let boundSkillNames: string[] = [];

      try {
        boundSkillNames = await bindSelectedSkills(id, detail?.draft?.lock_version ?? 1);
      } catch {
        notice = t('createFlow.form.bindingFailed');
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

      setCreated({ id, published: !notice, notice, boundSkillNames });
      if (!notice) setDialogOpen(true);
    } catch {
      // Keep every field so the user can retry.
      setSubmitError(t('createFlow.form.createFailed'));
      toast.error(t('createFlow.form.createFailed'));
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
        submitLabel={created ? t('createFlow.success.continueSetup') : undefined}
        notice={notice ?? (created ? t('createFlow.form.draftCreatedNotice') : null)}
      />

      <DigitalHumanCreateSuccessDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        name={value.name}
        templateTitle={templateTitle}
        responsibility={value.responsibility}
        tags={tags}
        boundSkillNames={created?.boundSkillNames ?? []}
        icon={value.icon}
        onContinueSetup={() => router.push(`${draftBase}/configuration`)}
        onChannels={() => router.push(`${draftBase}/channels`)}
        onAutonomy={() => router.push(`${draftBase}/configuration#configuration-behavior`)}
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
          onBack();
        }}
      />
    </>
  );
}
