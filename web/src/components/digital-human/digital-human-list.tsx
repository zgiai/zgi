'use client';

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Bot, Clock3, MoreHorizontal, Pencil, Plus, Search } from 'lucide-react';
import { toast } from 'sonner';
import { DigitalHumanBasicInfoDialog } from '@/components/digital-human/digital-human-basic-info-dialog';
import { IconInput } from '@/components/common/icon-input';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { createTextIconValue, type IconValue } from '@/components/common/icon-input/types';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import type { DigitalHumanDefinition } from '@/services/types/digital-human';
import { getAgentTextIconDisplay } from '@/components/agents/agent-runtime/utils';

export function DigitalHumanList() {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const [search, setSearch] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const list = useDigitalHumans(true, search.trim());
  const items = list.data?.data?.items ?? [];
  const canCreate = permissions.hasPermission('digital_human.create');
  const canUpdate = permissions.hasPermission('digital_human.update');

  return (
    <div className="flex h-full flex-col space-y-6 overflow-y-auto p-4 md:p-6 xl:p-8">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
        <h1 className="text-xl font-semibold sm:text-2xl">{t('title')}</h1>
        <div className="flex w-full flex-col gap-3 sm:flex-row lg:w-auto lg:justify-end">
          <div className="relative w-full sm:w-64">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('list.search')}
              className="w-full rounded-lg bg-background pl-9 text-sm"
            />
          </div>
          {canCreate ? (
            <Button className="w-full sm:w-auto" onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              {t('list.create')}
            </Button>
          ) : null}
        </div>
      </div>

      {list.isLoading ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4">
          {Array.from({ length: 12 }).map((_, item) => (
            <div key={item} className="h-48 animate-pulse rounded-xl border bg-muted/30" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex min-h-72 flex-col items-center justify-center rounded-xl border border-dashed text-center">
          <Bot className="mb-3 size-9 text-muted-foreground" />
          <p className="max-w-md text-sm text-muted-foreground">{t('list.empty')}</p>
        </div>
      ) : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4">
          {items.map(item => (
            <DigitalHumanCard key={item.id} item={item} canUpdate={canUpdate} />
          ))}
        </div>
      )}

      <CreateDigitalHumanDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

function DigitalHumanCard({
  item,
  canUpdate,
}: {
  item: DigitalHumanDefinition;
  canUpdate: boolean;
}) {
  const t = useT('digitalHumans');
  const [editOpen, setEditOpen] = useState(false);
  const textIcon = getAgentTextIconDisplay(item.icon_type, item.icon, item.name);

  return (
    <div className="relative h-48">
      <Link
        href={
          item.published_version_id
            ? `/console/digital-humans/${item.id}`
            : `/console/digital-humans/${item.id}/configuration`
        }
        className="block h-full rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      >
        <Card className="flex h-full flex-col border border-border/80 shadow-sm transition-colors hover:border-border hover:bg-muted/20 hover:shadow-sm">
          <CardContent className="flex h-full min-w-0 flex-1 flex-col p-4">
            <div className="flex items-start justify-between gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center">
                <IconPreview
                  icon={textIcon.text}
                  iconType={item.icon_type || 'text'}
                  src={item.icon_type === 'image' ? item.icon_url || item.avatar || '' : ''}
                  iconBackground={textIcon.background}
                  editable={false}
                  size="sm"
                />
              </div>
              <Badge
                variant="outline"
                className={
                  item.published_version_id
                    ? 'border-success/40 bg-success/10 text-[11px] font-normal text-success'
                    : 'bg-muted text-[11px] font-normal text-muted-foreground'
                }
              >
                {item.published_version_id ? t('list.published') : t('status.draft')}
              </Badge>
            </div>
            <h2
              className="mt-3 line-clamp-2 min-h-10 break-all text-sm font-semibold leading-5 text-foreground"
              title={item.name}
            >
              {item.name}
            </h2>
            <p className="mt-3 line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">
              {item.description || t('list.noDescription')}
            </p>
            <div className="mt-auto flex items-center gap-2 pr-8 text-xs text-muted-foreground">
              <Clock3 className="size-4 shrink-0" />
              <span className="truncate">
                {t('list.updatedAt', {
                  date: new Date(item.updated_at).toLocaleDateString(),
                })}
              </span>
            </div>
          </CardContent>
        </Card>
      </Link>

      {canUpdate ? (
        <div className="absolute bottom-2 right-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className="rounded-md p-1 text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                aria-label={t('list.actions')}
              >
                <MoreHorizontal className="size-4" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem inset onSelect={() => setEditOpen(true)}>
                <Pencil className="size-4" />
                {t('basicInfo.edit')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ) : null}

      <DigitalHumanBasicInfoDialog definition={item} open={editOpen} onOpenChange={setEditOpen} />
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
  const [iconValue, setIconValue] = useState<IconValue>(createTextIconValue('', ICON_BG));
  const previousNameRef = useRef('');
  // The available-models endpoint already applies the agent-runtime capability
  // filter. Its compact response intentionally omits the ModelItem `callable`
  // projection used by the full model catalog.
  const defaultModel = availableModels.models[0]?.model ?? '';

  useEffect(() => {
    const previousDefault = previousNameRef.current.slice(0, 2).toUpperCase() || ICON_TEXT;
    previousNameRef.current = name;
    if (!name) return;
    setIconValue(current => {
      if (current.type !== 'text' || (current.icon && current.icon !== previousDefault)) {
        return current;
      }
      return createTextIconValue(
        name.slice(0, 2).toUpperCase() || ICON_TEXT,
        current.iconBackground || ICON_BG
      );
    });
  }, [name]);

  const submit = async () => {
    if (!name.trim() || !purpose.trim() || !defaultModel) {
      toast.error(t('errors.required'));
      return;
    }
    const response = await mutations.create.mutateAsync({
      name: name.trim(),
      description: purpose.trim(),
      icon_type: iconValue.type,
      icon:
        iconValue.type === 'text'
          ? JSON.stringify({
              icon: iconValue.icon || name.trim().slice(0, 2).toUpperCase() || ICON_TEXT,
              icon_background: iconValue.iconBackground || ICON_BG,
            })
          : iconValue.imageId || iconValue.iconUrl,
      identity_name: name.trim(),
      persona: purpose.trim(),
      goals: [],
      rules: [],
      default_model: defaultModel,
      autonomy_level: 'supervised',
      visibility: 'workspace',
      memory_policy: {
        enabled: false,
        ownership: 'digital_human',
        user_relationship: { enabled: false, auto_capture_allowed: false },
      },
      trigger_policy: { manual: true },
    });
    const id = response.data?.definition.id;
    onOpenChange(false);
    if (id) router.push(`/console/digital-humans/${id}/configuration`);
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
          <Field label={t('basicInfo.avatar')}>
            <IconInput
              showAvatarPresets
              value={iconValue}
              defaultValue={createTextIconValue(
                name.slice(0, 2).toUpperCase() || ICON_TEXT,
                ICON_BG
              )}
              onChange={setIconValue}
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
