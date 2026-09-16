import type {
  ConnectorCatalogItem,
  ConnectorOnboardingFlow,
} from '../../../services/types/connector.ts';

export function supportsMailSetup(connector: ConnectorCatalogItem): boolean {
  const fields = connector.credential_fields;
  const gmail = connector.id === 'gmail';
  if (!['gmail', 'qqmail', 'netease163'].includes(connector.id)) return false;
  return (
    connector.extension_schema_version === (gmail ? '1.4.0' : '1.5.0') &&
    connector.setup?.handler ===
      (gmail ? 'gmail-app-password-v1' : `${connector.id}-authorization-code-v1`) &&
    connector.setup.methods.includes('manual') &&
    fields?.length === 2 &&
    fields.some(field => field.name === 'email' && !field.secret && field.required) &&
    fields.some(
      field =>
        field.name === (gmail ? 'app_password' : 'authorization_code') &&
        field.secret &&
        field.required
    )
  );
}

export function mailSetupEnabled(connector: ConnectorCatalogItem): boolean {
  return (
    supportsMailSetup(connector) &&
    connector.setup_availability?.enabled !== false &&
    (!connector.setup_availability || connector.setup_availability.methods.includes('manual'))
  );
}

export function supportsMessagingContract(connector: ConnectorCatalogItem): boolean {
  return (
    (connector.extension_schema_version === '1.2.0' ||
      connector.extension_schema_version === '1.3.0') &&
    ['dingtalk', 'wecom', 'weixin'].includes(connector.id) &&
    connector.setup?.handler === `${connector.id}-registration-v1` &&
    connector.setup.protocol_version === 1
  );
}
export function messagingMethod(connector: ConnectorCatalogItem, method: string): boolean {
  return (
    supportsMessagingContract(connector) &&
    connector.setup_availability?.enabled !== false &&
    (!connector.setup_availability || connector.setup_availability.methods.includes(method)) &&
    (connector.setup?.methods as string[]).includes(method)
  );
}

export function supportsAppSetup(connector: ConnectorCatalogItem): boolean {
  return (
    (connector.extension_schema_version === '1.1.0' ||
      connector.extension_schema_version === '1.3.0') &&
    connector.setup?.handler === 'feishu-app-registration-v1' &&
    connector.setup.methods?.includes('manual') === true &&
    connector.credential_fields?.length === 3
  );
}
export function activeOnboarding(flow?: ConnectorOnboardingFlow): boolean {
  return Boolean(
    flow &&
      ['starting', 'awaiting_authorization', 'verifying', 'verification_waiting'].includes(
        flow.state
      )
  );
}

/** Setup availability is not the same as a connection's verified permissions. */
export function supportsConnectorSetup(connector: ConnectorCatalogItem): boolean {
  if (
    connector.capabilities.some(
      capability => capability.unavailable_reason === 'plugin_unavailable'
    )
  ) {
    return false;
  }
  if (supportsMessagingContract(connector)) {
    return connector.setup?.methods.some(method => messagingMethod(connector, method)) === true;
  }
  if (supportsAppSetup(connector)) return true;
  if (mailSetupEnabled(connector)) return true;
  // Schema 1.3 adds host surfaces while retaining each service's credential contract.
  if (connector.extension_schema_version && connector.extension_schema_version !== '1.3.0') {
    return false;
  }
  return (
    !connector.setup &&
    !connector.credential_fields &&
    connector.key_fields.length > 0 &&
    connector.capabilities.length > 0
  );
}
export function safeVerificationURL(value?: string): string | undefined {
  if (!value || value.length > 32768) return undefined;
  try {
    const url = new URL(value);
    if (
      url.protocol !== 'https:' ||
      url.username ||
      url.password ||
      url.hash ||
      (url.port && url.port !== '443')
    ) {
      return undefined;
    }
    return [
      'accounts.feishu.cn',
      'accounts.larksuite.com',
      'open.feishu.cn',
      'open.larksuite.com',
    ].includes(url.hostname)
      ? value
      : undefined;
  } catch {
    return undefined;
  }
}
export function appCredentialErrorKey(code?: string) {
  switch (code) {
    case 'connector_app_credentials_invalid':
      return 'app.invalid' as const;
    case 'connector_provider_permission_missing':
      return 'app.permissions' as const;
    case 'connector_application_not_ready':
      return 'app.notReady' as const;
    case 'connector_rate_limited':
      return 'app.rate' as const;
    case 'connector_flow_expired':
      return 'app.expired' as const;
    case 'connector_operation_outcome_unknown':
    case 'transport_unconfirmed':
      return 'app.recovery' as const;
    default:
      return 'app.requestFailed' as const;
  }
}
