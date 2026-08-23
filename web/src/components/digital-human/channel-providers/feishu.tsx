'use client';

import { useEffect, useState, type ReactNode } from 'react';
import { CircleHelp, ExternalLink, KeyRound, Loader2, Pencil } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import type {
  ChannelCredentialDialogProps,
  ChannelCredentialDraft,
  ChannelCredentialFieldsProps,
} from '@/components/digital-human/channel-providers/types';
import { useDigitalHumanChannelMutations } from '@/hooks/digital-human/use-digital-human-channels';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';

const FEISHU_CREDENTIAL_KEYS = [
  'app_id',
  'app_secret',
  'verification_token',
  'encrypt_key',
] as const;

export function createEmptyFeishuCredentialDraft(): ChannelCredentialDraft {
  return Object.fromEntries(FEISHU_CREDENTIAL_KEYS.map(key => [key, '']));
}

export function serializeFeishuCredential(value: ChannelCredentialDraft): Record<string, string> {
  return Object.fromEntries(
    FEISHU_CREDENTIAL_KEYS.map(key => [key, value[key]?.trim() ?? '']).filter(([, item]) =>
      Boolean(item)
    )
  );
}

export function isFeishuCredentialComplete(value: ChannelCredentialDraft): boolean {
  return Boolean(
    value.app_id?.trim() && value.app_secret?.trim() && value.verification_token?.trim()
  );
}

function CredentialPrivacyNotice() {
  const t = useT('digitalHumans');
  return (
    <div className="flex gap-3 rounded-xl border border-blue-500/20 bg-blue-500/5 p-4 text-sm">
      <KeyRound className="mt-0.5 size-4 shrink-0 text-blue-600" />
      <div>
        <p className="font-medium">{t('channels.installations.credential.noticeTitle')}</p>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          {t('channels.installations.credential.noticeDescription')}
        </p>
      </div>
    </div>
  );
}

function CredentialLocationGuide() {
  const t = useT('digitalHumans');
  return (
    <div className="rounded-xl border bg-muted/25 p-4 text-sm">
      <div className="flex items-start gap-3">
        <CircleHelp className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <p className="font-medium">{t('channels.installations.credential.guide.title')}</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('channels.installations.credential.guide.description')}
          </p>
          <div className="mt-3 grid gap-2 sm:grid-cols-2">
            <div className="rounded-lg border bg-background px-3 py-2.5">
              <p className="text-xs font-medium">
                {t('channels.installations.credential.guide.appCredentials')}
              </p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('channels.installations.credential.guide.appCredentialsPath')}
              </p>
            </div>
            <div className="rounded-lg border bg-background px-3 py-2.5">
              <p className="text-xs font-medium">
                {t('channels.installations.credential.guide.eventSecurity')}
              </p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('channels.installations.credential.guide.eventSecurityPath')}
              </p>
            </div>
          </div>
          <Button variant="link" className="mt-2 h-auto p-0 text-xs" asChild>
            <a href="https://open.feishu.cn/app" target="_blank" rel="noreferrer">
              {t('channels.installations.credential.openFeishuConsole')}
              <ExternalLink className="size-3.5" />
            </a>
          </Button>
        </div>
      </div>
    </div>
  );
}

export function FeishuCredentialFields({
  value,
  onChange,
  editing = false,
}: ChannelCredentialFieldsProps) {
  const t = useT('digitalHumans');
  const update = (key: (typeof FEISHU_CREDENTIAL_KEYS)[number], nextValue: string) =>
    onChange({ ...value, [key]: nextValue });
  const keepExisting = editing
    ? t('channels.installations.credential.keepExistingPlaceholder')
    : undefined;
  return (
    <div className="space-y-4">
      <CredentialPrivacyNotice />
      <CredentialLocationGuide />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="App ID">
          <Input
            value={value.app_id ?? ''}
            placeholder={keepExisting ?? 'cli_…'}
            autoComplete="off"
            onChange={event => update('app_id', event.target.value)}
          />
        </Field>
        <Field label="App Secret">
          <Input
            type="password"
            value={value.app_secret ?? ''}
            placeholder={keepExisting}
            autoComplete="new-password"
            onChange={event => update('app_secret', event.target.value)}
          />
        </Field>
        <Field label="Verification Token">
          <Input
            type="password"
            value={value.verification_token ?? ''}
            placeholder={keepExisting}
            autoComplete="new-password"
            onChange={event => update('verification_token', event.target.value)}
          />
        </Field>
        <div className="sm:col-span-2">
          <Field label="Encrypt Key">
            <Input
              type="password"
              value={value.encrypt_key ?? ''}
              placeholder={
                keepExisting ?? t('channels.installations.credential.encryptKeyPlaceholder')
              }
              autoComplete="new-password"
              onChange={event => update('encrypt_key', event.target.value)}
            />
          </Field>
        </div>
      </div>
    </div>
  );
}

export function FeishuCredentialDialog({
  open,
  onOpenChange,
  digitalHumanID,
  installation,
}: ChannelCredentialDialogProps) {
  const t = useT('digitalHumans');
  const mutations = useDigitalHumanChannelMutations(digitalHumanID);
  const [value, setValue] = useState<ChannelCredentialDraft>(createEmptyFeishuCredentialDraft());
  const [error, setError] = useState('');
  useEffect(() => {
    if (!open) return;
    setValue(createEmptyFeishuCredentialDraft());
    setError('');
  }, [open]);
  const submit = async () => {
    const credentials = serializeFeishuCredential(value);
    if (Object.keys(credentials).length === 0) return;
    setError('');
    try {
      await mutations.updateCredential.mutateAsync({
        installationId: installation.id,
        expectedRevision: installation.revision,
        credentials,
      });
      toast.success(t('channels.messages.credentialUpdated'));
      onOpenChange(false);
    } catch (cause) {
      setError(getErrorMessage(cause) || t('channels.installations.credential.updateError'));
    }
  };
  const credentialFields = serializeFeishuCredential(value);
  const canSubmit =
    installation.credential_mode === 'embedded'
      ? Object.keys(credentialFields).length > 0
      : isFeishuCredentialComplete(value);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="p-0">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Pencil className="size-4" />
            {t('channels.installations.credential.editTitle')}
          </DialogTitle>
          <DialogDescription>
            {t('channels.installations.credential.editDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4 pb-5">
          <FeishuCredentialFields
            value={value}
            onChange={setValue}
            editing={installation.credential_mode === 'embedded'}
          />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </DialogBody>
        <DialogFooter className="border-t bg-muted/30">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={() => void submit()}
            disabled={!canSubmit || mutations.updateCredential.isPending}
          >
            {mutations.updateCredential.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : null}
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {children}
    </div>
  );
}
