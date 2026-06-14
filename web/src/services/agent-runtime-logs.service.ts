import { BaseService } from '@/lib/http/services';
import { sanitizeModelOutputValue } from '@/utils/model-output-filter';
import type { ApiResponseData } from './types/common';
import type {
  AgentRuntimeRunDetail,
  AgentRuntimeRunsList,
  AgentRuntimeRunsQuery,
  AgentRuntimeStep,
} from './types/agent-runtime-log';

function sanitizeAgentRuntimeDetailResponse(
  response: ApiResponseData<AgentRuntimeRunDetail>
): ApiResponseData<AgentRuntimeRunDetail> {
  return {
    ...response,
    data: {
      ...response.data,
      answer: sanitizeModelOutputValue(response.data.answer) as string,
    },
  };
}

function sanitizeAgentRuntimeStepsResponse(
  response: ApiResponseData<{ data: AgentRuntimeStep[] }>
): ApiResponseData<{ data: AgentRuntimeStep[] }> {
  return {
    ...response,
    data: {
      ...response.data,
      data: response.data.data.map(step => ({
        ...step,
        output: sanitizeModelOutputValue(step.output),
      })),
    },
  };
}

export class AgentRuntimeLogsService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  async getRuntimeRuns(
    agentId: string,
    params?: AgentRuntimeRunsQuery
  ): Promise<ApiResponseData<AgentRuntimeRunsList>> {
    return this.request<ApiResponseData<AgentRuntimeRunsList>>(
      'get',
      `/agents/${agentId}/runtime-runs`,
      undefined,
      { params }
    );
  }

  async getRuntimeRunDetail(
    agentId: string,
    messageId: string
  ): Promise<ApiResponseData<AgentRuntimeRunDetail>> {
    const response = await this.request<ApiResponseData<AgentRuntimeRunDetail>>(
      'get',
      `/agents/${agentId}/runtime-runs/${messageId}`
    );
    return sanitizeAgentRuntimeDetailResponse(response);
  }

  async getRuntimeRunSteps(
    agentId: string,
    messageId: string
  ): Promise<ApiResponseData<{ data: AgentRuntimeStep[] }>> {
    const response = await this.request<ApiResponseData<{ data: AgentRuntimeStep[] }>>(
      'get',
      `/agents/${agentId}/runtime-runs/${messageId}/steps`
    );
    return sanitizeAgentRuntimeStepsResponse(response);
  }
}

export const agentRuntimeLogsService = new AgentRuntimeLogsService();
