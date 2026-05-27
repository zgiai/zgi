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
import { RadioCardGroup, RadioCard } from '@/components/ui/radio-card';

import { useT } from '@/i18n';
import { useRouter } from 'next/navigation';
import { IconInput } from '@/components/common/icon-input';
import { createTextIconValue, type IconValue } from '@/components/common/icon-input/types';
import { getNameValidationErrors } from '@/utils/validation';
import { cn } from '@/lib/utils';
import {
  AgentType,
  type CreateAgentRequest,
  type AgentCreateResponse,
} from '@/services/types/agent';
import type { ApiResponseData } from '@/services/types/common';
import { useCreateAgent } from '@/hooks/agent/use-agents';
import { Bot, MessageSquareQuote, Workflow } from 'lucide-react';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useCurrentWorkspace, useIsOrganizationMode } from '@/store/workspace-store';
import { ICON_BG, ICON_TEXT } from '@/lib/config';

interface CreateAgentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateAgentDialog({ open, onOpenChange }: CreateAgentDialogProps) {
  const t = useT('agents');
  const router = useRouter();
  const portalHostRef = useRef<HTMLDivElement | null>(null);

  const createMutation = useCreateAgent();
  const currentWorkspaceFromStore = useCurrentWorkspace();
  const isOrganizationMode = useIsOrganizationMode();

  const [iconValue, setIconValue] = useState<IconValue>(createTextIconValue('', ICON_BG));
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();

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

  const createSchema = useMemo(
    () => baseSchema.extend({ agent_type: z.nativeEnum(AgentType) }),
    [baseSchema]
  );

  type CreateFormDataLocal = z.infer<typeof createSchema>;

  const form = useForm<CreateFormDataLocal>({
    resolver: zodResolver(createSchema),
    mode: 'onChange',
    reValidateMode: 'onChange',
    shouldFocusError: true,
    defaultValues: {
      name: '',
      description: '',
      icon: '',
      icon_type: 'text',
      agent_type: AgentType.AGENT,
    },
  });

  const watchedName = form.watch('name');
  useEffect(() => {
    if (watchedName && iconValue.type === 'text') {
      setIconValue(
        createTextIconValue(watchedName.slice(0, 2).toUpperCase() || ICON_TEXT, ICON_BG)
      );
    }
  }, [watchedName, iconValue.type]);

  const resetFormState = () => {
    form.reset();
    setIconValue(createTextIconValue('', ICON_BG));
    setSelectedWorkspace(undefined);
  };

  const onSubmit = (data: CreateFormDataLocal) => {
    const workspaceId = isOrganizationMode ? selectedWorkspace?.id : currentWorkspaceFromStore?.id;

    if (!workspaceId) {
      form.setError('workspace_id', {
        type: 'manual',
        message: t('validation.workspace.required'),
      });
      return;
    }

    const payload: CreateAgentRequest = {
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
      agent_type: data.agent_type,
      workspace_id: workspaceId,
    };

    createMutation.mutate(payload, {
      onSuccess: (res: ApiResponseData<AgentCreateResponse>) => {
        const newId = res.data?.id;
        if (newId) {
          router.push(
            data.agent_type === AgentType.AGENT
              ? `/console/agents/${newId}/agent`
              : `/console/agents/${newId}/workflow`
          );
        }
        resetFormState();
        onOpenChange(false);
      },
    });
  };

  const handleClose = () => {
    resetFormState();
    onOpenChange(false);
  };

  const handleDialogOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      resetFormState();
    }
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent
        className="max-w-2xl"
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
          <DialogTitle>{t('create')}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)}>
            <DialogBody className="space-y-4">
              <div className="flex gap-10">
                <div className="space-y-2 w-full max-w-48">
                  <FormField
                    control={form.control}
                    name="agent_type"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('form.mode')}</FormLabel>
                        <FormControl>
                          <RadioCardGroup
                            value={field.value}
                            onValueChange={field.onChange}
                            className="gap-2"
                          >
                            <RadioCard
                              value={AgentType.AGENT}
                              title={t('modes.agent')}
                              description={t('modes.agentDesc')}
                              checked={field.value === AgentType.AGENT}
                              hiddenRadio
                              icon={<Bot className="w-6 h-6" />}
                            />
                            <RadioCard
                              value={AgentType.CONVERSATIONAL_AGENT}
                              title={t('modes.chatWorkflow')}
                              description={t('modes.chatWorkflowDesc')}
                              checked={field.value === AgentType.CONVERSATIONAL_AGENT}
                              hiddenRadio
                              icon={<MessageSquareQuote className="w-6 h-6" />}
                            />
                            <RadioCard
                              value={AgentType.WORKFLOW}
                              title={t('modes.taskWorkflow')}
                              description={t('modes.taskWorkflowDesc')}
                              checked={field.value === AgentType.WORKFLOW}
                              hiddenRadio
                              icon={<Workflow className="w-6 h-6" />}
                            />
                          </RadioCardGroup>
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <div className="space-y-4 flex-1">
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

                  {isOrganizationMode ? (
                    <FormField
                      control={form.control}
                      name="workspace_id"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('form.workspace')}</FormLabel>
                          <FormControl>
                            <WorkspaceSelector
                              value={selectedWorkspace}
                              placeholder={t('form.workspacePlaceholder')}
                              autoSelectFirst
                              onChange={workspace => {
                                setSelectedWorkspace(workspace);
                                field.onChange(workspace.id);
                                form.clearErrors('workspace_id');
                              }}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  ) : null}

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
                                (form.watch('name') as string)?.slice(0, 2).toUpperCase() ||
                                  ICON_TEXT,
                                ICON_BG
                              )
                            }
                            defaultValue={createTextIconValue(
                              (form.watch('name') as string)?.slice(0, 2).toUpperCase() ||
                                ICON_TEXT,
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
                </div>
              </div>
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={handleClose}>
                {t('form.cancel')}
              </Button>
              <Button type="submit" disabled={createMutation.isPending}>
                {createMutation.isPending ? t('form.creating') : t('form.create')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

export default CreateAgentDialog;
