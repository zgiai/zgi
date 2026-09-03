import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from '@/services/types/common';
import type {
  ConnectorCatalogItem,
  ConnectorOnboardingFlow,
  ConnectorOnboardingRequest,
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

  reconnectConnection(
    connectionId: string,
    data: ReconnectKeyRequest,
    idempotencyKey: string
  ): Promise<ApiResponseData<KeySetupResult>> {
    return this.request(
      'post',
      `/connector-connections/${encodeURIComponent(connectionId)}/reconnect`,
      data,
      { headers: { 'Idempotency-Key': idempotencyKey }, retryAttemptsOverride: 0 }
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
   * The request intentionally has no Connector input, Key, policy, or reusable grant fields.
   */
  decideRuntimeConfirmation(
    data: ConnectorRuntimeConfirmationCommand
  ): Promise<ApiResponseData<ConnectorRuntimeConfirmationResponse>> {
    return this.request('post', '/connectors/runtime/confirmations', data, {
      retryAttemptsOverride: 0,
    });
  }

  startCredentialSetup(
    connectorId: string,
    data: KeySetupRequest,
    idempotencyKey: string
  ): Promise<ApiResponseData<KeySetupResult>> {
    return this.request(
      'post',
      `/connectors/${encodeURIComponent(connectorId)}/credential-setups`,
      data,
      {
        headers: { 'Idempotency-Key': idempotencyKey },
        retryAttemptsOverride: 0,
      }
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
    receipt: string
  ): Promise<ApiResponseData<ConnectorRuntimeStatusResponse>> {
    return this.request(
      'get',
      `/connectors/runtime/confirmations/${encodeURIComponent(receipt)}`,
      undefined,
      { retryAttemptsOverride: 0 }
    );
  }
}

export const connectorService = new ConnectorService();
