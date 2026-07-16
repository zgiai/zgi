const messages = {
  search: 'Search',
  filter: 'Filter',
  actions: 'Actions',
  edit: 'Edit',
  delete: 'Delete',
  cancel: 'Cancel',
  confirm: 'Confirm',
  close: 'Close',
  save: 'Save',
  loading: 'Loading...',
  error: 'Error',
  success: 'Success',
  add: 'Add',
  create: 'Create',
  update: 'Update',
  view: 'View',
  accessDenied: 'Access Denied',
  unauthorizedDescription:
    'You do not have permission to access this page. Please contact your administrator.',
  name: 'Name',
  description: 'Description',
  noDescription: 'No description',
  model: 'Model',
  status: 'Status',
  active: 'Active',
  inactive: 'Inactive',
  pending: 'Pending',
  banned: 'Banned',
  createdAt: 'Created At',
  updatedAt: 'Updated At',
  all: 'All',
  yes: 'Yes',
  no: 'No',
  enabled: 'Enabled',
  disabled: 'Disabled',
  next: 'Next',
  previous: 'Previous',
  more: 'More',
  comingSoon: 'Coming Soon',
  items: 'items',
  characters: 'characters',
  selected: 'Selected',
  reset: 'Reset',
  resetFilters: 'Reset Filters',
  tokens: 'tokens',
  refresh: 'Refresh',
  refreshSuccess: 'Refresh successful',
  saveSuccess: 'Saved successfully',
  saveFailed: 'Save failed',
  back: 'Back',
  clear: 'Clear',
  copy: 'Copy',
  copyOutput: 'Copy output',
  paste: 'Paste',
  results: 'Results',

  // Network error
  networkError: 'Network connection failed, please check and retry',
  requestErrors: {
    generic: 'Something went wrong. Please try again.',
    forbidden: 'You do not have permission to perform this action.',
    notFound: 'The requested resource could not be found.',
    rateLimited: 'Too many requests right now. Please try again shortly.',
    serverBusy: 'The service is temporarily unavailable. Please try again later.',
    timeout: 'The request took too long. Please try again.',
    sessionExpired: 'Your session expired. Please sign in again.',
    passwordValidation: 'The password does not meet the requirements. Please check and try again.',
  },

  // Pagination
  pagination: {
    prev: 'Previous',
    next: 'Next',
    info: 'Showing {start} - {end} of {total}',
    jumpPlaceholder: 'Page',
    jump: 'Go',
  },

  // Icon input
  iconInput: {
    uploadImage: 'Upload Image',
    aiGenerate: 'AI Generate',
    textIcon: 'Text Icon',
    supportedFormats: 'Supports jpg, png format, size not exceeding 2M',
    defaultIcon: 'Z',
    dropToUpload: 'Release to upload image',
    textIconDialog: {
      title: 'Edit Text Icon',
      iconText: 'Icon Text',
      iconTextPlaceholder: 'Enter icon text (max 2 characters)',
      backgroundColor: 'Background Color',
      cancel: 'Cancel',
      save: 'Save',
    },
    imageCropDialog: {
      title: 'Upload & Crop Image',
      uploadText: 'Drag and drop an image here, or click to select',
      selectFile: 'Select File',
      cancel: 'Cancel',
      crop: 'Crop',
    },
  },

  // Image Cropper
  imageCropper: {
    title: 'Upload & Crop Image',
    uploadText: 'Drag and drop an image here, or click to select',
    dropHere: 'Release to upload image',
    supportedFormats: 'Supports jpg, png format, size not exceeding 2M',
    uploadProgress: 'Upload progress',
    cancel: 'Cancel',
    crop: 'Crop',
  },

  // Workspace Selector
  workspaceSelector: {
    placeholder: 'Select Workspace',
    search: 'Search Workspaces',
    loading: 'Loading...',
    defaultWorkspace: 'Default Workspace',
    noResults: 'No matching workspaces found',
    noWorkspaces: 'No workspaces available',
    noWorkspacesMember: 'You are not assigned to a workspace yet.',
    noWorkspacesAdmin: 'No workspaces are assigned or available yet.',
    organizationMode: 'Personal workbench',
  },

  workspaceRequired: {
    title: 'Select a workspace to continue',
    description:
      'Asset management and scheduled tasks run inside a specific workspace. Select a workspace to continue.',
    noWorkspacesTitle: 'No workspace is available',
    memberNoWorkspacesDescription:
      'You have joined the organization, but you have not been assigned to any workspace yet.',
    adminNoWorkspacesDescription:
      'Asset management and scheduled tasks need a concrete workspace. Create a workspace or assign members first.',
    memberNoWorkspacesHint:
      'Ask an organization administrator to add you to the right workspace before managing assets or scheduled tasks.',
    adminNoWorkspacesHint:
      'Create a workspace or assign members in workspace management, then return here.',
    loadingWorkspaces: 'Loading workspaces...',
    manageWorkspaces: 'Manage workspaces',
    refreshWorkspaces: 'Refresh workspaces',
  },

  assetMove: {
    title: 'Move to Workspace',
    description:
      'Related asset dependencies have been checked. Select a target workspace and confirm to validate its status and access permissions. Members of the current workspace may lose access after the move.',
    descriptionWithName:
      'Related dependencies for "{name}" have been checked. Select a target workspace and confirm to validate its status and access permissions.',
    locationTitle: 'Move location',
    locationDescription:
      'The asset will move from its current workspace to the selected workspace.',
    currentWorkspace: 'Current workspace',
    targetWorkspace: 'Target workspace',
    targetWorkspacePlaceholder: 'Select target workspace',
    preflightChecking: 'Checking related asset dependencies...',
    dependencyPreflightFailed: 'Could not check related asset dependencies. Please try again.',
    continueToTargetSelection: 'Continue to workspace selection',
    bindingPreflightWarningTitle: 'These bindings will be removed by the move',
    bindingPreflightWarningDescription:
      'Continue to select a target workspace. Agent bindings are removed only after the final move confirmation; this step does not change any configuration.',
    targetLoadFailedTitle: 'Could not load available workspaces',
    targetLoadFailedDescription:
      'Refresh and try again. Your permissions will be checked again before the asset is moved.',
    noTargetWorkspaceTitle: 'No other workspace is available',
    noTargetWorkspaceAdminDescription:
      'Create another workspace first, then return here to move this asset.',
    noTargetWorkspaceMemberDescription:
      'Ask an organization administrator or owner to grant move permission in another workspace.',
    createWorkspace: 'Create workspace',
    previewing: 'Checking the target workspace and access permissions...',
    unknownWorkspace: 'Unknown workspace',
    readyTitle: 'Ready to move',
    readyDescription:
      'Asset dependencies and access permissions passed the check. Confirm to move it to the target workspace.',
    bindingImpactTitle: 'Agent bindings must be removed',
    bindingImpactDescription:
      'This resource is used by {count} Agent(s). Confirming the move will first show which Agents will be unbound.',
    blockersTitle: 'Cannot move yet',
    warningsTitle: 'Review before moving',
    confirm: 'Confirm move',
    unbindAndMove: 'Unbind and move',
    previewFailed: 'Could not check asset dependencies and access permissions',
    moveSuccess: 'Moved successfully',
    moveFailed: 'Failed to move',
  },
  agentResourceBound: {
    title: 'This resource is used by Agents',
    description: '{count} Agent(s) will be affected by this change.',
    warningTitle: 'Agent bindings will change',
    warningDescription:
      'Continuing will unbind this resource from these Agents, and the related capability will no longer be available.',
    draft: 'Draft',
    published: 'Published',
    bindingCount: '{count} binding(s)',
    viewDetails: 'View details',
    unavailableAgent: 'Unavailable Agent',
    noDescription: 'No description',
    previewFailed: 'Could not check Agent bindings. Please try again.',
    confirm: 'Unbind and continue',
    retainSuspendedDescription:
      '{count} Agent(s) use the Skill you are disabling. Their bindings will be retained.',
    retainSuspendedWarningTitle: 'Bindings will be suspended',
    retainSuspendedWarningDescription:
      'Affected Agents cannot call this Skill while it is disabled. Re-enabling the Skill automatically restores these bindings.',
    retainSuspendedConfirm: 'Keep bindings and disable',
  },
  // Error Boundary
  errorBoundary: {
    somethingWentWrong: 'Something went wrong',
    unexpectedError: 'An unexpected error occurred in the application.',
    tryAgain: 'Try again',
    configurationError: 'Configuration Error',
    configurationErrorMessage:
      'There was an error loading the configuration form. Please try refreshing the page.',
  },

  // Error Pages
  errorPages: {
    notFound: {
      title: '404',
      subtitle: 'Page Not Found',
      description: 'Sorry, the page you are looking for does not exist or has been removed.',
      backHome: 'Back to Home',
    },
    error: {
      title: 'Something Went Wrong',
      description: 'This page hit a frontend rendering error. Your data was not changed.',
      recoveryHint:
        'Try again first. If it keeps failing, copy the diagnostics and send them to an administrator.',
      diagnostics: 'Diagnostics',
      retry: 'Try again',
      reload: 'Reload page',
      back: 'Go back',
      home: 'Back to Console',
      copy: 'Copy diagnostics',
      copied: 'Copied',
      refresh: 'Refresh Page',
    },
  },

  modelMultiSelector: {
    placeholder: 'Select models…',
    selectProviderFirst: 'Please select a provider to load models',
    selectedTitle: 'Selected models',
    remove: 'Remove {name}',
    selectAll: 'Select all',
    clear: 'Clear',
    searchPlaceholder: 'Search models…',
    noModels: 'No models available',
    loading: 'Loading...',
    scrollForMore: 'Scroll for more',
  },

  // Personal workbench empty states
  personalSpaceEmpty: {
    agents: 'No agents available',
    datasets: 'No datasets available',
    databases: 'No databases available',
    files: 'No files available',
    description:
      'In the personal workbench, you can use organization-level product entry points. Switch to a workspace for workspace-specific actions.',
    startCreating: 'Start Creating',
    selectWorkspaceHint: 'Select a workspace to continue',
    overlayHint: 'Click the area to select a workspace, or click elsewhere to close',
    noWorkspacesHint:
      'You have not joined any workspaces. Please contact an administrator for an invitation.',
  },

  // Workspace Mismatch Guard
  workspaceMismatch: {
    title: 'Workspace Mismatch',
    description:
      'This resource belongs to "{workspaceName}", but you are currently in "{currentWorkspaceName}".',
    descriptionInOrg:
      'This resource belongs to "{workspaceName}", but you are currently in the personal workbench.',
    switchButton: 'Switch to this workspace',
    actionHint: 'Please switch your current workspace and try again.',
  },

  organization: {
    switchOrgFailed: 'Failed to switch organization',
    switchOrgSuccess: 'Switched organization successfully',
    fetchOrgFailed: 'Failed to fetch organizations',
  },

  // Status labels
  statusLabels: {
    loading: 'Loading...',
    success: 'Success',
    failed: 'Failed',
    error: 'Error',
    processing: 'Processing',
    completed: 'Completed',
    enabled: 'Enabled',
    disabled: 'Disabled',
    waiting: 'Waiting',
    paused: 'Paused',
    saving: 'Saving...',
    creating: 'Creating...',
    deleting: 'Deleting...',
    updating: 'Updating...',
  },

  // Toast messages
  toasts: {
    createSuccess: 'Created successfully',
    createFailed: 'Failed to create',
    updateSuccess: 'Updated successfully',
    updateFailed: 'Failed to update',
    deleteSuccess: 'Deleted successfully',
    deleteFailed: 'Failed to delete',
    saveSuccess: 'Saved successfully',
    saveFailed: 'Failed to save',
    copySuccess: 'Copied to clipboard',
    copyFailed: 'Failed to copy',
    operationSuccess: 'Operation successful',
    operationFailed: 'Operation failed',
  },

  // Table headers
  table: {
    name: 'Name',
    status: 'Status',
    actions: 'Actions',
    createdAt: 'Created At',
    updatedAt: 'Updated At',
    description: 'Description',
    type: 'Type',
    size: 'Size',
  },

  // Confirmation dialogs
  confirmDialog: {
    deleteTitle: 'Confirm Delete',
    deleteDescription: 'Are you sure you want to delete "{name}"?',
    irreversibleWarning: 'This action cannot be undone.',
    confirmButton: 'Confirm',
    cancelButton: 'Cancel',
  },

  sensitiveOutput: {
    blocked: 'Sorry, I cannot process this request. Please ask another compliant question.',
  },

  notificationSms: {
    fields: {
      recipients: 'Phone numbers',
      template: 'SMS template',
    },
    setup: {
      title: 'SMS service is not configured',
      description:
        'Configure the SMS provider, SMS signature, and SMS templates before using SMS notifications.',
      templatePlaceholder: 'SMS templates are not configured',
    },
    templates: {
      pendingActionNotification: 'Pending action notification',
      workflowAlert: 'Workflow alert',
    },
    params: {
      notificationTitle: 'Notification title',
      linkCode: 'Link code',
      remark: 'Remark',
      summary: 'Summary',
    },
    placeholders: {
      recipient: 'Phone number {index}',
      recipientSingle: 'Phone numbers, separated by commas',
      notificationTitle: 'New task pending',
      linkCode: 'task or notice link code',
      remark: 'Enter remark',
      summary: 'Enter summary',
      param: 'Enter {label}',
    },
    actions: {
      addRecipient: 'Add phone',
      removeRecipient: 'Remove phone {index}',
    },
    help: {
      recipients:
        'Supports one phone number, comma-separated phone numbers, or a variable that resolves to phone numbers.',
      linkCode:
        'Use a short code such as abc123. Do not include -, _, Chinese characters, or a full URL.',
    },
    validation: {
      paramRequired: '{label} is required.',
      paramInvalid: '{label} has an invalid format.',
      paramTooLong: '{label} must be at most {max} characters.',
    },
    preview: 'Template preview',
    previewHint:
      'The actual SMS content follows the provider-approved template configured on the backend.',
    previewUnavailable: 'No preview text is configured for this template.',
  },

  // Form elements
  form: {
    required: 'Required',
    optional: 'Optional',
    selectPlaceholder: 'Please select...',
    inputPlaceholder: 'Please enter...',
    searchPlaceholder: 'Search...',
  },
};

export default messages;
export type CommonMessages = typeof messages;
