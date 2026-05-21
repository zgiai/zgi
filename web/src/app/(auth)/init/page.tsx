'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import { useCreateSetupAdmin, useSetupStatus } from '@/hooks/use-setup';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input, PasswordInput } from '@/components/ui/input';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Info } from 'lucide-react';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useForm } from 'react-hook-form';

interface InitFormValues {
  email: string;
  name: string;
  password: string;
}

function validateInitPassword(password: string) {
  if (password.length < 8) return false;
  return /[A-Za-z]/.test(password) && /\d/.test(password);
}

export default function InitPage() {
  const router = useRouter();
  const t = useT();
  const { status, isInitialized, isLoading } = useSetupStatus();
  const createAdmin = useCreateSetupAdmin();

  // If system already initialized, redirect to login page
  useEffect(() => {
    if (isInitialized) {
      router.replace('/login');
    }
  }, [isInitialized, router]);

  const form = useForm<InitFormValues>({
    defaultValues: { email: '', name: '', password: '' },
    mode: 'onChange',
  });
  const passwordValue = form.watch('password');
  const hasMinLength = passwordValue.length >= 8;
  const hasLetterAndNumber = /[A-Za-z]/.test(passwordValue) && /\d/.test(passwordValue);

  const onSubmit = (values: InitFormValues) => {
    // Trigger admin creation; hook shows toasts on success/failure
    createAdmin.mutate(values, {
      onSuccess: result => {
        if (result?.data?.result === 'success') {
          // After successful initialization, navigate to login
          router.replace('/login');
        }
      },
    });
  };

  // Initial skeleton while checking setup status
  if (isLoading && !isInitialized) {
    return (
      <div className="flex items-center justify-center p-6">
        <div className="w-full max-w-md">
          <Skeleton className="h-10 w-40 mb-4" />
          <Skeleton className="h-24 w-full mb-4" />
          <Skeleton className="h-10 w-full mb-3" />
          <Skeleton className="h-10 w-full mb-3" />
          <Skeleton className="h-10 w-full" />
        </div>
      </div>
    );
  }

  // Only show form when not initialized
  if (status?.step === 'not_started') {
    return (
      <div className="w-full flex items-center justify-center p-6">
        <Card className="w-full">
          <CardHeader>
            <CardTitle className="text-center">{t('auth.initAdminTitle')}</CardTitle>
            <CardDescription className="text-center">
              {t('auth.initAdminDescription')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
                <FormField
                  control={form.control}
                  name="email"
                  rules={{ required: t('auth.emailRequired') }}
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('auth.email')}</FormLabel>
                      <FormControl>
                        <Input type="email" placeholder={t('auth.emailPlaceholder')} {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="name"
                  rules={{ required: t('auth.nameRequired') }}
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('auth.name')}</FormLabel>
                      <FormControl>
                        <Input placeholder={t('auth.enterName')} {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="password"
                  rules={{
                    required: t('auth.passwordRequired'),
                    validate: value =>
                      validateInitPassword(value) || t('auth.initPasswordRule'),
                  }}
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('auth.password')}</FormLabel>
                      <FormControl>
                        <PasswordInput placeholder={t('auth.passwordPlaceholder')} {...field} />
                      </FormControl>
                      <Alert className="border-border/70 bg-muted/30 text-foreground">
                        <Info className="h-4 w-4" />
                        <AlertDescription className="space-y-1 text-xs leading-5">
                          <div className="font-medium">{t('auth.passwordRequirements')}</div>
                          <ul className="space-y-1 text-muted-foreground">
                            <li className={hasMinLength ? 'text-foreground' : undefined}>
                              • {t('auth.initPasswordRuleMin')}
                            </li>
                            <li className={hasLetterAndNumber ? 'text-foreground' : undefined}>
                              • {t('auth.initPasswordRuleLettersNumbers')}
                            </li>
                          </ul>
                        </AlertDescription>
                      </Alert>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <Button type="submit" className="w-full" disabled={createAdmin.isPending}>
                  {createAdmin.isPending ? t('auth.initializing') : t('auth.initialize')}
                </Button>
              </form>
            </Form>
          </CardContent>
        </Card>
      </div>
    );
  }

  // If initialized (or unknown), rely on redirect effect
  return null;
}
