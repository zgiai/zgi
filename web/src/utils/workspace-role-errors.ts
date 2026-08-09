const WORKSPACE_ROLE_ERROR_TRANSLATION_KEYS = {
  '105001': 'organization.permissions.errors.nameRequired',
  '105002': 'organization.permissions.errors.nameTooLong',
  '105003': 'organization.permissions.errors.descriptionTooLong',
  '105004': 'organization.permissions.errors.invalidRequest',
  '205022': 'organization.permissions.errors.nameExists',
  '205023': 'organization.permissions.errors.reservedName',
  '205024': 'organization.permissions.errors.templateInUse',
  '205025': 'organization.permissions.errors.lastRemaining',
  '205026': 'organization.permissions.errors.builtinImmutable',
  '205027': 'organization.permissions.errors.notFound',
  '205028': 'organization.permissions.errors.deleted',
  '205029': 'organization.permissions.errors.ownerNotApplicable',
} as const;

export type WorkspaceRoleErrorTranslationKey =
  (typeof WORKSPACE_ROLE_ERROR_TRANSLATION_KEYS)[keyof typeof WORKSPACE_ROLE_ERROR_TRANSLATION_KEYS];

interface WorkspaceRoleErrorLike {
  businessError?: {
    code?: unknown;
  };
  response?: {
    data?: {
      code?: unknown;
    };
  };
}

export function getWorkspaceRoleErrorCode(error: unknown): string | undefined {
  if (!error || typeof error !== 'object') return undefined;

  const errorLike = error as WorkspaceRoleErrorLike;
  const code = errorLike.businessError?.code ?? errorLike.response?.data?.code;
  if (typeof code !== 'string' && typeof code !== 'number') return undefined;

  return String(code);
}

export function getWorkspaceRoleErrorTranslationKey(
  error: unknown
): WorkspaceRoleErrorTranslationKey | undefined {
  const code = getWorkspaceRoleErrorCode(error);
  if (!code) return undefined;

  return WORKSPACE_ROLE_ERROR_TRANSLATION_KEYS[
    code as keyof typeof WORKSPACE_ROLE_ERROR_TRANSLATION_KEYS
  ];
}
