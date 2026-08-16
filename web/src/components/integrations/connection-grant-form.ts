import type {
  IntegrationActionDefinition,
  IntegrationConnectionGrantAccessMode,
  IntegrationConnectionGrantPrincipalType,
} from '@/services/types/integration';

const READ_EFFECT = 'read';

export type GrantPrincipalSelection = IntegrationConnectionGrantPrincipalType | '';

/**
 * Grants use the smallest access mode that can execute every selected action.
 * Missing or unknown action metadata is treated as write-capable so a stale
 * catalog can never accidentally weaken an existing authorization.
 */
export function deriveConnectionGrantAccessMode(
  actions: IntegrationActionDefinition[],
  selectedActionIds: string[]
): IntegrationConnectionGrantAccessMode {
  const actionsById = new Map(actions.map(action => [action.id, action]));
  return selectedActionIds.every(actionId => {
    const action = actionsById.get(actionId);
    return action?.effect?.trim().toLowerCase() === READ_EFFECT;
  })
    ? 'read'
    : 'write';
}

export function isConnectionGrantFormValid(
  principalType: GrantPrincipalSelection,
  principalId: string,
  selectedActionIds: string[]
): principalType is IntegrationConnectionGrantPrincipalType {
  return (
    Boolean(principalType) &&
    selectedActionIds.length > 0 &&
    (principalType === 'organization' || Boolean(principalId.trim()))
  );
}
