'use client';

import * as React from 'react';
import { toast } from 'sonner';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useMusicModels } from '@/hooks/music/use-music-models';
import {
  useDeleteMusicTask,
  useMusicTask,
  useMusicTasks,
} from '@/hooks/music/use-music-tasks';
import { useT } from '@/i18n';
import { API_URL } from '@/lib/config';
import { musicService } from '@/services/music.service';
import type { MusicTask } from '@/services/types/music';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { getErrorMessage } from '@/utils/error-notifications';
import { MusicComposer } from './music-composer';
import { MusicLyricsDialog } from './music-lyrics-dialog';
import { MusicPlayer } from './music-player';
import {
  resolveMusicSourcePlaybackTransition,
  shouldPrepareMusicTask,
  toMusicAssetURL,
  toMusicDownloadURL,
} from './music-task-state';
import { MusicTrackList } from './music-track-list';
import type { MusicWaveformData } from './music-waveform-data';

const PAGE_SIZE = 20;

export function MusicWorkbench() {
  const t = useT('music');
  const workspaceId = useCurrentWorkspace()?.id;
  const modelsQuery = useMusicModels();
  const models = modelsQuery.models;
  const [model, setModel] = React.useState('');
  const [selectedTaskId, setSelectedTaskId] = React.useState<string | null>(null);
  const [page, setPage] = React.useState(1);
  const [searchInput, setSearchInput] = React.useState('');
  const querySearch = React.useDeferredValue(searchInput);
  const [reuseTask, setReuseTask] = React.useState<MusicTask | null>(null);
  const [lyricsTask, setLyricsTask] = React.useState<MusicTask | null>(null);
  const [deletingTask, setDeletingTask] = React.useState<MusicTask | null>(null);
  const [pendingPlaybackId, setPendingPlaybackId] = React.useState<string | null>(null);
  const [playerTask, setPlayerTask] = React.useState<MusicTask | null>(null);
  const [playerSource, setPlayerSource] = React.useState<string | null>(null);
  const [playRequestToken, setPlayRequestToken] = React.useState(0);
  const [playRequestAction, setPlayRequestAction] = React.useState<'play' | 'toggle'>('play');
  const [seekRequestToken, setSeekRequestToken] = React.useState(0);
  const [seekRequestProgress, setSeekRequestProgress] = React.useState(0);
  const [pendingSeek, setPendingSeek] = React.useState<{
    taskId: string;
    progress: number;
  } | null>(null);
  const [playbackProgress, setPlaybackProgress] = React.useState(0);
  const [isPlaying, setIsPlaying] = React.useState(false);
  const playbackSnapshotRef = React.useRef<{
    taskId: string | null;
    source: string | null;
    playing: boolean;
    progress: number;
  }>({ taskId: null, source: null, playing: false, progress: 0 });
  const [downloadingTaskId, setDownloadingTaskId] = React.useState<string | null>(null);
  const [waveformDataByTaskId, setWaveformDataByTaskId] = React.useState<
    Record<string, MusicWaveformData>
  >({});

  const listQuery = useMusicTasks({
    page,
    page_size: PAGE_SIZE,
    search: querySearch.trim() || undefined,
  });
  const list = listQuery.data?.data;
  const detailQuery = useMusicTask(selectedTaskId);
  const selectedTask = detailQuery.data?.data;
  const deleteTaskMutation = useDeleteMusicTask();

  React.useEffect(() => {
    if (models.some(item => item.model === model)) return;
    setModel(models[0]?.model ?? '');
  }, [model, models]);

  React.useEffect(() => {
    setSelectedTaskId(null);
    setSearchInput('');
    setPage(1);
    setPendingPlaybackId(null);
    setPlayerTask(null);
    setPlayerSource(null);
    setPendingSeek(null);
    setPlaybackProgress(0);
    playbackSnapshotRef.current = { taskId: null, source: null, playing: false, progress: 0 };
    setDownloadingTaskId(null);
    setDeletingTask(null);
    setWaveformDataByTaskId({});
  }, [workspaceId]);

  React.useEffect(() => {
    if (!selectedTaskId && list?.items[0]) {
      setSelectedTaskId(list.items[0].id);
    }
  }, [list?.items, selectedTaskId]);

  React.useEffect(() => {
    if (!selectedTask || !shouldPrepareMusicTask(selectedTask.status, selectedTask.url)) return;
    const source = selectedTask.url ? toMusicAssetURL(selectedTask.url, API_URL) : null;
    if (!source) return;

    const snapshot = playbackSnapshotRef.current;
    const sameTask = snapshot.taskId !== null && snapshot.taskId === selectedTask.id;
    const sourceChanged = snapshot.source !== source;
    const transition = resolveMusicSourcePlaybackTransition(
      snapshot.taskId,
      selectedTask.id,
      snapshot.playing,
      snapshot.progress
    );
    const shouldPlay = pendingPlaybackId === selectedTask.id;
    snapshot.taskId = selectedTask.id;
    snapshot.source = source;
    snapshot.progress = transition.progress;
    if (!sameTask) snapshot.playing = false;
    setPlayerTask(selectedTask);
    setPlayerSource(source);
    if (pendingSeek?.taskId === selectedTask.id) {
      setSeekRequestProgress(pendingSeek.progress);
      setSeekRequestToken(token => token + 1);
      setPendingSeek(null);
    } else if (sameTask && sourceChanged) {
      setSeekRequestProgress(transition.progress);
      setSeekRequestToken(token => token + 1);
    }
    const shouldResume = sourceChanged && transition.shouldResume;
    if (!shouldPlay && !shouldResume) return;
    setPlayRequestAction('play');
    setPlayRequestToken(token => token + 1);
    if (shouldPlay) setPendingPlaybackId(null);
  }, [pendingPlaybackId, pendingSeek, selectedTask]);

  const handleCreated = React.useCallback((task: MusicTask) => {
    setSearchInput('');
    setPage(1);
    setPendingPlaybackId(null);
    setPendingSeek(null);
    setSelectedTaskId(task.id);
  }, []);

  function handleSelect(task: MusicTask) {
    setPendingPlaybackId(null);
    setPendingSeek(null);
    setSelectedTaskId(task.id);
  }

  function handlePlay(task: MusicTask) {
    setSelectedTaskId(task.id);
    setPendingSeek(null);
    if (playerTask?.id === task.id && playerSource) {
      setPendingPlaybackId(null);
      setPlayRequestAction('toggle');
      setPlayRequestToken(token => token + 1);
      return;
    }
    setPendingPlaybackId(task.id);
  }

  function handleTrackSeek(task: MusicTask, progress: number) {
    const normalizedProgress = Math.min(1, Math.max(0, progress));
    setSelectedTaskId(task.id);
    if (playerTask?.id === task.id && playerSource) {
      setSeekRequestProgress(normalizedProgress);
      setSeekRequestToken(token => token + 1);
      return;
    }
    setPendingSeek({ taskId: task.id, progress: normalizedProgress });
    setPendingPlaybackId(task.id);
  }

  function handleReuse(task: MusicTask) {
    setReuseTask({
      ...task,
      style_tags: [...(task.style_tags ?? [])],
      waveform_peaks: [...(task.waveform_peaks ?? [])],
    });
  }

  async function handleDownload(task: MusicTask) {
    if (downloadingTaskId) return;
    setDownloadingTaskId(task.id);
    try {
      const detail = task.url ? task : (await musicService.getTask(task.id)).data;
      const source = detail.url ? toMusicAssetURL(detail.url, API_URL) : null;
      if (!source) {
        toast.error(t('resultUnavailable'));
        return;
      }
      const link = document.createElement('a');
      link.href = toMusicDownloadURL(source);
      link.download = `${detail.title || detail.prompt || detail.id}.mp3`;
      document.body.appendChild(link);
      link.click();
      link.remove();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('downloadFailed'));
    } finally {
      setDownloadingTaskId(null);
    }
  }

  async function handleDelete() {
    const task = deletingTask;
    if (!task || deleteTaskMutation.isPending) return;
    try {
      await deleteTaskMutation.mutateAsync(task.id);
      if (selectedTaskId === task.id) setSelectedTaskId(null);
      if (reuseTask?.id === task.id) setReuseTask(null);
      if (lyricsTask?.id === task.id) setLyricsTask(null);
      if (pendingPlaybackId === task.id) setPendingPlaybackId(null);
      if (pendingSeek?.taskId === task.id) setPendingSeek(null);
      if (playerTask?.id === task.id) {
        setPlayerTask(null);
        setPlayerSource(null);
        setPlaybackProgress(0);
        setIsPlaying(false);
        playbackSnapshotRef.current = {
          taskId: null,
          source: null,
          playing: false,
          progress: 0,
        };
      }
      setWaveformDataByTaskId(current => {
        if (!(task.id in current)) return current;
        const next = { ...current };
        delete next[task.id];
        return next;
      });
      if ((list?.items.length ?? 0) === 1 && page > 1) setPage(current => current - 1);
      setDeletingTask(null);
      toast.success(t('deleteSuccess'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('deleteFailed'));
    }
  }

  const handleWaveformChange = React.useCallback((taskId: string, waveform: MusicWaveformData) => {
    setWaveformDataByTaskId(current =>
      current[taskId] === waveform ? current : { ...current, [taskId]: waveform }
    );
  }, []);

  const handlePlayingChange = React.useCallback((next: boolean) => {
    playbackSnapshotRef.current.playing = next;
    setIsPlaying(next);
  }, []);

  const handleProgressChange = React.useCallback((next: number) => {
    playbackSnapshotRef.current.progress = next;
    setPlaybackProgress(next);
  }, []);

  return (
    <div
      data-ui="music-studio"
      className="flex h-full min-h-0 flex-col overflow-hidden bg-background"
    >
      <header className="shrink-0 px-5 pt-5 lg:px-8 lg:pt-7">
        <h1 className="text-[28px] font-semibold tracking-[-0.025em]">{t('title')}</h1>
        <div className="mt-4 flex min-w-0 items-center gap-3 border-b border-border">
          <div
            data-ui="music-generation-tab"
            className="inline-flex h-11 items-center border-b-2 border-foreground px-3 text-sm font-medium"
          >
            {t('generationTab')}
          </div>
        </div>
      </header>

      <main
        data-ui="music-workbench-grid"
        className="min-h-0 flex-1 gap-5 overflow-y-auto px-4 py-4 lg:grid lg:grid-cols-[minmax(420px,500px)_minmax(520px,1fr)] lg:overflow-hidden lg:px-8 lg:py-5"
      >
        <MusicComposer
          onCreated={handleCreated}
          reuseTask={reuseTask}
          model={model}
          models={models}
          modelsLoading={modelsQuery.isLoading}
          modelsError={modelsQuery.error}
          onModelChange={setModel}
        />
        <MusicTrackList
          tasks={list?.items ?? []}
          isLoading={listQuery.isLoading}
          error={listQuery.error}
          onRetry={() => void listQuery.refetch()}
          selectedTaskId={selectedTaskId}
          playerTaskId={playerTask?.id ?? null}
          isPlaying={isPlaying}
          playbackProgress={playbackProgress}
          onSelect={handleSelect}
          onPlay={handlePlay}
          onSeek={handleTrackSeek}
          onShowLyrics={setLyricsTask}
          onReuse={handleReuse}
          onDownload={task => void handleDownload(task)}
          onDelete={setDeletingTask}
          downloadingTaskId={downloadingTaskId}
          deletingTaskId={
            deleteTaskMutation.isPending ? (deleteTaskMutation.variables ?? null) : null
          }
          searchInput={searchInput}
          onSearchChange={value => {
            setSearchInput(value);
            setPage(1);
          }}
          page={list?.page ?? page}
          hasMore={list?.has_more ?? false}
          onPageChange={setPage}
          waveformDataByTaskId={waveformDataByTaskId}
        />
      </main>

      <MusicPlayer
        task={playerTask}
        source={playerSource}
        playRequestToken={playRequestToken}
        playRequestAction={playRequestAction}
        seekRequestToken={seekRequestToken}
        seekRequestProgress={seekRequestProgress}
        onPlayingChange={handlePlayingChange}
        onProgressChange={handleProgressChange}
        onShowLyrics={setLyricsTask}
        onWaveformChange={handleWaveformChange}
      />
      <MusicLyricsDialog
        task={lyricsTask}
        open={Boolean(lyricsTask)}
        onOpenChange={open => !open && setLyricsTask(null)}
      />
      <ConfirmDialog
        open={Boolean(deletingTask)}
        onOpenChange={open => !open && setDeletingTask(null)}
        title={t('deleteTitle')}
        description={t('deleteDescription')}
        confirmText={t('deleteConfirm')}
        cancelText={t('cancel')}
        onConfirm={() => void handleDelete()}
        loading={deleteTaskMutation.isPending}
        variant="warning"
      />
    </div>
  );
}
