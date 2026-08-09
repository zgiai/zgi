'use client';

import * as React from 'react';
import Image from 'next/image';
import {
  CalendarClock,
  CheckCircle2,
  Clock3,
  Coins,
  Copy,
  Download,
  Film,
  ImagePlus,
  Loader2,
  Play,
  RefreshCw,
  Search,
  Sparkles,
  SlidersHorizontal,
  Timer,
  Trash2,
  Video,
  Volume2,
  X,
  XCircle,
} from 'lucide-react';
import { toast } from 'sonner';

import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Textarea } from '@/components/ui/textarea';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import {
  useDeleteVideoTask,
  useGenerateVideoTask,
  useVideoRuntimeTask,
  useVideoRuntimeTasks,
} from '@/hooks/video-runtime/use-video-runtime';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { ModelItem } from '@/services/types/model';
import type { VideoRuntimeTask } from '@/services/types/video-runtime';
import { fileManageService } from '@/services/file-manage.service';
import { uploadService, type UploadResponse } from '@/services/upload.service';
import { formatAiCreditValue, normalizeAiCreditValue } from '@/utils/ai-credits';
import { formatDate } from '@/utils/format';

const VIDEO_ASPECT_RATIOS = ['1:1', '16:9', '9:16', '4:3', '3:4'] as const;
const VIDEO_DURATIONS = ['5', '8', '10'] as const;
const VIDEO_RESOLUTIONS = ['720p', '1080p'] as const;
const VIDEO_REFERENCE_MODES = ['auto'] as const;
const VIDEO_FRAME_REFERENCE_MODE = 'first_last_frame';
const VIDEO_AUDIO_MODES = ['off', 'on'] as const;
const REFERENCE_KIND_ORDER: ReferenceKind[] = ['image', 'video', 'audio'];
const REFERENCE_ACCEPT_BY_KIND: Record<ReferenceKind, string> = {
  image: 'image/*',
  video: 'video/*',
  audio: 'audio/*',
};

type ReferenceKind = 'image' | 'video' | 'audio';
type NormalizedVideoStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled';

const VIDEO_STATUS_LABEL_KEYS = {
  pending: 'chat.videoWorkbench.status.pending',
  running: 'chat.videoWorkbench.status.running',
  succeeded: 'chat.videoWorkbench.status.succeeded',
  failed: 'chat.videoWorkbench.status.failed',
  cancelled: 'chat.videoWorkbench.status.cancelled',
} as const;

const REFERENCE_KIND_LABEL_KEYS = {
  image: 'chat.videoWorkbench.referenceKinds.image',
  video: 'chat.videoWorkbench.referenceKinds.video',
  audio: 'chat.videoWorkbench.referenceKinds.audio',
} as const;

interface VideoGenerationSettings {
  aspectRatio: string;
  duration: string;
  resolution: string;
  count: string;
  referenceMode: string;
  audioMode: string;
}

interface SelectedReferenceFile {
  id: string;
  file: File;
  kind: ReferenceKind;
  previewUrl: string;
}

interface VideoGenerationOptions {
  aspectRatios: string[];
  durations: string[];
  resolutions: string[];
  referenceModes: string[];
  audioModes: string[];
}

const DEFAULT_VIDEO_SETTINGS: VideoGenerationSettings = {
  aspectRatio: '1:1',
  duration: '5',
  resolution: '720p',
  count: '1',
  referenceMode: 'auto',
  audioMode: 'on',
};

const DEFAULT_VIDEO_GENERATION_OPTIONS: VideoGenerationOptions = {
  aspectRatios: [...VIDEO_ASPECT_RATIOS],
  durations: [...VIDEO_DURATIONS],
  resolutions: [...VIDEO_RESOLUTIONS],
  referenceModes: [...VIDEO_REFERENCE_MODES],
  audioModes: [...VIDEO_AUDIO_MODES],
};

export function VideoWorkbench() {
  const t = useT('webapp');
  const { models, isLoading, error } = useAvailableModels({
    use_case: 'video-gen',
  });
  const [historyQuery, setHistoryQuery] = React.useState('');
  const debouncedHistoryQuery = useDebouncedValue(historyQuery.trim(), 300);
  const {
    tasks,
    total: totalTasks,
    reload: reloadTasks,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading: isLoadingTasks,
    isFetching: isFetchingTasks,
    isError: isTasksError,
  } = useVideoRuntimeTasks(debouncedHistoryQuery);
  const generateMutation = useGenerateVideoTask();
  const deleteTaskMutation = useDeleteVideoTask();
  const [selectedModel, setSelectedModel] = React.useState<ModelSelectorValue>({
    provider: '',
    model: '',
  });
  const [prompt, setPrompt] = React.useState('');
  const [settings, setSettings] = React.useState<VideoGenerationSettings>(DEFAULT_VIDEO_SETTINGS);
  const [selectedTaskId, setSelectedTaskId] = React.useState<string | null>(null);
  const [pollingTaskId, setPollingTaskId] = React.useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const { task: selectedTaskDetail } = useVideoRuntimeTask(selectedTaskId);
  const { task: pollingTaskDetail } = useVideoRuntimeTask(
    pollingTaskId && pollingTaskId !== selectedTaskId ? pollingTaskId : null
  );
  const pollingTaskStatus = pollingTaskDetail?.status;
  const pollingTaskDetailId = pollingTaskDetail?.task_id;
  const [referenceFiles, setReferenceFiles] = React.useState<SelectedReferenceFile[]>([]);
  const referenceFilesRef = React.useRef<SelectedReferenceFile[]>([]);
  const completedPollingTaskIdsRef = React.useRef(new Set<string>());

  const selectedTask = React.useMemo(
    () => selectedTaskDetail ?? tasks.find(task => task.task_id === selectedTaskId) ?? null,
    [selectedTaskDetail, selectedTaskId, tasks]
  );
  const selectedModelItem = React.useMemo(
    () =>
      models.find(
        model => model.provider === selectedModel.provider && model.model === selectedModel.model
      ),
    [models, selectedModel.model, selectedModel.provider]
  );

  React.useEffect(() => {
    if (pollingTaskId) return;
    const activeTask = tasks.find(
      task =>
        isActiveVideoTaskStatus(task.status) &&
        !completedPollingTaskIdsRef.current.has(task.task_id)
    );
    if (activeTask) setPollingTaskId(activeTask.task_id);
  }, [pollingTaskId, tasks]);

  React.useEffect(() => {
    if (!pollingTaskId || !pollingTaskDetailId || !pollingTaskStatus) return;
    if (!isActiveVideoTaskStatus(pollingTaskStatus)) {
      completedPollingTaskIdsRef.current.add(pollingTaskId);
      setPollingTaskId(null);
    }
  }, [pollingTaskDetailId, pollingTaskId, pollingTaskStatus]);
  const supportedReferenceKinds = React.useMemo(
    () => getVideoReferenceKinds(selectedModelItem),
    [selectedModelItem]
  );
  const hasReferenceMaterials = supportedReferenceKinds.length > 0;
  const allowedReferenceKinds = React.useMemo(
    () => getAllowedReferenceKinds(selectedModelItem, settings.referenceMode),
    [selectedModelItem, settings.referenceMode]
  );
  const maxReferenceFiles = getMaxReferenceFiles(selectedModelItem, settings.referenceMode);
  const allowedReferenceKey = allowedReferenceKinds.join(',');
  const generationOptions = React.useMemo(
    () => getVideoGenerationOptions(selectedModelItem),
    [selectedModelItem]
  );
  const generationOptionsKey = React.useMemo(
    () => JSON.stringify(generationOptions),
    [generationOptions]
  );

  React.useEffect(() => {
    setReferenceFiles(files => {
      const nextFiles = files
        .filter(file => allowedReferenceKinds.includes(file.kind))
        .slice(0, maxReferenceFiles);
      files.forEach(file => {
        if (!nextFiles.includes(file)) URL.revokeObjectURL(file.previewUrl);
      });
      return nextFiles.length === files.length ? files : nextFiles;
    });
    // Only the scalar capability key should trigger cleanup. Model objects are re-created by the
    // query normalization path, so depending on the array itself can loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allowedReferenceKey, maxReferenceFiles]);

  React.useEffect(() => {
    referenceFilesRef.current = referenceFiles;
  }, [referenceFiles]);

  React.useEffect(() => {
    setSettings(prev => normalizeVideoSettings(prev, generationOptions));
    // generationOptionsKey is the stable scalar signature for the selected model capabilities.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [generationOptionsKey]);

  React.useEffect(() => {
    return () => {
      referenceFilesRef.current.forEach(file => URL.revokeObjectURL(file.previewUrl));
    };
  }, []);

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

  const handleGenerate = React.useCallback(async () => {
    if (!selectedModel.provider || !selectedModel.model || !prompt.trim()) return;
    setIsSubmitting(true);
    try {
      const referenceUrls =
        referenceFiles.length > 0
          ? await Promise.all(
              referenceFiles.map(referenceFile =>
                uploadVideoReferenceFile(
                  referenceFile.file,
                  t('chat.videoWorkbench.referenceUploadNoUrl')
                )
              )
            )
          : [];
      const isFrameReferenceMode = settings.referenceMode === VIDEO_FRAME_REFERENCE_MODE;
      const referenceTypes = referenceFiles.map(referenceFile => referenceFile.kind);
      const response = await generateMutation.mutateAsync({
        provider: selectedModel.provider,
        model: selectedModel.model,
        prompt: prompt.trim(),
        ...(isFrameReferenceMode
          ? {
              ...(referenceUrls[0] ? { first_frame_url: referenceUrls[0] } : {}),
              ...(referenceUrls[1] ? { last_frame_url: referenceUrls[1] } : {}),
            }
          : {
              ...(referenceUrls[0] ? { reference_url: referenceUrls[0] } : {}),
              ...(referenceUrls.length > 0 ? { reference_urls: referenceUrls } : {}),
              ...(referenceTypes.length > 0 ? { reference_types: referenceTypes } : {}),
            }),
        options: {
          ratio: settings.aspectRatio,
          resolution: settings.resolution.toLowerCase(),
          duration: Number.parseInt(settings.duration, 10),
          count: Number.parseInt(settings.count, 10),
          ...(generationOptions.audioModes.length > 1
            ? { generate_audio: settings.audioMode === 'on' }
            : {}),
        },
      });
      const taskID = response.data?.task?.task_id;
      if (taskID) {
        completedPollingTaskIdsRef.current.delete(taskID);
        setPollingTaskId(taskID);
        void reloadTasks();
      }
      setPrompt('');
      setReferenceFiles(files => {
        files.forEach(file => URL.revokeObjectURL(file.previewUrl));
        return [];
      });
      toast.success(t('chat.videoWorkbench.submitSuccess'));
    } catch (err) {
      const message = err instanceof Error ? err.message : t('chat.videoWorkbench.submitFailed');
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  }, [
    generateMutation,
    generationOptions.audioModes.length,
    prompt,
    referenceFiles,
    reloadTasks,
    selectedModel.model,
    selectedModel.provider,
    settings,
    t,
  ]);

  const handleDeleteTask = React.useCallback(
    async (taskId: string) => {
      try {
        await deleteTaskMutation.mutateAsync(taskId);
        completedPollingTaskIdsRef.current.delete(taskId);
        if (selectedTaskId === taskId) setSelectedTaskId(null);
        if (pollingTaskId === taskId) setPollingTaskId(null);
        toast.success(t('chat.videoWorkbench.deleteRecordSuccess'));
      } catch (err) {
        const message =
          err instanceof Error ? err.message : t('chat.videoWorkbench.deleteRecordFailed');
        toast.error(message);
      }
    },
    [deleteTaskMutation, pollingTaskId, selectedTaskId, t]
  );

  return (
    <div className="flex h-full min-h-0 w-full bg-background">
      <GenerationRecordsSidebar
        tasks={tasks}
        total={totalTasks}
        query={historyQuery}
        onQueryChange={setHistoryQuery}
        isSearchPending={
          historyQuery.trim() !== debouncedHistoryQuery ||
          (isFetchingTasks && !isFetchingNextPage)
        }
        isLoading={isLoadingTasks}
        isError={isTasksError}
        hasNextPage={hasNextPage}
        isFetchingNextPage={isFetchingNextPage}
        onLoadMore={fetchNextPage}
        selectedTaskId={selectedTaskId}
        onSelectTask={setSelectedTaskId}
        onDeleteTask={handleDeleteTask}
        deletingTaskId={deleteTaskMutation.variables ?? null}
        onRefresh={() => void reloadTasks()}
      />

      <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="flex min-h-0 flex-1 flex-col px-4 py-5 sm:px-6 lg:px-8">
          <div className="flex min-h-0 flex-1 items-start justify-center pt-10">
            <section className="w-full max-w-5xl">
              <h1 className="mb-5 text-center text-2xl font-semibold tracking-normal text-foreground">
                {t('chat.videoWorkbench.heroTitle')}
              </h1>
              <ComposerPanel
                prompt={prompt}
                onPromptChange={setPrompt}
                settings={settings}
                generationOptions={generationOptions}
                onSettingsChange={setSettings}
                selectedModel={selectedModel}
                onModelChange={setSelectedModel}
                allowedReferenceKinds={allowedReferenceKinds}
                hasReferenceMaterials={hasReferenceMaterials}
                maxReferenceFiles={maxReferenceFiles}
                referenceFiles={referenceFiles}
                onReferenceFilesChange={setReferenceFiles}
                isModelLoading={isLoading}
                hasModels={models.length > 0}
                modelError={error}
                isGenerating={isSubmitting || generateMutation.isPending}
                onGenerate={handleGenerate}
              />
            </section>
          </div>
        </div>
      </main>

      <TaskDetailSheet
        task={selectedTask}
        onOpenChange={open => !open && setSelectedTaskId(null)}
      />
    </div>
  );
}

function GenerationRecordsSidebar({
  tasks,
  total,
  query,
  onQueryChange,
  isSearchPending,
  isLoading,
  isError,
  hasNextPage,
  isFetchingNextPage,
  onLoadMore,
  selectedTaskId,
  onSelectTask,
  onDeleteTask,
  deletingTaskId,
  onRefresh,
}: {
  tasks: VideoRuntimeTask[];
  total: number;
  query: string;
  onQueryChange: (query: string) => void;
  isSearchPending: boolean;
  isLoading: boolean;
  isError: boolean;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  onLoadMore: () => Promise<unknown>;
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
  onDeleteTask: (taskId: string) => void;
  deletingTaskId: string | null;
  onRefresh: () => void;
}) {
  const t = useT('webapp');
  const [searchOpen, setSearchOpen] = React.useState(false);
  const searchInputRef = React.useRef<HTMLInputElement>(null);
  const listRef = React.useRef<HTMLDivElement>(null);
  const loadMoreRef = useInfiniteObserver({
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage: onLoadMore,
    rootRef: listRef,
    rootMargin: '160px',
    enabled: !isSearchPending && tasks.length > 0,
  });

  React.useEffect(() => {
    listRef.current?.scrollTo({ top: 0 });
  }, [query]);

  return (
    <aside className="hidden h-full w-[328px] shrink-0 flex-col border-r border-border bg-background md:flex">
      <div className="flex h-16 items-center justify-between border-b border-border px-4">
        <div className="min-w-0">
          <h2 className="text-[15px] font-semibold tracking-tight text-foreground">
            {t('chat.videoWorkbench.recordsTitle')}
          </h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {t('chat.videoWorkbench.recordsCount', { count: total })}
          </p>
        </div>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            isIcon
            className="h-8 w-8 rounded-full text-muted-foreground"
            title={t('chat.videoWorkbench.searchRecords')}
            onClick={() => {
              const next = !searchOpen;
              setSearchOpen(next);
              if (next) window.requestAnimationFrame(() => searchInputRef.current?.focus());
              else onQueryChange('');
            }}
          >
            <Search className="h-4 w-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            isIcon
            className="h-8 w-8 rounded-full text-muted-foreground"
            onClick={onRefresh}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {searchOpen ? (
        <div className="border-b border-border p-3">
          <Input
            ref={searchInputRef}
            value={query}
            onChange={event => onQueryChange(event.target.value)}
            onKeyDown={event => {
              if (event.key === 'Escape') {
                onQueryChange('');
                setSearchOpen(false);
              }
            }}
            leftIcon={<Search className="h-4 w-4" />}
            rightIcon={isSearchPending ? <Loader2 className="h-4 w-4 animate-spin" /> : undefined}
            placeholder={t('chat.videoWorkbench.searchRecords')}
            maxLength={200}
            className="h-9 rounded-lg bg-muted/30 text-xs shadow-none"
          />
        </div>
      ) : null}

      <div className="flex min-h-0 flex-1 flex-col">
        {isLoading ? (
          <div className="space-y-2 p-2" aria-label={t('chat.videoWorkbench.loadingRecords')}>
            {Array.from({ length: 5 }, (_, index) => (
              <div key={index} className="flex gap-3 rounded-xl p-2.5">
                <div className="h-[72px] w-[92px] shrink-0 animate-pulse rounded-lg bg-muted" />
                <div className="min-w-0 flex-1 space-y-2 py-1">
                  <div className="h-4 w-4/5 animate-pulse rounded bg-muted" />
                  <div className="h-3 w-3/5 animate-pulse rounded bg-muted" />
                  <div className="h-5 w-2/3 animate-pulse rounded bg-muted" />
                </div>
              </div>
            ))}
          </div>
        ) : isError ? (
          <div className="grid flex-1 place-items-center px-5 text-center">
            <div>
              <p className="text-sm font-medium text-foreground">
                {t('chat.videoWorkbench.recordsLoadFailed')}
              </p>
              <Button type="button" variant="outline" className="mt-3 h-8" onClick={onRefresh}>
                {t('chat.videoWorkbench.retry')}
              </Button>
            </div>
          </div>
        ) : tasks.length === 0 ? (
          <div className="grid flex-1 place-items-center px-5">
            <div className="text-center">
              <div className="mx-auto flex h-11 w-11 items-center justify-center rounded-full bg-background shadow-sm">
                <Film className="h-5 w-5 text-muted-foreground" />
              </div>
              <h3 className="mt-4 text-sm font-semibold text-foreground">
                {query.trim()
                  ? t('chat.videoWorkbench.searchEmptyTitle')
                  : t('chat.videoWorkbench.emptyRecordsTitle')}
              </h3>
            </div>
          </div>
        ) : (
          <div ref={listRef} className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
            {tasks.map(task => (
              <VideoTaskCard
                key={task.task_id}
                task={task}
                isSelected={task.task_id === selectedTaskId}
                isDeleting={task.task_id === deletingTaskId}
                onSelect={() => onSelectTask(task.task_id)}
                onDelete={() => onDeleteTask(task.task_id)}
              />
            ))}
            <div ref={loadMoreRef} className="flex min-h-9 items-center justify-center py-2">
              {isFetchingNextPage ? (
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  {t('chat.videoWorkbench.loadingMoreRecords')}
                </div>
              ) : hasNextPage ? (
                <Button
                  type="button"
                  variant="ghost"
                  className="h-8 text-xs text-muted-foreground"
                  onClick={() => void onLoadMore()}
                >
                  {t('chat.videoWorkbench.loadMoreRecords')}
                </Button>
              ) : total > 20 ? (
                <span className="text-[11px] text-muted-foreground">
                  {t('chat.videoWorkbench.allRecordsLoaded')}
                </span>
              ) : null}
            </div>
          </div>
        )}
      </div>
    </aside>
  );
}

function VideoTaskCard({
  task,
  isSelected,
  isDeleting,
  onSelect,
  onDelete,
}: {
  task: VideoRuntimeTask;
  isSelected: boolean;
  isDeleting: boolean;
  onSelect: () => void;
  onDelete: () => void;
}) {
  const t = useT('webapp');
  const status = getTaskDisplayStatus(task);
  const isLoadingStatus = status === 'pending' || status === 'running';
  const deleteDisabled = isDeleting || isActiveVideoTaskStatus(task.status);
  const Icon = isLoadingStatus
    ? Loader2
    : status === 'succeeded'
      ? CheckCircle2
      : status === 'failed'
        ? XCircle
        : Clock3;
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={event => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onSelect();
        }
      }}
      className={cn(
        'group relative flex w-full cursor-pointer gap-3 rounded-xl border border-transparent p-2.5 text-left',
        'transition-[background-color,border-color,transform] duration-150 ease-[cubic-bezier(0.23,1,0.32,1)] active:scale-[0.985]',
        'hover:border-border hover:bg-muted/40',
        isSelected && 'border-border-strong bg-muted/60'
      )}
    >
      <div className="relative h-[72px] w-[92px] shrink-0 overflow-hidden rounded-lg border border-white/10 bg-gradient-to-br from-slate-800 via-slate-900 to-black shadow-sm">
        {task.video_url && status === 'succeeded' ? (
          <video
            src={task.video_url}
            muted
            playsInline
            preload="metadata"
            className="h-full w-full object-cover opacity-90"
          />
        ) : (
          <Image
            src="/assets/video/default-video-cover.webp"
            alt=""
            fill
            sizes="92px"
            className="object-cover"
          />
        )}
        <div className="absolute inset-0 bg-gradient-to-t from-black/35 via-transparent to-white/5" />
        <div className="absolute inset-0 grid place-items-center text-white">
          {isLoadingStatus ? (
            <Loader2 className="h-5 w-5 animate-spin" />
          ) : status === 'failed' ? (
            <span className="grid h-8 w-8 place-items-center rounded-full bg-black/55 backdrop-blur-sm">
              <XCircle className="h-5 w-5 text-red-300" />
            </span>
          ) : task.video_url ? (
            <span className="grid h-8 w-8 place-items-center rounded-full bg-black/45 backdrop-blur-sm">
              <Play className="h-4 w-4 fill-white" />
            </span>
          ) : null}
        </div>
        <Badge
          className={cn(
            'absolute left-1.5 top-1.5 rounded-full border-white/10 bg-black/50 px-1.5 py-0.5 text-[10px] text-white backdrop-blur-sm',
            status === 'failed' && 'bg-red-600/85',
            isLoadingStatus && 'bg-amber-500/85'
          )}
        >
          <Icon className={cn('mr-1 h-2.5 w-2.5', isLoadingStatus && 'animate-spin')} />
          {t(VIDEO_STATUS_LABEL_KEYS[status])}
        </Badge>
      </div>

      <div className="min-w-0 flex-1 py-0.5">
        <div className="flex items-start justify-between gap-2">
          <p className="line-clamp-2 text-sm font-medium leading-5 text-foreground">
            {task.prompt || task.model_label || task.model}
          </p>
          <Button
            type="button"
            variant="ghost"
            isIcon
            className={cn(
              'h-7 w-7 shrink-0 rounded-full text-muted-foreground opacity-0 transition-opacity duration-150',
              'group-hover:opacity-100 group-focus-within:opacity-100 hover:!text-red-600'
            )}
            disabled={deleteDisabled}
            title={
              isActiveVideoTaskStatus(task.status)
                ? t('chat.videoWorkbench.deleteActiveRecordDisabled')
                : t('chat.videoWorkbench.deleteRecord')
            }
            onClick={event => {
              event.stopPropagation();
              if (!deleteDisabled) onDelete();
            }}
          >
            {isDeleting ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Trash2 className="h-3.5 w-3.5" />
            )}
          </Button>
        </div>
        <div className="mt-1 truncate text-[11px] text-muted-foreground">
          {task.model_label || task.model}
          <span className="px-1.5 text-border-strong">·</span>
          {formatDate(task.created_at, 'MM-DD HH:mm')}
        </div>
        <div className="mt-2 flex min-w-0 items-center gap-1.5 text-[11px] text-muted-foreground">
          {[
            task.resolution,
            task.ratio,
            task.duration_seconds
              ? `${task.duration_seconds}${t('chat.videoWorkbench.secondsSuffix')}`
              : '',
          ]
            .filter(Boolean)
            .map(value => (
              <span key={String(value)} className="rounded-md bg-muted px-1.5 py-0.5">
                {value}
              </span>
            ))}
          {task.generate_audio ? <Volume2 className="ml-auto h-3.5 w-3.5 shrink-0" /> : null}
        </div>
        {status === 'failed' ? (
          <div className="mt-1.5 truncate text-[11px] font-medium text-red-600">
            {t('chat.videoWorkbench.recordFailedHint')}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function ComposerPanel({
  prompt,
  onPromptChange,
  settings,
  generationOptions,
  onSettingsChange,
  selectedModel,
  onModelChange,
  allowedReferenceKinds,
  hasReferenceMaterials,
  maxReferenceFiles,
  referenceFiles,
  onReferenceFilesChange,
  isModelLoading,
  hasModels,
  modelError,
  isGenerating,
  onGenerate,
}: {
  prompt: string;
  onPromptChange: (value: string) => void;
  settings: VideoGenerationSettings;
  generationOptions: VideoGenerationOptions;
  onSettingsChange: (settings: VideoGenerationSettings) => void;
  selectedModel: ModelSelectorValue;
  onModelChange: (value: ModelSelectorValue) => void;
  allowedReferenceKinds: ReferenceKind[];
  hasReferenceMaterials: boolean;
  maxReferenceFiles: number;
  referenceFiles: SelectedReferenceFile[];
  onReferenceFilesChange: React.Dispatch<React.SetStateAction<SelectedReferenceFile[]>>;
  isModelLoading: boolean;
  hasModels: boolean;
  modelError: string | null;
  isGenerating: boolean;
  onGenerate: () => void;
}) {
  const t = useT('webapp');
  const submitDisabled = !prompt.trim() || !selectedModel.model || isGenerating;
  const [confirmOpen, setConfirmOpen] = React.useState(false);

  return (
    <div className="rounded-lg border border-border bg-background p-4 shadow-sm">
      <div className="flex min-h-[184px] flex-col gap-4">
        {hasReferenceMaterials ? (
          <ReferenceMaterialButton
            allowedKinds={allowedReferenceKinds}
            maxFiles={maxReferenceFiles}
            mode={settings.referenceMode}
            files={referenceFiles}
            onFilesSelected={files =>
              onReferenceFilesChange(prev => {
                const availableSlots = Math.max(0, maxReferenceFiles - prev.length);
                const accepted = files.slice(0, availableSlots);
                files.slice(availableSlots).forEach(file => URL.revokeObjectURL(file.previewUrl));
                return [...prev, ...accepted];
              })
            }
            onRemoveFile={fileId =>
              onReferenceFilesChange(prev => prev.filter(file => file.id !== fileId))
            }
          />
        ) : null}
        <Textarea
          value={prompt}
          onChange={event => onPromptChange(event.target.value)}
          placeholder={t('chat.videoWorkbench.promptPlaceholder')}
          className={cn(
            'min-h-[96px] resize-none border-0 bg-transparent px-0 py-0 text-[15px] leading-7 text-foreground shadow-none',
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
          {hasReferenceMaterials ? (
            <SpecSelectPill
              icon={<Sparkles className="h-3.5 w-3.5" />}
              value={settings.referenceMode}
              options={generationOptions.referenceModes}
              labels={{
                auto: t('chat.videoWorkbench.referenceModeAuto'),
                [VIDEO_FRAME_REFERENCE_MODE]: t('chat.videoWorkbench.referenceModeFirstLastFrame'),
              }}
              menuHint={t('chat.videoWorkbench.referenceModePriceHintShort')}
              onChange={referenceMode => onSettingsChange({ ...settings, referenceMode })}
            />
          ) : null}
          {generationOptions.audioModes.length > 1 ? (
            <SpecSelectPill
              icon={<Volume2 className="h-3.5 w-3.5" />}
              value={settings.audioMode}
              options={generationOptions.audioModes}
              labels={{
                off: t('chat.videoWorkbench.audioOff'),
                on: t('chat.videoWorkbench.audioOn'),
              }}
              onChange={audioMode => onSettingsChange({ ...settings, audioMode })}
            />
          ) : null}
          <SpecSelectPill
            icon={<SlidersHorizontal className="h-3.5 w-3.5" />}
            value={settings.aspectRatio}
            options={generationOptions.aspectRatios}
            menuHint={t('chat.videoWorkbench.aspectRatioPriceHintShort')}
            onChange={aspectRatio => onSettingsChange({ ...settings, aspectRatio })}
          />
          <SpecSelectPill
            value={settings.resolution}
            options={generationOptions.resolutions}
            menuHint={t('chat.videoWorkbench.resolutionPriceHintShort')}
            onChange={resolution => onSettingsChange({ ...settings, resolution })}
          />
          <SpecSelectPill
            icon={<Timer className="h-3.5 w-3.5" />}
            value={settings.duration}
            options={generationOptions.durations}
            suffix={t('chat.videoWorkbench.secondsSuffix')}
            menuHint={t('chat.videoWorkbench.durationPriceHintShort')}
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
          <Button
            type="button"
            className="h-9 rounded-full px-4 transition-transform duration-150 ease-[cubic-bezier(0.23,1,0.32,1)] active:scale-[0.97]"
            disabled={submitDisabled}
            onClick={() => setConfirmOpen(true)}
          >
            {isGenerating ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Video className="h-4 w-4" />
            )}
            {isGenerating ? t('chat.videoWorkbench.submitting') : t('chat.videoWorkbench.generate')}
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('chat.videoWorkbench.confirmTitle')}
        description={
          <div className="space-y-4">
            <p className="font-normal leading-6 text-muted-foreground">
              {t('chat.videoWorkbench.confirmDescription')}
            </p>
            <div className="grid grid-cols-2 gap-2 rounded-xl border border-border bg-muted/25 p-3 text-xs">
              <ConfirmationSpec
                label={t('chat.videoWorkbench.modelLabel')}
                value={selectedModel.model}
              />
              <ConfirmationSpec
                label={t('chat.videoWorkbench.duration')}
                value={`${settings.duration}${t('chat.videoWorkbench.secondsSuffix')}`}
              />
              <ConfirmationSpec
                label={t('chat.videoWorkbench.resolution')}
                value={settings.resolution}
              />
              <ConfirmationSpec
                label={t('chat.videoWorkbench.aspectRatio')}
                value={settings.aspectRatio}
              />
              <ConfirmationSpec
                label={t('chat.videoWorkbench.outputAudio')}
                value={
                  settings.audioMode === 'on'
                    ? t('chat.videoWorkbench.audioOn')
                    : t('chat.videoWorkbench.audioOff')
                }
              />
              <ConfirmationSpec
                label={t('chat.videoWorkbench.referenceMaterial')}
                value={t('chat.videoWorkbench.referenceCount', {
                  count: referenceFiles.length,
                })}
              />
            </div>
            <div className="flex items-start gap-2 rounded-lg bg-amber-500/10 px-3 py-2.5 text-xs font-normal leading-5 text-amber-800 dark:text-amber-200">
              <Coins className="mt-0.5 h-4 w-4 shrink-0" />
              {t('chat.videoWorkbench.confirmCostNotice')}
            </div>
          </div>
        }
        confirmText={t('chat.videoWorkbench.confirmAction')}
        cancelText={t('chat.videoWorkbench.cancelAction')}
        onConfirm={onGenerate}
        loading={isGenerating}
        confirmDisabled={submitDisabled}
        contentClassName="max-w-md rounded-2xl"
        titleClassName="text-lg font-semibold"
        descriptionClassName="font-normal"
        footerClassName="bg-background"
        cancelClassName="h-9 rounded-full px-5 text-sm"
        confirmClassName="h-9 rounded-full px-5 text-sm"
      />
    </div>
  );
}

function TaskDetailSheet({
  task,
  onOpenChange,
}: {
  task: VideoRuntimeTask | null;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT('webapp');
  const status = task ? getTaskDisplayStatus(task) : normalizeStatus('');

  return (
    <Sheet open={!!task} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-[420px] max-w-[calc(100vw-1rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-[420px]"
      >
        {task ? (
          <>
            <SheetHeader className="border-b border-border px-5 py-4 pr-12">
              <SheetTitle className="truncate text-base">
                {task.model_label || task.model}
              </SheetTitle>
              <SheetDescription className="truncate font-mono text-xs">
                {task.task_id}
              </SheetDescription>
            </SheetHeader>

            <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
              <div className="space-y-4">
                {task.video_url ? (
                  <video
                    className="aspect-video w-full rounded-lg border border-border bg-black object-contain"
                    controls
                    src={task.video_url}
                  />
                ) : (
                  <div className="relative aspect-video w-full overflow-hidden rounded-lg border border-border bg-[#030711]">
                    <Image
                      src="/assets/video/default-video-cover.webp"
                      alt=""
                      fill
                      sizes="380px"
                      className="object-cover"
                    />
                  </div>
                )}

                <div className="flex flex-wrap items-center gap-2">
                  <StatusBadge status={status} />
                  {task.resolution ? <MetaPill>{task.resolution}</MetaPill> : null}
                  {task.ratio ? <MetaPill>{task.ratio}</MetaPill> : null}
                  {task.duration_seconds ? (
                    <MetaPill>
                      {task.duration_seconds}
                      {t('chat.videoWorkbench.secondsSuffix')}
                    </MetaPill>
                  ) : null}
                  {task.generate_audio ? (
                    <MetaPill>{task.voice || t('chat.videoWorkbench.outputAudio')}</MetaPill>
                  ) : null}
                  {taskHasVideoInput(task) ? (
                    <MetaPill>{t('chat.videoWorkbench.inputVideo')}</MetaPill>
                  ) : null}
                </div>

                <DetailSection title={t('chat.videoWorkbench.promptLabel')}>
                  <p className="whitespace-pre-wrap text-sm leading-6 text-foreground">
                    {task.prompt}
                  </p>
                </DetailSection>

                <div className="grid grid-cols-2 gap-3">
                  <DetailMetric
                    icon={<Coins className="h-4 w-4" />}
                    label={t('chat.videoWorkbench.deductedCredits')}
                    value={formatCredit(videoTaskDeductedCredits(task))}
                  />
                  <DetailMetric
                    icon={<CalendarClock className="h-4 w-4" />}
                    label={t('chat.videoWorkbench.createdAt')}
                    value={formatDate(task.created_at, 'MM-DD HH:mm')}
                  />
                  <DetailMetric
                    icon={<CalendarClock className="h-4 w-4" />}
                    label={t('chat.videoWorkbench.completedAt')}
                    value={formatDate(task.completed_at || task.updated_at, 'MM-DD HH:mm')}
                  />
                </div>

                {task.error_message && status === 'failed' ? (
                  <DetailSection title={t('chat.videoWorkbench.errorMessage')}>
                    <p className="text-sm leading-6 text-red-600">{task.error_message}</p>
                  </DetailSection>
                ) : null}
              </div>
            </div>

            <div className="flex items-center justify-between gap-2 border-t border-border p-4">
              <Button
                type="button"
                variant="outline"
                className="h-9 rounded-md"
                onClick={() => {
                  void navigator.clipboard.writeText(task.task_id);
                  toast.success(t('chat.videoWorkbench.taskIdCopied'));
                }}
              >
                <Copy className="mr-2 h-4 w-4" />
                {t('chat.videoWorkbench.copyTaskId')}
              </Button>
              {task.video_url ? (
                <Button asChild type="button" className="h-9 rounded-md">
                  <a href={task.video_url} download rel="noreferrer">
                    {t('chat.videoWorkbench.downloadVideo')}
                    <Download className="ml-2 h-4 w-4" />
                  </a>
                </Button>
              ) : null}
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}

function taskHasVideoInput(task: VideoRuntimeTask) {
  if (!task.has_input_video) return false;
  const payload = task.request_payload;
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return true;
  if (hasNonEmptyString(payload.video_url)) return true;
  if (Array.isArray(payload.reference_types)) {
    return payload.reference_types.some(value => String(value).trim().toLowerCase() === 'video');
  }
  return false;
}

function getTaskDisplayStatus(task: VideoRuntimeTask): NormalizedVideoStatus {
  const status = normalizeStatus(task.status);
  if (status === 'succeeded' && task.error_message && !task.video_url) return 'failed';
  return status;
}

function ConfirmationSpec({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-lg bg-background px-3 py-2 shadow-sm ring-1 ring-border/70">
      <div className="text-[11px] font-normal text-muted-foreground">{label}</div>
      <div className="mt-0.5 truncate font-medium text-foreground">{value}</div>
    </div>
  );
}

function videoTaskDeductedCredits(task: VideoRuntimeTask) {
  const status = normalizeStatus(task.status);
  if (status === 'failed' || status === 'cancelled') return task.actual_credits ?? 0;
  if (status === 'pending' || status === 'running') return task.estimated_credits;
  return task.actual_credits > 0 ? task.actual_credits : task.estimated_credits;
}

function hasNonEmptyString(value: unknown) {
  return typeof value === 'string' && value.trim() !== '';
}

function StatusBadge({ status }: { status: ReturnType<typeof normalizeStatus> }) {
  const t = useT('webapp');
  const isLoadingStatus = status === 'pending' || status === 'running';
  const Icon = isLoadingStatus
    ? Loader2
    : status === 'succeeded'
      ? CheckCircle2
      : status === 'failed'
        ? XCircle
        : Clock3;
  const statusClass =
    status === 'succeeded'
      ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700'
      : status === 'failed'
        ? 'border-red-500/30 bg-red-500/10 text-red-700'
        : 'border-amber-500/30 bg-amber-500/10 text-amber-700';

  return (
    <Badge className={cn('rounded-md px-2 py-1 text-xs', statusClass)}>
      <Icon className={cn('mr-1 h-3.5 w-3.5', isLoadingStatus && 'animate-spin')} />
      {t(VIDEO_STATUS_LABEL_KEYS[status])}
    </Badge>
  );
}

function MetaPill({ children }: { children: React.ReactNode }) {
  return (
    <span className="rounded-md border border-border bg-muted/30 px-2 py-1 text-xs text-foreground">
      {children}
    </span>
  );
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="rounded-lg border border-border bg-muted/10 p-3">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {title}
      </h3>
      {children}
    </section>
  );
}

function DetailMetric({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="rounded-lg border border-border bg-muted/10 p-3">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon}
        {label}
      </div>
      <div className="mt-2 truncate text-sm font-semibold text-foreground">{value}</div>
    </div>
  );
}

function formatCredit(value: number | null | undefined) {
  if (value === undefined || value === null) return '-';
  const normalized = normalizeAiCreditValue(value, { precision: 3 });
  if (normalized === undefined || normalized === null) return '-';
  return (
    formatAiCreditValue(normalized, {
      maximumFractionDigits: 3,
      minimumFractionDigits: 0,
    }) +
    ' ' +
    '\u70b9'
  );
}

function normalizeStatus(status: string): NormalizedVideoStatus {
  const normalized = status?.toLowerCase?.() ?? '';
  if (normalized === 'success' || normalized === 'completed' || normalized === 'done') {
    return 'succeeded';
  }
  if (normalized === 'processing' || normalized === 'in_progress') return 'running';
  if (normalized === 'error') return 'failed';
  if (normalized === 'cancelled' || normalized === 'canceled') return 'cancelled';
  if (
    normalized === 'pending' ||
    normalized === 'running' ||
    normalized === 'succeeded' ||
    normalized === 'failed'
  ) {
    return normalized;
  }
  return 'pending';
}

function isActiveVideoTaskStatus(status: string) {
  const normalized = normalizeStatus(status);
  return normalized === 'pending' || normalized === 'running';
}

function ReferenceMaterialButton({
  allowedKinds,
  maxFiles,
  mode,
  files,
  onFilesSelected,
  onRemoveFile,
}: {
  allowedKinds: ReferenceKind[];
  maxFiles: number;
  mode: string;
  files: SelectedReferenceFile[];
  onFilesSelected: (files: SelectedReferenceFile[]) => void;
  onRemoveFile: (fileId: string) => void;
}) {
  const t = useT('webapp');
  const inputRef = React.useRef<HTMLInputElement>(null);
  const referenceLabel = getReferenceUploadLabel(allowedKinds, mode, t);
  const disabled = allowedKinds.length === 0 || files.length >= maxFiles;
  const accept = allowedKinds.map(kind => REFERENCE_ACCEPT_BY_KIND[kind]).join(',');

  const handleFilesChange = React.useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const availableSlots = Math.max(0, maxFiles - files.length);
      const selectedFiles = Array.from(event.target.files ?? [])
        .slice(0, availableSlots)
        .map(file => {
          const kind = getReferenceKindFromFile(file, allowedKinds);
          if (!kind) return null;
          return {
            id: [file.name, file.size, file.lastModified, crypto.randomUUID()].join('-'),
            file,
            kind,
            previewUrl: URL.createObjectURL(file),
          };
        })
        .filter((file): file is SelectedReferenceFile => Boolean(file));

      if (selectedFiles.length > 0) {
        onFilesSelected(selectedFiles);
      }
      event.target.value = '';
    },
    [allowedKinds, files.length, maxFiles, onFilesSelected]
  );

  return (
    <div className="flex flex-wrap items-start gap-2">
      <input
        ref={inputRef}
        type="file"
        className="hidden"
        accept={accept}
        multiple={maxFiles > 1}
        disabled={disabled}
        onChange={handleFilesChange}
      />
      <button
        type="button"
        disabled={disabled}
        className={cn(
          'flex size-[90px] shrink-0 flex-col items-center justify-center gap-1 rounded-md border border-dashed border-border bg-muted/20 text-muted-foreground transition',
          disabled
            ? 'cursor-not-allowed opacity-50'
            : 'hover:border-border-strong hover:bg-muted/30 hover:text-foreground'
        )}
        onClick={() => inputRef.current?.click()}
      >
        <ImagePlus className="h-4 w-4" />
        <span className="text-[11px] font-medium leading-none">{referenceLabel}</span>
      </button>

      {files.length > 0 ? (
        <div className="flex min-w-0 flex-1 flex-wrap gap-2">
          {files.map((item, index) => (
            <div
              key={item.id}
              className="group relative flex size-[90px] shrink-0 overflow-hidden rounded-md border border-border bg-muted/20 text-xs text-foreground"
            >
              <ReferencePreview item={item} />
              {mode === VIDEO_FRAME_REFERENCE_MODE ? (
                <span className="absolute bottom-1 left-1 rounded bg-background/90 px-1.5 py-0.5 text-[10px] font-medium text-foreground shadow-sm">
                  {index === 0
                    ? t('chat.videoWorkbench.firstFrame')
                    : t('chat.videoWorkbench.lastFrame')}
                </span>
              ) : null}
              <button
                type="button"
                className="absolute right-1 top-1 rounded-full bg-background/90 p-0.5 text-muted-foreground shadow-sm hover:text-foreground"
                onClick={() => {
                  URL.revokeObjectURL(item.previewUrl);
                  onRemoveFile(item.id);
                }}
                aria-label={t('chat.videoWorkbench.removeReference')}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ReferencePreview({ item }: { item: SelectedReferenceFile }) {
  if (item.kind === 'image') {
    return <img src={item.previewUrl} alt="" className="h-full w-full object-cover" />;
  }

  if (item.kind === 'video') {
    return (
      <video
        src={item.previewUrl}
        className="h-full w-full bg-black object-cover"
        muted
        preload="metadata"
      />
    );
  }

  return (
    <div className="flex h-full w-full items-center justify-center bg-muted/30">
      <Volume2 className="h-6 w-6 text-muted-foreground" />
    </div>
  );
}

function getVideoGenerationOptions(model: ModelItem | undefined): VideoGenerationOptions {
  const videoConfig = getModelVideoConfig(model);
  const supportsAudioGeneration = supportsVideoAudioGeneration(videoConfig);

  return {
    aspectRatios: getStringOptions(
      videoConfig,
      ['aspect_ratios', 'ratios', 'ratio_options'],
      [...DEFAULT_VIDEO_GENERATION_OPTIONS.aspectRatios]
    ),
    durations: getDurationOptions(videoConfig),
    resolutions: getStringOptions(
      videoConfig,
      ['resolutions', 'resolution_options'],
      [...DEFAULT_VIDEO_GENERATION_OPTIONS.resolutions]
    ),
    referenceModes: getReferenceModeOptions(videoConfig),
    audioModes: supportsAudioGeneration ? [...VIDEO_AUDIO_MODES] : ['off'],
  };
}

function normalizeVideoSettings(
  settings: VideoGenerationSettings,
  options: VideoGenerationOptions
): VideoGenerationSettings {
  const supportsAudioOn = options.audioModes.includes('on');
  const preferredAudioMode = supportsAudioOn ? 'on' : DEFAULT_VIDEO_SETTINGS.audioMode;
  const next: VideoGenerationSettings = {
    aspectRatio: pickSupportedOption(settings.aspectRatio, options.aspectRatios, '1:1'),
    duration: pickSupportedOption(
      settings.duration,
      options.durations,
      DEFAULT_VIDEO_SETTINGS.duration
    ),
    resolution: pickSupportedOption(
      settings.resolution,
      options.resolutions,
      DEFAULT_VIDEO_SETTINGS.resolution
    ),
    count: DEFAULT_VIDEO_SETTINGS.count,
    referenceMode: pickSupportedOption(
      settings.referenceMode,
      options.referenceModes,
      DEFAULT_VIDEO_SETTINGS.referenceMode
    ),
    audioMode: pickSupportedOption(
      supportsAudioOn && settings.audioMode === 'off' ? preferredAudioMode : settings.audioMode,
      options.audioModes,
      preferredAudioMode
    ),
  };

  return Object.keys(next).every(
    key =>
      next[key as keyof VideoGenerationSettings] === settings[key as keyof VideoGenerationSettings]
  )
    ? settings
    : next;
}

function pickSupportedOption(value: string, options: string[], preferred: string): string {
  if (options.includes(value)) return value;

  const valueMatch = options.find(option => option.toLowerCase() === value.toLowerCase());
  if (valueMatch) return valueMatch;

  const preferredMatch = options.find(option => option.toLowerCase() === preferred.toLowerCase());
  return preferredMatch ?? options[0] ?? '';
}

function getDurationOptions(videoConfig: Record<string, unknown> | null): string[] {
  const durationValue = getNestedValue(videoConfig, ['duration']);
  const durationConfigs = Array.isArray(durationValue) ? durationValue : [durationValue];

  for (const duration of durationConfigs) {
    const options = getDurationOptionsFromValue(duration);
    if (options.length > 0) return options;
  }

  return [...DEFAULT_VIDEO_GENERATION_OPTIONS.durations];
}

function getDurationOptionsFromValue(duration: unknown): string[] {
  if (typeof duration === 'string' || typeof duration === 'number') {
    const value = String(duration).trim();
    return value ? [value] : [];
  }

  const durationConfig = getNestedRecord(duration, []);
  const explicit = getStringOptions(durationConfig, ['values', 'seconds', 'options'], []);
  if (explicit.length > 0) return explicit;

  if (!durationConfig) return [];

  const min = toPositiveNumber(durationConfig.min_seconds);
  const max = toPositiveNumber(durationConfig.max_seconds);
  const step = toPositiveNumber(durationConfig.step_seconds) || 1;
  if (min > 0 && max >= min) {
    const options: string[] = [];
    for (let value = min; value <= max; value += step) {
      options.push(String(value));
    }
    return options;
  }

  return [];
}

function getStringOptions(
  source: Record<string, unknown> | null,
  keys: string[],
  fallback: string[]
): string[] {
  if (!source) return fallback;

  for (const key of keys) {
    const value = source[key];
    const options = Array.isArray(value)
      ? value.map(item => String(item).trim()).filter(Boolean)
      : typeof value === 'string' || typeof value === 'number'
        ? [String(value).trim()]
        : [];
    const uniqueOptions = Array.from(new Set(options));
    if (uniqueOptions.length > 0) return uniqueOptions;
  }

  return fallback;
}

function supportsVideoAudioGeneration(videoConfig: Record<string, unknown> | null): boolean {
  if (!videoConfig) return false;

  const candidates = [
    getNestedValue(videoConfig, ['audio', 'generation']),
    getNestedValue(videoConfig, ['audio', 'generate_audio']),
    getNestedValue(videoConfig, ['audio', 'controllable']),
    getNestedValue(videoConfig, ['audio', 'enabled']),
    videoConfig.generate_audio,
    videoConfig.audio_generation,
    videoConfig.controllable_audio,
  ];

  return candidates.some(isTruthy);
}

function getReferenceModeOptions(videoConfig: Record<string, unknown> | null): string[] {
  return getStringOptions(
    videoConfig,
    ['reference_modes', 'referenceModes'],
    [...VIDEO_REFERENCE_MODES]
  );
}

function getAllowedReferenceKinds(
  model: ModelItem | undefined,
  referenceMode: string
): ReferenceKind[] {
  const kinds = getVideoReferenceKinds(model);
  if (referenceMode === VIDEO_FRAME_REFERENCE_MODE) {
    return kinds.includes('image') ? ['image'] : [];
  }
  return kinds;
}

function getMaxReferenceFiles(model: ModelItem | undefined, referenceMode: string): number {
  if (referenceMode !== VIDEO_FRAME_REFERENCE_MODE) return Number.POSITIVE_INFINITY;
  const videoConfig = getModelVideoConfig(model);
  const frameConfig = getNestedRecord(videoConfig, [VIDEO_FRAME_REFERENCE_MODE]);
  return toPositiveNumber(frameConfig?.image_max_items) || 2;
}

function getVideoReferenceKinds(model?: ModelItem): ReferenceKind[] {
  const references = getModelVideoReferences(model);
  const configuredKinds = uniqueReferenceKinds([
    ...getReferenceKindsFromReferences(references),
    ...getReferenceKindsFromModalities(model?.input_modalities),
  ]);
  return configuredKinds;
}

function getReferenceKindsFromReferences(
  references: Record<string, unknown> | null
): ReferenceKind[] {
  if (!references) return [];

  const kinds: ReferenceKind[] = [];
  if (toPositiveNumber(references.image_max_items) > 0) kinds.push('image');
  if (toPositiveNumber(references.video_max_items) > 0) kinds.push('video');
  if (toPositiveNumber(references.audio_max_items) > 0) kinds.push('audio');
  return kinds;
}

function getReferenceKindsFromModalities(modalities?: unknown[]): ReferenceKind[] {
  if (!Array.isArray(modalities)) return [];
  return modalities
    .map(getReferenceKindFromModality)
    .filter((kind): kind is ReferenceKind => Boolean(kind));
}

function getReferenceKindFromModality(modality: unknown): ReferenceKind | null {
  const normalized = normalizeModality(modality);
  if (normalized.includes('image')) return 'image';
  if (normalized.includes('video')) return 'video';
  if (normalized.includes('audio')) return 'audio';
  return null;
}

function normalizeModality(modality: unknown): string {
  return String(modality ?? '')
    .trim()
    .toLowerCase()
    .replace(/_/g, '-');
}

function uniqueReferenceKinds(kinds: ReferenceKind[]): ReferenceKind[] {
  return REFERENCE_KIND_ORDER.filter(kind => kinds.includes(kind));
}

function getModelVideoReferences(model?: ModelItem): Record<string, unknown> | null {
  const videoConfig = getModelVideoConfig(model);
  return getNestedRecord(videoConfig, ['references']);
}

function getModelVideoConfig(model?: ModelItem): Record<string, unknown> | null {
  if (!model) return null;
  const maybeModel = model as ModelItem & {
    video?: unknown;
    capabilities?: unknown;
    config?: unknown;
    default_parameters?: unknown;
    config_parameters?: unknown;
    parameters_metadata?: unknown;
    supported_parameters?: unknown;
    features?: unknown;
    parameters?: unknown;
  };

  return (
    getNestedRecord(maybeModel.video, []) ??
    getNestedRecord(maybeModel.capabilities, ['video']) ??
    getNestedRecord(maybeModel.default_parameters, ['capabilities', 'video']) ??
    getNestedRecord(maybeModel.default_parameters, ['video']) ??
    getNestedRecord(maybeModel.config, ['video']) ??
    getNestedRecord(maybeModel.config_parameters, ['video']) ??
    getNestedRecord(maybeModel.parameters_metadata, ['video']) ??
    getNestedRecord(maybeModel.supported_parameters, ['video']) ??
    getNestedRecord(maybeModel.features, ['video']) ??
    getNestedRecord(maybeModel.parameters, ['video'])
  );
}

function getNestedValue(value: unknown, path: string[]): unknown {
  let current = value;
  for (const key of path) {
    if (!current || typeof current !== 'object') return undefined;
    current = (current as Record<string, unknown>)[key];
  }
  return current;
}

function getNestedRecord(value: unknown, path: string[]): Record<string, unknown> | null {
  let current = value;
  for (const key of path) {
    if (!current || typeof current !== 'object') return null;
    current = (current as Record<string, unknown>)[key];
  }
  return current && typeof current === 'object' && !Array.isArray(current)
    ? (current as Record<string, unknown>)
    : null;
}

function toPositiveNumber(value: unknown): number {
  const num = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(num) && num > 0 ? num : 0;
}

function isTruthy(value: unknown): boolean {
  return value === true || value === 'true' || value === 1 || value === '1';
}

function getReferenceKindFromFile(file: File, allowedKinds: ReferenceKind[]): ReferenceKind | null {
  const kind = allowedKinds.find(candidate => file.type.startsWith(candidate + '/'));
  return kind ?? null;
}

async function uploadVideoReferenceFile(file: File, missingUrlMessage: string): Promise<string> {
  const uploaded = await uploadService.uploadSingle(file, {
    is_temporary: true,
    processing_mode: 'store_only',
  });
  const directUrl = firstUploadURL(uploaded);
  if (directUrl) return directUrl;

  const preview = await fileManageService.getOriginalPreviewUrl(uploaded.id);
  const previewUrl = preview.data?.url?.trim();
  if (previewUrl) return previewUrl;

  throw new Error(missingUrlMessage);
}

function firstUploadURL(uploaded: UploadResponse): string {
  return uploaded.source_url?.trim() || uploaded.url?.trim() || uploaded.download_url?.trim() || '';
}

function getReferenceUploadLabel(
  kinds: ReferenceKind[],
  mode: string,
  t: ReturnType<typeof useT<'webapp'>>
): string {
  if (kinds.length === 0) return t('chat.videoWorkbench.referenceTextOnly');
  if (mode === VIDEO_FRAME_REFERENCE_MODE) {
    return t('chat.videoWorkbench.referenceFirstLastFrameMaterial');
  }
  return formatReferenceKinds(kinds, t);
}

function formatReferenceKinds(
  kinds: ReferenceKind[],
  t: ReturnType<typeof useT<'webapp'>>
): string {
  return kinds.map(kind => t(REFERENCE_KIND_LABEL_KEYS[kind])).join('/');
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
  menuHint,
  onChange,
}: {
  icon?: React.ReactNode;
  value: string;
  options: readonly string[];
  labels?: Record<string, string>;
  suffix?: string;
  menuHint?: string;
  onChange: (value: string) => void;
}) {
  const display = labels?.[value] ?? (suffix ? value + suffix : value);

  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-8 w-auto min-w-0 rounded-md border-border bg-background px-2 text-xs font-medium text-foreground hover:bg-muted/40 [&>div]:w-auto [&>div]:grow-0">
        <div className="flex items-center gap-1.5">
          {icon}
          <SelectValue>{display}</SelectValue>
        </div>
      </SelectTrigger>
      <SelectContent>
        {menuHint ? (
          <div className="px-8 py-1.5 text-xs text-muted-foreground">{menuHint}</div>
        ) : null}
        {options.map(option => {
          const label = labels?.[option] ?? (suffix ? option + suffix : option);

          return (
            <SelectItem key={option} value={option}>
              {label}
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}
