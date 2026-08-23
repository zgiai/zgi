'use client';

import { useEffect, useRef, useState } from 'react';
import { IconInput } from '@/components/common/icon-input';
import {
  createImageIconValue,
  createTextIconValue,
  type IconValue,
} from '@/components/common/icon-input/types';
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
import { Textarea } from '@/components/ui/textarea';
import { useDigitalHumanMutations } from '@/hooks/digital-human/use-digital-humans';
import { useT } from '@/i18n';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import type { DigitalHumanDefinition } from '@/services/types/digital-human';

interface DigitalHumanBasicInfoDialogProps {
  definition?: DigitalHumanDefinition;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function DigitalHumanBasicInfoDialog({
  definition,
  open,
  onOpenChange,
}: DigitalHumanBasicInfoDialogProps) {
  const t = useT('digitalHumans');
  const mutations = useDigitalHumanMutations(definition?.id);
  const [displayName, setDisplayName] = useState('');
  const [displayDescription, setDisplayDescription] = useState('');
  const [iconValue, setIconValue] = useState<IconValue>(createTextIconValue('', ICON_BG));
  const previousNameRef = useRef('');

  useEffect(() => {
    if (!open || !definition) return;
    previousNameRef.current = definition.name;
    setDisplayName(definition.name);
    setDisplayDescription(definition.description ?? '');
    if (definition.icon_type === 'image') {
      setIconValue(
        createImageIconValue(
          definition.icon_url || definition.avatar || '',
          definition.icon || undefined
        )
      );
      return;
    }
    try {
      const parsed = JSON.parse(definition.icon || '{}') as {
        icon?: string;
        icon_background?: string;
      };
      setIconValue(
        createTextIconValue(
          parsed.icon || definition.name.slice(0, 2).toUpperCase() || ICON_TEXT,
          parsed.icon_background || ICON_BG
        )
      );
    } catch {
      setIconValue(
        createTextIconValue(
          definition.icon || definition.name.slice(0, 2).toUpperCase() || ICON_TEXT,
          ICON_BG
        )
      );
    }
  }, [definition, open]);

  useEffect(() => {
    const previousDefault = previousNameRef.current.slice(0, 2).toUpperCase() || ICON_TEXT;
    previousNameRef.current = displayName;
    if (!displayName) return;
    setIconValue(current => {
      if (current.type !== 'text' || (current.icon && current.icon !== previousDefault)) {
        return current;
      }
      return createTextIconValue(
        displayName.slice(0, 2).toUpperCase() || ICON_TEXT,
        current.iconBackground || ICON_BG
      );
    });
  }, [displayName]);

  const save = async () => {
    if (!definition || !displayName.trim()) return;
    await mutations.update.mutateAsync({
      digitalHumanId: definition.id,
      request: {
        expected_revision: definition.revision,
        name: displayName.trim(),
        description: displayDescription.trim(),
        icon_type: iconValue.type,
        icon:
          iconValue.type === 'text'
            ? JSON.stringify({
                icon: iconValue.icon || displayName.trim().slice(0, 2).toUpperCase() || ICON_TEXT,
                icon_background: iconValue.iconBackground || ICON_BG,
              })
            : iconValue.imageId || iconValue.iconUrl,
      },
    });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{t('basicInfo.title')}</DialogTitle>
          <DialogDescription>{t('basicInfo.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <label className="block space-y-1.5 text-sm font-medium">
            <span>{t('basicInfo.displayName')}</span>
            <Input
              value={displayName}
              onChange={event => setDisplayName(event.target.value)}
              autoFocus
            />
          </label>
          <label className="block space-y-1.5 text-sm font-medium">
            <span>{t('basicInfo.displayDescription')}</span>
            <Textarea
              rows={3}
              value={displayDescription}
              onChange={event => setDisplayDescription(event.target.value)}
            />
          </label>
          <label className="block space-y-1.5 text-sm font-medium">
            <span>{t('basicInfo.avatar')}</span>
            <IconInput
              showAvatarPresets
              value={iconValue}
              defaultValue={createTextIconValue(
                displayName.slice(0, 2).toUpperCase() || ICON_TEXT,
                ICON_BG
              )}
              onChange={setIconValue}
            />
          </label>
          <p className="text-xs leading-5 text-muted-foreground">{t('basicInfo.hint')}</p>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            loading={mutations.update.isPending}
            disabled={!displayName.trim() || mutations.update.isPending}
            onClick={() => void save()}
          >
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
