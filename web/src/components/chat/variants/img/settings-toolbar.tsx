import * as React from 'react';
import { ChevronDown } from 'lucide-react';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { IMAGE_ASPECT_RATIOS, IMAGE_COUNTS } from './constants';

export interface ImageSettings {
  ratio: string;
  count: number;
  customRatio: { width: number; height: number };
  isCustomRatio: boolean;
}

export interface ImageSettingsPatch {
  ratio: string;
  count: number;
  customRatio?: { width: number; height: number };
  isCustomRatio?: boolean;
}

interface SettingsToolbarProps {
  onSettingsChange?: (settings: ImageSettingsPatch) => void;
  initialSettings?: Pick<ImageSettings, 'ratio' | 'count'>;
}

export function SettingsToolbar({ onSettingsChange, initialSettings }: SettingsToolbarProps) {
  const t = useT('webapp');

  const [selectedRatio, setSelectedRatio] = React.useState<string>(initialSettings?.ratio || '1:1');
  const [selectedCount, setSelectedCount] = React.useState<number>(
    initialSettings?.count || IMAGE_COUNTS[0].id
  );

  // Popover state
  const [isRatioOpen, setIsRatioOpen] = React.useState(false);
  const [isCountOpen, setIsCountOpen] = React.useState(false);

  // Notify parent on change
  React.useEffect(() => {
    onSettingsChange?.({
      ratio: selectedRatio,
      count: selectedCount,
    });
  }, [selectedRatio, selectedCount, onSettingsChange]);

  // Option button styles
  const optionBtnClass =
    'flex items-center gap-1.5 rounded-md border border-border/50 bg-background px-2 py-1 text-xs text-foreground hover:bg-muted/40 hover:border-border transition-colors shadow-sm whitespace-nowrap';

  return (
    <div className="flex items-center gap-2 flex-wrap">
      {/* Aspect ratio selection */}
      <Popover open={isRatioOpen} onOpenChange={setIsRatioOpen}>
        <PopoverTrigger asChild>
          <button type="button" className={optionBtnClass}>
            <span>
              {selectedRatio
                ? IMAGE_ASPECT_RATIOS.find(r => r.id === selectedRatio)?.name
                : t('chat.imageInput.ratio')}
            </span>
            <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
          </button>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-[160px] p-1 rounded-xl border border-border/50 shadow-lg"
        >
          {IMAGE_ASPECT_RATIOS.map(ratio => {
            // Calculate preview dimensions for the aspect ratio
            const [w, h] = ratio.id.split(':').map(Number);
            const maxSize = 18;
            const scale = maxSize / Math.max(w, h);
            const previewW = w * scale;
            const previewH = h * scale;

            return (
              <button
                key={ratio.id}
                type="button"
                onClick={() => {
                  setSelectedRatio(ratio.id);
                  setIsRatioOpen(false);
                }}
                className={cn(
                  'w-full flex items-center gap-2 px-2 py-1.5 rounded-lg text-xs transition-colors',
                  selectedRatio === ratio.id
                    ? 'bg-neutral-100 dark:bg-neutral-800 text-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
                )}
              >
                {/* Aspect ratio preview */}
                <div className="flex items-center justify-center w-[18px] h-[18px] shrink-0">
                  <div
                    className="border border-current rounded-[1.5px]"
                    style={{ width: previewW, height: previewH }}
                  />
                </div>
                <span>{ratio.name}</span>
                <span className="text-[10px] text-muted-foreground ml-auto">
                  {t(`chat.imageInput.aspectRatios.${ratio.labelKey}` as const)}
                </span>
              </button>
            );
          })}
        </PopoverContent>
      </Popover>

      {/* Count selection */}
      <Popover open={isCountOpen} onOpenChange={setIsCountOpen}>
        <PopoverTrigger asChild>
          <button type="button" className={optionBtnClass}>
            <span>
              {t('chat.imageInput.countValue', { count: selectedCount })}
            </span>
            <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
          </button>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-[80px] p-1 rounded-xl border border-border/50 shadow-lg"
        >
          {IMAGE_COUNTS.map(count => (
            <button
              key={count.id}
              type="button"
              onClick={() => {
                setSelectedCount(count.id);
                setIsCountOpen(false);
              }}
              className={cn(
                'w-full text-center px-2 py-1.5 rounded-lg text-xs transition-colors',
                selectedCount === count.id
                  ? 'bg-neutral-100 dark:bg-neutral-800 text-foreground'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
              )}
            >
              {t('chat.imageInput.countValue', { count: count.id })}
            </button>
          ))}
        </PopoverContent>
      </Popover>
    </div>
  );
}
