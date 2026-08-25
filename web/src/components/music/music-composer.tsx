'use client';

import * as React from 'react';
import { AlertCircle, ArrowUp, Clock3, Layers3, Mic2, Music2, Sparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import {
  WorkspaceVoiceInputControl,
  type WorkspaceVoiceInputControlHandle,
} from '@/components/chat/variants/aichat/voice/workspace-voice-input-control';
import { useCreateMusicTasks } from '@/hooks/music/use-music-tasks';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { ModelItem } from '@/services/types/model';
import type { MusicMode, MusicTask, MusicVariantCount } from '@/services/types/music';
import { getErrorMessage } from '@/utils/error-notifications';

const MAX_PROMPT_RUNES = 2000;
const MAX_LYRICS_RUNES = 3500;
const SUBMIT_COOLDOWN_MS = 2000;
const VARIANT_COUNTS: MusicVariantCount[] = [1, 2, 3, 4];
const MUSIC_MODES: MusicMode[] = ['instrumental', 'auto_lyrics', 'vocal'];

type MusicVoiceField = MusicMode | 'lyrics';

function createEmptyPrompts(): Record<MusicMode, string> {
  return { instrumental: '', auto_lyrics: '', vocal: '' };
}

function runeCount(value: string): number {
  return Array.from(value).length;
}

interface MusicComposerProps {
  onCreated: (task: MusicTask) => void;
  reuseTask: MusicTask | null;
  model: string;
  models: ModelItem[];
  modelsLoading: boolean;
  modelsError: unknown;
  onModelChange: (model: string) => void;
}

export function MusicComposer({
  onCreated,
  reuseTask,
  model,
  models,
  modelsLoading,
  modelsError,
  onModelChange,
}: MusicComposerProps) {
  const t = useT('music');
  const mutation = useCreateMusicTasks();
  const submitInFlightRef = React.useRef(false);
  const submitCooldownRef = React.useRef(false);
  const submitCooldownTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const promptVoiceInputRefs = React.useRef<
    Partial<Record<MusicMode, WorkspaceVoiceInputControlHandle | null>>
  >({});
  const lyricsVoiceInputRef = React.useRef<WorkspaceVoiceInputControlHandle>(null);
  const [mode, setMode] = React.useState<MusicMode>('instrumental');
  const [prompts, setPrompts] = React.useState<Record<MusicMode, string>>(createEmptyPrompts);
  const [lyrics, setLyrics] = React.useState('');
  const [activeVoiceField, setActiveVoiceField] = React.useState<MusicVoiceField | null>(null);
  const [variantCount, setVariantCount] = React.useState<MusicVariantCount>(2);
  const [validationError, setValidationError] = React.useState<string | null>(null);
  const [isSubmitCoolingDown, setIsSubmitCoolingDown] = React.useState(false);
  React.useEffect(
    () => () => {
      if (submitCooldownTimerRef.current) clearTimeout(submitCooldownTimerRef.current);
    },
    []
  );
  React.useEffect(() => {
    if (!reuseTask) return;
    onModelChange(reuseTask.model);
    setMode(reuseTask.mode);
    setPrompts(current => ({ ...current, [reuseTask.mode]: reuseTask.prompt }));
    setLyrics(reuseTask.mode === 'vocal' ? (reuseTask.lyrics ?? '') : '');
    setValidationError(null);
  }, [onModelChange, reuseTask]);

  const prompt = prompts[mode];
  const promptLength = runeCount(prompt);
  const lyricsLength = runeCount(lyrics);
  const handleLyricsVoiceActiveChange = React.useCallback((active: boolean) => {
    setActiveVoiceField(current => (active ? 'lyrics' : current === 'lyrics' ? null : current));
  }, []);
  const handleModeChange = React.useCallback(
    (nextMode: MusicMode) => {
      if (nextMode === mode) return;
      if (activeVoiceField && activeVoiceField !== 'lyrics') {
        void promptVoiceInputRefs.current[activeVoiceField]?.finish();
      }
      if (activeVoiceField === 'lyrics') void lyricsVoiceInputRef.current?.finish();
      setActiveVoiceField(null);
      setMode(nextMode);
      setValidationError(null);
    },
    [activeVoiceField, mode]
  );

  async function handleSubmit(event: React.FormEvent) {
    event.preventDefault();
    if (submitInFlightRef.current || submitCooldownRef.current || activeVoiceField) return;
    mutation.reset();
    const normalizedPrompt = prompt.trim();
    const normalizedLyrics = lyrics.trim();
    if (!model) {
      return setValidationError(t('requiredModel'));
    }
    if (!normalizedPrompt) {
      return setValidationError(t('requiredPrompt'));
    }
    if (promptLength > MAX_PROMPT_RUNES) {
      return setValidationError(t('promptTooLong'));
    }
    if (mode === 'vocal' && !normalizedLyrics) {
      return setValidationError(t('requiredLyrics'));
    }
    if (mode === 'vocal' && lyricsLength > MAX_LYRICS_RUNES) {
      return setValidationError(t('lyricsTooLong'));
    }

    setValidationError(null);
    submitInFlightRef.current = true;
    submitCooldownRef.current = true;
    setIsSubmitCoolingDown(true);
    submitCooldownTimerRef.current = setTimeout(() => {
      submitCooldownRef.current = false;
      submitCooldownTimerRef.current = null;
      setIsSubmitCoolingDown(false);
    }, SUBMIT_COOLDOWN_MS);
    try {
      const result = await mutation.mutateAsync({
        model,
        mode,
        prompt: normalizedPrompt,
        variant_count: variantCount,
        ...(mode === 'vocal' ? { lyrics: normalizedLyrics } : {}),
      });
      for (const response of result.responses) onCreated(response.data);
      if (result.failedCount > 0) {
        setValidationError(
          t('partialCreateFailed', {
            created: result.responses.length,
            failed: result.failedCount,
          })
        );
      }
    } catch {
      // Mutation state renders the API error next to the action.
    } finally {
      submitInFlightRef.current = false;
    }
  }

  const modes: Array<{ value: MusicMode; icon: React.ElementType; label: string }> = [
    { value: 'instrumental', icon: Music2, label: t('modes.instrumental') },
    { value: 'auto_lyrics', icon: Sparkles, label: t('modes.autoLyrics') },
    { value: 'vocal', icon: Mic2, label: t('modes.vocal') },
  ];

  return (
    <section
      data-ui="music-composer-card"
      className="flex min-h-[520px] min-w-0 flex-col rounded-[24px] border border-border bg-background shadow-sm lg:min-h-0"
    >
      <form className="flex min-h-0 flex-1 flex-col" onSubmit={handleSubmit}>
        <div className="flex gap-2 overflow-x-auto p-4 pb-3">
          {modes.map(item => {
            const Icon = item.icon;
            return (
              <button
                key={item.value}
                type="button"
                aria-pressed={mode === item.value}
                onClick={() => handleModeChange(item.value)}
                className={cn(
                  'inline-flex h-10 shrink-0 items-center gap-2 rounded-xl border px-3.5 text-xs font-medium transition-[border-color,background-color,box-shadow]',
                  mode === item.value
                    ? 'border-foreground/55 bg-background text-foreground shadow-sm ring-1 ring-foreground/10'
                    : 'border-border bg-muted/20 text-muted-foreground hover:border-foreground/25 hover:bg-muted/40'
                )}
              >
                <Icon className="size-3.5" />
                {item.label}
              </button>
            );
          })}
        </div>

        <div
          data-ui="music-prompt-surface"
          className="mx-4 flex min-h-0 flex-1 flex-col overflow-y-auto rounded-2xl border border-border"
        >
          {MUSIC_MODES.map(promptMode => {
            const promptValue = prompts[promptMode];
            const promptValueLength = runeCount(promptValue);
            return (
              <div
                key={promptMode}
                className={cn(
                  'min-h-44 flex-col p-4',
                  promptMode === mode ? 'flex' : 'hidden',
                  mode !== 'vocal' && 'flex-1'
                )}
              >
                <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
                  <span>{t('prompt')}</span>
                  <div className="flex items-center gap-3">
                    <WorkspaceVoiceInputControl
                      ref={handle => {
                        promptVoiceInputRefs.current[promptMode] = handle;
                      }}
                      value={promptValue}
                      onChange={value => {
                        setPrompts(current => ({ ...current, [promptMode]: value }));
                        setValidationError(null);
                      }}
                      disabled={
                        mutation.isPending ||
                        (activeVoiceField !== null && activeVoiceField !== promptMode)
                      }
                      onActiveChange={active => {
                        setActiveVoiceField(current =>
                          active ? promptMode : current === promptMode ? null : current
                        );
                      }}
                    />
                    <span
                      className={cn(
                        promptValueLength > MAX_PROMPT_RUNES && 'text-destructive'
                      )}
                    >
                      {promptValueLength}/{MAX_PROMPT_RUNES}
                    </span>
                    {promptValue ? (
                      <button
                        type="button"
                        className="transition-colors hover:text-foreground"
                        onClick={() => {
                          setPrompts(current => ({ ...current, [promptMode]: '' }));
                          setValidationError(null);
                        }}
                      >
                        {t('clear')}
                      </button>
                    ) : null}
                  </div>
                </div>
                <Textarea
                  value={promptValue}
                  onChange={event => {
                    const value = event.target.value;
                    setPrompts(current => ({ ...current, [promptMode]: value }));
                    setValidationError(null);
                  }}
                  placeholder={t('promptPlaceholder')}
                  className={cn(
                    'mt-2 min-h-32 resize-none border-0 bg-transparent px-0 text-[15px] leading-6 shadow-none focus-visible:ring-0',
                    mode !== 'vocal' && '!max-h-none flex-1'
                  )}
                  aria-invalid={promptValueLength > MAX_PROMPT_RUNES}
                />
              </div>
            );
          })}

          <div
            className={cn(
              'min-h-52 border-t border-border p-4',
              mode !== 'vocal' && 'hidden'
            )}
          >
            <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
              <span>{t('lyrics')}</span>
              <div className="flex items-center gap-3">
                <WorkspaceVoiceInputControl
                  ref={lyricsVoiceInputRef}
                  value={lyrics}
                  onChange={value => {
                    setLyrics(value);
                    setValidationError(null);
                  }}
                  disabled={
                    mutation.isPending ||
                    (activeVoiceField !== null && activeVoiceField !== 'lyrics')
                  }
                  onActiveChange={handleLyricsVoiceActiveChange}
                />
                <span className={cn(lyricsLength > MAX_LYRICS_RUNES && 'text-destructive')}>
                  {lyricsLength}/{MAX_LYRICS_RUNES}
                </span>
                {lyrics ? (
                  <button
                    type="button"
                    className="transition-colors hover:text-foreground"
                    onClick={() => {
                      setLyrics('');
                      setValidationError(null);
                    }}
                  >
                    {t('clear')}
                  </button>
                ) : null}
              </div>
            </div>
            <Textarea
              value={lyrics}
              onChange={event => {
                setLyrics(event.target.value);
                setValidationError(null);
              }}
              placeholder={t('lyricsPlaceholder')}
              className="mt-2 min-h-44 resize-none border-0 bg-transparent px-0 font-mono text-sm leading-6 shadow-none focus-visible:ring-0"
              aria-invalid={lyricsLength > MAX_LYRICS_RUNES}
            />
          </div>
        </div>

        {modelsError ? (
          <div className="mx-4 mt-3 rounded-xl border border-destructive/25 bg-destructive/5 p-3 text-sm text-destructive">
            {getErrorMessage(modelsError) || t('noModels')}
          </div>
        ) : !modelsLoading && !models.length ? (
          <p className="mx-4 mt-3 text-xs leading-5 text-muted-foreground">{t('noModels')}</p>
        ) : null}

        {validationError || mutation.error ? (
          <div
            role="alert"
            className="mx-4 mt-3 flex gap-2 rounded-xl bg-destructive/8 p-3 text-sm text-destructive"
          >
            <AlertCircle className="mt-0.5 size-4 shrink-0" />
            <span>{validationError || getErrorMessage(mutation.error) || t('createFailed')}</span>
          </div>
        ) : null}

        <div data-ui="music-composer-footer" className="mt-auto p-4">
          <div className="flex items-center gap-2">
            <span
              title={t('automaticDurationHint')}
              className="inline-flex h-10 items-center gap-2 rounded-xl border border-border bg-background px-3 text-xs text-muted-foreground"
            >
              <Clock3 className="size-4" />
              {t('automaticDuration')}
            </span>
            <Select
              value={String(variantCount)}
              onValueChange={value => setVariantCount(Number(value) as MusicVariantCount)}
            >
              <SelectTrigger
                data-ui="music-variant-select"
                aria-label={t('variants')}
                className="h-10 w-[88px] rounded-xl bg-background px-3 shadow-none [&>svg:last-child]:ml-auto"
              >
                <span className="flex min-w-0 items-center gap-2">
                  <Layers3 className="size-4 shrink-0" />
                  <SelectValue />
                </span>
              </SelectTrigger>
              <SelectContent align="start">
                {VARIANT_COUNTS.map(count => (
                  <SelectItem key={count} value={String(count)}>
                    {count}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div data-ui="music-model-selector" className="min-w-0 flex-1">
              <Select
                value={model}
                onValueChange={onModelChange}
                disabled={modelsLoading || !models.length}
              >
                <SelectTrigger
                  className="h-10 w-full min-w-0 rounded-xl bg-background px-3 text-xs shadow-none"
                  isLoading={modelsLoading}
                  aria-label={t('model')}
                >
                  <SelectValue
                    placeholder={modelsLoading ? t('loadingModels') : t('selectModel')}
                  />
                </SelectTrigger>
                <SelectContent>
                  {models.map(item => (
                    <SelectItem key={`${item.provider}:${item.model}`} value={item.model}>
                      {item.model_name || item.model}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <Button
              type="submit"
              size="sm"
              className="ml-auto size-11 shrink-0 rounded-full bg-foreground text-background hover:bg-foreground/85"
              loading={mutation.isPending}
              disabled={
                isSubmitCoolingDown ||
                Boolean(activeVoiceField) ||
                !models.length ||
                promptLength > MAX_PROMPT_RUNES ||
                lyricsLength > MAX_LYRICS_RUNES
              }
              aria-label={t('generate')}
            >
              {!mutation.isPending ? <ArrowUp className="size-5" /> : null}
            </Button>
          </div>
        </div>
      </form>
    </section>
  );
}
