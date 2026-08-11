import type { ApiResponseData } from './common';

export interface VideoRuntimeModel {
  provider: string;
  model: string;
  model_label: string;
}

export interface VideoRuntimeGenerateOptions {
  ratio?: string;
  resolution?: string;
  duration?: number;
  count?: number;
  generate_audio?: boolean;
  prompt_extend?: boolean;
  watermark?: boolean;
  voice?: string;
}

export interface VideoRuntimeGenerateRequest {
  prompt: string;
  provider: string;
  model: string;
  client_request_id?: string;
  options: VideoRuntimeGenerateOptions;
  callback_url?: string;
  reference_url?: string;
  reference_urls?: string[];
  reference_types?: string[];
  first_frame_url?: string;
  last_frame_url?: string;
}

export type VideoTaskStatus = 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled' | string;

export interface VideoRuntimeTask {
  id: string;
  task_id: string;
  upstream_task_id?: string;
  provider: string;
  model: string;
  model_label?: string;
  prompt: string;
  status: VideoTaskStatus;
  video_url?: string;
  error_message?: string;
  duration_seconds?: number;
  resolution?: string;
  ratio?: string;
  has_input_video: boolean;
  generate_audio: boolean;
  voice?: string;
  estimated_credits: number;
  actual_credits: number;
  request_payload?: Record<string, unknown>;
  response_payload?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

export interface VideoRuntimeGenerateResult {
  task: VideoRuntimeTask;
}

export interface VideoRuntimeTasksPage {
  data: VideoRuntimeTask[];
  total: number;
  has_more: boolean;
  next_cursor?: string;
}

export interface VideoRuntimeTasksQuery {
  limit?: number;
  cursor?: string;
  search?: string;
}

export type VideoRuntimeModelsResponse = ApiResponseData<VideoRuntimeModel[]>;
export type VideoRuntimeTasksResponse = ApiResponseData<VideoRuntimeTasksPage>;
export type VideoRuntimeTaskResponse = ApiResponseData<VideoRuntimeTask>;
export type VideoRuntimeGenerateResponse = ApiResponseData<VideoRuntimeGenerateResult>;
export type VideoRuntimeDeleteTaskResponse = ApiResponseData<{ deleted: boolean }>;
