export interface PermissionItem {
  code: PermissionCode;
  name: string;
  description: string;
}

export interface PermissionModule {
  key: string;
  title: string;
  permissions: PermissionItem[];
}

/**
 * All permission codes as a const array for type derivation
 */
export const ALL_PERMISSION_CODES = [
  // Workspace permissions
  'workspace.view',
  'workspace.manage',
  'workspace.billing_audit',
  'workspace.transfer_archive',
  // Agent permissions
  'agent.view',
  'agent.manage',
  'agent.lock',
  // Knowledge base permissions
  'knowledge_base.view',
  'knowledge_base.manage',
  'knowledge_base.retrieval_test',
  'knowledge_base.folder_manage',
  'knowledge_base.lock',
  // Database permissions
  'database.view',
  'database.manage',
  'database.data_edit',
  'database.ai_query',
  'database.lock',
  // File permissions
  'file.view',
  'file.manage',
  'file.upload_create',
  'file.download',
  'file.move_create',
] as const;

/**
 * Union type of all permission codes derived from the const array
 */
export type PermissionCode = (typeof ALL_PERMISSION_CODES)[number];

export const PERMISSION_MODULES: PermissionModule[] = [
  {
    key: 'workspace',
    title: 'permissions.modules.workspace',
    permissions: [
      {
        code: 'workspace.view',
        name: 'permissions.workspace.view.name',
        description: 'permissions.workspace.view.description',
      },
      {
        code: 'workspace.manage',
        name: 'permissions.workspace.manage.name',
        description: 'permissions.workspace.manage.description',
      },
      {
        code: 'workspace.billing_audit',
        name: 'permissions.workspace.billing_audit.name',
        description: 'permissions.workspace.billing_audit.description',
      },
      // {
      //   code: 'workspace.transfer_archive',
      //   name: 'permissions.workspace.transfer_archive.name',
      //   description: 'permissions.workspace.transfer_archive.description',
      // },
    ],
  },
  {
    key: 'agent',
    title: 'permissions.modules.agent',
    permissions: [
      {
        code: 'agent.view',
        name: 'permissions.agent.view.name',
        description: 'permissions.agent.view.description',
      },
      {
        code: 'agent.manage',
        name: 'permissions.agent.manage.name',
        description: 'permissions.agent.manage.description',
      },
      {
        code: 'agent.lock',
        name: 'permissions.agent.lock.name',
        description: 'permissions.agent.lock.description',
      },
    ],
  },
  {
    key: 'knowledge',
    title: 'permissions.modules.knowledge',
    permissions: [
      {
        code: 'knowledge_base.view',
        name: 'permissions.knowledge_base.view.name',
        description: 'permissions.knowledge_base.view.description',
      },
      {
        code: 'knowledge_base.manage',
        name: 'permissions.knowledge_base.manage.name',
        description: 'permissions.knowledge_base.manage.description',
      },
      {
        code: 'knowledge_base.retrieval_test',
        name: 'permissions.knowledge_base.retrieval_test.name',
        description: 'permissions.knowledge_base.retrieval_test.description',
      },
      {
        code: 'knowledge_base.folder_manage',
        name: 'permissions.knowledge_base.folder_manage.name',
        description: 'permissions.knowledge_base.folder_manage.description',
      },
      {
        code: 'knowledge_base.lock',
        name: 'permissions.knowledge_base.lock.name',
        description: 'permissions.knowledge_base.lock.description',
      },
    ],
  },
  {
    key: 'database',
    title: 'permissions.modules.database',
    permissions: [
      {
        code: 'database.view',
        name: 'permissions.database.view.name',
        description: 'permissions.database.view.description',
      },
      {
        code: 'database.manage',
        name: 'permissions.database.manage.name',
        description: 'permissions.database.manage.description',
      },
      {
        code: 'database.data_edit',
        name: 'permissions.database.data_edit.name',
        description: 'permissions.database.data_edit.description',
      },
      {
        code: 'database.ai_query',
        name: 'permissions.database.ai_query.name',
        description: 'permissions.database.ai_query.description',
      },
      {
        code: 'database.lock',
        name: 'permissions.database.lock.name',
        description: 'permissions.database.lock.description',
      },
    ],
  },
  {
    key: 'file',
    title: 'permissions.modules.file',
    permissions: [
      {
        code: 'file.view',
        name: 'permissions.file.view.name',
        description: 'permissions.file.view.description',
      },
      {
        code: 'file.manage',
        name: 'permissions.file.manage.name',
        description: 'permissions.file.manage.description',
      },
      {
        code: 'file.upload_create',
        name: 'permissions.file.upload_create.name',
        description: 'permissions.file.upload_create.description',
      },
      {
        code: 'file.download',
        name: 'permissions.file.download.name',
        description: 'permissions.file.download.description',
      },
      {
        code: 'file.move_create',
        name: 'permissions.file.move_create.name',
        description: 'permissions.file.move_create.description',
      },
    ],
  },
];
