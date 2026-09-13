import { createDigitalHumanWebAppHttp, http } from '@/lib/http';
import type { ApiResponseData } from '@/services/types/common';
import type {
  CreateDigitalHumanTaskRequest,
  DigitalHumanTask,
  DigitalHumanTaskActivity,
  DigitalHumanTaskList,
  DigitalHumanTaskPage,
  DigitalHumanTaskRun,
  DigitalHumanWebAppConfig,
  UpdateDigitalHumanTaskRequest,
} from '@/services/types/digital-human';

export const digitalHumanWebAppService = {
  getConfig(webAppId: string): Promise<ApiResponseData<DigitalHumanWebAppConfig>> {
    return http.get(`/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/config`, {
      skipAuth: true,
      skipErrorHandling: true,
    });
  },

  listTasks(webAppId: string, params?: Record<string, unknown>): Promise<ApiResponseData<DigitalHumanTaskList>> {
    return createDigitalHumanWebAppHttp(webAppId).get(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks`,
      { params }
    );
  },

  createTask(webAppId: string, data: CreateDigitalHumanTaskRequest): Promise<ApiResponseData<DigitalHumanTask>> {
    return createDigitalHumanWebAppHttp(webAppId).post(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks`,
      data
    );
  },

  getTask(webAppId: string, taskId: string): Promise<ApiResponseData<DigitalHumanTask>> {
    return createDigitalHumanWebAppHttp(webAppId).get(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks/${encodeURIComponent(taskId)}`
    );
  },

  updateTask(webAppId: string, taskId: string, data: UpdateDigitalHumanTaskRequest): Promise<ApiResponseData<DigitalHumanTask>> {
    return createDigitalHumanWebAppHttp(webAppId).patch(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks/${encodeURIComponent(taskId)}`,
      data
    );
  },

  applyTaskAction(webAppId: string, taskId: string, action: string, expectedRevision: number): Promise<ApiResponseData<DigitalHumanTask>> {
    return createDigitalHumanWebAppHttp(webAppId).post(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks/${encodeURIComponent(taskId)}/actions/${encodeURIComponent(action)}`,
      { expected_revision: expectedRevision }
    );
  },

  listTaskRuns(webAppId: string, taskId: string): Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskRun>>> {
    return createDigitalHumanWebAppHttp(webAppId).get(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks/${encodeURIComponent(taskId)}/runs`
    );
  },

  listTaskActivities(webAppId: string, taskId: string): Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskActivity>>> {
    return createDigitalHumanWebAppHttp(webAppId).get(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks/${encodeURIComponent(taskId)}/activities`
    );
  },
};
