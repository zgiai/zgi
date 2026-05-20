const messages = {
  // User roles
  roles: {
    owner: 'Owner',
    admin: 'Admin',
    normal: 'Member',
    editor: 'Editor',
  },

  // User status
  status: {
    label: 'Status',
    active: 'Active',
    pending: 'Pending',
    uninitialized: 'Uninitialized',
    banned: 'Banned',
    closed: 'Closed',
  },

  // User information
  name: 'Name',
  email: 'Email',
  avatar: 'Avatar',
  position: 'Position',
  department: 'Department',
  mobile: 'Mobile',

  // User fields for tables and forms
  fields: {
    name: 'Name',
    email: 'Email',
    position: 'Position',
    role: 'Role',
    status: 'Status',
    permissions: 'Permissions',
    department: 'Department',
    mobile: 'Mobile',
    avatar: 'Avatar',
    joinedAt: 'Joined At',
    lastLogin: 'Last Login',
  },

  // Account information
  accountInfo: 'Account Information',
  basicInfo: 'Basic Information',
  contactInfo: 'Contact Information',
  roleInfo: 'Role Information',

  // User status control
  statusControl: 'Status Control',
  enableAccount: 'Enable Account',
  disableAccount: 'Disable Account',

  // Permissions
  permissions: 'Permissions',
  noPermissions: 'No Permissions',
  create_agent: 'Create Agent',
  create_knowledge: 'Create Dataset',
  create_database: 'Create Database',
  createAgent: 'Create Agent',
  createKnowledge: 'Create Dataset',
  createDatabase: 'Create Database',

  // User actions
  editUser: 'Edit User',
  deleteUser: 'Delete User',
  inviteUser: 'Invite User',
  actions: {
    label: 'Actions',
    edit: 'Edit',
    remove: 'Remove',
    view: 'View',
    getInviteLink: 'Get Invite Link',
  },

  // Action items
  view: 'View',
  edit: 'Edit',
  remove: 'Remove',
  getInviteLink: 'Get Invite Link',

  // Modal titles and descriptions
  editMember: 'Edit Member',
  viewMember: 'View Member',
  editMemberDesc: 'Edit member basic information and permissions',
  viewMemberDesc: 'View member detailed information',

  // Member details
  accountDetails: 'Account Details',
  currentRole: 'Current Role',
  departmentRole: 'Department Role',
  permissionsAndRoles: 'Permissions & Roles',
  activity: 'Activity Log',
  activityComingSoon: 'Activity log feature coming soon',

  // Time and dates
  never: 'Never',
  invalid: 'Invalid Date',
  lastLogin: 'Last Login',
  joinedAt: 'Joined At',

  // Delete/Remove confirmation
  confirmRemove: 'Confirm Remove',
  confirmRemoveDesc: 'This action will remove the member from the department',
  confirmRemoveButton: 'Confirm Remove',
  removeWarning: 'Remove Warning',
  removeWarningPoint1: 'The member will lose all permissions in this department',
  removeWarningPoint2: 'Related workflows and data may be affected',

  // Form validation
  validation: {
    nameRequired: 'Name is required',
    emailFormat: 'Please enter a valid email format',
    mobileFormat: 'Please enter a valid mobile number format',
    positionRequired: 'Position is required',
  },

  // Placeholders
  placeholders: {
    position: 'Enter position',
    mobile: 'Enter mobile number',
    selectRole: 'Select role',
    selectPermissions: 'Select permissions',
    selectDepartment: 'Select department',
  },

  // Save button
  saveInfo: 'Save Information',

  // Messages
  updateSuccess: 'User information updated successfully',
  updateError: 'Failed to update user information',
  deleteSuccess: 'User deleted successfully',
  deleteError: 'Failed to delete user',
};

export default messages;
export type UsersMessages = typeof messages;
