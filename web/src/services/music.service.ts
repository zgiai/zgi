import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  CreateMusicTaskRequest,
  CreateMusicTasksRequest,
  ListMusicTasksParams,
  MusicTask,
  MusicTaskList,
} from './types/music';

export interface CreateMusicTasksResult {
  responses: Array<ApiResponseData<MusicTask>>;
  failedCount: number;
}

export class MusicService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api/music' });
  }

  createTask(data: CreateMusicTaskRequest): Promise<ApiResponseData<MusicTask>> {
    return this.request('post', '/tasks', data);
  }

  async createTasks(data: CreateMusicTasksRequest): Promise<CreateMusicTasksResult> {
    const { variant_count: variantCount, ...task } = data;
    const results = await Promise.allSettled(
      Array.from({ length: variantCount }, () =>
        this.createTask({
          ...task,
          request_id: crypto.randomUUID(),
        })
      )
    );
    const responses = results.flatMap(result =>
      result.status === 'fulfilled' ? [result.value] : []
    );
    const failures = results.filter(result => result.status === 'rejected');
    if (responses.length === 0 && failures[0]) throw failures[0].reason;
    return { responses, failedCount: failures.length };
  }

  listTasks(params?: ListMusicTasksParams): Promise<ApiResponseData<MusicTaskList>> {
    return this.request('get', '/tasks', undefined, { params });
  }

  getTask(id: string): Promise<ApiResponseData<MusicTask>> {
    return this.request('get', `/tasks/${encodeURIComponent(id)}`);
  }

  deleteTask(id: string): Promise<ApiResponseData<{ deleted: boolean }>> {
    return this.request('delete', `/tasks/${encodeURIComponent(id)}`);
  }
}

export const musicService = new MusicService();
export default musicService;
