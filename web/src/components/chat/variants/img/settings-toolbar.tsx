import * as React from 'react';
import { ChevronDown } from 'lucide-react';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import type { ImageGenerationProfile } from '@/services/types/image-runtime';

export interface ImageSettings {
  size?: string;
  count?: number;
  generationMode?: 'single' | 'sequence';
  maxImages?: number;
}

export type ImageSettingsPatch = ImageSettings;

interface SettingsToolbarProps {
  onSettingsChange?: (settings: ImageSettingsPatch) => void;
  settings: ImageSettings;
  profile?: ImageGenerationProfile;
}

export function SettingsToolbar({ onSettingsChange, settings, profile }: SettingsToolbarProps) {
  const t = useT('webapp');
  const [isSizeOpen, setIsSizeOpen] = React.useState(false);
  const [isQuantityOpen, setIsQuantityOpen] = React.useState(false);
  const sizes = profile?.size?.options ?? [];
  const quantity = profile?.quantity;
  const optionBtnClass =
    'flex items-center gap-1.5 rounded-md border border-border/50 bg-background px-2 py-1 text-xs text-foreground hover:bg-muted/40 hover:border-border transition-colors shadow-sm whitespace-nowrap';

  return (
    <div className="flex items-center gap-2 flex-wrap">
      {sizes.length > 0 ? (
        <Popover open={isSizeOpen} onOpenChange={setIsSizeOpen}>
          <PopoverTrigger asChild>
            <button type="button" className={optionBtnClass}>
              <span>{sizes.find(item => item.value === settings.size)?.label ?? settings.size}</span>
              <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
            </button>
          </PopoverTrigger>
          <PopoverContent align="start" className="w-[190px] p-1 rounded-xl">
            {sizes.map(size => (
              <button
                key={size.value}
                type="button"
                onClick={() => {
                  onSettingsChange?.({ ...settings, size: size.value });
                  setIsSizeOpen(false);
                }}
                className={cn(
                  'w-full flex justify-between px-2 py-1.5 rounded-lg text-xs',
                  settings.size === size.value
                    ? 'bg-neutral-100 dark:bg-neutral-800 text-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
                )}
              >
                <span>{size.label}</span>
                <span>{size.aspect_ratio}</span>
              </button>
            ))}
          </PopoverContent>
        </Popover>
      ) : null}

      {quantity?.mode === 'exact' ? (
        <QuantityPopover
          value={settings.count ?? quantity.default}
          min={quantity.min}
          max={quantity.max}
          open={isQuantityOpen}
          onOpenChange={setIsQuantityOpen}
          onChange={count => onSettingsChange?.({ ...settings, count })}
        />
      ) : null}

      {quantity?.mode === 'sequence' ? (
        <>
          <button
            type="button"
            className={optionBtnClass}
            onClick={() =>
              onSettingsChange?.({
                ...settings,
                generationMode: settings.generationMode === 'sequence' ? 'single' : 'sequence',
                maxImages:
                  settings.generationMode === 'sequence' ? undefined : quantity.default,
              })
            }
          >
            {settings.generationMode === 'sequence'
              ? t('chat.imageInput.sequenceMode')
              : t('chat.imageInput.singleMode')}
          </button>
          {settings.generationMode === 'sequence' ? (
            <QuantityPopover
              value={settings.maxImages ?? quantity.default}
              min={quantity.min}
              max={quantity.max}
              maximum
              open={isQuantityOpen}
              onOpenChange={setIsQuantityOpen}
              onChange={maxImages => onSettingsChange?.({ ...settings, maxImages })}
            />
          ) : null}
        </>
      ) : null}
    </div>
  );
}

function QuantityPopover({
  value,
  min,
  max,
  maximum = false,
  open,
  onOpenChange,
  onChange,
}: {
  value: number;
  min: number;
  max: number;
  maximum?: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onChange: (value: number) => void;
}) {
  const t = useT('webapp');
  const values = Array.from({ length: max - min + 1 }, (_, index) => min + index);
  const countLabel = (count: number) =>
    t(maximum ? 'chat.imageInput.maxCountValue' : 'chat.imageInput.countValue', { count });

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="flex items-center gap-1.5 rounded-md border border-border/50 bg-background px-2 py-1 text-xs shadow-sm"
        >
          {countLabel(value)}
          <ChevronDown className="h-3 w-3 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className="max-h-64 w-[90px] overflow-y-auto p-1 rounded-xl">
        {values.map(item => (
          <button
            key={item}
            type="button"
            onClick={() => {
              onChange(item);
              onOpenChange(false);
            }}
            className={cn(
              'w-full px-2 py-1.5 rounded-lg text-xs',
              item === value
                ? 'bg-neutral-100 dark:bg-neutral-800 text-foreground'
                : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
            )}
          >
            {countLabel(item)}
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}
