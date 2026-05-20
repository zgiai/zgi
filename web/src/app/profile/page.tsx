'use client';

import React from 'react';
import { useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import { ProtectedRoute } from '@/components/auth/protected-route';
import { useAuthStore } from '@/store/auth-store';
import { useProfile, useUpdateProfile } from '@/hooks/use-profile';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Icons } from '@/components/ui/icons';
import { SafeAvatar } from '@/components/ui/avatar';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useForm } from 'react-hook-form';
import { TimezoneSelector } from '@/components/common/timezone-selector';
import { ImageCropper } from '@/components/common/image-cropper';
import { Skeleton } from '@/components/ui/skeleton';
import type { User } from '@/services/types/auth';
import { toast } from 'sonner';
import type { TimezoneValue } from '@/lib/constants';

interface ProfileFormValues {
  name: string;
  timezone: string;
}

export default function ProfilePage() {
  const router = useRouter();
  const t = useT('profile');

  const user = useAuthStore.use.user();
  const { data: profile, isLoading, isFetching } = useProfile();
  const updateMutation = useUpdateProfile();

  const form = useForm<ProfileFormValues>({
    defaultValues: {
      name: user?.name || '',
      timezone: user?.timezone || 'Asia/Shanghai',
    },
    values: profile
      ? {
          name: profile.name || '',
          timezone: profile.timezone || 'Asia/Shanghai',
        }
      : undefined,
  });

  const [cropperOpen, setCropperOpen] = React.useState(false);
  const [avatarPreview, setAvatarPreview] = React.useState<string | null>(user?.avatar_url || null);
  const [avatarFileId, setAvatarFileId] = React.useState<string | null>(null);
  const [avatarRemoved, setAvatarRemoved] = React.useState<boolean>(false);
  const [avatarError, setAvatarError] = React.useState<string | null>(null);

  const handleCropComplete = (previewUrl: string, imageId?: string) => {
    setAvatarError(null);
    setAvatarPreview(previewUrl);
    setAvatarFileId(imageId ?? null);
    setAvatarRemoved(false);
  };

  const onSubmit = (values: ProfileFormValues) => {
    const current: User | null = useAuthStore.getState().user;
    const payload: { name?: string | null; timezone?: string | null; avatar?: string | null } = {};

    if (current) {
      if (values.name.trim() !== (current.name || '')) {
        payload.name = values.name.trim();
      }
      if (values.timezone !== (current.timezone || '')) {
        payload.timezone = values.timezone;
      }
      if (avatarRemoved) {
        payload.avatar = '';
      } else if (avatarFileId !== null) {
        payload.avatar = avatarFileId;
      }
    } else {
      if (values.name.trim().length > 0) {
        payload.name = values.name.trim();
      }
      if (values.timezone) {
        payload.timezone = values.timezone;
      }
      if (avatarRemoved) {
        payload.avatar = '';
      } else if (avatarFileId) {
        payload.avatar = avatarFileId;
      }
    }

    // Do nothing if no changes
    if (Object.keys(payload).length === 0) {
      toast(t('noChanges'));
      return;
    }
    updateMutation.mutate(payload);
  };

  return (
    <ProtectedRoute>
      <div className="min-h-screen bg-gradient-to-b from-background to-muted/50">
        <div className="max-w-4xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
          {/* Header */}
          <div className="mb-8">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => router.back()}
              className="mb-4 hover:bg-muted/50 transition-colors"
            >
              <Icons.ChevronLeft className="h-4 w-4 mr-1" />
              {t('backButton')}
            </Button>
            <h1 className="text-3xl font-semibold tracking-tight text-foreground">{t('title')}</h1>
            <p className="text-sm text-muted-foreground mt-1">{t('personalInfoDesc')}</p>
          </div>

          {/* Main Content Card */}
          <div className="bg-background rounded-xl border border-border shadow-sm overflow-hidden">
            {(isLoading && !profile) || (isFetching && !profile) ? (
              <div className="p-8 space-y-8">
                <div className="space-y-6">
                  <div className="flex items-start gap-6">
                    <Skeleton className="h-24 w-24 rounded-full flex-shrink-0" />
                    <div className="flex-1 space-y-3">
                      <Skeleton className="h-6 w-48" />
                      <Skeleton className="h-4 w-64" />
                    </div>
                  </div>
                </div>
                <div className="space-y-6 pt-6 border-t border-border/50">
                  <div className="space-y-2">
                    <Skeleton className="h-4 w-20" />
                    <Skeleton className="h-10 w-full" />
                  </div>
                  <div className="space-y-2">
                    <Skeleton className="h-4 w-20" />
                    <Skeleton className="h-10 w-full" />
                  </div>
                </div>
              </div>
            ) : (
              <Form {...form}>
                <form onSubmit={form.handleSubmit(onSubmit)}>
                  {/* Avatar Section */}
                  <div className="p-8 bg-muted/30">
                    <div className="flex items-start gap-6">
                      <div
                        className="relative group cursor-pointer flex-shrink-0"
                        onClick={() => setCropperOpen(true)}
                        role="button"
                        aria-label={t('changeAvatar')}
                      >
                        <SafeAvatar
                          src={avatarPreview || profile?.avatar_url || user?.avatar_url || null}
                          alt={profile?.name || user?.name || null}
                          fallback={profile?.name || user?.name || null}
                          className="h-24 w-24 text-2xl border-4 border-background shadow-sm"
                        />
                        <div className="absolute inset-0 rounded-full bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity duration-200 flex items-center justify-center">
                          <div className="text-center">
                            <Icons.Upload className="h-5 w-5 text-white mx-auto mb-1" />
                            <span className="text-white text-xs font-medium">
                              {t('changeAvatar')}
                            </span>
                          </div>
                        </div>
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="text-xl font-semibold text-foreground truncate">
                          {profile?.name || user?.name || '—'}
                        </div>
                        <div className="text-sm text-muted-foreground truncate mt-1">
                          {profile?.email || user?.email || '—'}
                        </div>
                        <div className="mt-4">
                          {avatarPreview && (
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={() => {
                                setAvatarFileId(null);
                                setAvatarPreview(null);
                                setAvatarRemoved(true);
                              }}
                              className="h-8 text-xs hover:bg-destructive/10 hover:text-destructive hover:border-destructive/50 transition-colors"
                            >
                              <Icons.Trash className="h-3.5 w-3.5 mr-1.5" />
                              {t('removeAvatar')}
                            </Button>
                          )}
                          {avatarError && (
                            <p className="text-sm text-destructive mt-2">{avatarError}</p>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* Form Fields */}
                  <div className="p-8 space-y-6">
                    <div className="space-y-1.5 mb-6">
                      <h2 className="text-lg font-medium text-foreground">{t('accountDetails')}</h2>
                      <p className="text-sm text-muted-foreground">{t('personalInfo')}</p>
                    </div>

                    <FormField
                      control={form.control}
                      name="name"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel className="text-sm font-medium text-foreground">
                            {t('username')}
                          </FormLabel>
                          <FormControl>
                            <Input
                              placeholder={t('usernamePlaceholder')}
                              className="h-10 bg-background border-border/50 focus:border-primary transition-colors"
                              {...field}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name="timezone"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel className="text-sm font-medium text-foreground">
                            {t('timezone')}
                          </FormLabel>
                          <FormControl>
                            <TimezoneSelector
                              value={field.value as TimezoneValue}
                              onChange={field.onChange}
                              placeholder={t('selectTimezone')}
                              triggerClassName="h-10 bg-background border-border/50 focus:border-primary transition-colors"
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  {/* Actions Footer */}
                  <div className="px-8 py-4 bg-muted/30 border-t border-border/50 flex justify-end gap-3">
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => router.back()}
                      className="h-10 px-6 hover:bg-muted transition-colors"
                    >
                      {t('cancel')}
                    </Button>
                    <Button
                      type="submit"
                      disabled={updateMutation.isPending}
                      className="h-10 px-6 bg-primary hover:bg-primary/90 transition-colors shadow-sm"
                    >
                      {updateMutation.isPending ? (
                        <span className="inline-flex items-center">
                          <Icons.Loader className="h-4 w-4 mr-2 animate-spin" />
                          {t('saving')}
                        </span>
                      ) : (
                        t('saveChanges')
                      )}
                    </Button>
                  </div>
                </form>
              </Form>
            )}
          </div>
        </div>

        <ImageCropper
          isOpen={cropperOpen}
          onOpenChange={setCropperOpen}
          onCropComplete={handleCropComplete}
          mode="upload"
          title={t('cropAvatar')}
          aspect={1}
        />
      </div>
    </ProtectedRoute>
  );
}
