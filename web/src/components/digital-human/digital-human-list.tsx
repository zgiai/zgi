'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { Bot, MessageSquare, Plus, Search } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
import { Textarea } from '@/components/ui/textarea';
import {
  useDigitalHumans,
  useDigitalHumanMutations,
} from '@/hooks/digital-human/use-digital-humans';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';

export function DigitalHumanList() {
  const t = useT('digitalHumans');
  const router = useRouter();
  const permissions = useAccountPermissions();
  const [search, setSearch] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const list = useDigitalHumans(true, search.trim());
  const items = list.data?.data?.items ?? [];
  const canCreate = permissions.hasPermission('digital_human.create');

  return (
    <div className="mx-auto w-full max-w-7xl space-y-6 px-4 py-6 md:px-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{t('title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('subtitle')}</p>
        </div>
        {canCreate ? (
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 size-4" />
            {t('list.create')}
          </Button>
        ) : null}
      </div>

      <div className="relative max-w-md">
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={search}
          onChange={event => setSearch(event.target.value)}
          placeholder={t('list.search')}
          className="pl-9"
        />
      </div>

      {list.isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {[0, 1, 2].map(item => (
            <div key={item} className="h-44 animate-pulse rounded-xl border bg-muted/30" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex min-h-72 flex-col items-center justify-center rounded-xl border border-dashed text-center">
          <Bot className="mb-3 size-9 text-muted-foreground" />
          <p className="max-w-md text-sm text-muted-foreground">{t('list.empty')}</p>
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {items.map(item => (
            <Card
              key={item.id}
              className="group transition-all hover:-translate-y-0.5 hover:shadow-md"
            >
              <CardHeader>
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 gap-3">
                    <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                      <Bot className="size-5" />
                    </div>
                    <div className="min-w-0">
                      <CardTitle className="truncate text-base">{item.name}</CardTitle>
                      <CardDescription className="mt-1 line-clamp-2 min-h-10">
                        {item.description || t('list.noDescription')}
                      </CardDescription>
                    </div>
                  </div>
                  <span className="rounded-full border px-2 py-0.5 text-xs text-muted-foreground">
                    {item.published_version_id ? t('list.published') : t('status.draft')}
                  </span>
                </div>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="text-xs text-muted-foreground">
                  {t('list.updatedAt', { date: new Date(item.updated_at).toLocaleDateString() })}
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    className="flex-1"
                    onClick={() => router.push(`/console/digital-humans/${item.id}`)}
                  >
                    {t('list.open')}
                  </Button>
                  {item.published_version_id ? (
                    <Button
                      className="flex-1"
                      onClick={() => router.push(`/console/digital-humans/${item.id}/chat`)}
                    >
                      <MessageSquare className="mr-2 size-4" />
                      {t('list.chat')}
                    </Button>
                  ) : null}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <CreateDigitalHumanDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

function CreateDigitalHumanDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT('digitalHumans');
  const router = useRouter();
  const mutations = useDigitalHumanMutations();
  const availableModels = useAvailableModels({ use_case: 'agent-runtime' });
  const [name, setName] = useState('');
  const [purpose, setPurpose] = useState('');
  // The available-models endpoint already applies the agent-runtime capability
  // filter. Its compact response intentionally omits the ModelItem `callable`
  // projection used by the full model catalog.
  const defaultModel = availableModels.models[0]?.model ?? '';

  const submit = async () => {
    if (!name.trim() || !purpose.trim() || !defaultModel) {
      toast.error(t('errors.required'));
      return;
    }
    const response = await mutations.create.mutateAsync({
      name: name.trim(),
      description: purpose.trim(),
      persona: purpose.trim(),
      rules: [],
      default_model: defaultModel,
      autonomy_level: 'supervised',
      visibility: 'workspace',
      memory_policy: { enabled: false },
      trigger_policy: { manual: true },
    });
    const id = response.data?.definition.id;
    onOpenChange(false);
    if (id) router.push(`/console/digital-humans/${id}`);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <Field label={t('fields.name')}>
            <Input
              value={name}
              onChange={event => setName(event.target.value)}
              placeholder={t('create.namePlaceholder')}
              autoFocus
            />
          </Field>
          <Field label={t('create.purpose')}>
            <Textarea
              value={purpose}
              onChange={event => setPurpose(event.target.value)}
              placeholder={t('create.purposePlaceholder')}
              rows={4}
            />
          </Field>
          <p className="text-xs text-muted-foreground">{t('create.followupHint')}</p>
          {!availableModels.isLoading && !defaultModel ? (
            <p className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
              {t('create.modelUnavailable')}
            </p>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={() => void submit()}
            disabled={
              !name.trim() ||
              !purpose.trim() ||
              !defaultModel ||
              availableModels.isLoading ||
              mutations.create.isPending
            }
          >
            {mutations.create.isPending ? t('create.submitting') : t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1.5 text-sm font-medium">
      <span>{label}</span>
      {children}
    </label>
  );
}
