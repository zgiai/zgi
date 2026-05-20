'use client';

import React, { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import * as z from 'zod';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
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
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import type { ProviderItem } from '@/services/types/provider';

const createSchema = (t: any) =>
  z.object({
    provider: z
      .string()
      .min(1, t('custom.dialog.validation.providerRequired'))
      .regex(/^[a-z0-9-]+$/, t('custom.dialog.validation.providerPattern')),
    provider_name: z.string().min(1, t('custom.dialog.validation.providerNameRequired')),
    api_base_url: z.string().url().optional().or(z.literal('')),
    logo_url: z.string().url().optional().or(z.literal('')),
    documentation_url: z.string().url().optional().or(z.literal('')),
    description: z.string().optional(),
    is_enabled: z.boolean().optional(),
  });

interface CustomProviderDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialData?: ProviderItem;
  onSubmit: (data: any) => Promise<void>;
  isSubmitting?: boolean;
}

export function CustomProviderDialog({
  open,
  onOpenChange,
  initialData,
  onSubmit,
  isSubmitting,
}: CustomProviderDialogProps) {
  const t = useT('aiProviders');
  const formSchema = createSchema(t);

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      provider: '',
      provider_name: '',
      api_base_url: '',
      logo_url: '',
      documentation_url: '',
      description: '',
      is_enabled: true,
    },
  });

  useEffect(() => {
    if (initialData) {
      form.reset({
        provider: initialData.provider,
        provider_name: initialData.provider_name,
        api_base_url: initialData.api_base_url || '',
        logo_url: initialData.logo_url || '',
        documentation_url: initialData.api_docs_url || '',
        description: initialData.description || '',
        is_enabled: initialData.is_enabled,
      });
    } else {
      form.reset({
        provider: '',
        provider_name: '',
        api_base_url: '',
        logo_url: '',
        documentation_url: '',
        description: '',
        is_enabled: true,
      });
    }
  }, [initialData, form, open]);

  const handleSubmit = async (values: z.infer<typeof formSchema>) => {
    await onSubmit(values);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>
            {initialData ? t('custom.dialog.editTitle') : t('custom.dialog.createTitle')}
          </DialogTitle>
          <DialogDescription>
            {initialData
              ? t('custom.dialog.fields.descriptionPlaceholder')
              : t('custom.dialog.fields.descriptionPlaceholder')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="provider"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('custom.dialog.fields.provider')} <span className="text-red-500">*</span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('custom.dialog.fields.providerPlaceholder')}
                        {...field}
                        disabled={!!initialData}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="provider_name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('custom.dialog.fields.providerName')}{' '}
                      <span className="text-red-500">*</span>
                    </FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('custom.dialog.fields.providerNamePlaceholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="api_base_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('custom.dialog.fields.apiBaseUrl')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('custom.dialog.fields.apiBaseUrlPlaceholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="logo_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('custom.dialog.fields.logoUrl')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('custom.dialog.fields.logoUrlPlaceholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="documentation_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('custom.dialog.fields.documentationUrl')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('custom.dialog.fields.documentationUrlPlaceholder')}
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
                    <FormLabel>{t('custom.dialog.fields.description')}</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={t('custom.dialog.fields.descriptionPlaceholder')}
                        className="resize-none"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <DialogFooter className="pt-4 px-0">
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                  {t('cancel') || 'Cancel'}
                </Button>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? t('actions.saving') || 'Saving...' : t('save') || 'Save'}
                </Button>
              </DialogFooter>
            </form>
          </Form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
