import React, { forwardRef, useMemo } from 'react';
import { cn } from '@/lib/utils';
import WorkflowInputForm, {
  type FormInputs,
  type WorkflowInputFormHandle,
  type WorkflowFileUploadAccessMode,
} from '@/components/workflow/common/workflow-input-form';
import type { InputVar } from '@/components/workflow/types/input-var';
import { Button } from '@/components/ui/button';
import { X } from 'lucide-react';
import { getEffectiveAllowedFileExtensions } from '@/utils/file-helpers';

interface ToolbarFormSpecLite {
  variables: InputVar[];
  initialValues?: Record<string, unknown>;
  icon?: React.ReactNode;
  title?: string;
}

interface ToolbarFormPanelProps {
  toolbarForm: ToolbarFormSpecLite;
  isOpen: boolean;
  onValuesChange: (vals: FormInputs) => void;
  handleClose: () => void;
  fileUploadAccessMode?: WorkflowFileUploadAccessMode;
  allowWorkspaceSwitch?: boolean;
}

const ToolbarFormPanel = forwardRef<WorkflowInputFormHandle, ToolbarFormPanelProps>(
  (
    {
      toolbarForm,
      isOpen,
      onValuesChange,
      handleClose,
      fileUploadAccessMode,
      allowWorkspaceSwitch,
    },
    ref
  ) => {
    const visibleVariables = useMemo(
      () => toolbarForm.variables.filter(v => !v.hide),
      [toolbarForm.variables]
    );
    const varsSig = useMemo(
      () =>
        JSON.stringify(
          visibleVariables.map(v => ({
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
      [visibleVariables]
    );

    return (
      <div className={cn('relative', isOpen ? 'block' : 'hidden')}>
        <div className="relative flex max-h-[500px] flex-col overflow-hidden rounded-xl border border-border/80 bg-popover shadow-lg shadow-black/10">
          {toolbarForm.title ? (
            <div className="flex items-center justify-between border-b bg-muted/20 px-2.5 py-1.5">
              <div className="flex min-w-0 items-center gap-1.5">
                {toolbarForm.icon ? (
                  <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary [&_svg]:size-3.5">
                    {toolbarForm.icon}
                  </span>
                ) : null}
                <span className="truncate text-xs font-semibold text-foreground">
                  {toolbarForm.title}
                </span>
              </div>
              <Button
                variant="ghost"
                className="size-6 shrink-0 rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
                isIcon
                onClick={handleClose}
              >
                <X className="size-3.5" />
              </Button>
            </div>
          ) : null}
          <div className="flex-1 overflow-y-auto px-3 pt-3">
            <WorkflowInputForm
              key={varsSig}
              ref={ref}
              startVariables={visibleVariables}
              initialValues={(toolbarForm.initialValues as FormInputs) ?? undefined}
              isStarting={false}
              onSubmit={() => {}}
              onChange={onValuesChange}
              hideSubmitButton
              fileUploadAccessMode={fileUploadAccessMode}
              allowWorkspaceSwitch={allowWorkspaceSwitch}
            />
          </div>
          <div className="absolute -bottom-1.5 left-7 size-3 rotate-45 border-b border-r border-border/80 bg-popover" />
        </div>
      </div>
    );
  }
);

ToolbarFormPanel.displayName = 'ToolbarFormPanel';

export default ToolbarFormPanel;
