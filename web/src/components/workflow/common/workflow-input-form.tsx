import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { FileUpload } from '@/components/common/file-upload';
import type { UploadedFile } from '@/services/types/dataset';
import { useForm } from 'react-hook-form';
import type { InputVar, InputVarType } from '@/components/workflow/types/input-var';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { getEffectiveAllowedFileExtensions } from '@/utils/file-helpers';
import { useUploadConfig } from '@/hooks/use-upload';

// Allowed input value types for run payload
export interface FileInputPayload {
  type: string; // e.g., 'document' | 'image' | 'audio' | 'video' | 'custom'
  transfer_method: 'local_file'; // future: 'remote_url'
  url: string;
  upload_file_id: string;
}

export type WorkflowFileUploadAccessMode = 'enabled' | 'login-required';

export type FormInputs = Record<
  string,
  | string
  | number
  | boolean
  | string[]
  | number[]
  | boolean[]
  | FileInputPayload
  | FileInputPayload[]
  | null
  | undefined
>;

/**
 * Centrally transform raw form inputs (containing file/file-list IDs)
 * into structured payloads expected by the workflow runner.
 */
export function transformFilesToPayload(
  values: Record<string, unknown>,
  variables: InputVar[]
): FormInputs {
  const transformed: FormInputs = { ...values } as FormInputs;
  variables.forEach(v => {
    if (v.type === 'file') {
      const id = values[v.variable] as string | undefined;
      if (id) {
        const fileType = (v.allowed_file_types && v.allowed_file_types[0]) || 'document';
        transformed[v.variable] = {
          type: fileType,
          transfer_method: 'local_file',
          url: '',
          upload_file_id: id,
        } as FileInputPayload;
      } else {
        transformed[v.variable] = undefined;
      }
    }
    if (v.type === 'file-list') {
      const ids = (values[v.variable] as string[] | undefined) ?? [];
      const fileType = (v.allowed_file_types && v.allowed_file_types[0]) || 'document';
      transformed[v.variable] = ids.map(fid => ({
        type: fileType,
        transfer_method: 'local_file',
        url: '',
        upload_file_id: fid,
      })) as FileInputPayload[];
    }
  });
  return transformed;
}

interface WorkflowInputFormProps {
  // Variables from start node used to render the form
  startVariables: InputVar[];
  // Optional initial values to populate/override defaults
  initialValues?: FormInputs;
  // Loading state to disable submit button
  isStarting: boolean;
  // Upstream submit handler receives raw values keyed by variable name
  onSubmit: (values: FormInputs) => void;
  // Optional change callback invoked on any value change
  onChange?: (values: FormInputs) => void;
  // Optional: hide submit button (used when embedding as a settings-only form)
  hideSubmitButton?: boolean;
  // Optional: show reset button to restore defaults
  showResetButton?: boolean;
  // Optional: notify parent when form validity changes
  onValidChange?: (valid: boolean) => void;
  // Optional compact notice shown above the form area
  topNotice?: React.ReactNode;
  // Optional file input access policy for webapp anonymous mode
  fileUploadAccessMode?: WorkflowFileUploadAccessMode;
  // Optional: allow switching current workspace inside system file selector
  allowWorkspaceSwitch?: boolean;
}

/**
 * WorkflowInputForm - Adaptive form that renders fields based on startVariables
 * Pure presentational component that manages its own RHF state and file mapping,
 * and emits raw values via onSubmit for the parent to transform and submit.
 */
export interface WorkflowInputFormHandle {
  submit: () => void;
  reset: () => void;
  setValues: (values: FormInputs) => void;
  validate: () => Promise<boolean>;
}

const FORM_LABEL_CLASS =
  'flex items-center gap-1 mb-1.5 text-[13px] font-medium text-muted-foreground';

function toPositiveNumber(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) return value;
  if (typeof value === 'string') {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && parsed > 0) return parsed;
  }
  return undefined;
}

const WorkflowInputForm = React.forwardRef<WorkflowInputFormHandle, WorkflowInputFormProps>(
  (
    {
      startVariables,
      initialValues,
      isStarting,
      onSubmit,
      onChange,
      hideSubmitButton = false,
      showResetButton = false,
      onValidChange,
      topNotice,
      fileUploadAccessMode = 'enabled',
      allowWorkspaceSwitch = false,
    },
    ref
  ) => {
    const t = useT('agents');
    const tUi = useT('ui');
    const router = useRouter();
    const pathname = usePathname();
    const normalizeNumberInputValue = useCallback((inputEl: HTMLInputElement) => {
      const rawValue = inputEl.value.trim();
      if (rawValue === '') return undefined;

      const numericValue = inputEl.valueAsNumber;
      return Number.isFinite(numericValue) ? numericValue : undefined;
    }, []);

    // Local controlled state for uploaded files per variable
    const [fileStates, setFileStates] = useState<Record<string, UploadedFile[]>>({});
    const fileStatesRef = useRef(fileStates);
    const [loginDialogOpen, setLoginDialogOpen] = useState(false);
    const isFileUploadLoginRequired = fileUploadAccessMode === 'login-required';
    const { data: uploadConfig } = useUploadConfig({
      enabled: !isFileUploadLoginRequired,
    });
    const maxSizeMB = toPositiveNumber(uploadConfig?.file_size_limit) ?? 15;
    const workflowFileUploadLimit =
      toPositiveNumber(uploadConfig?.workflow_file_upload_limit) ??
      toPositiveNumber(uploadConfig?.batch_count_limit);
    const maxCountLimit = workflowFileUploadLimit ?? Number.POSITIVE_INFINITY;
    const getFileListMaxCount = useCallback(
      (configuredMaxLength?: number) => {
        if (typeof configuredMaxLength === 'number' && configuredMaxLength > 0) {
          return configuredMaxLength;
        }
        return Number.isFinite(maxCountLimit) ? maxCountLimit : 5;
      },
      [maxCountLimit]
    );

    useEffect(() => {
      fileStatesRef.current = fileStates;
    }, [fileStates]);

    // Build schema defaults from start variables only. Runtime initial values are kept separate
    // so "restore defaults" does not accidentally restore the user's latest typed input.
    const schemaDefaultValues = useMemo<FormInputs>(() => {
      const result: FormInputs = {};
      startVariables.forEach(v => {
        switch (v.type) {
          case 'checkbox':
            result[v.variable] = typeof v.default === 'boolean' ? v.default : false;
            break;
          case 'number': {
            const num =
              typeof v.default === 'number'
                ? v.default
                : typeof v.default === 'string'
                  ? Number(v.default)
                  : undefined;
            result[v.variable] = Number.isFinite(num as number) ? (num as number) : undefined;
            break;
          }
          case 'file':
            result[v.variable] = undefined; // single file id
            break;
          case 'file-list':
            result[v.variable] = [] as string[]; // multiple file ids
            break;
          default:
            result[v.variable] = typeof v.default === 'string' ? v.default : '';
        }
      });
      return result;
    }, [startVariables]);

    // Merge provided initial values (if any) to allow repopulating a previous run.
    const defaultValues = useMemo<FormInputs>(
      () => ({ ...schemaDefaultValues, ...(initialValues ?? {}) }) as FormInputs,
      [schemaDefaultValues, initialValues]
    );

    // Schema signature for stability: only reset when actual schema changes
    const variablesSignature = useMemo(
      () =>
        JSON.stringify(
          startVariables.map(v => ({
            variable: v.variable,
            type: v.type,
            required: Boolean(v.required),
            options: v.options ?? [],
            allowed_file_types: v.allowed_file_types ?? [],
            effective_allowed_file_extensions: getEffectiveAllowedFileExtensions(
              v.allowed_file_types ?? [],
              v.allowed_file_extensions ?? []
            ),
            max_length: v.max_length ?? undefined,
            default: (v as { default?: unknown }).default ?? undefined,
          }))
        ),
      [startVariables]
    );

    // Initial values signature: reset when true external initial values change
    const initialSig = useMemo(() => JSON.stringify(initialValues ?? {}), [initialValues]);

    // RHF form state
    const form = useForm<FormInputs>({
      defaultValues,
      mode: 'onBlur',
      reValidateMode: 'onBlur',
    });

    const emitValuesChange = useCallback(() => {
      onChange?.(form.getValues() as FormInputs);
    }, [form, onChange]);

    // Reset only when schema or explicit initial values actually change
    const prevVarSigRef = useRef<string>('');
    const prevInitSigRef = useRef<string>('');
    useEffect(() => {
      const shouldReset =
        prevVarSigRef.current === '' ||
        prevVarSigRef.current !== variablesSignature ||
        prevInitSigRef.current !== initialSig;

      if (shouldReset) {
        form.reset(defaultValues);
        prevVarSigRef.current = variablesSignature;
        prevInitSigRef.current = initialSig;
      }
    }, [variablesSignature, initialSig, defaultValues, form]);

    useEffect(() => {
      onValidChange?.(form.formState.isValid);
    }, [form.formState.isValid, onValidChange]);
    // Hydrate file states from initialValues IDs
    useEffect(() => {
      if (!initialValues) return;

      const hydrateFiles = async () => {
        const fileVars = startVariables.filter(v => v.type === 'file' || v.type === 'file-list');
        if (fileVars.length === 0) return;

        // Collect all needed file IDs
        const idsToFetch: string[] = [];
        const varMap: Record<string, string[]> = {};

        fileVars.forEach(v => {
          const val = initialValues[v.variable];
          if (!val) return;

          // Skip if we already have state for this variable
          if (fileStatesRef.current[v.variable]?.length > 0) return;

          const ids = Array.isArray(val) ? (val as string[]) : [val as string];
          // Filter out IDs that look like file objects (just in case) or empty strings
          const cleanIds = ids.filter(id => typeof id === 'string' && id.trim().length > 0);

          if (cleanIds.length > 0) {
            idsToFetch.push(...cleanIds);
            varMap[v.variable] = cleanIds;
          }
        });

        if (idsToFetch.length === 0) return;

        try {
          setFileStates(prev => {
            const next = { ...prev };
            Object.entries(varMap).forEach(([key, ids]) => {
              next[key] = ids.map(id => ({
                id,
                name: `file-${id}`, // Fallback name
                size: 0,
                type: 'application/octet-stream',
                created_at: Date.now(),
                extension: 'bin',
                mime_type: 'application/octet-stream',
                created_by: '',
              }));
            });
            return next;
          });
        } catch (e) {
          console.error('Failed to hydrate files', e);
        }
      };

      hydrateFiles();
    }, [initialValues, startVariables, t]); // Removed fileStates from deps to avoid loop, handled inside

    // Submit wrapper to emit values upstream
    const handleSubmit = useCallback(
      (values: FormInputs) => {
        const transformed = transformFilesToPayload(
          values as Record<string, unknown>,
          startVariables
        );
        onSubmit(transformed);
      },
      [onSubmit, startVariables]
    );

    const handleReset = useCallback(() => {
      form.reset(schemaDefaultValues);
      setFileStates({});
      onChange?.(schemaDefaultValues);
    }, [form, schemaDefaultValues, onChange]);

    const handleSetValues = useCallback(
      (values: FormInputs) => {
        const nextValues = { ...schemaDefaultValues, ...values };
        form.reset(nextValues);
        onChange?.(nextValues);
      },
      [form, onChange, schemaDefaultValues]
    );

    const handleLoginConfirm = useCallback(() => {
      setLoginDialogOpen(false);
      const currentSearch = typeof window !== 'undefined' ? window.location.search : '';
      const currentUrl = currentSearch ? `${pathname}${currentSearch}` : pathname || '/';
      router.push(`/login?redirect=${encodeURIComponent(currentUrl)}`);
    }, [pathname, router]);

    React.useImperativeHandle(
      ref,
      () => ({
        submit: () => {
          form.handleSubmit(
            vals => {
              handleSubmit(vals);
            },
            errs => {
              console.error('[WorkflowInputForm] form.handleSubmit validation errors', errs);
            }
          )();
        },
        reset: handleReset,
        setValues: handleSetValues,
        validate: async () => {
          // handleSubmit identifies the form as "attempted to submit",
          // which allows FormMessage to show errors even for untouched fields.
          await form.handleSubmit(
            () => {},
            () => {}
          )();
          const valid = await form.trigger();
          emitValuesChange();
          return valid;
        },
      }),
      [form, handleSubmit, handleReset, handleSetValues, emitValuesChange]
    );

    const isSameUploadedFiles = useCallback((a: UploadedFile[], b: UploadedFile[]) => {
      if (a.length !== b.length) return false;
      return a.every((item, index) => item.id === b[index]?.id);
    }, []);

    const isSameFileFieldValue = useCallback(
      (current: FormInputs[string], next: FormInputs[string], isList: boolean) => {
        if (isList) {
          const currentIds = Array.isArray(current) ? (current as string[]) : [];
          const nextIds = Array.isArray(next) ? (next as string[]) : [];
          if (currentIds.length !== nextIds.length) return false;
          return currentIds.every((id, index) => id === nextIds[index]);
        }
        return (current as string | undefined) === (next as string | undefined);
      },
      []
    );

    // Render field by type
    const renderField = useCallback(
      (input: InputVar) => {
        // i18n required message
        const requiredMsg = t('workflow.startForm.requiredField');
        const commonRules = input.required ? { required: requiredMsg } : {};

        switch (input.type as InputVarType) {
          case 'text-input':
            return (
              <FormField
                key={input.variable}
                control={form.control}
                name={input.variable}
                rules={commonRules}
                render={({ field }) => (
                  <FormItem className="animate-in fade-in-0 slide-in-from-bottom-2 duration-300">
                    <FormLabel className={FORM_LABEL_CLASS}>
                      {input.label}
                      {input.required && <span className="text-red-500 select-none">*</span>}
                    </FormLabel>
                    <FormControl>
                      <Input
                        placeholder={input.label}
                        maxLength={input.max_length}
                        {...field}
                        value={(field.value as string) ?? ''}
                        aria-invalid={!!form.formState.errors[input.variable]}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            );
          case 'paragraph':
            return (
              <FormField
                key={input.variable}
                control={form.control}
                name={input.variable}
                rules={commonRules}
                render={({ field }) => (
                  <FormItem className="animate-in fade-in-0 slide-in-from-bottom-2 duration-400">
                    <FormLabel className={FORM_LABEL_CLASS}>
                      {input.label}
                      {input.required && <span className="text-red-500 select-none">*</span>}
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={input.label}
                        maxLength={input.max_length}
                        {...field}
                        value={(field.value as string) ?? ''}
                        aria-invalid={!!form.formState.errors[input.variable]}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            );
          case 'select':
            return (
              <FormField
                key={input.variable}
                control={form.control}
                name={input.variable}
                rules={commonRules}
                render={({ field }) => (
                  <FormItem className="animate-in fade-in-0 slide-in-from-bottom-2 duration-500">
                    <FormLabel className={FORM_LABEL_CLASS}>
                      {input.label}
                      {input.required && <span className="text-red-500 select-none">*</span>}
                    </FormLabel>
                    <FormControl>
                      <Select onValueChange={field.onChange} value={(field.value as string) ?? ''}>
                        <SelectTrigger aria-invalid={!!form.formState.errors[input.variable]}>
                          <SelectValue placeholder={input.label} />
                        </SelectTrigger>
                        <SelectContent>
                          {(input.options ?? []).map(opt => (
                            <SelectItem key={opt} value={opt}>
                              {opt}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            );
          case 'number':
            return (
              <FormField
                key={input.variable}
                control={form.control}
                name={input.variable}
                rules={commonRules}
                render={({ field }) => (
                  <FormItem className="animate-in fade-in-0 slide-in-from-bottom-2 duration-600">
                    <FormLabel className={FORM_LABEL_CLASS}>
                      {input.label}
                      {input.required && <span className="text-red-500 select-none">*</span>}
                    </FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        value={
                          field.value === undefined || field.value === null
                            ? ''
                            : (field.value as number | string)
                        }
                        aria-invalid={!!form.formState.errors[input.variable]}
                        onChange={e => {
                          form.setValue(input.variable, e.target.value, {
                            shouldDirty: true,
                            shouldValidate: false,
                          });
                        }}
                        onBlur={e => {
                          field.onBlur();
                          form.setValue(
                            input.variable,
                            normalizeNumberInputValue(e.currentTarget),
                            {
                              shouldDirty: true,
                              shouldValidate: true,
                            }
                          );
                        }}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            );
          case 'checkbox':
            return (
              <FormField
                key={input.variable}
                control={form.control}
                name={input.variable}
                rules={commonRules}
                render={({ field }) => (
                  <FormItem className="flex flex-row items-center gap-3 py-2 animate-in fade-in-0 slide-in-from-bottom-2 duration-700 space-y-0">
                    <FormLabel className={cn(FORM_LABEL_CLASS, 'm-0')}>
                      {input.label}
                      {input.required && <span className="text-red-500 select-none">*</span>}
                    </FormLabel>
                    <FormControl>
                      <Checkbox checked={Boolean(field.value)} onCheckedChange={field.onChange} />
                    </FormControl>
                    <FormMessage className="!mt-0" />
                  </FormItem>
                )}
              />
            );
          case 'file':
          case 'file-list': {
            const isList = input.type === 'file-list';
            const acceptExt = getEffectiveAllowedFileExtensions(
              input.allowed_file_types ?? [],
              input.allowed_file_extensions ?? []
            );
            const valueFiles = fileStates[input.variable] ?? [];
            return (
              <FormField
                key={input.variable}
                control={form.control}
                name={input.variable}
                rules={
                  input.required
                    ? {
                        validate: v =>
                          (isList
                            ? Array.isArray(v) && (v as unknown[]).length > 0
                            : typeof v === 'string' && Boolean(v)) || requiredMsg,
                      }
                    : undefined
                }
                render={() => (
                  <FormItem className="animate-in fade-in-0 slide-in-from-bottom-2 duration-200">
                    <FormLabel className={FORM_LABEL_CLASS}>
                      {input.label}
                      {input.required && <span className="text-red-500 select-none">*</span>}
                    </FormLabel>
                    <FormControl>
                      <div>
                        {isFileUploadLoginRequired ? (
                          <div className="rounded-md border border-dashed border-border bg-muted/30 p-3">
                            <Button
                              type="button"
                              variant="outline"
                              className="w-full justify-start"
                              onClick={() => setLoginDialogOpen(true)}
                            >
                              {tUi('fileUpload.loginToUpload')}
                            </Button>
                            <p className="mt-2 text-xs text-muted-foreground">
                              {input.required
                                ? tUi('fileUpload.loginRequiredRequiredHint')
                                : tUi('fileUpload.loginRequiredHint')}
                            </p>
                          </div>
                        ) : (
                          <FileUpload
                            controlled
                            showSystemSelect
                            allowWorkspaceSwitch={allowWorkspaceSwitch}
                            value={valueFiles}
                            acceptExt={acceptExt}
                            maxCount={isList ? getFileListMaxCount(input.max_length) : 1}
                            maxSizeMB={maxSizeMB}
                            isTemporary
                            onChange={(files: UploadedFile[]) => {
                              setFileStates(prev => {
                                const prevFiles = prev[input.variable] ?? [];
                                if (isSameUploadedFiles(prevFiles, files)) return prev;
                                return { ...prev, [input.variable]: files };
                              });
                              const ids = files.map(f => f.id);
                              const nextValue = (
                                isList ? ids : (ids[0] ?? undefined)
                              ) as FormInputs[string];
                              const currentValue = form.getValues(input.variable);
                              if (!isSameFileFieldValue(currentValue, nextValue, isList)) {
                                form.setValue(input.variable, nextValue, {
                                  shouldDirty: true,
                                  shouldValidate: true,
                                });
                                onChange?.({
                                  ...form.getValues(),
                                  [input.variable]: nextValue,
                                } as FormInputs);
                              }
                            }}
                          />
                        )}
                      </div>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            );
          }
          default:
            return null;
        }
      },
      [
        fileStates,
        form,
        getFileListMaxCount,
        isFileUploadLoginRequired,
        isSameFileFieldValue,
        isSameUploadedFiles,
        allowWorkspaceSwitch,
        maxSizeMB,
        t,
        tUi,
        onChange,
        normalizeNumberInputValue,
      ]
    );

    return (
      <div className="relative">
        {topNotice}
        <Form {...form}>
          <form
            className="space-y-3 pb-4"
            onBlur={emitValuesChange}
            onSubmit={form.handleSubmit(handleSubmit)}
          >
            {startVariables.map(renderField)}
            {(!hideSubmitButton || showResetButton) && (
              <div className="flex items-center gap-2 pt-2">
                {!hideSubmitButton && (
                  <button
                    className="inline-flex items-center justify-center h-9 px-3 rounded-md text-sm bg-primary text-primary-foreground disabled:opacity-50"
                    type="submit"
                    disabled={isStarting}
                  >
                    {isStarting ? t('workflow.starting') : t('workflow.runNow')}
                  </button>
                )}
                {showResetButton && (
                  <button
                    type="button"
                    className="inline-flex items-center justify-center h-9 px-3 rounded-md text-sm bg-muted text-muted-foreground hover:bg-muted/80"
                    onClick={handleReset}
                  >
                    {t('workflow.restoreDefaults')}
                  </button>
                )}
              </div>
            )}
          </form>
        </Form>
        <ConfirmDialog
          open={loginDialogOpen}
          onOpenChange={setLoginDialogOpen}
          title={tUi('fileUpload.loginRequiredTitle')}
          description={tUi('fileUpload.loginRequiredForUploadDescription')}
          confirmText={tUi('fileUpload.goToLogin')}
          cancelText={tUi('fileUpload.cancelAction')}
          onConfirm={handleLoginConfirm}
        />
      </div>
    );
  }
);

export default WorkflowInputForm;
WorkflowInputForm.displayName = 'WorkflowInputForm';
