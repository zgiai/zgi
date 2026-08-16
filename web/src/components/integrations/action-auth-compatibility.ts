import type { IntegrationActionDefinition } from '@/services/types/integration';

function normalizeAuthMethodID(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? '';
}

/**
 * Mirrors the backend ActionSupportsAuthMethod contract. Providers can leave
 * the list empty for actions that work with every authentication method.
 */
export function actionSupportsAuthMethod(
  action: Pick<IntegrationActionDefinition, 'supported_auth_method_ids'>,
  authMethodID: string | null | undefined
): boolean {
  const supported = (action.supported_auth_method_ids ?? [])
    .map(normalizeAuthMethodID)
    .filter(Boolean);
  if (supported.length === 0) return true;

  const normalizedAuthMethodID = normalizeAuthMethodID(authMethodID);
  return Boolean(normalizedAuthMethodID) && supported.includes(normalizedAuthMethodID);
}

export function actionsForAuthMethod(
  actions: IntegrationActionDefinition[],
  authMethodID: string | null | undefined
): IntegrationActionDefinition[] {
  return actions.filter(action => actionSupportsAuthMethod(action, authMethodID));
}

export function actionIDsForAuthMethod(
  actions: IntegrationActionDefinition[],
  authMethodID: string | null | undefined,
  candidateActionIDs?: string[] | null
): string[] {
  const candidates =
    candidateActionIDs == null
      ? null
      : new Set(candidateActionIDs.map(value => value.trim().toLowerCase()).filter(Boolean));

  return actionsForAuthMethod(actions, authMethodID)
    .map(action => action.id.trim().toLowerCase())
    .filter(actionID => Boolean(actionID) && (candidates == null || candidates.has(actionID)));
}
