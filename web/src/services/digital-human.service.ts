import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  CreateDigitalHumanRequest,
  CreateDigitalHumanRoleTemplateRequest,
  DigitalHumanDetail,
  DigitalHumanList,
  DigitalHumanRoleAssignmentInput,
  DigitalHumanRoleTemplateDetail,
  DigitalHumanRoleTemplateList,
  DigitalHumanRun,
  DigitalHumanRunList,
  DigitalHumanMemoryList,
  DigitalHumanMemoryListParams,
  DigitalHumanMemoryMutationResult,
  DigitalHumanMemoryState,
  DigitalHumanMemoryProposalRequest,
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidateList,
  DraftResourceBindings,
  DraftRoleAssignments,
  ManualDigitalHumanInvocationRequest,
  ManualDigitalHumanInvocationResponse,
  PublishDigitalHumanRequest,
  UpdateDigitalHumanDraftRequest,
  UpdateDigitalHumanRequest,
} from './types/digital-human';

class DigitalHumanService extends BaseService {
  constructor() {
    super({ basePath: '/console/api', endpoint: 'main' });
  }

  list(params?: {
    page?: number;
    page_size?: number;
    search?: string;
  }): Promise<ApiResponseData<DigitalHumanList>> {
    return this.request('get', '/digital-humans', undefined, { params });
  }

  create(data: CreateDigitalHumanRequest): Promise<ApiResponseData<DigitalHumanDetail>> {
    return this.request('post', '/digital-humans', data);
  }

  get(id: string): Promise<ApiResponseData<DigitalHumanDetail>> {
    return this.request('get', `/digital-humans/${id}`);
  }

  update(
    id: string,
    data: UpdateDigitalHumanRequest
  ): Promise<ApiResponseData<DigitalHumanDetail>> {
    return this.request('patch', `/digital-humans/${id}`, data);
  }

  updateDraft(
    id: string,
    data: UpdateDigitalHumanDraftRequest
  ): Promise<ApiResponseData<DigitalHumanDetail['draft']>> {
    return this.request('patch', `/digital-humans/${id}/draft`, data);
  }

  publish(
    id: string,
    data: PublishDigitalHumanRequest
  ): Promise<ApiResponseData<DigitalHumanDetail>> {
    return this.request('post', `/digital-humans/${id}/publish`, data);
  }

  listRoleTemplates(params?: {
    page?: number;
    page_size?: number;
    search?: string;
  }): Promise<ApiResponseData<DigitalHumanRoleTemplateList>> {
    return this.request('get', '/digital-human-role-templates', undefined, { params });
  }

  getRoleTemplate(id: string): Promise<ApiResponseData<DigitalHumanRoleTemplateDetail>> {
    return this.request('get', `/digital-human-role-templates/${id}`);
  }

  createRoleTemplate(
    data: CreateDigitalHumanRoleTemplateRequest
  ): Promise<ApiResponseData<DigitalHumanRoleTemplateDetail>> {
    return this.request('post', '/digital-human-role-templates', data);
  }

  publishRoleTemplate(
    id: string,
    data: PublishDigitalHumanRequest
  ): Promise<ApiResponseData<DigitalHumanRoleTemplateDetail>> {
    return this.request('post', `/digital-human-role-templates/${id}/publish`, data);
  }

  getDraftRoles(id: string): Promise<ApiResponseData<DraftRoleAssignments>> {
    return this.request('get', `/digital-humans/${id}/draft/roles`);
  }

  replaceDraftRoles(
    id: string,
    expectedDraftLockVersion: number,
    items: DigitalHumanRoleAssignmentInput[]
  ): Promise<ApiResponseData<DraftRoleAssignments>> {
    return this.request('put', `/digital-humans/${id}/draft/roles`, {
      expected_draft_lock_version: expectedDraftLockVersion,
      items,
    });
  }

  getDraftBindings(id: string): Promise<ApiResponseData<DraftResourceBindings>> {
    return this.request('get', `/digital-humans/${id}/draft/bindings`);
  }

  listBindingCandidates(
    id: string,
    params: {
      type: DigitalHumanResourceBindingInput['type'];
      query?: string;
      parent_resource_id?: string;
      limit?: number;
    }
  ): Promise<ApiResponseData<DigitalHumanResourceCandidateList>> {
    return this.request('get', `/digital-humans/${id}/draft/binding-candidates`, undefined, {
      params,
    });
  }

  replaceDraftBindings(
    id: string,
    expectedDraftLockVersion: number,
    items: DigitalHumanResourceBindingInput[]
  ): Promise<ApiResponseData<DraftResourceBindings>> {
    return this.request('put', `/digital-humans/${id}/draft/bindings`, {
      expected_draft_lock_version: expectedDraftLockVersion,
      items,
    });
  }

  submitInvocation(
    id: string,
    data: ManualDigitalHumanInvocationRequest
  ): Promise<ApiResponseData<ManualDigitalHumanInvocationResponse>> {
    return this.request('post', `/digital-humans/${id}/invocations`, data);
  }

  listRuns(
    id: string,
    params?: { page?: number; page_size?: number }
  ): Promise<ApiResponseData<DigitalHumanRunList>> {
    return this.request('get', `/digital-humans/${id}/runs`, undefined, { params });
  }

  getRun(id: string, runId: string): Promise<ApiResponseData<DigitalHumanRun>> {
    return this.request('get', `/digital-humans/${id}/runs/${runId}`);
  }

  cancelRun(id: string, runId: string): Promise<ApiResponseData<{ run: DigitalHumanRun }>> {
    return this.request('post', `/digital-humans/${id}/runs/${runId}/cancel`, {});
  }

  listMemories(
    id: string,
    params?: DigitalHumanMemoryListParams
  ): Promise<ApiResponseData<DigitalHumanMemoryList>> {
    return this.request('get', `/digital-humans/${id}/memory`, undefined, { params });
  }

  applyMemoryProposal(
    id: string,
    data: DigitalHumanMemoryProposalRequest
  ): Promise<ApiResponseData<DigitalHumanMemoryMutationResult>> {
    return this.request('post', `/digital-humans/${id}/memory/proposals`, data);
  }

  undoMemory(
    id: string,
    operationId: string,
    expectedRevision: number
  ): Promise<ApiResponseData<DigitalHumanMemoryMutationResult>> {
    return this.request(
      'post',
      `/digital-humans/${id}/memory/operations/${operationId}/undo`,
      { expected_revision: expectedRevision }
    );
  }

  updateMemoryState(
    id: string,
    expectedMemoryEpoch: number,
    autoExtractionEnabled: boolean
  ): Promise<ApiResponseData<DigitalHumanMemoryState>> {
    return this.request('patch', `/digital-humans/${id}/memory/state`, {
      expected_memory_epoch: expectedMemoryEpoch,
      auto_extraction_enabled: autoExtractionEnabled,
    });
  }
}

export const digitalHumanService = new DigitalHumanService();
