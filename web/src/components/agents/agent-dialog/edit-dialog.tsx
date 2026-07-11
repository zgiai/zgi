'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import * as z from 'zod';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
import { Pencil } from 'lucide-react';
import { IconInput } from '@/components/common/icon-input';
import { type WorkspaceSelectorValue } from '@/components/common/workspace-selector';
import {
  createTextIconValue,
  createImageIconValue,
  type IconValue,
} from '@/components/common/icon-input/types';
import type { IconType } from '@/utils/icon-helpers';
import { getNameValidationErrors } from '@/utils/validation';
import { cn } from '@/lib/utils';
import { type UpdateAgentRequest, type AgentDetail } from '@/services/types/agent';
import { useAgent, useUpdateAgent } from '@/hooks/agent/use-agents';
import { useT } from '@/i18n';
import { ICON_BG, ICON_TEXT } from '@/lib/config';

interface EditAgentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agentId: string;
}

export function EditAgentDialog({ open, onOpenChange, agentId }: EditAgentDialogProps) {
  const t = useT('agents');
  const tCommon = useT('common');
  const portalHostRef = useRef<HTMLDivElement | null>(null);

  const updateMutation = useUpdateAgent();

  const [iconValue, setIconValue] = useState<IconValue>(createTextIconValue('', ICON_BG));
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();

  const { agent: agentDetailResp } = useAgent(agentId || null, open);

  const baseSchema = useMemo(
    () =>
      z.object({
        name: z.string().superRefine((val, ctx) => {
          const errs = getNameValidationErrors(val, { allowSpace: true });
          if (errs.length) {
            const code = errs[0];
            const msg =
              code === 'required'
                ? t('validation.name.required')
                : code === 'tooShort'
                  ? t('validation.name.tooShort')
                  : code === 'tooLong'
                    ? t('validation.name.tooLong')
                    : code === 'invalidChars'
                      ? t('validation.name.invalidChars')
                      : t('validation.name.onlySpaces');
            ctx.addIssue({ code: z.ZodIssueCode.custom, message: msg });
          }
        }),
        description: z.string().optional(),
        icon: z.string().optional(),
        icon_type: z.enum(['text', 'image']),
        icon_background: z.string().optional(),
        workspace_id: z.string().optional(),
      }),
    [t]
  );

  type EditFormDataLocal = z.infer<typeof baseSchema>;

  const form = useForm<EditFormDataLocal>({
    resolver: zodResolver(baseSchema),
    mode: 'onChange',
    reValidateMode: 'onChange',
    shouldFocusError: true,
    defaultValues: {
      name: '',
      description: '',
      icon: '',
      icon_type: 'text',
    },
  });

  useEffect(() => {
    const detail = agentDetailResp?.data as AgentDetail | undefined;
    if (!detail) return;
    const workspace = detail.workspace ?? detail.tenant;
    const workspaceId = workspace?.id || detail.workspace_id || detail.tenant_id;
    form.reset({
      name: detail.name || '',
      description: detail.description || '',
      icon: detail.icon || '',
      icon_type: (detail.icon_type as IconType) || 'text',
      workspace_id: workspaceId,
    });
    setSelectedWorkspace(workspaceId ? { id: workspaceId, name: workspace?.name ?? '' } : undefined);

    if (detail.icon_type === 'text') {
      try {
        const parsed = JSON.parse(detail.icon || '{}') as {
          icon?: string;
          icon_background?: string;
        };
        setIconValue(
          createTextIconValue(
            parsed.icon || detail.name?.slice(0, 2).toUpperCase() || ICON_TEXT,
            parsed.icon_background || ICON_BG
          )
        );
      } catch {
        setIconValue(
          createTextIconValue(detail.name?.slice(0, 2).toUpperCase() || ICON_TEXT, ICON_BG)
        );
      }
    } else if (detail.icon_type === 'image') {
      setIconValue(createImageIconValue(detail.icon_url || '', detail.icon || ''));
    }
  }, [agentDetailResp?.data, form]);

  const watchedName = form.watch('name');
  useEffect(() => {
    if (watchedName && iconValue.type === 'text') {
      setIconValue(
        createTextIconValue(watchedName.slice(0, 2).toUpperCase() || ICON_TEXT, ICON_BG)
      );
    }
  }, [watchedName, iconValue.type]);

  const onSubmit = (data: EditFormDataLocal) => {
    const payload: UpdateAgentRequest = {
      name: data.name,
      description: data.description || '',
      icon:
        iconValue.type === 'text'
          ? JSON.stringify({
              icon: iconValue.icon || data.name.slice(0, 2).toUpperCase() || ICON_TEXT,
              icon_background: iconValue.iconBackground,
            })
          : iconValue.imageId || iconValue.iconUrl || '',
      icon_type: iconValue.type,
      workspace_id: selectedWorkspace?.id,
    };

    updateMutation.mutate(
      { agentId, data: payload },
      {
        onSuccess: () => {
          onOpenChange(false);
        },
      }
    );
  };

  const handleClose = () => {
    form.reset();
    setIconValue(createTextIconValue('', ICON_BG));
    setSelectedWorkspace(undefined);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-lg"
        onInteractOutside={e => {
          const target = e.target as Element | null;
          if (
            target &&
            (target.closest('[data-select-content]') ||
              target.closest('[data-workspace-selector-content]'))
          ) {
            e.preventDefault();
          }
        }}
      >
        <div ref={portalHostRef} data-portal-host className="contents" />
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-xl font-semibold">
            <Pencil className="h-5 w-5" />
            {t('editAgent')}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)}>
            <DialogBody className="space-y-4 pb-4">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('form.name')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('form.namePlaceholder')}
                        aria-invalid={!!form.formState.errors.name}
                        className={cn(
                          form.formState.errors.name ? 'focus-visible:ring-destructive' : ''
                        )}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="description"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('form.description')}</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={t('form.descriptionPlaceholder')}
                        className="resize-none"
                        rows={3}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="icon"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('form.icon')}</FormLabel>
                    <FormControl>
                      <IconInput
                        value={
                          iconValue ||
                          createTextIconValue(
                            (form.watch('name') as string)?.slice(0, 2).toUpperCase() || ICON_TEXT,
                            ICON_BG
                          )
                        }
                        defaultValue={createTextIconValue(
                          (form.watch('name') as string)?.slice(0, 2).toUpperCase() || ICON_TEXT,
                          ICON_BG
                        )}
                        onChange={newIconValue => {
                          setIconValue(newIconValue);
                          if (newIconValue.type === 'text') {
                            field.onChange(newIconValue.icon);
                            form.setValue('icon_type', 'text');
                            form.setValue('icon_background', newIconValue.iconBackground);
                          } else {
                            field.onChange(newIconValue.imageId || newIconValue.iconUrl || '');
                            form.setValue('icon_type', 'image');
                          }
                        }}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={handleClose}>
                {t('form.cancel')}
              </Button>
              <Button type="submit" disabled={updateMutation.isPending}>
                {updateMutation.isPending ? t('workflow.saving') : tCommon('save')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

export default EditAgentDialog;
