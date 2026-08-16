import type {
  IntegrationAuthAcquisitionStrategy,
  IntegrationAuthDefinition,
  IntegrationAuthIdentityKind,
  IntegrationAuthLifecycleStrategy,
  IntegrationCredentialSource,
  IntegrationRequestAuthStrategy,
} from '@/services/types/integration';
import { integrationAuthCredentialSource } from './integration-utils';

export interface IntegrationAuthMethodPresentation {
  identityKind: IntegrationAuthIdentityKind;
  acquisitionStrategy: IntegrationAuthAcquisitionStrategy;
  lifecycleStrategy: IntegrationAuthLifecycleStrategy;
  requestAuthStrategy: IntegrationRequestAuthStrategy;
  credentialSource: IntegrationCredentialSource;
}

export function isOAuthAuthMethod(auth: IntegrationAuthDefinition): boolean {
  return (
    auth.acquisition_strategy === 'browser_redirect' ||
    auth.type === 'oauth' ||
    auth.type === 'oauth2'
  );
}

export function oauthClientConfigID(auth: IntegrationAuthDefinition): string {
  return auth.oauth?.client_config_id?.trim() || auth.id.trim();
}

export function authMethodsSharingOAuthClient(
  methods: IntegrationAuthDefinition[],
  selected: IntegrationAuthDefinition | null | undefined
): IntegrationAuthDefinition[] {
  if (!selected || !isOAuthAuthMethod(selected)) return [];
  const clientConfigID = oauthClientConfigID(selected);
  return methods.filter(
    method => isOAuthAuthMethod(method) && oauthClientConfigID(method) === clientConfigID
  );
}

export function resolveAuthMethodPresentation(
  auth: IntegrationAuthDefinition
): IntegrationAuthMethodPresentation {
  const credentialSource = integrationAuthCredentialSource(auth);
  const oauth = isOAuthAuthMethod(auth);
  const noAuth = auth.type === 'no_auth';

  return {
    credentialSource,
    identityKind:
      auth.identity_kind ??
      (oauth ? 'user' : auth.type === 'service_account' ? 'service' : 'application'),
    acquisitionStrategy:
      auth.acquisition_strategy ?? (oauth ? 'browser_redirect' : noAuth ? 'none' : 'manual_form'),
    lifecycleStrategy:
      auth.lifecycle_strategy ??
      (oauth ? 'oauth_refresh' : auth.type === 'service_account' ? 'exchange_on_demand' : 'static'),
    requestAuthStrategy:
      auth.request_auth_strategy ??
      (noAuth
        ? 'none'
        : oauth
          ? 'bearer_header'
          : auth.type === 'api_key'
            ? 'api_key_header'
            : 'provider_custom'),
  };
}

export function authMethodCanStart(
  auth: IntegrationAuthDefinition,
  canConfigureOAuthClient: boolean
): boolean {
  if (!auth.available) return false;
  if (!isOAuthAuthMethod(auth)) return true;
  if (!auth.oauth?.connect_enabled) return false;
  return Boolean(auth.oauth.client_configured || canConfigureOAuthClient);
}

function sourcePriority(source: IntegrationCredentialSource): number {
  if (source === 'account') return 0;
  if (source === 'organization') return 1;
  return 2;
}

function acquisitionPriority(strategy: IntegrationAuthAcquisitionStrategy): number {
  if (strategy === 'browser_redirect') return 0;
  if (strategy === 'manual_form') return 1;
  return 2;
}

/**
 * Selects the card's primary method without provider-specific knowledge.
 * Browser authorization is the default user experience; credential ownership
 * breaks ties so a personal method is preferred before a shared one.
 */
export function selectPrimaryAuthMethod(
  methods: IntegrationAuthDefinition[],
  canConfigureOAuthClient: boolean
): IntegrationAuthDefinition | undefined {
  const sorted = [...methods].sort((left, right) => {
    const leftPresentation = resolveAuthMethodPresentation(left);
    const rightPresentation = resolveAuthMethodPresentation(right);
    return (
      acquisitionPriority(leftPresentation.acquisitionStrategy) -
        acquisitionPriority(rightPresentation.acquisitionStrategy) ||
      sourcePriority(leftPresentation.credentialSource) -
        sourcePriority(rightPresentation.credentialSource) ||
      left.id.localeCompare(right.id)
    );
  });
  return (
    sorted.find(method => authMethodCanStart(method, canConfigureOAuthClient)) ??
    sorted.find(method => method.available)
  );
}
