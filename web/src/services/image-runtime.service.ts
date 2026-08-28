import { http } from '@/lib/http';
import type {
  ImageRuntimeGenerateRequest,
  ImageRuntimeGenerateResponse,
  ImageRuntimeModelsResponse,
  ImageRuntimeTaskResponse,
  ImageRuntimeTasksQuery,
  ImageRuntimeTasksResponse,
} from './types/image-runtime';

const IMAGE_RUNTIME_BASE_PATH = '/console/api/image-runtime';
const IMAGE_RUNTIME_GENERATE_TIMEOUT_MS = 120000;

export const ImageRuntimeService = {
  listModels() {
    return http.get<ImageRuntimeModelsResponse>(`${IMAGE_RUNTIME_BASE_PATH}/models`);
  },

  listTasks(params: ImageRuntimeTasksQuery = {}) {
    return http.get<ImageRuntimeTasksResponse>(`${IMAGE_RUNTIME_BASE_PATH}/tasks`, { params });
  },

  getTask(taskId: string) {
    return http.get<ImageRuntimeTaskResponse>(
      `${IMAGE_RUNTIME_BASE_PATH}/tasks/${encodeURIComponent(taskId)}`
    );
  },

  cancelTask(taskId: string) {
    return http.post<ImageRuntimeTaskResponse>(
      `${IMAGE_RUNTIME_BASE_PATH}/tasks/${encodeURIComponent(taskId)}/cancel`,
      {}
    );
  },

  generate(payload: ImageRuntimeGenerateRequest, signal?: AbortSignal) {
    return http.post<ImageRuntimeGenerateResponse>(`${IMAGE_RUNTIME_BASE_PATH}/generate`, payload, {
      signal,
      timeout: IMAGE_RUNTIME_GENERATE_TIMEOUT_MS,
    });
  },
};
