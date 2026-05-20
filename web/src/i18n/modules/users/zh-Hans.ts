import type { UsersMessages } from './en-US';

const messages: UsersMessages = {
  // User roles
  roles: {
    owner: '所有者',
    admin: '管理员',
    normal: '普通成员',
    editor: '编辑者',
  },

  // User status
  status: {
    label: '状态',
    active: '正常',
    pending: '待激活',
    uninitialized: '未初始化',
    banned: '已禁用',
    closed: '已关闭',
  },

  // User information
  name: '姓名',
  email: '邮箱',
  avatar: '头像',
  position: '职位',
  department: '部门',
  mobile: '手机号',

  // User fields for tables and forms
  fields: {
    name: '姓名',
    email: '邮箱',
    position: '职位',
    role: '角色',
    status: '状态',
    permissions: '权限',
    department: '部门',
    mobile: '手机号',
    avatar: '头像',
    joinedAt: '加入时间',
    lastLogin: '最后登录',
  },

  // Account information
  accountInfo: '账户信息',
  basicInfo: '基本信息',
  contactInfo: '联系信息',
  roleInfo: '角色信息',

  // User status control
  statusControl: '状态控制',
  enableAccount: '启用账户',
  disableAccount: '禁用账户',

  // Permissions
  permissions: '权限',
  noPermissions: '暂无权限',
  create_agent: '创建智能体',
  create_knowledge: '创建知识库',
  create_database: '创建数据库',
  createAgent: '创建智能体',
  createKnowledge: '创建知识库',
  createDatabase: '创建数据库',

  // User actions
  editUser: '编辑用户',
  deleteUser: '删除用户',
  inviteUser: '邀请用户',
  actions: {
    label: '操作',
    edit: '编辑',
    remove: '移除',
    view: '查看',
    getInviteLink: '获取邀请链接',
  },

  // Action items
  view: '查看',
  edit: '编辑',
  remove: '移除',
  getInviteLink: '获取邀请链接',

  // Modal titles and descriptions
  editMember: '编辑成员',
  viewMember: '查看成员',
  editMemberDesc: '编辑成员的基本信息和权限',
  viewMemberDesc: '查看成员的详细信息',

  // Member details
  accountDetails: '账户详情',
  currentRole: '当前角色',
  departmentRole: '部门角色',
  permissionsAndRoles: '权限与角色',
  activity: '活动记录',
  activityComingSoon: '活动记录功能即将推出',

  // Time and dates
  never: '从未',
  invalid: '无效日期',
  lastLogin: '最后登录',
  joinedAt: '加入时间',

  // Delete/Remove confirmation
  confirmRemove: '确认移除',
  confirmRemoveDesc: '此操作将从部门中移除该成员',
  confirmRemoveButton: '确认移除',
  removeWarning: '移除警告',
  removeWarningPoint1: '该成员将失去在此部门的所有权限',
  removeWarningPoint2: '相关的工作流程和数据可能受到影响',

  // Form validation
  validation: {
    nameRequired: '姓名不能为空',
    emailFormat: '请输入正确的邮箱格式',
    mobileFormat: '请输入正确的手机号格式',
    positionRequired: '职位不能为空',
  },

  // Placeholders
  placeholders: {
    position: '请输入职位',
    mobile: '请输入手机号',
    selectRole: '选择角色',
    selectPermissions: '选择权限',
    selectDepartment: '选择部门',
  },

  // common (save, cancel, close, confirm, edit, delete, view) moved to common module

  // Save button
  saveInfo: '保存信息',

  // Messages
  updateSuccess: '用户信息更新成功',
  updateError: '用户信息更新失败',
  deleteSuccess: '用户删除成功',
  deleteError: '用户删除失败',
};

export default messages;
