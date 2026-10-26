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
  DigitalHumanTaskReport,
  DigitalHumanTaskReportList,
  DigitalHumanTaskReportParams,
  ReviewDigitalHumanTaskReportRequest,
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

  listTaskRuns(webAppId: string, taskId: string, page = 1): Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskRun>>> {
    return createDigitalHumanWebAppHttp(webAppId).get(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks/${encodeURIComponent(taskId)}/runs`,
      { params: { page } }
    );
  },

  listTaskActivities(webAppId: string, taskId: string, page = 1): Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskActivity>>> {
    return createDigitalHumanWebAppHttp(webAppId).get(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/tasks/${encodeURIComponent(taskId)}/activities`,
      { params: { page } }
    );
  },
  listTaskReports(webAppId: string, taskId: string): Promise<ApiResponseData<DigitalHumanTaskReportList>> {
    return createDigitalHumanWebAppHttp(webAppId).get(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/task-reports`,
      { params: { task_id: taskId } }
    );
  },
  // The reports of all the visitor's tasks, filtered like the console feed.
  listTaskReportFeed(webAppId: string, params: DigitalHumanTaskReportParams): Promise<ApiResponseData<DigitalHumanTaskReportList>> {
    return createDigitalHumanWebAppHttp(webAppId).get(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/task-reports`,
      { params }
    );
  },
  reviewTaskReport(
    webAppId: string,
    reportId: string,
    request: ReviewDigitalHumanTaskReportRequest
  ): Promise<ApiResponseData<DigitalHumanTaskReport>> {
    return createDigitalHumanWebAppHttp(webAppId).post(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/task-reports/${encodeURIComponent(reportId)}/review`,
      request
    );
  },
  downloadTaskReportFile(webAppId: string, reportId: string, fileId: string, signal?: AbortSignal): Promise<Blob> {
    return createDigitalHumanWebAppHttp(webAppId).get<Blob>(
      `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}/task-reports/${encodeURIComponent(reportId)}/files/${encodeURIComponent(fileId)}`,
      { responseType: 'blob', signal }
    );
  },
};
