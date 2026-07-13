const messages = {
  title: 'Database Management',
  // confirm, close, refresh, loading moved to common module
  empty: 'No databases',
  emptyDesc: 'Create your first database to start storing and querying data.',
  noName: 'No Name',
  noDescription: 'No description',
  database: 'Database',
  create: 'Create Database',
  edit: 'Edit Database',
  databaseSettings: 'Database Settings',
  backToDatabaseList: 'Back to database list',
  // Tables & features
  tables: 'Tables',
  createTable: 'Create Table',
  goToDetail: 'Go to Detail',
  features: {
    dataQuery: 'Data Query',
    logs: 'Logs',
  },
  search: {
    placeholder: 'Search databases...',
    filter: 'Filter',
    all: 'All',
    byStatus: 'Filter by status',
    byType: 'Filter by type',
    byDate: 'Filter by date',
    noResults: 'No matching databases',
    noResultsDesc: 'Try adjusting search keywords or clearing filters.',
    noResultsFor: 'No results for “{query}”',
    clearFilters: 'Clear search',
  },
  tabs: {
    structure: 'Table Structure',
    data: 'Table Data',
  },
  actions: {
    edit: 'Edit Table Information',
    delete: 'Delete',
    smartGenerate: 'AI Generate Table Structure',
    smartIngest: 'Smart Ingest',
    manageStructure: 'Manage Table Structure',
    viewData: 'View Table Data',
    more: 'More actions',
  },
  createSuccess: 'Database created',
  updateSuccess: 'Settings saved',
  deleteSuccess: 'Deleted successfully',
  // Standardized toast titles for DB-related operations
  tableCreateSuccess: 'Table created',
  tableUpdateSuccess: 'Table updated',
  tableDeleteSuccess: 'Table deleted',
  columnsUpdateSuccess: 'Columns updated',
  // Added from zh-Hans
  promptUpdateSuccess: 'Prompt updated',
  recordsEditSuccess: 'Edit successful',
  failed: 'Operation failed',
  deleteConfirmTitle: 'Delete {name}?',
  deleteConfirmDescription: 'This action cannot be undone.',
  deleteTableConfirmDescription:
    'Deleting this table will remove all its data. Proceed with caution.',

  // Create DB Modal
  createModal: {
    title: 'Create Database',
    nameLabel: 'Database Name',
    namePlaceholder: 'Name',
    departmentLabel: 'Department',
    departmentPlaceholder: 'Please select department',
    descriptionLabel: 'Database Description',
    descriptionPlaceholder: 'Please enter database description',
    workspaceLabel: 'Owning Workspace',
    workspacePlaceholder: 'Select an owning workspace',
    permissionsLabel: 'Permissions',
    advancedSettingsLabel: 'Advanced Settings',
    iconLabel: 'Database Icon',
    createButton: 'Create',
    creatingButton: 'Creating...',
  },

  // Validation messages
  validation: {
    name: {
      required: 'Database name is required',
      tooShort: 'Name must be at least 2 characters',
      tooLong: 'Name must be at most 32 characters',
      invalidChars: 'Only letters, numbers, underscores, hyphens and spaces are allowed',
      onlySpaces: 'Name must contain at least one non-space character',
    },
    workspace: {
      required: 'Please select an owning workspace',
    },
    tableName: {
      required: 'Table name is required',
      tooLong: 'Table name must be at most 63 characters',
      invalidChars: 'Only lowercase letters, digits and underscores are allowed',
      mustStartWithLowercaseLetter: 'Must start with a lowercase letter',
      noConsecutiveUnderscores: 'Consecutive underscores are not allowed',
      duplicate: 'Table name already exists',
    },
  },

  // Table management modal
  tableModal: {
    createTitle: 'Create Table',
    editTitle: 'Edit Table',
    nameLimitHint: '{count}/63 characters',
  },

  // Table columns UI
  columns: {
    previewTitle: 'Table Structure Preview',
    name: 'Storage Field Name',
    description: 'Description',
    type: 'Data Type',
    required: 'Required',
    actions: 'Actions',
    add: 'Add',
    remove: 'Remove',
    save: 'Save Changes',
    cancel: 'Cancel',
    system: 'System',
    selectTypePlaceholder: 'Select type',
    syncing: 'Syncing...',
    deleteWarning: 'Deleting existing fields may clear user data. Proceed with caution.',
    doNotShowAgain: "Don't show again",
    duplicateNamesNotAllowed: 'Duplicate column names are not allowed. Please fix before saving.',
    duplicateNameTip: 'Duplicate name',
    invalidNameTip: 'Only lowercase letters, digits and underscores; must start with a letter',
    reservedNameTip: 'Reserved keyword is not allowed',
  },
  // Table data UI
  tableData: {
    title: 'Data Table',
    discardConfirmTitle: 'Discard unsaved changes?',
    discardConfirmDescription: 'Your current changes will not be saved.',
    discardConfirmAction: 'Discard changes',
    rowsPerPage: 'Rows per page',
    sortBy: 'Sort by',
    ascending: 'Ascending',
    descending: 'Descending',
    addRow: 'Add Row',
    edit: 'Edit',
    save: 'Save',
    cancel: 'Cancel',
    actions: 'Actions',
    deleteRow: 'Delete row',
    copyToClipboard: 'Copied to clipboard',
    search: 'Search current page',
    searchPlaceholder: 'Search loaded rows...',
    columns: {
      title: 'Show/Hide Columns',
      visibleColumns: 'Visible columns',
      keepOneVisible: 'Keep at least one column visible',
    },
    rowDetail: {
      title: 'Row details',
      open: 'View row details',
      openShort: 'Details',
      businessFields: 'Business fields',
      systemFields: 'System fields',
    },
    inputs: {
      textTitle: 'Enter text',
      numberTitle: 'Enter number',
      booleanTitle: 'Toggle value',
      timestampTitle: 'Enter date & time',
    },
    validation: {
      saveBlocked: 'Please complete required fields before saving',
      fieldMissing: '{field} is required',
      timestampInvalid: '{field} must be a valid date-time',
    },
    empty: {
      title: 'No data',
      desc: 'Start adding data to populate your table',
    },
    noData: {
      title: 'No Data',
      desc: 'No records found in this table. Start by adding some data.',
    },
    noFields: {
      title: 'Add fields to start',
      desc: 'This table only contains system fields. Add at least one non-system field to create data.',
      editDisabledTip: 'Add fields first to edit data',
    },
  },
  tableCreate: {
    steps: {
      generateStructure: 'Smart Generate',
      generateStructureDesc: 'AI generates table structure',
      mergeEdit: 'Confirm Table Structure',
      mergeEditDesc: 'Merge AI and existing fields, confirm final table structure',
    },
  },
  // Create page under /console/db/[dbId]/table/[tableId]/create
  createPage: {
    headerTitle: 'AI Generate Table Structure',
    prevStep: 'Previous',
    nextStep: 'Next',
    requirementLabel: 'Requirement Description',
    requirementPlaceholder:
      'Describe the desired table structure, e.g., user info fields: name, email, phone...',
    defaultPrompt:
      'Based on the data and business context, infer and design a suitable table structure.',
    referenceFiles: 'Reference Files',
    fileSelected: 'File selected',
    removeFileAria: 'Remove file',
    noFileSelected: 'No file selected',
    chooseFromFileManager: 'Choose from File Manager',
    chooseFromFileManagerDesc: 'Select an uploaded file for intelligent analysis',
    startAnalyze: 'Generate Structure',
    startAnalyzeLoading: 'Generating structure...',
    previewTitle: 'File Content Preview',
    noPreview: 'No preview content',
    emptyHint: 'Enter a prompt and select a file on the left, then click "Generate Structure".',
    colFieldName: 'Field Name',
    colDescription: 'Description',
    colType: 'Type',
    colRequired: 'Required',
    requiredYes: 'Yes',
    requiredNo: 'No',
    step2Header: 'Merge and Edit Final Table Structure',
    addField: 'Add Field',
    save: 'Save',
    saving: 'Saving...',
    colActions: 'Actions',
    // Prompt template selector
    promptTemplates: {
      title: 'Prompt Templates',
      selectTemplate: 'Select Template',
      footerTip: 'Select a template to quickly fill in the prompt',
      items: {
        general: {
          title: 'General Table',
          description: 'Infer table structure based on data content and business context',
        },
        userManagement: {
          title: 'User Management',
          description: 'User information table including basic info, contact, and status fields',
        },
        orderSystem: {
          title: 'Order System',
          description: 'E-commerce order table with order info, products, amounts, and status',
        },
        inventory: {
          title: 'Inventory Management',
          description: 'Inventory tracking table with product info, quantities, and locations',
        },
      },
      preview: {
        description:
          'This template will help you generate a table structure for the specified scenario.',
        previewLabel: 'Template Content',
        warning: 'Applying this template will replace the current prompt content.',
        cancel: 'Cancel',
        apply: 'Apply Template',
      },
    },
  },
  // Data ingest page under /console/db/[dbId]/table/[tableId]/data
  dataIngestPage: {
    headerTitle: 'Intelligent data extraction to table',
    leaveProcessingConfirm:
      'Files are still being recognized. Leaving or going back may interrupt the current page state and lose unfinished results. Leave anyway?',
    leaveUnsavedConfirm:
      'Recognition results have not been reviewed and committed yet. This page state will not be preserved after leaving. Leave anyway?',
    leaveGuard: {
      processingTitle: 'Recognition is still running',
      processingDescription:
        'Files are still being recognized. Leaving this review page will interrupt pending work and unfinished recognition results will be lost.',
      unsavedTitle: 'Leave without committing results?',
      unsavedDescription:
        'Recognition results only live in this review page until you approve and commit them. Leaving now will discard the current review state.',
      continueReview: 'Keep reviewing',
      leaveAndDiscard: 'Leave and discard',
    },
  },
  // Table ingest flow components
  tableIngest: {
    progress: {
      step1: 'Select Files',
      step1Desc: 'Choose files from File Manager for recognition',
      step2: 'AI Recognition',
      step2Desc: 'Extract and map fields',
      step3: 'Human Review',
      step3Desc: 'Review mappings and required values',
      step4: 'Commit',
      step4Desc: 'Write reviewed rows to table',
    },
    stepOne: {
      bannerTitle: 'Uploads use the internal File Manager pipeline',
      bannerText: 'AI Recognition: AI will auto map content to table fields. {desc}',
      supportedDesc:
        'Supports PDF, Word, images, and more. Multi-file selection is supported for AI recognition.',
      chooseFromFiles: 'Select files from File Manager',
      selectedTitle: 'Selected Files ({count})',
      startRecognition: 'Start AI Recognition ({count} files)',
      removeFileAria: 'Remove file',
      unsupportedFileSkipped: 'Some unsupported file types were skipped. Supported now: {types}',
      imageFileHint: 'Images are processed by file parsing',
      pipeline: {
        fileManager: 'Store and permission files in File Manager first',
        review: 'Recognition enters review instead of writing immediately',
        commit: 'Commit after review and keep source lineage',
      },
    },
    stepTwo: {
      placeholderTitle: 'AI Recognition & Data Preview (placeholder)',
      placeholderDesc:
        'Preview and extracted data will be shown, with options to confirm write-back. Currently selected {count} files.',
      leftPanelTitle: 'File List',
      previewPanelTitle: 'File Content',
      fieldsPanelTitle: 'Field Extraction',
      recognizing: 'Recognizing...',
      noPreview: 'No preview content',
      notRecognized: 'Not recognized',
      recognizedTag: 'Recognized',
      saveToTable: 'Save to Table',
      reviewAndSave: 'Approve and Commit',
      saveSafetyHint:
        'Only validated files will be written. Invalid files stay in this review page.',
      saving: 'Saving...',
      reRecognize: 'Re-recognize',
      workspaceTitle: 'Review workspace',
      processingLeaveHint:
        'Recognition is in progress. Keep this page open. Refreshing, closing, or going back may lose this run.',
      unsavedLeaveHint:
        'Recognition results only live in this review page until you approve and commit them.',
      contentTabs: {
        original: 'Original',
        text: 'Recognized text',
        details: 'Parse details',
      },
      fieldStats: {
        recognized: 'Recognized {count}',
        needs: 'Needs {count}',
        invalid: 'Invalid {count}',
      },
      fieldValueStatus: {
        normalized: 'Converted',
        needsConfirmation: 'Needs confirmation',
        normalizedHint: 'Converted from "{raw}" to "{value}".',
        candidateHint:
          'The model found "{raw}", but this field expects {type}. Review and fill it manually.',
      },
      parseDetails: {
        title: 'Parse path',
        strategy: 'Strategy',
        sourceType: 'Source type',
        fallbackReason: 'Fallback reason',
        contentHash: 'Content hash',
        attempts: 'Recognition attempts',
        attemptIndex: 'Attempt {index}',
        noAttempts: 'No attempts yet',
        empty: 'None',
        durationMs: '{ms} ms',
        durationSeconds: '{seconds}s',
      },
      methodLabels: {
        fileParse: 'File parsing',
      },
      attemptStatuses: {
        completed: 'Completed',
        failed: 'Failed',
      },
      attemptResults: {
        content: 'Content captured',
        records: '{count} records produced',
        noRecords: 'No importable records',
        emptyContent: 'No content captured',
        error: 'Failed',
      },
      loadingStates: {
        queuedTitle: 'Waiting in recognition queue',
        queuedDesc: 'Files are processed independently, with up to 2 files running at once.',
        fileParseTitle: 'Running file parsing',
        fileParseDesc:
          'The system is reading file content and extracting fields. PDFs or scanned files may take longer.',
        fileParseSlowTitle: 'File parsing is still running',
        fileParseSlowDesc:
          'The parser is still processing this file. Images, scanned files, and large PDFs may take longer.',
        textRecognitionTitle: 'Running text recognition',
        textRecognitionDesc:
          'The system is extracting table field values from the parsed text and table schema.',
        textRecognitionSlowTitle: 'Text recognition is still running',
        textRecognitionSlowDesc: 'The model is producing field results. Keep this page open.',
        longRunningTitle: 'Recognition is still running. Do not close or refresh this page.',
        longRunningDesc:
          'Large PDFs, scanned files, and image parsing can take longer. Refreshing before the response returns will lose this page state.',
      },
      stageStatus: {
        fileParsing: 'File parsing',
        fileParsed: 'File parsed / waiting for text recognition',
        fileParseFailed: 'File parsing failed',
        textRecognizing: 'File parsed / text recognition running',
        textRecognitionFailed: 'File parsed / text recognition failed',
        textRecognitionNeedsCompletion: 'File parsed / needs completion',
        ready: 'File parsed / ready',
      },
      reviewSteps: {
        recognizeTitle: 'AI Recognition',
        recognizeDesc: 'Extract field values from file content',
        reviewTitle: 'Human Review',
        reviewDesc: 'Check mappings, formats, and required fields',
        commitTitle: 'Commit',
        commitDesc: 'Write table rows and preserve source lineage',
      },
      fileStatus: {
        normal: 'Normal',
        pending: 'Pending',
        queued: 'Queued',
        recognizing: 'Recognizing',
        success: 'Valid',
        failed: 'Invalid',
        parseFailed: 'Parse failed',
        validationFailed: 'Needs fields',
        needsCompletion: 'Needs completion',
        skipped: 'Skipped',
      },
      filters: {
        all: 'All',
        needs_action: 'Needs action',
        failed: 'Failed',
        ready: 'Ready',
      },
      stats: {
        processing: 'Processing {count}',
        ready: 'Ready {count}',
        needs: 'Needs completion {count}',
        failed: 'Failed {count}',
      },
      statusNotice: {
        processing:
          '{count} files are being recognized. Keep this page open; completed files update one by one.',
        needsAction:
          '{count} files are not ready. If recognition did not finish, it may be a temporary model response or parsing service issue. Try retrying first, or complete required fields before saving.',
        unsaved: 'Recognition results only live in this review page until you approve and commit.',
      },
      requiredEmptyTag: 'Required field is empty',
      validationAlert:
        'Some files are not ready. Fix parse failures or complete required fields before saving.',
      leftFilesInvalidTip: 'Some files are invalid',
      activeFileInvalidTip: 'This file is invalid',
      activeFileParseFailedTip: 'Current file parsing failed',
      activeFileValidationTip: 'Fields need completion',
      fileErrorTitle: 'Recognition did not finish',
      fileWarningTitle: 'Recognition result needs review',
      fileErrorRetryHint:
        'The file content is kept here, and retrying often works. This can happen when the model returns an unexpected format, the network is unstable, or the parsing service is temporarily unavailable. If it still fails, try again later or switch models.',
      parseErrorRetryHint:
        'File parsing has not produced usable text yet. Retry file parsing first; text recognition will continue after parsing succeeds.',
      recognitionErrorRetryHint:
        'The parsed file text is kept here, so you usually only need to retry text recognition. This can happen when the model returns an unexpected format, the network is unstable, or the model service is temporarily unavailable.',
      fileErrorDetails: 'View technical details',
      fileErrorFallback: 'No records were recognized from this file.',
      noRecordWarning:
        'The final model result did not return importable field records. Review the recognized content and complete fields manually if needed.',
      noParsedContentForRecognition:
        'This file does not have parsed content for text recognition yet. Retry file parsing first.',
      promptLoadFailedTitle: 'Recognition prompt failed to load',
      promptLoadFailedDesc: 'Unable to load the recognition prompt for this table: {error}',
      promptEmptyDesc:
        'The recognition prompt for this table is empty. Retry or save the prompt before recognizing files.',
      retryPrompt: 'Retry',
      retryCurrentFile: 'Retry current file',
      retryFailedFiles: 'Retry failed files',
      retryFileParse: 'Retry file parsing',
      retryTextRecognition: 'Re-extract fields',
      reprocessCurrentFile: 'Re-recognize current file',
      retryParseFailedFiles: 'Retry parse failures',
      retryRecognitionFailedFiles: 'Retry field extraction failures',
      reRecognizeAll: 'Re-recognize all',
      skipCurrentFile: 'Skip',
      removeCurrentFile: 'Remove',
      noFailedFiles: 'There are no failed files to retry',
      noParseFailedFiles: 'There are no file parsing failures to retry',
      noRecognitionFailedFiles: 'There are no field extraction failures to retry',
      confirmOverwriteCurrent:
        'Retrying this file will overwrite its recognition result and manual edits. Continue?',
      confirmOverwriteParseCurrent:
        'Retrying file parsing will read the file again and overwrite parsed text, field results, and manual edits for this file. Continue?',
      confirmOverwriteRecognitionCurrent:
        'Re-extracting fields will reuse the current parsed text and overwrite field results and manual edits for this file. Continue?',
      confirmOverwriteAll:
        'Re-recognizing all files will overwrite every recognition result and manual edit. Continue?',
      overwriteConfirmTitle: 'Overwrite recognition result?',
      overwriteConfirmAction: 'Retry and overwrite',
      keepCurrentResult: 'Keep current result',
      timestampInvalidTag: 'Invalid date format. Use a readable date.',
      extractionStrategy: 'Recognized by {strategy}',
      extractionFallback: '{reason}; switched to {strategy}',
      sourceTypes: {
        mineru: 'File parsing content',
      },
      originalImagePreview: 'Original image',
      originalFilePreview: 'Original file preview',
      loadingImagePreview: 'Loading original image...',
      imagePreviewUnavailable: 'Original image preview is unavailable',
      extractedContentPreview: 'Extracted text',
      batchAllSuccess: 'Recognized {count} files',
      batchPartialFailed: 'Recognized {success} of {total} files. {failed} failed.',
      batchAllFailed: 'Failed to recognize {count} files',
      batchRequestFailed: 'Failed to ingest files into table',
      parseRequestFailed: 'File parsing request failed',
      recognitionRequestFailed: 'Text recognition request failed',
    },
  },
  schemaHealth: {
    title: 'Schema needs semantic cleanup',
    description:
      '{count} imported placeholder fields were detected. Convert them to English business fields to improve query, templates, and smart ingest.',
    previewAction: 'Preview cleanup',
    previewTitle: 'Semantic field preview',
    previewDesc:
      'This is a safe preview only and will not change data. Applying it should use a rename/migration API and preserve original source fields.',
    currentField: 'Current field',
    sourceField: 'Source field',
    suggestedField: 'Suggested English field',
    confidence: 'Confidence',
    high: 'High',
    medium: 'Medium',
    later: 'Later',
    applyInStructure: 'Open structure',
  },
  // Prompt dialog for table ingest
  promptDialog: {
    title: 'Recognition Prompt',
    contentLabel: 'Prompt Content',
    resetDefault: 'Reset to Default',
    hintText:
      'Hint: A detailed prompt helps AI accurately identify the information you need. You can describe field types, formatting requirements, and key focus areas.',
    defaultText:
      'Based on the data and business context, infer and design a suitable table structure.',
  },
  // Model selector shared labels
  modelSelector: {
    label: 'LLM Model',
    placeholder: 'Select a model',
  },
  // Added from zh-Hans
  analyze: {
    success: 'AI analysis complete',
  },
  // SQL operations (record page)
  sqlOps: {
    title: 'SQL Operations',
    types: {
      create: 'Create',
      update: 'Update',
      delete: 'Delete',
      query: 'Query',
    },
    status: {
      success: 'Success',
      failed: 'Failed',
    },
  },
  all: 'All',
  operationType: 'Operation type',
  startTime: 'Start time',
  endTime: 'End time',
  time: 'Time',
  operation: 'Operation',
  table: 'Table',
  user: 'User',
  sql: 'SQL',
  // copy, status moved to common module
  noData: 'No data',
  // BI Chat Search page
  overview: {
    welcome: 'Welcome to',
    tablesCount: '{count} tables',
    noTables: 'No tables yet',
    noTablesDesc: 'Create your first table to start storing and querying data.',
    quickActions: 'Quick Actions',
    createFirstTable: 'Create First Table',
    dataSearch: 'Data Search',
    dataSearchDesc: 'Use AI to query your data with natural language',
    createTableDesc: 'Create a new table to store data',
    viewLogsDesc: 'View database operation history',
  },
  // Batch import dialog
  batchImport: {
    title: 'Batch Import',
    step1Title: '1. Download Current Table Template',
    step1Desc:
      'Generate an Excel template from this table structure, then fill rows using those field headers.',
    downloadTemplate: 'Download Template',
    downloadSuccess: 'Template downloaded successfully',
    downloadFailed: 'Failed to download template',
    step2Title: '2. Select File',
    dropOrClick: 'Choose a file from file management',
    supportedFormats: 'Supports Excel format files',
    selectFile: 'Select File',
    invalidFileType: 'Invalid file type. Please upload an Excel file.',
    cancel: 'Cancel',
    import: 'Import',
    importing: 'Importing...',
    importSuccess: 'Import completed',
    importFailed: 'Import failed',
    importResult: 'Total: {total}, Success: {success}, Failed: {failed}',
    skipUnmatchedColumns: 'Skip unmatched fields',
    skipUnmatchedColumnsDesc:
      'Columns that do not exist in the current table will not be imported. Required fields still need values.',
    errors: {
      noMatchingColumns: 'No Excel headers match the current table fields. Check the header row and try again.',
      missingRequiredColumns:
        'Excel is missing required fields: {fields}. Add these columns and try again, or make the fields optional first.',
    },
  },
  excelImport: {
    title: 'Import Excel',
    subtitle: 'Create a low-code table from spreadsheet structure and rows',
    entry: 'Import Excel',
    entryDesc: 'Recognize spreadsheet columns, create a table, and import rows',
    steps: {
      file: 'File',
      preview: 'Preview',
      schema: 'Schema',
      result: 'Result',
    },
    actions: {
      next: 'Next',
      previous: 'Previous',
      back: 'Back',
    },
    permissions: {
      title: 'No import permission',
      description:
        'Importing Excel creates tables and writes rows, so database manage permission is required.',
    },
    file: {
      choose: 'Choose spreadsheet from File Manager',
      supported: 'Supports .xlsx, .xls, and .csv',
      analyze: 'Analyze spreadsheet',
    },
    preview: {
      sheets: 'Sheets',
      rows: 'Preview rows',
      loadingSheet: 'Loading selected sheet...',
    },
    schema: {
      tableName: 'Table name',
      description: 'Table description',
      enabled: 'Import',
      source: 'Source column',
      name: 'Field name',
      type: 'Type',
      required: 'Required',
      requiredYes: 'Yes',
      requiredNo: 'No',
      descriptionColumn: 'Description',
      samples: 'Samples',
      import: 'Create table and import',
      tableInfoTitle: 'Table information',
      smartRecognizeTitle: 'Smart recognition',
      smartRecognizeDesc:
        'Choose a model to suggest the table name, table description, and field names. Apply them only after review.',
      smartRecognizeAction: 'Smart recognize',
      recognitionDialogTitle: 'Review smart recognition result',
      recognitionDialogDesc:
        'Suggestions will not overwrite the current draft until you apply them.',
      recognitionItem: 'Item',
      recognitionCurrent: 'Current',
      recognitionSuggested: 'Suggested',
      recognitionTableName: 'Table name',
      recognitionTableDescription: 'Table description',
      recognitionSourceColumn: 'Source column',
      recognitionEmpty: 'Empty',
      cancelRecognition: 'Cancel',
      applyRecognition: 'Apply recognition result',
      validation: {
        invalidTableName:
          'Table name may only contain lowercase letters, digits, and underscores, and must start with a lowercase letter.',
        duplicateFields: 'Duplicate field names exist. Fix them before importing.',
        invalidFieldNames:
          'Field names may only contain lowercase letters, digits, and underscores, and cannot use system fields or reserved keywords.',
      },
    },
    result: {
      title: 'Import finished',
      description: 'The spreadsheet has been converted into a low-code database table.',
      openTable: 'Open table',
      success: 'Import finished',
      partial: 'Import finished with row errors',
      totalRows: 'Total rows',
      importedRows: 'Imported',
      failedRows: 'Failed rows',
      failedItemsTitle: 'Failed rows',
      failedItemsDescription: '{count} rows were not imported. The first errors are shown below.',
      errorRow: 'Row',
      errorColumn: 'Field',
      errorReason: 'Reason',
    },
    errors: {
      analyzeFailed: 'Failed to analyze spreadsheet',
      importFailed: 'Failed to import spreadsheet',
      recognizeFailed: 'Failed to recognize spreadsheet metadata',
    },
  },
  biSearch: {
    title: 'Data Tables',
    searchPlaceholder: 'Search tables...',
    selectAll: 'Select all',
    selectedCount: '{count} selected',
    columns: 'columns',
    columnTypes: {
      integer: 'Integer',
      numeric: 'Number',
      boolean: 'Boolean',
      timestamp: 'DateTime',
      text: 'Text',
    },
    tableFields: 'Table Fields',
    totalFields: '{count} total fields',
    expand: 'Expand',
    collapse: 'Collapse',
    noWorkflowConfig: 'BI Chat workflow not configured',
    loadingWorkflow: 'Loading workflow configuration...',
    tableRequired: 'Please select at least one table',
    modelRequired: 'Please select a model',
    modelConfig: 'Model',
  },
};

export default messages;
export type DbsMessages = typeof messages;
