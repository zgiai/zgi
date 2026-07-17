import { http } from '@/lib/http';
import type {
  ImageRuntimeGenerateRequest,
  ImageRuntimeGenerateResponse,
  ImageRuntimeModelsResponse,
} from './types/image-runtime';

const IMAGE_RUNTIME_BASE_PATH = '/console/api/image-runtime';
const IMAGE_RUNTIME_GENERATE_TIMEOUT_MS = 500000;

export const ImageRuntimeService = {
  listModels() {
    return http.get<ImageRuntimeModelsResponse>(`${IMAGE_RUNTIME_BASE_PATH}/models`);
  },

  generate(payload: ImageRuntimeGenerateRequest, signal?: AbortSignal) {
    return http.post<ImageRuntimeGenerateResponse>(`${IMAGE_RUNTIME_BASE_PATH}/generate`, payload, {
      signal,
      timeout: IMAGE_RUNTIME_GENERATE_TIMEOUT_MS,
    });
  },
};
