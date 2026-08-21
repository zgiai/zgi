import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  CreateDigitalHumanRequest,
  CreateDigitalHumanRoleTemplateRequest,
  DigitalHumanDetail,
  DigitalHumanChannelBindingView,
  DigitalHumanChannelDelivery,
  DigitalHumanChannelConversationDiscovery,
  DigitalHumanChannelInboundEvent,
  DigitalHumanChannelInstallation,
  DigitalHumanChannelPage,
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
  CreateDigitalHumanChannelBindingRequest,
  CreateDigitalHumanChannelInstallationRequest,
  PublishDigitalHumanRequest,
  UpdateDigitalHumanDraftRequest,
  UpdateDigitalHumanChannelInstallationRequest,
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
    return this.request('post', `/digital-humans/${id}/memory/operations/${operationId}/undo`, {
      expected_revision: expectedRevision,
    });
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

  listChannelInstallations(
    id: string
  ): Promise<ApiResponseData<DigitalHumanChannelInstallation[]>> {
    return this.request('get', `/digital-humans/${id}/channels/installations`);
  }

  createChannelInstallation(
    id: string,
    data: CreateDigitalHumanChannelInstallationRequest
  ): Promise<ApiResponseData<DigitalHumanChannelInstallation>> {
    return this.request('post', `/digital-humans/${id}/channels/installations`, data);
  }

  updateChannelInstallation(
    id: string,
    installationId: string,
    data: UpdateDigitalHumanChannelInstallationRequest
  ): Promise<ApiResponseData<DigitalHumanChannelInstallation>> {
    return this.request(
      'patch',
      `/digital-humans/${id}/channels/installations/${installationId}`,
      data
    );
  }

  deleteChannelInstallation(
    id: string,
    installationId: string,
    removeBindings = false
  ): Promise<ApiResponseData<unknown>> {
    const query = removeBindings ? '?remove_bindings=true' : '';
    return this.request(
      'delete',
      `/digital-humans/${id}/channels/installations/${installationId}${query}`
    );
  }

  listChannelBindings(id: string): Promise<ApiResponseData<DigitalHumanChannelBindingView[]>> {
    return this.request('get', `/digital-humans/${id}/channels/bindings`);
  }

  listChannelDiscoveries(
    id: string
  ): Promise<ApiResponseData<DigitalHumanChannelConversationDiscovery[]>> {
    return this.request('get', `/digital-humans/${id}/channels/discoveries`);
  }

  createChannelBinding(
    id: string,
    data: CreateDigitalHumanChannelBindingRequest
  ): Promise<ApiResponseData<DigitalHumanChannelBindingView>> {
    return this.request('post', `/digital-humans/${id}/channels/bindings`, data);
  }

  deleteChannelBinding(id: string, bindingId: string): Promise<ApiResponseData<unknown>> {
    return this.request('delete', `/digital-humans/${id}/channels/bindings/${bindingId}`);
  }

  listChannelEvents(
    id: string,
    params?: { page?: number; page_size?: number }
  ): Promise<ApiResponseData<DigitalHumanChannelPage<DigitalHumanChannelInboundEvent>>> {
    return this.request('get', `/digital-humans/${id}/channels/events`, undefined, { params });
  }

  listChannelDeliveries(
    id: string,
    params?: { page?: number; page_size?: number }
  ): Promise<ApiResponseData<DigitalHumanChannelPage<DigitalHumanChannelDelivery>>> {
    return this.request('get', `/digital-humans/${id}/channels/deliveries`, undefined, {
      params,
    });
  }

  retryChannelDelivery(id: string, deliveryId: string): Promise<ApiResponseData<unknown>> {
    return this.request(
      'post',
      `/digital-humans/${id}/channels/deliveries/${deliveryId}/retry`,
      {}
    );
  }
}

export const digitalHumanService = new DigitalHumanService();
