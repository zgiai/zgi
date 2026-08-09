import assert from 'node:assert/strict';

import {
  getWorkspaceRoleDisplayDescription,
  getWorkspaceRoleDisplayName,
  isSelectableWorkspacePermissionTemplate,
  isWorkspaceGovernanceRole,
} from '../src/utils/workspace-role-templates.ts';
import {
  getWorkspaceRoleErrorCode,
  getWorkspaceRoleErrorTranslationKey,
} from '../src/utils/workspace-role-errors.ts';
import enUSMessages from '../src/i18n/modules/dashboard/en-US.ts';
import zhHansMessages from '../src/i18n/modules/dashboard/zh-Hans.ts';

const defaultTemplate = {
  id: 'role-default-basic',
  name: 'Basic Member',
  name_i18n: {
    en_US: 'Basic Member',
    zh_Hans: '基础成员',
  },
  description: 'Can edit workspace assets.',
  description_i18n: {
    en_US: 'Can edit workspace assets.',
    zh_Hans: '可以编辑工作区资产。',
  },
  builtin: false,
  editable: true,
  applicable: true,
  role_kind: 'permission_template',
  system_key: 'default_basic',
  template_origin: 'system_default',
  status: 'active',
  permissions: [],
};

assert.equal(getWorkspaceRoleDisplayName(defaultTemplate, 'zh-Hans'), '基础成员');
assert.equal(
  getWorkspaceRoleDisplayDescription(defaultTemplate, 'zh-Hans'),
  '可以编辑工作区资产。'
);

const renamedTemplateWithHistoricalMetadata = {
  ...defaultTemplate,
  name: 'Project Editors',
  description: 'Project-specific publishing access.',
  name_customized: true,
  description_customized: true,
};
assert.equal(
  getWorkspaceRoleDisplayName(renamedTemplateWithHistoricalMetadata, 'zh-Hans'),
  'Project Editors'
);
assert.equal(
  getWorkspaceRoleDisplayDescription(renamedTemplateWithHistoricalMetadata, 'zh-Hans'),
  'Project-specific publishing access.'
);

const renamedToAnotherLocaleDefault = {
  ...defaultTemplate,
  name: 'Advanced Member',
  description: 'Can create, edit, publish, and manage most workspace assets.',
  name_customized: true,
  description_customized: true,
};
assert.equal(
  getWorkspaceRoleDisplayName(renamedToAnotherLocaleDefault, 'zh-Hans'),
  'Advanced Member'
);
assert.equal(
  getWorkspaceRoleDisplayDescription(renamedToAnotherLocaleDefault, 'zh-Hans'),
  'Can create, edit, publish, and manage most workspace assets.'
);

const customRoleNamedAdmin = {
  ...defaultTemplate,
  id: 'role-custom-admin-name',
  name: 'Admin',
  name_i18n: undefined,
  system_key: undefined,
  template_origin: 'custom',
};
assert.equal(isWorkspaceGovernanceRole(customRoleNamedAdmin), false);
assert.equal(isSelectableWorkspacePermissionTemplate(customRoleNamedAdmin), true);

const errorCodeTranslationKeys = {
  105001: 'organization.permissions.errors.nameRequired',
  105002: 'organization.permissions.errors.nameTooLong',
  105003: 'organization.permissions.errors.descriptionTooLong',
  105004: 'organization.permissions.errors.invalidRequest',
  205022: 'organization.permissions.errors.nameExists',
  205023: 'organization.permissions.errors.reservedName',
  205024: 'organization.permissions.errors.templateInUse',
  205025: 'organization.permissions.errors.lastRemaining',
  205026: 'organization.permissions.errors.builtinImmutable',
  205027: 'organization.permissions.errors.notFound',
  205028: 'organization.permissions.errors.deleted',
  205029: 'organization.permissions.errors.ownerNotApplicable',
};

const getMessage = (messages, key) =>
  key.split('.').reduce((value, segment) => value?.[segment], messages);

for (const [code, translationKey] of Object.entries(errorCodeTranslationKeys)) {
  assert.equal(
    getWorkspaceRoleErrorTranslationKey({ businessError: { code } }),
    translationKey
  );
  assert.equal(
    getWorkspaceRoleErrorTranslationKey({ response: { data: { code: Number(code) } } }),
    translationKey
  );
  assert.equal(typeof getMessage(zhHansMessages, translationKey), 'string');
  assert.equal(typeof getMessage(enUSMessages, translationKey), 'string');
  assert.notEqual(getMessage(zhHansMessages, translationKey), '');
  assert.notEqual(getMessage(enUSMessages, translationKey), '');
}

assert.equal(getWorkspaceRoleErrorCode({ businessError: { code: 205022 } }), '205022');
assert.equal(getWorkspaceRoleErrorTranslationKey({ businessError: { code: '999999' } }), undefined);

console.log('workspace role template display, classification, and error checks passed');
