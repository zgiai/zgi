'use client';

import { useEffect, useMemo, useState } from 'react';
import { Copy, ExternalLink, KeyRound, Loader2, Pencil, Plus, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
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
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import {
  useDigitalHumanAccess,
  useDigitalHumanAccessMutations,
  useDigitalHumanAPIKeys,
} from '@/hooks/digital-human/use-digital-human-access';
import { useT } from '@/i18n';
import type {
  DigitalHumanAPIKey,
  DigitalHumanPresentationPolicy,
} from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

interface DigitalHumanAccessPanelProps {
  id: string;
  webAppId: string;
  published: boolean;
  canPublish: boolean;
  canUpdatePresentation: boolean;
  presentation: DigitalHumanPresentationPolicy;
  onPresentationChange: (presentation: DigitalHumanPresentationPolicy) => void;
}

export function DigitalHumanAccessPanel({
  id,
  webAppId,
  published,
  canPublish,
  canUpdatePresentation,
  presentation,
  onPresentationChange,
}: DigitalHumanAccessPanelProps) {
  const t = useT('digitalHumans.access');
  const access = useDigitalHumanAccess(id, canPublish);
  const keys = useDigitalHumanAPIKeys(id, canPublish && published);
  const mutations = useDigitalHumanAccessMutations(id);
  const [keyDialogOpen, setKeyDialogOpen] = useState(false);
  const [editingKey, setEditingKey] = useState<DigitalHumanAPIKey | null>(null);
  const [revealedKey, setRevealedKey] = useState('');
  const landingURL = useMemo(
    () =>
      typeof window === 'undefined'
        ? `/digital-human/${webAppId}`
        : `${window.location.origin}/digital-human/${webAppId}`,
    [webAppId]
  );
  const settings = access.data?.data;

  const copy = async (value: string, successMessage: string) => {
    await navigator.clipboard.writeText(value);
    toast.success(successMessage);
  };

  const updateSurface = async (field: 'webapp_enabled' | 'api_enabled', enabled: boolean) => {
    try {
      await mutations.updateAccess.mutateAsync({ [field]: enabled });
      toast.success(t('surfaceUpdated'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('surfaceUpdateFailed'));
    }
  };

  const updatePresentation = <K extends keyof DigitalHumanPresentationPolicy>(
    field: K,
    value: DigitalHumanPresentationPolicy[K]
  ) => onPresentationChange({ ...presentation, [field]: value });

  return (
    <div className="space-y-6 p-4 md:p-5">
      <section className="space-y-3">
        <div>
          <h3 className="text-sm font-semibold">{t('surfacesTitle')}</h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('surfacesDescription')}</p>
        </div>
        {!published ? (
          <div className="rounded-lg border border-warning/30 bg-warning/5 px-3 py-2 text-xs text-warning-foreground">
            {t('publishFirst')}
          </div>
        ) : null}
        <div className="divide-y rounded-lg border px-4">
          <AccessSwitch
            title={t('webAppTitle')}
            description={t('webAppDescription')}
            checked={settings?.webapp_enabled ?? false}
            disabled={
              !canPublish || !published || access.isLoading || mutations.updateAccess.isPending
            }
            onCheckedChange={enabled => void updateSurface('webapp_enabled', enabled)}
          />
          <AccessSwitch
            title={t('apiTitle')}
            description={t('apiDescription')}
            checked={settings?.api_enabled ?? false}
            disabled={
              !canPublish || !published || access.isLoading || mutations.updateAccess.isPending
            }
            onCheckedChange={enabled => void updateSurface('api_enabled', enabled)}
          />
        </div>
        <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/20 p-3">
          <Input
            value={landingURL}
            readOnly
            className="min-w-0 flex-1"
            aria-label={t('landingURL')}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void copy(landingURL, t('linkCopied'))}
          >
            <Copy className="mr-2 size-4" />
            {t('copy')}
          </Button>
          <Button type="button" variant="outline" size="sm" asChild>
            <a href={landingURL} target="_blank" rel="noreferrer">
              <ExternalLink className="mr-2 size-4" />
              {t('open')}
            </a>
          </Button>
        </div>
      </section>

      <section className="space-y-4 border-t pt-6">
        <div>
          <h3 className="text-sm font-semibold">{t('presentationTitle')}</h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('presentationDescription')}
          </p>
        </div>
        <div className="grid gap-4">
          <AccessField label={t('homeTitle')} description={t('homeTitleDescription')}>
            <Input
              value={presentation.home_title}
              onChange={event => updatePresentation('home_title', event.target.value)}
              disabled={!canUpdatePresentation}
              maxLength={120}
            />
          </AccessField>
          <AccessField label={t('openingStatement')} description={t('openingStatementDescription')}>
            <Textarea
              rows={3}
              value={presentation.opening_statement}
              onChange={event => updatePresentation('opening_statement', event.target.value)}
              disabled={!canUpdatePresentation}
              maxLength={1000}
            />
          </AccessField>
          <AccessField label={t('inputPlaceholder')} description={t('inputPlaceholderDescription')}>
            <Input
              value={presentation.input_placeholder}
              onChange={event => updatePresentation('input_placeholder', event.target.value)}
              disabled={!canUpdatePresentation}
              maxLength={120}
            />
          </AccessField>
          <AccessField label={t('themeColor')} description={t('themeColorDescription')}>
            <div className="flex items-center gap-2">
              <input
                type="color"
                value={
                  /^#[0-9a-f]{6}$/i.test(presentation.theme_color)
                    ? presentation.theme_color
                    : '#6366f1'
                }
                onChange={event => updatePresentation('theme_color', event.target.value)}
                disabled={!canUpdatePresentation}
                className="size-9 cursor-pointer rounded-md border bg-background p-1 disabled:cursor-not-allowed"
                aria-label={t('themeColor')}
              />
              <Input
                value={presentation.theme_color}
                onChange={event => updatePresentation('theme_color', event.target.value)}
                disabled={!canUpdatePresentation}
                placeholder="#6366f1"
                maxLength={32}
              />
            </div>
          </AccessField>
          <AccessField
            label={t('suggestedQuestions')}
            description={t('suggestedQuestionsDescription')}
          >
            <div className="space-y-2">
              {presentation.suggested_questions.map((question, index) => (
                <div key={index} className="flex items-center gap-2">
                  <Input
                    value={question}
                    onChange={event => {
                      const next = [...presentation.suggested_questions];
                      next[index] = event.target.value;
                      updatePresentation('suggested_questions', next);
                    }}
                    disabled={!canUpdatePresentation}
                    maxLength={200}
                    aria-label={t('suggestionNumber', { number: index + 1 })}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    isIcon
                    aria-label={t('removeSuggestion')}
                    disabled={!canUpdatePresentation}
                    onClick={() =>
                      updatePresentation(
                        'suggested_questions',
                        presentation.suggested_questions.filter(
                          (_, itemIndex) => itemIndex !== index
                        )
                      )
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              ))}
              {presentation.suggested_questions.length < 6 ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!canUpdatePresentation}
                  onClick={() =>
                    updatePresentation('suggested_questions', [
                      ...presentation.suggested_questions,
                      '',
                    ])
                  }
                >
                  <Plus className="mr-2 size-4" />
                  {t('addSuggestion')}
                </Button>
              ) : null}
            </div>
          </AccessField>
        </div>
      </section>

      <section className="space-y-4 border-t pt-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">{t('keysTitle')}</h3>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('keysDescription')}</p>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" asChild>
              <a href="/api/openapi.yaml" target="_blank" rel="noreferrer">
                {t('openAPIDocument')}
              </a>
            </Button>
            <Button
              size="sm"
              disabled={!canPublish || !published}
              onClick={() => {
                setEditingKey(null);
                setKeyDialogOpen(true);
              }}
            >
              <Plus className="mr-2 size-4" />
              {t('createKey')}
            </Button>
          </div>
        </div>
        {keys.isLoading ? (
          <div className="flex items-center justify-center rounded-lg border py-8 text-muted-foreground">
            <Loader2 className="mr-2 size-4 animate-spin" />
            {t('loadingKeys')}
          </div>
        ) : (keys.data?.data.api_keys.length ?? 0) === 0 ? (
          <div className="rounded-lg border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
            <KeyRound className="mx-auto mb-2 size-6" />
            {t('noKeys')}
          </div>
        ) : (
          <div className="divide-y overflow-hidden rounded-lg border">
            {keys.data?.data.api_keys.map(key => (
              <div key={key.id} className="flex flex-wrap items-center gap-3 p-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">{key.name}</span>
                    <Badge variant={key.status === 'active' ? 'success' : 'secondary'}>
                      {key.status === 'active' ? t('active') : t('revoked')}
                    </Badge>
                  </div>
                  <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 font-mono text-xs text-muted-foreground">
                    <span>{key.key_prefix}…</span>
                    <span>
                      {key.expires_at
                        ? t('expiresAt', { date: new Date(key.expires_at).toLocaleString() })
                        : t('neverExpires')}
                    </span>
                  </div>
                </div>
                {key.status === 'active' ? (
                  <>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setEditingKey(key);
                        setKeyDialogOpen(true);
                      }}
                    >
                      <Pencil className="mr-2 size-3.5" />
                      {t('editKey')}
                    </Button>
                    <ConfirmDialog
                      title={t('revokeTitle')}
                      description={t('revokeDescription', { name: key.name })}
                      confirmText={t('revoke')}
                      cancelText={t('cancel')}
                      variant="warning"
                      onConfirm={() =>
                        void mutations.updateAPIKey
                          .mutateAsync({ keyId: key.id, request: { status: 'revoked' } })
                          .then(() => toast.success(t('keyRevoked')))
                          .catch(error =>
                            toast.error(getErrorMessage(error) || t('keyUpdateFailed'))
                          )
                      }
                      trigger={
                        <Button type="button" variant="outline" size="sm">
                          {t('revoke')}
                        </Button>
                      }
                    />
                  </>
                ) : null}
                <ConfirmDialog
                  title={t('deleteTitle')}
                  description={t('deleteDescription', { name: key.name })}
                  confirmText={t('delete')}
                  cancelText={t('cancel')}
                  variant="danger"
                  onConfirm={() =>
                    void mutations.deleteAPIKey
                      .mutateAsync(key.id)
                      .then(() => toast.success(t('keyDeleted')))
                      .catch(error => toast.error(getErrorMessage(error) || t('keyDeleteFailed')))
                  }
                  trigger={
                    <Button type="button" variant="ghost" size="sm" isIcon aria-label={t('delete')}>
                      <Trash2 className="size-4" />
                    </Button>
                  }
                />
              </div>
            ))}
          </div>
        )}
      </section>

      <APIKeyEditorDialog
        open={keyDialogOpen}
        onOpenChange={setKeyDialogOpen}
        apiKey={editingKey}
        pending={mutations.createAPIKey.isPending || mutations.updateAPIKey.isPending}
        onSubmit={async values => {
          try {
            if (editingKey) {
              await mutations.updateAPIKey.mutateAsync({ keyId: editingKey.id, request: values });
              toast.success(t('keyUpdated'));
            } else {
              const response = await mutations.createAPIKey.mutateAsync(values);
              setRevealedKey(response.data.api_key);
            }
            setKeyDialogOpen(false);
          } catch (error) {
            toast.error(getErrorMessage(error) || t('keyUpdateFailed'));
          }
        }}
      />
      <Dialog open={Boolean(revealedKey)} onOpenChange={open => !open && setRevealedKey('')}>
        <DialogContent size="md">
          <DialogHeader>
            <DialogTitle>{t('keyCreatedTitle')}</DialogTitle>
            <DialogDescription>{t('keyCreatedDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <div className="flex gap-2">
              <Input value={revealedKey} readOnly className="font-mono" />
              <Button
                type="button"
                variant="outline"
                onClick={() => void copy(revealedKey, t('keyCopied'))}
              >
                <Copy className="size-4" />
              </Button>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button onClick={() => setRevealedKey('')}>{t('done')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function AccessSwitch({
  title,
  description,
  checked,
  disabled,
  onCheckedChange,
}: {
  title: string;
  description: string;
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-start justify-between gap-5 py-4">
      <div>
        <div className="text-sm font-medium">{title}</div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
    </div>
  );
}

function AccessField({
  label,
  description,
  children,
}: {
  label: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <label className="grid gap-2 md:grid-cols-[180px_minmax(0,1fr)] md:items-start">
      <span>
        <span className="block text-sm font-medium">{label}</span>
        <span className="mt-1 block text-xs leading-5 text-muted-foreground">{description}</span>
      </span>
      {children}
    </label>
  );
}

function APIKeyEditorDialog({
  open,
  onOpenChange,
  apiKey,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  apiKey: DigitalHumanAPIKey | null;
  pending: boolean;
  onSubmit: (values: { name: string; expires_at?: string }) => Promise<void>;
}) {
  const t = useT('digitalHumans.access');
  const [name, setName] = useState('');
  const [expiresAt, setExpiresAt] = useState('');
  useEffect(() => {
    if (!open) return;
    setName(apiKey?.name ?? '');
    setExpiresAt(apiKey?.expires_at ? toLocalDateTime(apiKey.expires_at) : '');
  }, [apiKey, open]);
  const submit = () => {
    const normalized = name.trim();
    if (!normalized) return;
    void onSubmit({
      name: normalized,
      ...(expiresAt ? { expires_at: new Date(expiresAt).toISOString() } : {}),
    });
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{apiKey ? t('editKeyTitle') : t('createKeyTitle')}</DialogTitle>
          <DialogDescription>
            {apiKey ? t('editKeyDescription') : t('createKeyDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <AccessField label={t('keyName')} description={t('keyNameDescription')}>
            <Input value={name} onChange={event => setName(event.target.value)} maxLength={255} />
          </AccessField>
          <AccessField label={t('keyExpiry')} description={t('keyExpiryDescription')}>
            <Input
              type="datetime-local"
              value={expiresAt}
              min={toLocalDateTime(new Date().toISOString())}
              onChange={event => setExpiresAt(event.target.value)}
            />
          </AccessField>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('cancel')}
          </Button>
          <Button disabled={!name.trim() || pending} loading={pending} onClick={submit}>
            {apiKey ? t('save') : t('create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function toLocalDateTime(value: string): string {
  const date = new Date(value);
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}
