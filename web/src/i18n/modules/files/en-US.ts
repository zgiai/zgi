const messages = {
  title: 'File Management',
  eyebrow: 'Asset Library',
  description:
    'Manage uploaded files, folders, and resource references. Files can be used for knowledge bases, database imports, and chat workflows.',

  // Sidebar
  sidebar: {
    storage: 'Storage',
    newFolder: 'New Folder',
    uploadFile: 'Upload File',
    allFiles: 'All Files',
    uploadedFiles: 'Recently Uploaded',
    favorites: 'Favorites',
    defaultFolders: 'Default Folders',
    noFolders: 'No folders yet',
  },

  // File list
  fileList: {
    fileName: 'File Name',
    fileType: 'File Type',
    fileSize: 'File Size',
    processingStatus: 'Processing Status',
    relatedStatus: 'Related Status',
    uploadDate: 'Upload Date',
    lastModified: 'Last Modified',
    actions: 'Actions',
    selectAll: 'Select All',
    selected: 'Selected {count}',
    totalItems: 'Total {total} items',
    relatedCount: 'Related {count} items',
    notRelated: 'Not Related',
    pendingCount: '{count} pending',
    chunkCount: '{count} chunks',
    embeddingCount: '{count} vectors',
  },

  // File statuses
  status: {
    uploading: 'Uploading',
    completed: 'Completed',
    failed: 'Failed',
    processing: 'Processing',
    error: 'Error',
    archived: 'Archived',
  },

  processingStatus: {
    stored_only: 'Stored only',
    parsing: 'Parsing',
    confirming: 'Needs review',
    generating: 'Indexing',
    parse_failed: 'Failed',
    ready: 'Ready',
  },

  // Actions
  actions: {
    view: 'View',
    download: 'Download',
    delete: 'Delete',
    rename: 'Rename',
    move: 'Move',
    share: 'Share',
    preview: 'Preview',
    more: 'More',
    downloadFile: 'Download File',
    addToFavorites: 'Add to Favorites',
    removeFromFavorites: 'Remove from Favorites',
    bulkDelete: 'Batch Delete',
    deleting: 'Deleting...',
  },

  preview: {
    title: 'File Preview',
    description: 'Original file preview',
    fileMeta: '{extension} original preview',
    loading: 'Loading preview...',
    loadError: 'Failed to load file preview',
    noFileSelected: 'No file selected',
    unsupportedTitle: 'Preview is not available for this file',
    unsupportedDescription:
      'Original preview supports images, PDF, HTML, text-like files, DOCX, and XLSX.',
    unsupportedFormatTitle: '{format} preview is not supported',
    unsupportedFormatDescription:
      '{format} files are not supported in browser preview yet. Download the file to view it locally.',
    openInNewTab: 'Open in New Tab',
    unavailableTitle: 'Preview is unavailable',
    downloadOnlyDescription: 'Download the file to view it outside the browser preview.',
    htmlLimitedTitle: 'HTML is shown in isolated preview mode',
    htmlLimitedDescription:
      'Page scripts can run for animations, while forms, popups, network requests, embedded frames, and navigation stay blocked.',
    htmlOpenRiskTitle: 'Open original HTML in a new tab?',
    htmlOpenRiskDescription:
      'The new tab will load the original HTML outside the isolated preview. It may run scripts, navigate, or contact external resources from that page. Only continue if you trust this file.',
    htmlOpenRiskConfirm: 'Open Anyway',
    htmlOpenRiskCancel: 'Cancel',
    htmlTooLargeTitle: 'This HTML file is too large to preview safely',
    officeUnsupportedTitle: 'Preview is not available for this Office format',
    officeTooLargeTitle: 'This Office file is too large to preview',
    officeFallback: 'Download the file to view the original document.',
    textTooLargeTitle: 'This text file is too large to preview',
    textFallback: 'Download the file to view the full content.',
    emptyWorkbook: 'The workbook has no sheets',
    emptySheet: 'This sheet has no visible rows',
    rowLimit: 'Showing the first {count} rows.',
  },

  // Delete dialog
  delete: {
    cannotDelete: 'Cannot Delete File',
    associationWarning:
      'This file is associated with knowledge bases or databases and cannot be deleted directly.',
    unlinkFirst:
      'Please remove all associations first, making the file status "unlinked", then proceed with deletion.',
    understood: 'I Understand',
    viewRelated: 'View Related Items',
    folderConfirmTitle: 'Delete Folder "{name}"?',
    folderConfirmDescription:
      'This action will permanently delete the folder and cannot be undone.',
    bulkConfirmTitle: 'Delete {count} files?',
    bulkConfirmDescription:
      'This action will permanently delete the selected files and cannot be undone.',
  },

  // Search and filter
  search: {
    placeholder: 'Search files...',
    byName: 'By Name',
    byType: 'By Type',
    byDate: 'By Date',
  },

  filter: {
    allProcessingStatuses: 'All statuses',
  },

  // Messages
  messages: {
    uploadSuccess: 'File uploaded successfully',
    uploadFailed: 'File upload failed',
    deleteSuccess: 'File deleted successfully',
    deleteFailed: 'File deletion failed',
    deleteConfirm: 'Are you sure you want to delete "{name}"?',
    deleteConfirmDesc: 'This action cannot be undone.',
    noFiles: 'No files',
    noFilesDescWithUpload:
      'Upload documents for knowledge bases, spreadsheets for database imports, or files for chat workflows.',
    noFilesDescWithUploadInSelector:
      'No files are available yet. Use the upload entry in the left sidebar to add files.',
    noFilesDescWithoutUploadPermission:
      'No files are available in the current workspace, and you do not have upload permission here. Contact an administrator for access, or switch to another workspace.',
    empty: 'Nothing here',
  },

  // Toast messages for hooks
  toast: {
    loadFilesError: 'Failed to load files',
    storageUsageError: 'Failed to load storage usage',
    downloadSuccess: 'File downloaded successfully',
    downloadError: 'Failed to download file',
    deleteSuccess: 'File deleted successfully',
    deleteError: 'File deletion failed',
    addFavoriteSuccess: 'Added to favorites',
    addFavoriteError: 'Failed to add to favorites',
    removeFavoriteSuccess: 'Removed from favorites',
    removeFavoriteError: 'Failed to remove from favorites',
    foldersLoadError: 'Failed to load folders',
    childFoldersLoadError: 'Failed to load child folders',
    uploadSuccess: 'File uploaded successfully',
    uploadError: 'Failed to upload file',
    createFolderSuccess: 'Folder created successfully',
    createFolderError: 'Failed to create folder',
    updateFolderSuccess: 'Folder updated successfully',
    updateFolderError: 'Failed to update folder',
    deleteFolderSuccess: 'Folder deleted successfully',
    deleteFolderError: 'Failed to delete folder',
    createTextFileSuccess: 'Text file created successfully',
    createTextFileError: 'Failed to create text file',
  },

  // File selector
  selectFiles: 'Select Files',
  selectedCount: '{count} selected',
  selectedCountWithMax: '{count}/{max} selected',
  selectedSingle: '1 file selected',
  confirmSelect: 'Confirm Selection ({count})',
  confirmSelectSingle: 'Confirm Selection',
  maxCountExceeded: 'Maximum {max} files allowed',
  selectorContext: {
    title: 'Current context',
    action: 'Switch space',
    dialogTitle: 'Switch browsing space',
    dialogDescription:
      'Choose the organization and workspace you want to browse. The file list and upload entry update automatically after switching.',
    tipTitle: 'About upload permissions',
    tipDescription:
      'Upload is available only when the selected workspace grants file upload permission.',
  },
  mobileSelector: {
    browse: 'Browse folders',
    browseAndUpload: 'Folders & Upload',
    switchSpace: 'Switch space',
    emptyDescriptionWithUpload:
      'No files are available yet. Tap "Folders & Upload" above, then use the upload entry inside to add files.',
    emptyDescriptionWithoutUpload:
      'No files are available in the current workspace, and you do not have upload permission here. Contact an administrator for access, or switch to another workspace.',
  },
  selectorEmptyState: {
    badge: 'Empty State',
    title: 'No files are currently available',
    description:
      'There are no selectable files in Organization View right now. To upload files, choose an owning workspace first.',
    noticeTitle: 'Upload requires a workspace',
    noticeDescription:
      'System file uploads are stored under a workspace. Choose a workspace before uploading.',
    quickActionTitle: 'Choose a workspace',
    quickActionDescription: 'Select a workspace below, then upload into that workspace.',
    noWorkspaces: 'No workspaces are available to switch to.',
  },

  // Resource types
  resourceTypes: {
    dataset: 'Dataset',
    agent: 'Agent',
    workflow: 'Workflow',
    unknown: 'Unknown',
  },

  // Upload dialog
  upload: {
    selectSource: 'Select Source',
    workspaceLabel: 'Owning Workspace',
    workspacePlaceholder: 'Select an owning workspace',
    workspaceRequired: 'Please select an owning workspace',
    storageLocation: 'Storage Location',
    selectFolder: 'Select Folder',
    defaultFolder: 'Default Folder',
    sourceType: 'Source Type',
    processingMode: 'Processing Mode',
    processingModes: {
      processNow: {
        title: 'Upload and parse',
        desc: 'Parse, chunk, and index the document immediately after upload.',
      },
      storeOnly: {
        title: 'Store only',
        desc: 'Save the original file first. You can parse it later from file details.',
      },
    },
    processingHintTitle: 'Document files become searchable after parsing',
    processingHintDescription:
      'Images, icons, temporary files, and unsupported formats are stored without document processing.',
    uploadFiles: 'Upload Files',
    confirmUpload: 'Confirm Upload',
  },

  // Documents section (for compatibility)
  documents: {
    addDocument: 'Add Document',
  },

  // Folder management
  folder: {
    createFolder: 'Create Folder',
    folderName: 'Folder Name',
    folderNamePlaceholder: 'Enter folder name',
    workspaceLabel: 'Owning Workspace',
    workspacePlaceholder: 'Select an owning workspace',
    workspaceRequired: 'Please select an owning workspace',
    parentFolder: 'Parent Folder',
    selectParentFolder: 'Select parent folder',
    folderLabel: 'Folder:',
    rootFolder: 'Root Folder',
  },

  // Text file creation
  text: {
    createTitle: 'Add Text File',
    fileNameLabel: 'File Name',
    fileNamePlaceholder: 'Enter file name (without extension)',
    contentLabel: 'Text Content',
    contentPlaceholder: 'Enter text content',
    charCount: 'Character Count',
    fileSize: 'File Size',
    storageLocation: 'Storage Location',
    saveFile: 'Save File',
  },
};

export default messages;
export type FilesMessages = typeof messages;
