'use client';

import { useMemo, useRef, useState } from 'react';
import { ArrowLeft, ChevronDown, Plus, Puzzle } from 'lucide-react';
import { IconInput } from '@/components/common/icon-input';
import type { IconValue } from '@/components/common/icon-input/types';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import {
  DigitalHumanSkillBindingCard,
  DigitalHumanSkillPickerDialog,
} from '@/components/digital-human/digital-human-skill-picker';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { DigitalHumanResourceCandidate } from '@/services/types/digital-human';
import type { ModelItem } from '@/services/types/model';
import { cn } from '@/lib/utils';
import { DigitalHumanTemplatePreview } from './digital-human-template-preview';

export interface DigitalHumanTemplateFormValue {
  name: string;
  responsibility: string;
  icon: IconValue;
  model: string;
}

interface DigitalHumanTemplateFormProps {
  templateTitle: string;
  tags: readonly string[];
  /** Workspace skills the user may bind, in the order the server returned them. */
  skillCandidates: readonly DigitalHumanResourceCandidate[];
  /** Resource ids the user selected; empty means the digital human starts without skills. */
  selectedSkillIds: readonly string[];
  /** Commits the selection made in the picker dialog. */
  onChangeSkillSelection: (resourceIds: string[]) => void;
  skillsLoading: boolean;
  skillsError: boolean;
  onRetrySkills: () => void;
  value: DigitalHumanTemplateFormValue;
  onChange: (patch: Partial<DigitalHumanTemplateFormValue>) => void;
  models: ModelItem[];
  modelsLoading: boolean;
  modelsError: string | null;
  onRetryModels: () => void;
  submitError: string | null;
  submitting: boolean;
  onSubmit: () => void;
  onBack: () => void;
  /** Overrides the submit label, e.g. after the digital human already exists. */
  submitLabel?: string;
  /** Inline notice rendered above the form. */
  notice?: string | null;
}

const NAME_ERROR_ID = 'digital-human-create-name-error';
const RESPONSIBILITY_ERROR_ID = 'digital-human-create-responsibility-error';

/**
 * Configure step: basic info, the skills creation will bind, the advanced model
 * setting, live preview and the fixed bottom action bar.
 *
 * Skill selection is the user's: a template only seeds skills. This form only
 * records the choice; creation binds the selected capabilities once the digital
 * human exists. The list never renders a connect, test, or deploy action.
 */
export function DigitalHumanTemplateForm({
  templateTitle,
  tags,
  skillCandidates,
  selectedSkillIds,
  onChangeSkillSelection,
  skillsLoading,
  skillsError,
  onRetrySkills,
  value,
  onChange,
  models,
  modelsLoading,
  modelsError,
  onRetryModels,
  submitError,
  submitting,
  onSubmit,
  onBack,
  submitLabel,
  notice,
}: DigitalHumanTemplateFormProps) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [nameError, setNameError] = useState(false);
  const [responsibilityError, setResponsibilityError] = useState(false);
  const [skillPickerOpen, setSkillPickerOpen] = useState(false);
  // The dialog edits a draft selection and only commits it on save, exactly like
  // the configuration page's picker.
  const [pendingSkillIds, setPendingSkillIds] = useState<string[]>([]);
  const nameRef = useRef<HTMLInputElement>(null);
  const responsibilityRef = useRef<HTMLTextAreaElement>(null);

  const selectedSkills = useMemo(
    () =>
      selectedSkillIds
        .map(resourceId => skillCandidates.find(candidate => candidate.resource_id === resourceId))
        .filter((candidate): candidate is DigitalHumanResourceCandidate => Boolean(candidate)),
    [selectedSkillIds, skillCandidates]
  );

  const openSkillPicker = () => {
    setPendingSkillIds([...selectedSkillIds]);
    setSkillPickerOpen(true);
  };
  const togglePendingSkill = (candidate: DigitalHumanResourceCandidate) => {
    setPendingSkillIds(current =>
      current.includes(candidate.resource_id)
        ? current.filter(id => id !== candidate.resource_id)
        : [...current, candidate.resource_id]
    );
  };
  const saveSkillSelection = () => {
    onChangeSkillSelection(pendingSkillIds);
    setSkillPickerOpen(false);
  };
  const removeSkill = (resourceId: string) =>
    onChangeSkillSelection(selectedSkillIds.filter(id => id !== resourceId));

  const noModelAvailable = !modelsLoading && !modelsError && models.length === 0;
  const modelBlocked = modelsLoading || noModelAvailable || Boolean(modelsError);
  const selectedModel = models.find(item => item.model === value.model);
  const modelValue: ModelSelectorValue = {
    provider: selectedModel?.provider ?? '',
    model: selectedModel?.model ?? value.model,
  };

  const submit = () => {
    const missingName = !value.name.trim();
    const missingResponsibility = !value.responsibility.trim();
    setNameError(missingName);
    setResponsibilityError(missingResponsibility);
    if (missingName) {
      nameRef.current?.focus();
      return;
    }
    if (missingResponsibility) {
      responsibilityRef.current?.focus();
      return;
    }
    if (modelBlocked) return;
    onSubmit();
  };

  return (
    <div className="flex min-h-full flex-col">
      <header className="flex items-center gap-2 border-b px-4 py-3 md:px-6">
        <Button variant="ghost" size="sm" onClick={onBack} disabled={submitting}>
          <ArrowLeft className="size-4" />
          {t('createFlow.backToGallery')}
        </Button>
        <span className="truncate text-sm text-muted-foreground">
          {t('createFlow.title')} / {templateTitle}
        </span>
      </header>

      <div className="flex-1 px-4 py-5 md:px-6">
        <div className="mx-auto grid w-full max-w-[1440px] gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
          <div className="min-w-0 space-y-6">
            {notice ? (
              <p
                role="status"
                className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm text-amber-700 dark:text-amber-300"
              >
                {notice}
              </p>
            ) : null}

            {submitError ? (
              <p
                role="alert"
                className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive"
              >
                {submitError}
              </p>
            ) : null}

            <section className="space-y-4 rounded-xl border p-4 md:p-5">
              <div className="space-y-1.5">
                <label className="block text-sm font-medium" htmlFor="digital-human-create-name">
                  {t('createFlow.form.name')}
                </label>
                <Input
                  id="digital-human-create-name"
                  ref={nameRef}
                  value={value.name}
                  onChange={event => {
                    onChange({ name: event.target.value });
                    if (nameError) setNameError(false);
                  }}
                  placeholder={t('createFlow.form.namePlaceholder')}
                  aria-invalid={nameError}
                  aria-describedby={nameError ? NAME_ERROR_ID : undefined}
                  disabled={submitting}
                  autoFocus
                />
                {nameError ? (
                  <p id={NAME_ERROR_ID} className="text-xs text-destructive">
                    {t('createFlow.form.requiredName')}
                  </p>
                ) : null}
              </div>

              <div className="space-y-1.5">
                <label
                  className="block text-sm font-medium"
                  htmlFor="digital-human-create-responsibility"
                >
                  {t('createFlow.form.responsibility')}
                </label>
                <Textarea
                  id="digital-human-create-responsibility"
                  ref={responsibilityRef}
                  value={value.responsibility}
                  onChange={event => {
                    onChange({ responsibility: event.target.value });
                    if (responsibilityError) setResponsibilityError(false);
                  }}
                  placeholder={t('createFlow.form.responsibilityPlaceholder')}
                  aria-invalid={responsibilityError}
                  aria-describedby={responsibilityError ? RESPONSIBILITY_ERROR_ID : undefined}
                  disabled={submitting}
                  rows={4}
                />
                {responsibilityError ? (
                  <p id={RESPONSIBILITY_ERROR_ID} className="text-xs text-destructive">
                    {t('createFlow.form.requiredResponsibility')}
                  </p>
                ) : null}
              </div>

              <div className="space-y-1.5">
                <span className="block text-sm font-medium">{t('createFlow.form.avatar')}</span>
                <IconInput
                  showAvatarPresets
                  value={value.icon}
                  defaultValue={value.icon}
                  onChange={next => onChange({ icon: next })}
                  disabled={submitting}
                />
              </div>
            </section>

            {/* The configuration page's capability section and picker dialog, so a
                skill is added and reviewed the same way before and after creation. */}
            <div className="space-y-2">
              <p className="text-xs leading-5 text-muted-foreground">
                {t('createFlow.form.skillsHint')}
              </p>
              <section className="flex min-h-[180px] flex-col overflow-hidden rounded-xl border bg-card shadow-sm">
                <div className="flex items-center justify-between gap-3 border-b bg-muted/15 px-3 py-3">
                  <div className="flex min-w-0 items-center gap-2.5">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-primary">
                      <Puzzle className="size-4" />
                    </span>
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <h2 className="font-medium">{t('bindings.sections.skills.title')}</h2>
                      <Badge variant="subtle">
                        {t('bindings.selectedCount', { count: selectedSkillIds.length })}
                      </Badge>
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    isIcon
                    className="size-8 shrink-0"
                    aria-label={t('bindings.sections.skills.add')}
                    disabled={submitting || skillsError}
                    onClick={openSkillPicker}
                  >
                    <Plus className="size-4" />
                  </Button>
                </div>
                <div className="min-h-0 flex-1 p-3">
                  {skillsLoading ? (
                    <div className="space-y-2">
                      <div className="h-20 animate-pulse rounded-lg bg-muted" />
                      <div className="h-20 animate-pulse rounded-lg bg-muted" />
                    </div>
                  ) : skillsError ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-xs text-destructive">
                        {t('createFlow.form.skillsLoadFailed')}
                      </p>
                      <Button variant="outline" size="sm" onClick={onRetrySkills}>
                        {t('createFlow.form.retry')}
                      </Button>
                    </div>
                  ) : selectedSkills.length === 0 ? (
                    <button
                      type="button"
                      className="flex min-h-[130px] w-full flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 px-5 py-6 text-center transition-colors motion-reduce:transition-none hover:border-primary/40 hover:bg-primary/[0.03] disabled:pointer-events-none"
                      disabled={submitting || skillCandidates.length === 0}
                      onClick={openSkillPicker}
                    >
                      <Plus className="mb-2 size-5 text-muted-foreground" />
                      <span className="text-sm font-medium text-foreground">
                        {t('bindings.sections.skills.add')}
                      </span>
                      <span className="mt-1 text-xs leading-5 text-muted-foreground">
                        {skillCandidates.length === 0
                          ? t('bindings.noCandidatesDescription')
                          : t('createFlow.form.skillsOptional')}
                      </span>
                    </button>
                  ) : (
                    <div className="grid grid-cols-1 gap-3" data-resource-binding-grid>
                      {selectedSkills.map(candidate => (
                        <DigitalHumanSkillBindingCard
                          key={candidate.resource_id}
                          item={candidate}
                          locale={locale}
                          removeLabel={t('bindings.removeNamed', {
                            name: candidate.display_name || candidate.resource_id,
                          })}
                          onRemove={() => removeSkill(candidate.resource_id)}
                          disabled={submitting}
                        />
                      ))}
                    </div>
                  )}
                </div>
              </section>
            </div>

            <DigitalHumanSkillPickerDialog
              open={skillPickerOpen}
              onOpenChange={setSkillPickerOpen}
              candidates={skillCandidates}
              selectedIds={pendingSkillIds}
              onToggle={togglePendingSkill}
              onSave={saveSkillSelection}
              locale={locale}
              loading={skillsLoading}
              error={skillsError}
              onRetry={onRetrySkills}
            />

            <section className="rounded-xl border">
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <button
                    type="button"
                    className="group flex w-full items-center justify-between gap-3 p-4 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring md:p-5"
                  >
                    <span className="text-sm font-semibold">{t('createFlow.form.advanced')}</span>
                    <span className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                      <span className="truncate">
                        {value.model || t('createFlow.form.currentModel')}
                      </span>
                      <ChevronDown className="size-4 shrink-0 transition-transform motion-reduce:transition-none group-data-[state=open]:rotate-180" />
                    </span>
                  </button>
                </CollapsibleTrigger>
                <CollapsibleContent className="space-y-3 border-t p-4 md:p-5">
                  <span className="block text-sm font-medium">{t('createFlow.form.model')}</span>
                  <ModelSelector
                    modelType="text-chat"
                    availabilityUseCase="agent-runtime"
                    preferredUseCase="agent"
                    capabilityFilter={{ features_tool_call: true }}
                    modelsOverride={models}
                    value={modelValue}
                    onChange={next => onChange({ model: next.model })}
                    placeholder={t('createFlow.form.model')}
                    disabled={modelsLoading || submitting}
                    hasError={Boolean(modelsError)}
                    className="w-full"
                  />
                  {modelsLoading ? (
                    <p className="text-xs text-muted-foreground">
                      {t('createFlow.form.modelLoading')}
                    </p>
                  ) : null}
                  {modelsError ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-xs text-destructive">
                        {t('createFlow.form.modelLoadFailed')}
                      </p>
                      <Button variant="outline" size="sm" onClick={onRetryModels}>
                        {t('createFlow.form.retry')}
                      </Button>
                    </div>
                  ) : null}
                  {noModelAvailable ? (
                    <p className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">
                      {t('createFlow.form.modelUnavailable')}
                    </p>
                  ) : null}
                </CollapsibleContent>
              </Collapsible>
            </section>
          </div>

          <aside className="h-fit space-y-3 lg:sticky lg:top-4">
            <h2 className="text-xs font-medium text-muted-foreground">
              {t('createFlow.preview.title')}
            </h2>
            <DigitalHumanTemplatePreview
              icon={value.icon}
              name={value.name}
              templateTitle={templateTitle}
              responsibility={value.responsibility}
              tags={tags}
            />
          </aside>
        </div>
      </div>

      <div className="sticky bottom-0 border-t bg-background/95 px-4 py-3 backdrop-blur md:px-6">
        <div className="mx-auto flex w-full max-w-[1440px] items-center justify-between gap-3">
          <Button variant="outline" onClick={onBack} disabled={submitting}>
            {t('createFlow.backToGallery')}
          </Button>
          <Button
            onClick={submit}
            disabled={submitting || modelBlocked}
            aria-busy={submitting}
            className={cn(submitting && 'cursor-progress')}
          >
            {submitting
              ? t('createFlow.form.submitting')
              : (submitLabel ?? t('createFlow.form.submit'))}
          </Button>
        </div>
      </div>
    </div>
  );
}
