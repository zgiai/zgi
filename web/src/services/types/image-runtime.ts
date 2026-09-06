import type { ApiResponseData } from './common';
import type { ModelImagePriceRule, ModelPricing } from './model';

export interface ImageRuntimeModel {
  provider: string;
  model: string;
  model_label: string;
  generation_profile: ImageGenerationProfile;
  currency?: string | null;
  input_price?: number | null;
  output_price?: number | null;
  input_price_configured?: boolean | null;
  output_price_configured?: boolean | null;
  pricing?: ModelPricing | null;
  image_prices?: ModelImagePriceRule[] | null;
}

export interface ImageGenerationProfile {
  size?: {
    default: string;
    options: Array<{
      value: string;
      label: string;
      aspect_ratio: string;
    }>;
  };
  quantity?: {
    mode: 'fixed' | 'exact' | 'sequence';
    default: number;
    min: number;
    max: number;
  };
}

export interface ImageRuntimeGenerateOptions {
  size?: string;
  count?: number;
  generation_mode?: 'single' | 'sequence';
  max_images?: number;
}

export interface ImageRuntimeGenerateRequest {
  prompt: string;
  provider: string;
  model: string;
  client_request_id?: string;
  options: ImageRuntimeGenerateOptions;
  conversation_id?: string;
  reference_image?: ImageRuntimeReferenceImage;
}

export interface ImageRuntimeReferenceImage {
  file_id: string;
  url?: string;
  filename?: string;
  mime_type?: string;
}

export interface ImageRuntimeFile {
  file_id: string;
  url: string;
  download_url: string;
  filename: string;
  extension: string;
  mime_type: string;
}

export interface ImageRuntimeGeneration {
  provider: string;
  model: string;
  model_label: string;
  size: string;
  count: number;
  generation_mode?: string;
  max_images?: number;
  files: ImageRuntimeFile[];
  reference_image?: ImageRuntimeReferenceImage;
  status: 'succeeded' | string;
}

export interface ImageRuntimeGenerateResult {
  conversation_id: string;
  message_id: string;
  message: string;
  image_generation: ImageRuntimeGeneration;
}

export type ImageTaskStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled' | string;

export interface ImageRuntimeTask {
  id: string;
  task_id: string;
  client_request_id?: string;
  conversation_id?: string;
  message_id?: string;
  provider: string;
  model: string;
  model_label?: string;
  prompt: string;
  status: ImageTaskStatus;
  size?: string;
  count: number;
  generation_mode?: string;
  max_images?: number;
  files?: ImageRuntimeFile[];
  reference_image?: ImageRuntimeReferenceImage;
  error_message?: string;
  request_payload?: Record<string, unknown>;
  response_payload?: Record<string, unknown>;
  image_generation?: ImageRuntimeGeneration;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

export interface ImageRuntimeTaskPage {
  data: ImageRuntimeTask[];
  total: number;
  has_more: boolean;
  next_cursor?: string;
}

export interface ImageRuntimeTasksQuery {
  limit?: number;
  cursor?: string;
  search?: string;
}

export interface ImageRuntimeCreateTaskResult {
  task: ImageRuntimeTask;
}

export type ImageRuntimeModelsResponse = ApiResponseData<ImageRuntimeModel[]>;
export type ImageRuntimeGenerateResponse = ApiResponseData<ImageRuntimeCreateTaskResult>;
export type ImageRuntimeTaskResponse = ApiResponseData<ImageRuntimeTask>;
export type ImageRuntimeTasksResponse = ApiResponseData<ImageRuntimeTaskPage>;
