'use client';

import * as React from 'react';
import {
  ArrowUp,
  Film,
  ImagePlus,
  Loader2,
  Plus,
  Sparkles,
  SlidersHorizontal,
  Square,
  Timer,
  Video,
} from 'lucide-react';
import { toast } from 'sonner';

import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';

const VIDEO_ASPECT_RATIOS = ['1:1', '16:9', '9:16', '4:3'] as const;
const VIDEO_DURATIONS = ['5', '8', '10'] as const;
const VIDEO_RESOLUTIONS = ['720P', '1080P'] as const;
const VIDEO_COUNTS = ['1', '2', '4'] as const;
const VIDEO_REFERENCE_MODES = ['auto', 'none'] as const;

interface VideoGenerationSettings {
  aspectRatio: string;
  duration: string;
  resolution: string;
  count: string;
  referenceMode: string;
}

const DEFAULT_VIDEO_SETTINGS: VideoGenerationSettings = {
  aspectRatio: '1:1',
  duration: '5',
  resolution: '720P',
  count: '1',
  referenceMode: 'auto',
};

export function VideoWorkbench() {
  const t = useT('webapp');
  const { models, isLoading, error } = useAvailableModels({
    use_case: 'video-gen',
  });
  const [selectedModel, setSelectedModel] = React.useState<ModelSelectorValue>({
    provider: '',
    model: '',
  });
  const [prompt, setPrompt] = React.useState('');
  const [settings, setSettings] = React.useState<VideoGenerationSettings>(DEFAULT_VIDEO_SETTINGS);

  React.useEffect(() => {
    if (selectedModel.model) {
      const stillAvailable = models.some(
        model => model.provider === selectedModel.provider && model.model === selectedModel.model
      );
      if (stillAvailable) return;
    }

    const fallback = models[0];
    if (!fallback) {
      if (selectedModel.provider || selectedModel.model) {
        setSelectedModel({ provider: '', model: '' });
      }
      return;
    }

    setSelectedModel({ provider: fallback.provider, model: fallback.model });
  }, [models, selectedModel.model, selectedModel.provider]);

  const handleGenerate = React.useCallback(() => {
    toast.info(t('chat.videoWorkbench.pendingApiToast'));
  }, [t]);

  return (
    <div className="flex h-full min-h-0 w-full bg-background">
      <GenerationRecordsSidebar />

      <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="flex min-h-0 flex-1 flex-col px-4 py-5 sm:px-6 lg:px-8">
          <div className="flex min-h-0 flex-1 items-start justify-center pt-12">
            <section className="w-full max-w-5xl">
              <h1 className="mb-5 text-center text-2xl font-semibold tracking-normal text-foreground">
                {t('chat.videoWorkbench.heroTitle')}
              </h1>
              <ComposerPanel
                prompt={prompt}
                onPromptChange={setPrompt}
                settings={settings}
                onSettingsChange={setSettings}
                selectedModel={selectedModel}
                onModelChange={setSelectedModel}
                isModelLoading={isLoading}
                hasModels={models.length > 0}
                modelError={error}
                onGenerate={handleGenerate}
              />
            </section>
          </div>
        </div>
      </main>
    </div>
  );
}

function GenerationRecordsSidebar() {
  const t = useT('webapp');

  return (
    <aside className="hidden h-full w-[292px] shrink-0 flex-col border-r border-border bg-muted/20 md:flex">
      <div className="flex h-14 items-center justify-between border-b border-border px-4">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-foreground">
            {t('chat.videoWorkbench.recordsTitle')}
          </h2>
          <p className="text-xs text-muted-foreground">
            {t('chat.videoWorkbench.recordsCount', { count: 0 })}
          </p>
        </div>
        <Button type="button" variant="ghost" isIcon className="h-8 w-8">
          <Plus className="h-4 w-4" />
        </Button>
      </div>

      <div className="flex min-h-0 flex-1 flex-col">
        <div className="grid flex-1 place-items-center px-5">
          <div className="text-center">
            <div className="mx-auto flex h-11 w-11 items-center justify-center rounded-full bg-background shadow-sm">
              <Film className="h-5 w-5 text-muted-foreground" />
            </div>
            <h3 className="mt-4 text-sm font-semibold text-foreground">
              {t('chat.videoWorkbench.emptyRecordsTitle')}
            </h3>
          </div>
        </div>
      </div>
    </aside>
  );
}

function ComposerPanel({
  prompt,
  onPromptChange,
  settings,
  onSettingsChange,
  selectedModel,
  onModelChange,
  isModelLoading,
  hasModels,
  modelError,
  onGenerate,
}: {
  prompt: string;
  onPromptChange: (value: string) => void;
  settings: VideoGenerationSettings;
  onSettingsChange: (settings: VideoGenerationSettings) => void;
  selectedModel: ModelSelectorValue;
  onModelChange: (value: ModelSelectorValue) => void;
  isModelLoading: boolean;
  hasModels: boolean;
  modelError: string | null;
  onGenerate: () => void;
}) {
  const t = useT('webapp');
  const submitDisabled = !prompt.trim() || !selectedModel.model;

  return (
    <div className="rounded-2xl border border-border bg-background p-4 shadow-sm">
      <div className="flex min-h-[142px] flex-col gap-4">
        <ReferenceMaterialButton />
        <Textarea
          value={prompt}
          onChange={event => onPromptChange(event.target.value)}
          placeholder={t('chat.videoWorkbench.promptPlaceholder')}
          className={cn(
            'min-h-[54px] resize-none border-0 bg-transparent px-0 py-0 text-[15px] leading-7 text-foreground shadow-none',
            'placeholder:text-muted-foreground focus-visible:ring-0'
          )}
        />
      </div>

      <div className="mt-3 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <ModePill />
          <div className="w-[180px] shrink-0">
            <ModelSelector
              modelType="video-gen"
              value={selectedModel}
              onChange={onModelChange}
              disabled={isModelLoading || !hasModels}
              showCapabilities={false}
              emptyStateTitle={t('chat.videoWorkbench.emptyModelsTitle')}
              emptyStateDescription={t('chat.videoWorkbench.emptyModelsDescription')}
              className="h-8 rounded-md border-border bg-background px-2 text-xs font-medium text-foreground hover:bg-muted/40"
            />
          </div>
          <SpecSelectPill
            icon={<Sparkles className="h-3.5 w-3.5" />}
            value={settings.referenceMode}
            options={VIDEO_REFERENCE_MODES}
            labels={{
              auto: t('chat.videoWorkbench.referenceModeAuto'),
              none: t('chat.videoWorkbench.referenceModeNone'),
            }}
            onChange={referenceMode => onSettingsChange({ ...settings, referenceMode })}
          />
          <SpecSelectPill
            icon={<SlidersHorizontal className="h-3.5 w-3.5" />}
            value={settings.aspectRatio}
            options={VIDEO_ASPECT_RATIOS}
            onChange={aspectRatio => onSettingsChange({ ...settings, aspectRatio })}
          />
          <SpecSelectPill
            value={settings.resolution}
            options={VIDEO_RESOLUTIONS}
            onChange={resolution => onSettingsChange({ ...settings, resolution })}
          />
          <SpecSelectPill
            value={settings.count}
            options={VIDEO_COUNTS}
            onChange={count => onSettingsChange({ ...settings, count })}
          />
          <SpecSelectPill
            icon={<Timer className="h-3.5 w-3.5" />}
            value={settings.duration}
            options={VIDEO_DURATIONS}
            suffix={t('chat.videoWorkbench.secondsSuffix')}
            onChange={duration => onSettingsChange({ ...settings, duration })}
          />
        </div>

        <div className="flex items-center justify-end gap-3">
          {modelError ? (
            <Badge className="rounded-md border-red-500/30 bg-red-500/10 text-red-300">
              {t('chat.videoWorkbench.modelsLoadFailed')}
            </Badge>
          ) : null}
          {isModelLoading ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
              {t('chat.videoWorkbench.loadingModels')}
            </div>
          ) : null}
          <div className="text-xs font-medium text-muted-foreground">
            {t('chat.videoWorkbench.estimatedPrice')}
          </div>
          <Button
            type="button"
            isIcon
            className="h-8 w-8 rounded-md border-0 bg-primary/20 text-primary hover:bg-primary/30 disabled:!bg-primary/10 disabled:!text-primary/40"
            disabled={submitDisabled}
            onClick={onGenerate}
          >
            <ArrowUp className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

function ReferenceMaterialButton() {
  const t = useT('webapp');

  return (
    <button
      type="button"
      className="flex h-[62px] w-[116px] flex-col items-center justify-center gap-1 rounded-md border border-dashed border-border bg-muted/20 text-muted-foreground transition hover:border-border-strong hover:bg-muted/30 hover:text-foreground"
      onClick={() => toast.info(t('chat.videoWorkbench.referenceUploadPending'))}
    >
      <ImagePlus className="h-4 w-4" />
      <span className="text-xs font-medium leading-none">
        {t('chat.videoWorkbench.referenceMaterial')}
      </span>
    </button>
  );
}

function ModePill() {
  const t = useT('webapp');

  return (
    <button
      type="button"
      className="flex h-8 items-center gap-1.5 rounded-md border border-border bg-background px-2 text-xs font-medium text-foreground hover:bg-muted/40"
    >
      <Video className="h-3.5 w-3.5" />
      {t('chat.videoWorkbench.videoMode')}
    </button>
  );
}

function SpecSelectPill({
  icon,
  value,
  options,
  labels,
  suffix,
  onChange,
}: {
  icon?: React.ReactNode;
  value: string;
  options: readonly string[];
  labels?: Record<string, string>;
  suffix?: string;
  onChange: (value: string) => void;
}) {
  const display = labels?.[value] ?? (suffix ? `${value}${suffix}` : value);

  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-8 w-auto min-w-0 rounded-md border-border bg-background px-2 text-xs font-medium text-foreground hover:bg-muted/40 [&>div]:w-auto [&>div]:grow-0">
        <div className="flex items-center gap-1.5">
          {icon}
          <SelectValue>{display}</SelectValue>
        </div>
      </SelectTrigger>
      <SelectContent>
        {options.map(option => (
          <SelectItem key={option} value={option}>
            {labels?.[option] ?? (suffix ? `${option}${suffix}` : option)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
