import type { ApiResponseData } from './common';

export interface ImageRuntimeModel {
  provider: string;
  model: string;
  model_label: string;
  supported_sizes: string[];
  supported_counts: number[];
  default_size: string;
  default_count: number;
}

export interface ImageRuntimeGenerateRequest {
  prompt: string;
  provider: string;
  model: string;
  size: string;
  count: number;
  conversation_id?: string;
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
  files: ImageRuntimeFile[];
  status: 'succeeded';
}

export interface ImageRuntimeGenerateResult {
  conversation_id: string;
  message_id: string;
  message: string;
  image_generation: ImageRuntimeGeneration;
}

export type ImageRuntimeModelsResponse = ApiResponseData<ImageRuntimeModel[]>;
export type ImageRuntimeGenerateResponse = ApiResponseData<ImageRuntimeGenerateResult>;
