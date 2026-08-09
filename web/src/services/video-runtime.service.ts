import { http } from '@/lib/http';
import type {
  VideoRuntimeGenerateRequest,
  VideoRuntimeGenerateResponse,
  VideoRuntimeDeleteTaskResponse,
  VideoRuntimeModelsResponse,
  VideoRuntimeTaskResponse,
  VideoRuntimeTasksResponse,
} from './types/video-runtime';

const VIDEO_RUNTIME_BASE_PATH = '/console/api/video-runtime';
const VIDEO_RUNTIME_GENERATE_TIMEOUT_MS = 120000;

export const VideoRuntimeService = {
  listModels() {
    return http.get<VideoRuntimeModelsResponse>(`${VIDEO_RUNTIME_BASE_PATH}/models`);
  },

  listTasks() {
    return http.get<VideoRuntimeTasksResponse>(`${VIDEO_RUNTIME_BASE_PATH}/tasks`);
  },

  getTask(taskId: string) {
    return http.get<VideoRuntimeTaskResponse>(
      `${VIDEO_RUNTIME_BASE_PATH}/tasks/${encodeURIComponent(taskId)}`
    );
  },

  deleteTask(taskId: string) {
    return http.delete<VideoRuntimeDeleteTaskResponse>(
      `${VIDEO_RUNTIME_BASE_PATH}/tasks/${encodeURIComponent(taskId)}`
    );
  },

  generate(payload: VideoRuntimeGenerateRequest, signal?: AbortSignal) {
    return http.post<VideoRuntimeGenerateResponse>(`${VIDEO_RUNTIME_BASE_PATH}/generate`, payload, {
      signal,
      timeout: VIDEO_RUNTIME_GENERATE_TIMEOUT_MS,
    });
  },
};
