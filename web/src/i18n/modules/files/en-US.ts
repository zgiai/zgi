const messages = {
  title: 'File Management',
  eyebrow: 'Asset Library',
  description:
    'Upload and manage source files. Parsed content can be used for follow-up Q&A and knowledge base references.',

  // Sidebar
  sidebar: {
    storage: 'Storage',
    newFolder: 'New Folder',
    uploadFile: 'Upload File',
    viewsTitle: 'Views',
    fileSpaceTitle: 'File Space',
    allFiles: 'All Files',
    needsActionFiles: 'Needs Action',
    uploadedFiles: 'Recently Uploaded',
    favorites: 'Favorites',
    defaultFolders: 'My Files',
    noFolders: 'No folders yet',
  },

  // File list
  fileList: {
    fileName: 'File Name',
    fileType: 'File Type',
    fileSize: 'File Size',
    processingStatus: 'File Status',
    relatedStatus: 'Knowledge Base',
    uploadDate: 'Upload Time',
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
    startParseDialog: {
      title: 'Choose Parse Engine',
      description:
        'Choose a parse engine for "{name}". After submitting, the file will go through parsing, review, and indexing.',
      providerHint:
        'Auto follows the current provider routing strategy. You can also choose MinerU or another engine manually.',
      toasts: {
        started: 'Parse request submitted',
        failed: 'Failed to submit parse request',
      },
    },
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
    viewDetails: 'View Details',
    more: 'More',
    downloadFile: 'Download File',
    addToFavorites: 'Add to Favorites',
    removeFromFavorites: 'Remove from Favorites',
    bulkDelete: 'Batch Delete',
    batchParse: 'Batch Parse',
    batchMove: 'Batch Move',
    batchUnavailable:
      'This batch capability requires backend API support and is not available yet.',
    deleting: 'Deleting...',
    confirmParse: 'Review',
    startParse: 'Parse',
    startParsing: 'Submitting...',
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
    processingStatusLabel: 'File status',
    processingStatusAll: 'All',
    processingStatusNeedsAction: 'Needs action',
    processingStatusReady: 'Ready',
    processingStatusStoredOnly: 'Stored only',
  },

  detail: {
    backToFiles: 'Back to Files',
    fileBreadcrumb: 'Files',
    previewOriginal: 'Preview Original',
    downloadOriginal: 'Download Original',
    exportParsedContent: 'Export Parsed Content',
    exportParsedContentUnavailable: 'Parsed content is not ready yet.',
    exportParsedContentSuccess: 'Parsed content exported',
    exportParsedContentFailed: 'Failed to export parsed content',
    processing: 'Processing',
    fileType: '{extension} File',
    createdAt: 'Uploaded {time}',
    previewWorkspaceDescription:
      'Review the original file and parsed content, then resolve marked items.',
    loadErrorTitle: 'Failed to load file details',
    loadErrorDescription: 'The file may have been removed or you may not have access.',
    processingError: 'Processing failed',
    basicInfo: 'Basic Information',
    fileId: 'File ID',
    assetId: 'Asset ID',
    storageType: 'Storage Type',
    workspaceId: 'Workspace ID',
    createdBy: 'Created By',
    generationNo: 'Generation',
    nextViews: 'Detail Views',
    nextViewsDescription:
      'Original preview, parse review, content chunks, index information, and retry actions will be mounted here in the following phase-one frontend tasks.',
    processingSummary: 'Processing Summary',
    pendingConfirmationCount: 'Pending Reviews',
    chunkCount: 'Chunks',
    embeddingCount: 'Vectors',
    createdDate: 'Upload Date',
    indexInfo: 'Index Information',
    embeddingProvider: 'Embedding Provider',
    embeddingModel: 'Embedding Model',
    embeddingDimension: 'Embedding Dimension',
    vectorStatus: {
      none: 'Vector not ready',
      indexing: 'Vector indexing',
      ready: 'Vector ready',
      failed: 'Vector failed',
    },
    tabs: {
      overview: 'Overview',
      preview: 'File Preview',
      originalPreview: 'Original File',
      parseReview: 'Parse Review',
      chunks: 'Chunks',
      index: 'Index',
      qa: 'Document Q&A',
    },
    workbench: {
      title: 'Processing progress',
      description:
        '{pending} pending reviews, {chunks} chunks, and {embeddings} vectors generated.',
      pendingHint: '{count} pending',
      banners: {
        confirming: {
          title: 'Quality check found marked content',
          description: 'Quality check found {pending} items that need review.',
        },
        failed: {
          title: 'Processing failed',
          description: 'The processing flow stopped. Review the error and reparse when ready.',
        },
        ready: {
          title: 'Document asset ready',
          description:
            '{chunks} primary chunks and {embeddings} vectors are ready for document Q&A.',
        },
        processing: {
          title: 'Processing document',
          description:
            'The system is parsing content, generating chunks, or building the Q&A index.',
        },
        storedOnly: {
          title: 'File stored only',
          description: 'The original file is saved. Parse it to generate chunks and vectors.',
        },
      },
      steps: {
        uploaded: 'Uploaded',
        parsed: 'Parse content',
        quality: 'Quality check',
        chunks: 'Generate chunks',
        index: 'Build Q&A index',
        ready: 'Ready',
      },
      stepStates: {
        done: 'Done',
        active: 'In progress',
        attention: 'Needs action',
        failed: 'Failed',
        blocked: 'Waiting',
        pending: 'Not started',
      },
    },
    tabHints: {
      chunksReady: '{count} chunks generated',
      chunksWaiting: 'Waiting for review',
      qaReady: 'Ready for questions',
      qaWaiting: 'Available after review',
    },
    parseReview: {
      title: 'Parse Review',
      notReadyTitle: 'Parse review is not available',
      notReadyDescription: 'Wait until parsing finishes and the file enters review status.',
      loadErrorTitle: 'Failed to load parse preview',
      loadErrorDescription: 'The parse artifact may not be ready yet.',
      elementCount: '{count} elements',
      pendingCount: '{count} pending',
      pendingReviewTitle: '{count} items need review',
      pendingReviewDescription: 'Resolve marked content before chunking and document Q&A.',
      batchIgnore: 'Ignore All Pending',
      jumpNext: 'Jump to Next',
      emptyTitle: 'No parse elements',
      emptyDescription: 'The parser did not return structured elements for this file.',
      page: 'Page {page}',
      sourcePageCount: '{count} pages',
      boxes: '{count} boxes',
      confidence: 'Confidence {value}',
      hasLocation: 'Located',
      sourcePreviewFallback: 'Standard preview',
      sourcePreviewUnavailable:
        'Original page rendering is unavailable. Showing a standard document preview instead.',
      emptyContent: 'No text content',
      suggestedContent: 'Suggested content',
      keep: 'Keep',
      saveEdit: 'Save Edit',
      ignore: 'Ignore',
      resolvedHint: 'This item has been resolved.',
      status: {
        pending: 'Pending',
        kept: 'Kept',
        edited: 'Edited',
        ignored: 'Ignored',
      },
      toasts: {
        resolved: 'Review item resolved',
        batchIgnored: 'Pending review items ignored',
        generateQueued: 'Review complete. Generation has started.',
        resolveFailed: 'Failed to resolve review item',
        batchIgnoreFailed: 'Failed to ignore review items',
      },
      types: {
        title: 'Title',
        heading: 'Heading',
        text: 'Text',
        paragraph: 'Paragraph',
        table: 'Table',
        figure: 'Figure',
        image: 'Image',
        formula: 'Formula',
        list: 'List',
        listItem: 'List item',
        code: 'Code',
        element: 'Element',
      },
    },
    chunks: {
      title: 'Content Chunks',
      notReadyTitle: 'Chunks are not ready',
      notReadyDescription:
        'Chunks become available after parsing, review, and vector generation finish.',
      loadErrorTitle: 'Failed to load chunks',
      loadErrorDescription: 'The chunk result may not be ready yet.',
      total: '{count} chunks',
      generationNo: 'Generation {value}',
      emptyTitle: 'No chunks',
      emptyDescription: 'No chunk result is available for this file.',
      chunkTitle: 'Chunk {position}',
      primary: 'Primary Chunk',
      secondary: 'Secondary Chunk',
      secondaryCount: 'Secondary chunks ({count})',
      searchPlaceholder: 'Search chunk content...',
      filters: {
        all: 'All',
        enabled: 'Enabled',
        disabled: 'Disabled',
      },
      expandAll: 'Expand all',
      collapseAll: 'Collapse all',
      resegment: 'Regenerate',
      add: 'Add chunk',
      selectAll: 'Select all ({count} chunks)',
      manageSecondary: 'Manage secondary chunks',
      viewOriginal: 'View source',
      edit: 'Edit',
      editSecondaryTitle: 'Edit Secondary Chunk',
      editSecondaryDescription:
        'This secondary chunk has {count} characters. Saving will rebuild its vector.',
      delete: 'Delete',
      characters: '{count} characters',
      enabled: 'Enabled',
      disabled: 'Disabled',
      cancel: 'Cancel',
      save: 'Save',
      toasts: {
        updated: 'Chunk updated',
        updateFailed: 'Failed to update chunk',
      },
    },
    index: {
      title: 'Index Information',
      description:
        'File-level chunk and embedding assets generated before adding to a knowledge base.',
      notReadyTitle: 'Index information is not ready',
      notReadyDescription:
        'Index metadata becomes available after chunking and embedding generation starts.',
    },
    qa: {
      title: 'Document Q&A',
      description:
        'Retrieves secondary chunks, expands to primary chunks, and answers only from this document.',
      notReadyTitle: 'Document Q&A is not ready',
      notReadyDescription:
        'Ask questions after parsing finishes and secondary chunk vectors are available.',
      chunkSummary: '{count} chunks',
      vectorSummary: '{count} vectors',
      emptyTitle: 'Ask this document',
      emptyDescription:
        'After you ask, the system retrieves related secondary chunks and uses their primary chunks as context.',
      question: 'Question',
      answer: 'Answer',
      placeholder: 'Ask a question about this document...',
      send: 'Send',
      generating: 'Generating...',
      askFailedTitle: 'Q&A failed',
      askFailed: 'Failed to submit question',
      noSources: 'No related source was found in this document.',
      sources: 'Sources ({count})',
      distance: 'Distance {value}',
    },
    reparse: {
      action: 'Reparse',
      reparsing: 'Submitting...',
      confirmTitle: 'Reparse this file?',
      confirmDescription:
        'The current searchable asset will be unavailable while the file is parsing, reviewing, and indexing again.',
      confirm: 'Reparse',
      toasts: {
        started: 'Reparse request submitted',
        failed: 'Failed to submit reparse request',
      },
    },
    failure: {
      storeOnly: 'Mark as Stored Only',
      storeOnlyUnavailable:
        'This requires a backend status transition API and is not available yet.',
    },
    views: {
      storedOnly: {
        title: 'Stored only',
        description:
          'The original file is saved. Start parsing later to produce chunks and vectors.',
      },
      processing: {
        title: 'Parsing in progress',
        description:
          'The system is extracting document content. This page refreshes every 2 seconds.',
      },
      confirming: {
        title: 'Parse review required',
        description: 'Review parse elements before generating chunks and vectors.',
      },
      generating: {
        title: 'Generating chunks and vectors',
        description:
          'The system is chunking content and writing embeddings. This page refreshes every 2 seconds.',
      },
      ready: {
        title: 'Searchable asset ready',
        description:
          'Primary chunks, secondary chunks, and vectors are ready. You can inspect or edit secondary chunks next.',
      },
      failed: {
        title: 'Processing failed',
        description: 'Review the error message and retry after the retry action is available.',
      },
    },
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
    parseProvider: 'Parse Engine',
    parseProviderDescription:
      'Used for automatic parsing after upload. Auto follows the current provider routing strategy.',
    parseProviderUnavailable: 'Unavailable',
    parseProviderLoading: 'Checking available engines...',
    parseProviders: {
      auto: 'Auto (routing strategy)',
      mineru: 'MinerU',
      local: 'Local',
      reducto: 'Reducto',
      hyperparseApi: 'Hyperparse API',
      vlm: 'Vision LLM',
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
