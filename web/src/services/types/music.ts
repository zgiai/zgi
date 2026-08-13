export type MusicMode = 'vocal' | 'auto_lyrics' | 'instrumental';
export type MusicVariantCount = 1 | 2 | 3 | 4;

export type MusicTaskStatus =
  | 'queued'
  | 'generating_lyrics'
  | 'generating'
  | 'succeeded'
  | 'failed'
  | 'compensation_pending';

export type MusicTaskErrorCode =
  | 'queue_unavailable'
  | 'lyrics_generation_failed'
  | 'generation_failed'
  | 'delivery_failed'
  | 'delivery_unknown'
  | 'delivery_failed_refunded';

export interface CreateMusicTaskRequest {
  request_id: string;
  model: string;
  mode: MusicMode;
  prompt: string;
  lyrics?: string;
}

export interface CreateMusicTasksRequest extends Omit<CreateMusicTaskRequest, 'request_id'> {
  variant_count: MusicVariantCount;
}

export interface ListMusicTasksParams {
  page?: number;
  page_size?: number;
  search?: string;
}

export interface MusicTask {
  id: string;
  model: string;
  mode: MusicMode;
  prompt: string;
  lyrics?: string;
  title?: string;
  style_tags: string[];
  response_format: 'mp3';
  status: MusicTaskStatus;
  file_id?: string;
  url?: string;
  duration_ms: number;
  waveform_peaks: number[];
  error_code?: MusicTaskErrorCode;
  error_message?: string;
  created_at: string;
  updated_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface MusicTaskList {
  items: MusicTask[];
  total: number;
  page: number;
  page_size: number;
  has_more: boolean;
}
