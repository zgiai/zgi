import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  CreateDigitalHumanRequest,
  CreateDigitalHumanPersonaTemplateRequest,
  DigitalHumanDetail,
  DigitalHumanChannelDelivery,
  DigitalHumanChannelInboundEvent,
  DigitalHumanChannelInstallation,
  DigitalHumanChannelInstallationVerification,
  DigitalHumanChannelConnectionTest,
  DigitalHumanChannelPage,
  DigitalHumanList,
  DigitalHumanPersonaTemplate,
  DigitalHumanPersonaTemplateList,
  DigitalHumanRun,
  DigitalHumanRunList,
  DigitalHumanRunRuntimeLog,
  DigitalHumanVersionList,
  DigitalHumanMemoryList,
  DigitalHumanMemoryListParams,
  DigitalHumanMemoryMutationResult,
  DigitalHumanMemoryState,
  DigitalHumanMemoryProposalRequest,
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidateList,
  DraftResourceBindings,
  ReplaceDigitalHumanResourceBindingsRequest,
  ManualDigitalHumanInvocationRequest,
  ManualDigitalHumanInvocationResponse,
  CreateDigitalHumanChannelInstallationRequest,
  PublishDigitalHumanRequest,
  RestoreDigitalHumanVersionRequest,
  DigitalHumanVersionRestorePreview,
  UpdateDigitalHumanDraftRequest,
  UpdateDigitalHumanChannelInstallationRequest,
  UpdateDigitalHumanChannelCredentialRequest,
  UpdateDigitalHumanRequest,
  UpdateDigitalHumanPersonaTemplateRequest,
  CreateDigitalHumanTaskRequest,
  UpdateDigitalHumanTaskRequest,
  DigitalHumanTask,
  DigitalHumanTaskList,
  DigitalHumanTaskRun,
  DigitalHumanTaskActivity,
  DigitalHumanTaskPage,
  DigitalHumanAccessSettings,
  DigitalHumanAPIKey,
  DigitalHumanAPIKeyCreated,
  DigitalHumanAPIKeyList,
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

  /**
   * Soft-deletes a digital human. A digital human that still has an online
   * published version must be taken offline first (see `update` with
   * `status: 'disabled'`); the API answers 409 otherwise.
   */
  delete(id: string, expectedRevision: number): Promise<ApiResponseData<{ deleted: true }>> {
    return this.request('delete', `/digital-humans/${id}`, undefined, {
      params: { expected_revision: expectedRevision },
    });
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

  getAccessSettings(id: string): Promise<ApiResponseData<DigitalHumanAccessSettings>> {
    return this.request('get', `/digital-humans/${id}/access`);
  }

  updateAccessSettings(
    id: string,
    data: Partial<DigitalHumanAccessSettings>
  ): Promise<ApiResponseData<DigitalHumanAccessSettings>> {
    return this.request('patch', `/digital-humans/${id}/access`, data);
  }

  listAPIKeys(id: string): Promise<ApiResponseData<DigitalHumanAPIKeyList>> {
    return this.request('get', `/digital-humans/${id}/api-keys`);
  }

  createAPIKey(
    id: string,
    data: { name: string; expires_at?: string }
  ): Promise<ApiResponseData<DigitalHumanAPIKeyCreated>> {
    return this.request('post', `/digital-humans/${id}/api-keys`, data);
  }

  updateAPIKey(
    id: string,
    keyId: string,
    data: { name?: string; status?: DigitalHumanAPIKey['status']; expires_at?: string }
  ): Promise<ApiResponseData<DigitalHumanAPIKey>> {
    return this.request('patch', `/digital-humans/${id}/api-keys/${keyId}`, data);
  }

  deleteAPIKey(id: string, keyId: string): Promise<void> {
    return this.request('delete', `/digital-humans/${id}/api-keys/${keyId}`);
  }

  listVersions(
    id: string,
    params?: { page?: number; page_size?: number }
  ): Promise<ApiResponseData<DigitalHumanVersionList>> {
    return this.request('get', `/digital-humans/${id}/versions`, undefined, { params });
  }

  previewVersionRestore(
    id: string,
    versionId: string
  ): Promise<ApiResponseData<DigitalHumanVersionRestorePreview>> {
    return this.request('get', `/digital-humans/${id}/versions/${versionId}/restore-preview`);
  }

  restoreVersion(
    id: string,
    data: RestoreDigitalHumanVersionRequest
  ): Promise<ApiResponseData<DigitalHumanDetail>> {
    return this.request('post', `/digital-humans/${id}/versions/restore`, data);
  }

  listPersonaTemplates(params?: {
    page?: number;
    page_size?: number;
    search?: string;
  }): Promise<ApiResponseData<DigitalHumanPersonaTemplateList>> {
    return this.request('get', '/digital-human-persona-templates', undefined, { params });
  }

  getPersonaTemplate(id: string): Promise<ApiResponseData<DigitalHumanPersonaTemplate>> {
    return this.request('get', `/digital-human-persona-templates/${id}`);
  }

  createPersonaTemplate(
    data: CreateDigitalHumanPersonaTemplateRequest
  ): Promise<ApiResponseData<DigitalHumanPersonaTemplate>> {
    return this.request('post', '/digital-human-persona-templates', data);
  }

  updatePersonaTemplate(
    id: string,
    data: UpdateDigitalHumanPersonaTemplateRequest
  ): Promise<ApiResponseData<DigitalHumanPersonaTemplate>> {
    return this.request('patch', `/digital-human-persona-templates/${id}`, data);
  }

  deletePersonaTemplate(
    id: string,
    expectedRevision: number
  ): Promise<ApiResponseData<{ deleted: true }>> {
    return this.request('delete', `/digital-human-persona-templates/${id}`, undefined, {
      params: { expected_revision: expectedRevision },
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

  /**
   * The skills a brand new digital human may bind.
   *
   * The create flow runs before any draft exists, so it cannot use
   * `listBindingCandidates`; the server applies the same eligibility gate, which
   * keeps the creation and configuration pickers in sync.
   */
  listSkillCandidates(): Promise<ApiResponseData<DigitalHumanResourceCandidateList>> {
    return this.request('get', '/digital-human-skill-candidates');
  }

  /**
   * The owner-scoped Connector connections a new digital human may attach.
   *
   * Creation happens before any draft exists, so this mirrors
   * `listSkillCandidates` without requiring an id. The server forces the
   * Connector binding type and answers with the same safe, credential-free
   * `DigitalHumanResourceCandidateList` projection as the draft editor. The
   * caller may only narrow with `query` and page with `limit` (up to the
   * server's 512 maximum).
   */
  listConnectorCandidates(params: {
    query?: string;
    limit?: number;
  } = {}): Promise<ApiResponseData<DigitalHumanResourceCandidateList>> {
    return this.request('get', '/digital-human-connector-candidates', undefined, {
      params: { query: params.query, limit: params.limit },
    });
  }

  replaceDraftBindings(
    id: string,
    expectedDraftLockVersion: number,
    items: DigitalHumanResourceBindingInput[],
    expectedDraftVersionId?: string
  ): Promise<ApiResponseData<DraftResourceBindings>> {
    const body: ReplaceDigitalHumanResourceBindingsRequest = {
      expected_draft_lock_version: expectedDraftLockVersion,
      expected_draft_version_id: expectedDraftVersionId,
      items,
    };
    return this.request('put', `/digital-humans/${id}/draft/bindings`, body);
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

  getRunRuntimeLog(id: string, runId: string): Promise<ApiResponseData<DigitalHumanRunRuntimeLog>> {
    return this.request('get', `/digital-humans/${id}/runs/${runId}/runtime-log`);
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

  updateChannelInstallationCredential(
    id: string,
    installationId: string,
    data: UpdateDigitalHumanChannelCredentialRequest
  ): Promise<ApiResponseData<DigitalHumanChannelInstallation>> {
    return this.request(
      'put',
      `/digital-humans/${id}/channels/installations/${installationId}/credential`,
      data
    );
  }

  verifyChannelInstallation(
    id: string,
    installationId: string
  ): Promise<ApiResponseData<DigitalHumanChannelInstallationVerification>> {
    return this.request(
      'post',
      `/digital-humans/${id}/channels/installations/${installationId}/verify`
    );
  }

  startChannelConnectionTest(
    id: string,
    installationId: string,
    conversationType: 'direct' | 'group'
  ): Promise<ApiResponseData<DigitalHumanChannelConnectionTest>> {
    return this.request(
      'post',
      `/digital-humans/${id}/channels/installations/${installationId}/connection-tests`,
      { conversation_type: conversationType }
    );
  }

  getChannelConnectionTest(
    id: string,
    installationId: string,
    testId: string,
    conversationType: 'direct' | 'group',
    startedAt: string
  ): Promise<ApiResponseData<DigitalHumanChannelConnectionTest>> {
    return this.request(
      'get',
      `/digital-humans/${id}/channels/installations/${installationId}/connection-tests/${testId}`,
      undefined,
      { params: { started_at: startedAt, conversation_type: conversationType } }
    );
  }

  deleteChannelInstallation(id: string, installationId: string): Promise<ApiResponseData<unknown>> {
    return this.request('delete', `/digital-humans/${id}/channels/installations/${installationId}`);
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

  dismissChannelDelivery(id: string, deliveryId: string): Promise<ApiResponseData<unknown>> {
    return this.request(
      'post',
      `/digital-humans/${id}/channels/deliveries/${deliveryId}/dismiss`,
      {}
    );
  }

  listTasks(
    id: string,
    params?: {
      page?: number;
      page_size?: number;
      query?: string;
      status?: string[];
      priority?: string[];
      schedule_type?: string;
      run_finished_after?: string;
      run_finished_before?: string;
    }
  ): Promise<ApiResponseData<DigitalHumanTaskList>> {
    return this.request('get', `/digital-humans/${id}/tasks`, undefined, { params });
  }

  listMyTasks(
    id: string,
    params?: {
      page?: number;
      page_size?: number;
      query?: string;
      status?: string[];
      priority?: string[];
      schedule_type?: string;
    }
  ): Promise<ApiResponseData<DigitalHumanTaskList>> {
    return this.request('get', `/digital-humans/${id}/my-tasks`, undefined, { params });
  }

  createMyTask(id: string, data: CreateDigitalHumanTaskRequest): Promise<ApiResponseData<DigitalHumanTask>> {
    return this.request('post', `/digital-humans/${id}/my-tasks`, data);
  }

  getMyTask(id: string, taskId: string): Promise<ApiResponseData<DigitalHumanTask>> {
    return this.request('get', `/digital-humans/${id}/my-tasks/${taskId}`);
  }

  updateMyTask(id: string, taskId: string, data: UpdateDigitalHumanTaskRequest): Promise<ApiResponseData<DigitalHumanTask>> {
    return this.request('patch', `/digital-humans/${id}/my-tasks/${taskId}`, data);
  }

  applyMyTaskAction(id: string, taskId: string, action: string, expectedRevision: number): Promise<ApiResponseData<DigitalHumanTask>> {
    return this.request('post', `/digital-humans/${id}/my-tasks/${taskId}/actions/${action}`, { expected_revision: expectedRevision });
  }

  listMyTaskRuns(id: string, taskId: string): Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskRun>>> {
    return this.request('get', `/digital-humans/${id}/my-tasks/${taskId}/runs`);
  }

  listMyTaskActivities(id: string, taskId: string): Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskActivity>>> {
    return this.request('get', `/digital-humans/${id}/my-tasks/${taskId}/activities`);
  }

  createTask(
    id: string,
    data: CreateDigitalHumanTaskRequest
  ): Promise<ApiResponseData<DigitalHumanTask>> {
    return this.request('post', `/digital-humans/${id}/tasks`, data);
  }

  getTask(id: string, taskId: string): Promise<ApiResponseData<DigitalHumanTask>> {
    return this.request('get', `/digital-humans/${id}/tasks/${taskId}`);
  }

  updateTask(
    id: string,
    taskId: string,
    data: UpdateDigitalHumanTaskRequest
  ): Promise<ApiResponseData<DigitalHumanTask>> {
    return this.request('patch', `/digital-humans/${id}/tasks/${taskId}`, data);
  }

  applyTaskAction(
    id: string,
    taskId: string,
    action: string,
    expectedRevision: number
  ): Promise<ApiResponseData<DigitalHumanTask>> {
    return this.request('post', `/digital-humans/${id}/tasks/${taskId}/actions/${action}`, {
      expected_revision: expectedRevision,
    });
  }

  listTaskRuns(
    id: string,
    taskId: string
  ): Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskRun>>> {
    return this.request('get', `/digital-humans/${id}/tasks/${taskId}/runs`);
  }

  listTaskActivities(
    id: string,
    taskId: string
  ): Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskActivity>>> {
    return this.request('get', `/digital-humans/${id}/tasks/${taskId}/activities`);
  }
}

export const digitalHumanService = new DigitalHumanService();
