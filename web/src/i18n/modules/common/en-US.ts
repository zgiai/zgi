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
    noResults: 'No matching workspaces found',
    noWorkspaces: 'No workspaces available',
    noWorkspacesMember: 'You are not assigned to a workspace yet.',
    noWorkspacesAdmin: 'No workspaces are assigned or available yet.',
    organizationMode: 'Organization mode',
  },

  workspaceRequired: {
    title: 'Select a workspace to continue',
    description:
      'The workbench runs inside a specific workspace. Select a workspace before starting chats, apps, image generation, or tasks.',
    noWorkspacesTitle: 'No workspace is available',
    memberNoWorkspacesDescription:
      'You have joined the organization, but you have not been assigned to any workspace yet.',
    adminNoWorkspacesDescription:
      'The workbench needs a concrete workspace before chats, apps, image generation, or tasks can be used.',
    memberNoWorkspacesHint:
      'Ask an organization administrator to add you to a workspace before using the workbench.',
    adminNoWorkspacesHint:
      'Create a workspace or assign members in workspace management, then return to the workbench.',
    loadingWorkspaces: 'Loading workspaces...',
    manageWorkspaces: 'Manage workspaces',
    refreshWorkspaces: 'Refresh workspaces',
  },

  assetMove: {
    title: 'Move to Workspace',
    description: 'Select a target workspace and review the move check before confirming.',
    descriptionWithName: 'Move "{name}" to another workspace after reviewing the move check.',
    targetWorkspace: 'Target workspace',
    targetWorkspacePlaceholder: 'Select target workspace',
    previewing: 'Checking move...',
    unknownWorkspace: 'Unknown workspace',
    blockersTitle: 'Move blocked',
    warningsTitle: 'Review warnings',
    confirm: 'Move',
    previewFailed: 'Failed to check move',
    moveSuccess: 'Moved successfully',
    moveFailed: 'Failed to move',
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

  // Organization View empty states
  personalSpaceEmpty: {
    agents: 'No agents available',
    datasets: 'No datasets available',
    databases: 'No databases available',
    files: 'No files available',
    description:
      'In Organization View, you can browse organization resources. Switch to a workspace for workspace-specific actions.',
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
      'This resource belongs to "{workspaceName}", but you are currently in Organization View.',
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
