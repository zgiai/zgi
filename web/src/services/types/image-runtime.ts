import type { ApiResponseData } from './common';

export interface ImageRuntimeModel {
  provider: string;
  model: string;
  model_label: string;
  generation_profile: ImageGenerationProfile;
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
  files: ImageRuntimeFile[];
  reference_image?: ImageRuntimeReferenceImage;
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
