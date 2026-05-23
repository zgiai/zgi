import React, { useMemo } from 'react';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import WorkflowInputForm, {
  type FormInputs,
  type WorkflowInputFormHandle,
} from '@/components/workflow/common/workflow-input-form';
import type { InputVar } from '@/components/workflow/types/input-var';
import { useT } from '@/i18n';
import { getEffectiveAllowedFileExtensions } from '@/utils/file-helpers';

interface InputsTabProps {
  isLoadingDraft: boolean;
  hasLocalNodes: boolean;
  startVariables: InputVar[];
  initialValues?: FormInputs;
  isStarting: boolean;
  onSubmit: (values: FormInputs) => void;
  onRunNoInputs: () => void;
  onInputChange?: (values: FormInputs) => void;
  topNotice?: React.ReactNode;
  topContent?: React.ReactNode;
  formRef?: React.Ref<WorkflowInputFormHandle>;
  hideSubmitButton?: boolean;
}

/**
 * InputsTab - shows the inputs form/skeleton and run button
 */
const InputsTab: React.FC<InputsTabProps> = ({
  isLoadingDraft,
  hasLocalNodes,
  startVariables,
  initialValues,
  isStarting,
  onSubmit,
  onRunNoInputs,
  onInputChange,
  topNotice,
  topContent,
  formRef,
  hideSubmitButton = false,
}) => {
  const t = useT('agents');
  const varsSig = useMemo(
    () =>
      JSON.stringify(
        startVariables.map(v => ({
          variable: v.variable,
          description: v.description ?? undefined,
          type: v.type,
          required: Boolean(v.required),
          options: v.options ?? [],
          allowed_file_types: v.allowed_file_types ?? [],
          effective_allowed_file_extensions: getEffectiveAllowedFileExtensions(
            v.allowed_file_types ?? [],
            v.allowed_file_extensions ?? []
          ),
          max_length: v.max_length ?? undefined,
        }))
      ),
    [startVariables]
  );
  if (isLoadingDraft && !hasLocalNodes) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-5 w-56" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-9 w-full" />
      </div>
    );
  }

  if (startVariables.length === 0) {
    return (
      <div className="relative space-y-3 text-sm text-muted-foreground">
        {topContent}
        {topNotice}
        <div>{t('workflow.noInputsDefined')}</div>
        {!hideSubmitButton && (
          <Button size="sm" onClick={onRunNoInputs} disabled={isStarting}>
            {isStarting ? t('workflow.starting') : t('workflow.runNow')}
          </Button>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {topContent}
      <WorkflowInputForm
        ref={formRef}
        key={varsSig}
        startVariables={startVariables}
        initialValues={initialValues}
        isStarting={isStarting}
        onSubmit={onSubmit}
        onChange={onInputChange}
        hideSubmitButton={hideSubmitButton}
        showResetButton={!hideSubmitButton}
        topNotice={topNotice}
      />
    </div>
  );
};

export default InputsTab;
