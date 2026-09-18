import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from '@/services/types/common';
import type {
  ConnectorCatalogItem,
  ConnectorOnboardingFlow,
  ConnectorOnboardingRequest,
  ConnectorPendingSetup,
  ConnectorPendingSetupList,
  ConnectorChatPreferences,
  SetConnectorChatPreferenceRequest,
  ConnectorUnavailableReference,
  ConnectorReferenceCommand,
  ConnectorCatalogResponse,
  ConnectorRuntimeConfirmationCommand,
  ConnectorRuntimeConfirmationResponse,
  ConnectorRuntimeStatusResponse,
  ConnectorTargetBinding,
  ConnectorTargetBindingListResponse,
  ConnectorTargetRef,
  CreateConnectorTargetBindingRequest,
  DetachConnectorTargetBindingRequest,
  GetConnectorCatalogParams,
  GetPersonalConnectionsParams,
  KeySetupRequest,
  KeySetupResult,
  PersonalConnection,
  PersonalConnectionDetail,
  PersonalConnectionListResponse,
  ReconnectKeyRequest,
  RemovalImpact,
  RemovePersonalConnectionOptions,
  RenamePersonalConnectionRequest,
  ServerOpsSSHHostKeyTestRequest,
  ServerOpsSSHHostKeyTestResult,
} from '@/services/types/connector';

class ConnectorService extends BaseService {
  constructor() {
    super({ basePath: '/console/api', endpoint: 'main' });
  }

  getCatalog(
    params?: GetConnectorCatalogParams
  ): Promise<ApiResponseData<ConnectorCatalogResponse>> {
    return this.request('get', '/connectors/catalog', undefined, { params });
  }

  getConnector(connectorId: string): Promise<ApiResponseData<ConnectorCatalogItem>> {
    return this.request('get', `/connectors/catalog/${encodeURIComponent(connectorId)}`);
  }

  startKeySetup(
    connectorId: string,
    data: KeySetupRequest,
    idempotencyKey: string
  ): Promise<ApiResponseData<KeySetupResult>> {
    return this.request('post', `/connectors/${encodeURIComponent(connectorId)}/key-setups`, data, {
      headers: { 'Idempotency-Key': idempotencyKey },
      retryAttemptsOverride: 0,
    });
  }

  getKeySetup(attemptId: string): Promise<ApiResponseData<KeySetupResult>> {
    return this.request('get', `/connectors/key-setups/${encodeURIComponent(attemptId)}`);
  }

  getPendingSetups(connectorId?: string): Promise<ApiResponseData<ConnectorPendingSetupList>> {
    return this.request('get', '/connectors/pending-setups', undefined, {
      params: { connector_id: connectorId, limit: 200 },
      retryAttemptsOverride: 0,
    });
  }
  getPendingSetup(id: string): Promise<ApiResponseData<ConnectorPendingSetup>> {
    return this.request('get', `/connectors/pending-setups/${encodeURIComponent(id)}`, undefined, {
      retryAttemptsOverride: 0,
    });
  }
  cancelPendingSetup(
    id: string,
    revision: number
  ): Promise<ApiResponseData<ConnectorPendingSetup>> {
    return this.request(
      'delete',
      `/connectors/pending-setups/${encodeURIComponent(id)}`,
      undefined,
      { data: { expected_revision: revision }, retryAttemptsOverride: 0 }
    );
  }
  resumePendingSetup(
    id: string,
    revision: number,
    key: string
  ): Promise<ApiResponseData<ConnectorOnboardingFlow>> {
    return this.request(
      'post',
      `/connectors/pending-setups/${encodeURIComponent(id)}/onboarding-flows`,
      { expected_revision: revision },
      { headers: { 'Idempotency-Key': key }, retryAttemptsOverride: 0 }
    );
  }
  resumePendingManual(
    id: string,
    revision: number,
    values: Record<string, string>,
    key: string
  ): Promise<ApiResponseData<ConnectorOnboardingFlow>> {
    return this.request(
      'post',
      '/connectors/feishu/app-credential-setups',
      { pending_setup_id: id, expected_pending_revision: revision, values },
      { headers: { 'Idempotency-Key': key }, retryAttemptsOverride: 0 }
    );
  }

  startAppCredentialSetup(
    connectorId: string,
    data: KeySetupRequest,
    idempotencyKey: string
  ): Promise<ApiResponseData<KeySetupResult>> {
    return this.request(
      'post',
      `/connectors/${encodeURIComponent(connectorId)}/app-credential-setups`,
      data,
      {
        headers: { 'Idempotency-Key': idempotencyKey },
        retryAttemptsOverride: 0,
      }
    );
  }

  startOnboarding(
    connectorId: string,
    data: ConnectorOnboardingRequest,
    idempotencyKey: string,
    connectionId?: string
  ): Promise<ApiResponseData<ConnectorOnboardingFlow>> {
    const path = connectionId
      ? `/connector-connections/${encodeURIComponent(connectionId)}/onboarding-flows`
      : `/connectors/${encodeURIComponent(connectorId)}/onboarding-flows`;
    return this.request('post', path, data, {
      headers: { 'Idempotency-Key': idempotencyKey },
      retryAttemptsOverride: 0,
    });
  }

  getOnboarding(
    flowId: string,
    signal?: AbortSignal
  ): Promise<ApiResponseData<ConnectorOnboardingFlow>> {
    return this.request(
      'get',
      `/connectors/onboarding-flows/${encodeURIComponent(flowId)}`,
      undefined,
      { signal, retryAttemptsOverride: 0 }
    );
  }

  cancelOnboarding(
    flowId: string,
    revision: number
  ): Promise<ApiResponseData<ConnectorOnboardingFlow>> {
    return this.request(
      'delete',
      `/connectors/onboarding-flows/${encodeURIComponent(flowId)}`,
      undefined,
      { data: { expected_revision: revision }, retryAttemptsOverride: 0 }
    );
  }

  cancelKeySetup(attemptId: string): Promise<void> {
    return this.request('delete', `/connectors/key-setups/${encodeURIComponent(attemptId)}`);
  }

  getConnections(
    params?: GetPersonalConnectionsParams
  ): Promise<ApiResponseData<PersonalConnectionListResponse>> {
    return this.request('get', '/connector-connections', undefined, { params });
  }

  getChatPreferences(): Promise<ApiResponseData<ConnectorChatPreferences>> {
    return this.request('get', '/connector-preferences/chat/me');
  }

  setChatPreference(
    connectorId: string,
    data: SetConnectorChatPreferenceRequest,
    idempotencyKey: string
  ): Promise<ApiResponseData<ConnectorChatPreferences>> {
    return this.request(
      'put',
      `/connector-preferences/chat/me/${encodeURIComponent(connectorId)}`,
      data,
      { headers: { 'Idempotency-Key': idempotencyKey }, retryAttemptsOverride: 0 }
    );
  }

  getConnection(connectionId: string): Promise<ApiResponseData<PersonalConnectionDetail>> {
    return this.request('get', `/connector-connections/${encodeURIComponent(connectionId)}`);
  }

  renameConnection(
    connectionId: string,
    data: RenamePersonalConnectionRequest
  ): Promise<ApiResponseData<PersonalConnection>> {
    return this.request(
      'patch',
      `/connector-connections/${encodeURIComponent(connectionId)}`,
      data
    );
  }

  updateOperationPolicy(
    connectionId: string,
    data: { expected_revision: number; operation_ids: string[] },
    idempotencyKey: string
  ): Promise<
    ApiResponseData<{ connection_id: string; revision: number; operation_ids: string[] }>
  > {
    return this.request(
      'put',
      `/connector-connections/${encodeURIComponent(connectionId)}/operation-policy`,
      data,
      { headers: { 'Idempotency-Key': idempotencyKey }, retryAttemptsOverride: 0 }
    );
  }

  /**
   * Repairs an existing connection with fresh credentials. Pass
   * `sensitiveRequest` when `data` carries write-only secrets so the shared
   * HTTP client never forwards the submitted payload to error reporting.
   */
  reconnectConnection(
    connectionId: string,
    data: ReconnectKeyRequest,
    idempotencyKey: string,
    sensitiveRequest = false
  ): Promise<ApiResponseData<KeySetupResult>> {
    return this.request(
      'post',
      `/connector-connections/${encodeURIComponent(connectionId)}/reconnect`,
      data,
      {
        headers: { 'Idempotency-Key': idempotencyKey },
        retryAttemptsOverride: 0,
        ...(sensitiveRequest ? { skipErrorHandling: true } : {}),
      }
    );
  }

  diagnoseConnection(connectionId: string): Promise<ApiResponseData<PersonalConnectionDetail>> {
    return this.request(
      'get',
      `/connector-connections/${encodeURIComponent(connectionId)}/diagnostics`
    );
  }

  getRemovalImpact(connectionId: string): Promise<ApiResponseData<RemovalImpact>> {
    return this.request(
      'get',
      `/connector-connections/${encodeURIComponent(connectionId)}/removal-impact`
    );
  }

  removeConnection(
    connectionId: string,
    options: RemovePersonalConnectionOptions = {}
  ): Promise<void> {
    const headers = options.impactToken ? { 'X-Impact-Token': options.impactToken } : undefined;
    return this.request(
      'delete',
      `/connector-connections/${encodeURIComponent(connectionId)}`,
      undefined,
      {
        params: { force: options.force ?? false },
        ...(headers ? { headers } : {}),
      }
    );
  }

  getTargetBindings(
    target: ConnectorTargetRef
  ): Promise<ApiResponseData<ConnectorTargetBindingListResponse>> {
    return this.request('get', '/connector-target-bindings', undefined, {
      params: {
        target_type: target.type,
        target_id: target.id,
        target_scope: target.scope,
        target_version_id: target.version_id,
      },
    });
  }

  getUnavailableReferences(
    target: ConnectorTargetRef
  ): Promise<ApiResponseData<{ items: ConnectorUnavailableReference[] }>> {
    return this.request('get', '/connector-unavailable-references', undefined, {
      params: {
        target_type: target.type,
        target_id: target.id,
        target_scope: target.scope,
        target_version_id: target.version_id,
      },
    });
  }

  resolveUnavailableReference(
    target: ConnectorTargetRef,
    command: ConnectorReferenceCommand
  ): Promise<ApiResponseData<{ status: string; binding?: ConnectorTargetBinding }>> {
    if (target.type !== 'chat' || target.scope === 'published') {
      return Promise.reject(
        new Error('Connector reference repair must be included in the host draft save.')
      );
    }
    return this.request(
      'post',
      `/connector-unavailable-references/${encodeURIComponent(command.reference_id)}/resolve`,
      { target, ...command },
      { retryAttemptsOverride: 0 }
    );
  }

  createTargetBinding(
    data: CreateConnectorTargetBindingRequest,
    idempotencyKey: string
  ): Promise<ApiResponseData<ConnectorTargetBinding>> {
    return this.request('post', '/connector-target-bindings', data, {
      headers: { 'Idempotency-Key': idempotencyKey },
    });
  }

  detachTargetBinding(
    bindingId: string,
    data: DetachConnectorTargetBindingRequest
  ): Promise<ApiResponseData<ConnectorTargetBinding>> {
    return this.request(
      'delete',
      `/connector-target-bindings/${encodeURIComponent(bindingId)}`,
      data
    );
  }

  /**
   * Continues the server-pinned operation identified by an opaque route receipt.
   * The request carries no Connector input or Key; remember_for_session is an
   * explicit governance choice handled by the server-side grant bridge.
   */
  decideRuntimeConfirmation(
    data: ConnectorRuntimeConfirmationCommand
  ): Promise<ApiResponseData<ConnectorRuntimeConfirmationResponse>> {
    return this.request('post', '/connectors/runtime/confirmations', data, {
      retryAttemptsOverride: 0,
    });
  }

  /**
   * Creates a connection from submitted credentials. Pass `sensitiveRequest`
   * when `data` carries write-only secrets so the shared HTTP client never
   * forwards the submitted payload to error reporting.
   */
  startCredentialSetup(
    connectorId: string,
    data: KeySetupRequest,
    idempotencyKey: string,
    sensitiveRequest = false
  ): Promise<ApiResponseData<KeySetupResult>> {
    return this.request(
      'post',
      `/connectors/${encodeURIComponent(connectorId)}/credential-setups`,
      data,
      {
        headers: { 'Idempotency-Key': idempotencyKey },
        retryAttemptsOverride: 0,
        ...(sensitiveRequest ? { skipErrorHandling: true } : {}),
      }
    );
  }

  /**
   * Reads the SSH server host key without creating or mutating a setup attempt
   * or Connection. The request carries write-only secrets, so it always opts
   * out of shared error reporting, never retries, and deliberately sends no
   * Idempotency-Key.
   */
  testServerOpsSSHHostKey(
    connectorId: string,
    data: ServerOpsSSHHostKeyTestRequest
  ): Promise<ApiResponseData<ServerOpsSSHHostKeyTestResult>> {
    return this.request(
      'post',
      `/connectors/${encodeURIComponent(connectorId)}/host-key-tests`,
      data,
      { retryAttemptsOverride: 0, skipErrorHandling: true }
    );
  }

  retryOnboardingVerification(
    flow: ConnectorOnboardingFlow,
    idempotencyKey: string
  ): Promise<ApiResponseData<ConnectorOnboardingFlow>> {
    return this.request(
      'post',
      `/connectors/onboarding-flows/${encodeURIComponent(flow.id)}/actions`,
      { action: 'retry_verification', expected_revision: flow.revision },
      { headers: { 'Idempotency-Key': idempotencyKey }, retryAttemptsOverride: 0 }
    );
  }

  submitOnboardingCode(
    flow: ConnectorOnboardingFlow,
    code: string
  ): Promise<ApiResponseData<ConnectorOnboardingFlow>> {
    return this.request(
      'post',
      `/connectors/onboarding-flows/${encodeURIComponent(flow.id)}/actions`,
      {
        action: 'submit_code',
        expected_revision: flow.revision,
        challenge_revision: flow.challenge_revision,
        values: { code },
      },
      { retryAttemptsOverride: 0 }
    );
  }

  getRuntimeConfirmationStatus(
    receipt: string,
    presentationVersion?: 1
  ): Promise<ApiResponseData<ConnectorRuntimeStatusResponse>> {
    return this.request(
      'get',
      `/connectors/runtime/confirmations/${encodeURIComponent(receipt)}${presentationVersion ? '?presentation_version=1' : ''}`,
      undefined,
      { retryAttemptsOverride: 0 }
    );
  }
}

export const connectorService = new ConnectorService();
