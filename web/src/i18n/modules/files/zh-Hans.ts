import type { FilesMessages } from './en-US';

const messages: FilesMessages = {
  title: '文件管理',
  eyebrow: '资产库',
  description: '统一管理上传文件、文件夹和资源引用，上传后可用于知识库、数据库导入和对话工作流。',

  // Sidebar
  sidebar: {
    storage: '文件存储',
    newFolder: '新建文件夹',
    uploadFile: '上传文件',
    allFiles: '所有文件',
    uploadedFiles: '最近上传',
    favorites: '收藏夹',
    defaultFolders: '默认文件夹',
    noFolders: '暂无文件夹',
  },

  // File list
  fileList: {
    fileName: '文件名称',
    fileType: '文件类型',
    fileSize: '文件大小',
    processingStatus: '处理状态',
    relatedStatus: '关联状态',
    uploadDate: '上传日期',
    lastModified: '更新时间',
    actions: '操作',
    selectAll: '全选',
    selected: '已选择 {count}',
    totalItems: '共{total}项',
    relatedCount: '已关联{count}条',
    notRelated: '未关联',
    pendingCount: '{count} 项待确认',
    chunkCount: '{count} 个切片',
    embeddingCount: '{count} 个向量',
  },

  // File statuses
  status: {
    uploading: '上传中',
    completed: '已完成',
    failed: '失败',
    processing: '处理中',
    error: '错误',
    archived: '已归档',
  },

  processingStatus: {
    stored_only: '仅存储',
    parsing: '解析中',
    confirming: '待确认',
    generating: '索引中',
    parse_failed: '失败',
    ready: '已就绪',
  },

  // Actions
  actions: {
    view: '查看',
    download: '下载',
    delete: '删除',
    rename: '重命名',
    move: '移动',
    share: '分享',
    preview: '预览',
    viewDetails: '查看详情',
    more: '更多',
    downloadFile: '下载文件',
    addToFavorites: '加入收藏',
    removeFromFavorites: '取消收藏',
    bulkDelete: '批量删除',
    deleting: '删除中...',
  },

  preview: {
    title: '文件预览',
    description: '原文件预览',
    fileMeta: '{extension} 原文件预览',
    loading: '正在加载预览...',
    loadError: '文件预览加载失败',
    noFileSelected: '未选择文件',
    unsupportedTitle: '暂不支持预览该文件',
    unsupportedDescription: '当前支持图片、PDF、HTML、轻量文本类文件、DOCX 和 XLSX 原文件预览。',
    unsupportedFormatTitle: '{format} 文件暂不支持预览',
    unsupportedFormatDescription: '当前暂不支持预览 {format} 格式文件。请下载文件后在本地查看。',
    openInNewTab: '新窗口打开',
    unavailableTitle: '无法预览该文件',
    downloadOnlyDescription: '请下载文件后在本地查看。',
    htmlLimitedTitle: 'HTML 正在以隔离模式预览',
    htmlLimitedDescription:
      '页面脚本可用于动画效果，表单、弹窗、网络请求、嵌入框架和页面跳转仍会被阻止。',
    htmlOpenRiskTitle: '在新标签页打开原始 HTML？',
    htmlOpenRiskDescription:
      '新标签页会在隔离预览之外加载原始 HTML 文件。该页面可能执行脚本、跳转页面或访问外部资源。请仅在信任该文件时继续。',
    htmlOpenRiskConfirm: '仍然打开',
    htmlOpenRiskCancel: '取消',
    htmlTooLargeTitle: '该 HTML 文件过大，无法安全预览',
    officeUnsupportedTitle: '暂不支持预览该 Office 格式',
    officeTooLargeTitle: '该 Office 文件过大，无法预览',
    officeFallback: '请下载文件查看原始文档。',
    textTooLargeTitle: '该文本文件过大，无法预览',
    textFallback: '请下载文件查看完整内容。',
    emptyWorkbook: '该工作簿没有工作表',
    emptySheet: '该工作表没有可显示的行',
    rowLimit: '当前仅显示前 {count} 行。',
  },

  // Delete dialog
  delete: {
    cannotDelete: '无法删除文件',
    associationWarning: '该文件已关联到知识库或数据库，无法直接删除。',
    unlinkFirst: '请先删除所有关联项目，使文件状态变为"关联"后，再进行删除操作。',
    understood: '我知道了',
    viewRelated: '查看关联项目',
    folderConfirmTitle: '删除文件夹 "{name}"？',
    folderConfirmDescription: '此操作将永久删除文件夹，且无法撤销。',
    bulkConfirmTitle: '确定删除 {count} 个文件？',
    bulkConfirmDescription: '此操作将永久删除所选文件，且无法撤销。',
  },

  // Search and filter
  search: {
    placeholder: '搜索文件...',
    byName: '按名称',
    byType: '按类型',
    byDate: '按日期',
  },

  filter: {
    allProcessingStatuses: '全部状态',
  },

  detail: {
    backToFiles: '返回文件列表',
    previewOriginal: '预览原文件',
    processing: '处理进度',
    createdAt: '上传于 {time}',
    loadErrorTitle: '文件详情加载失败',
    loadErrorDescription: '文件可能已被删除，或当前账号没有访问权限。',
    processingError: '处理失败',
    basicInfo: '基础信息',
    fileId: '文件 ID',
    assetId: '资产 ID',
    storageType: '存储类型',
    workspaceId: '工作空间 ID',
    createdBy: '创建人',
    generationNo: '生成批次',
    nextViews: '详情视图',
    nextViewsDescription:
      '原文件预览、解析确认、内容切片、索引信息和重试操作会在后续阶段一前端任务中挂载到这里。',
    processingSummary: '处理汇总',
    pendingConfirmationCount: '待确认项',
    chunkCount: '切片数',
    embeddingCount: '向量数',
    createdDate: '上传日期',
    indexInfo: '索引信息',
    embeddingProvider: 'Embedding Provider',
    embeddingModel: 'Embedding Model',
    embeddingDimension: 'Embedding 维度',
    vectorStatus: {
      none: '向量未就绪',
      indexing: '向量索引中',
      ready: '向量已就绪',
      failed: '向量失败',
    },
    tabs: {
      overview: '概览',
      originalPreview: '原文件',
      parseReview: '解析确认',
      chunks: '内容切片',
      index: '索引信息',
    },
    parseReview: {
      title: '解析确认',
      notReadyTitle: '解析确认暂不可用',
      notReadyDescription: '请等待解析完成并进入待确认状态。',
      loadErrorTitle: '解析预览加载失败',
      loadErrorDescription: '解析产物可能尚未就绪。',
      elementCount: '{count} 个元素',
      pendingCount: '{count} 项待确认',
      batchIgnore: '忽略全部待确认项',
      emptyTitle: '暂无解析元素',
      emptyDescription: '解析器没有为该文件返回结构化元素。',
      page: '第 {page} 页',
      confidence: '置信度 {value}',
      emptyContent: '无文本内容',
      suggestedContent: '建议内容',
      keep: '保留',
      saveEdit: '保存修改',
      ignore: '忽略',
      resolvedHint: '该确认项已处理。',
      status: {
        pending: '待确认',
        kept: '已保留',
        edited: '已修改',
        ignored: '已忽略',
      },
      toasts: {
        resolved: '确认项已处理',
        batchIgnored: '待确认项已忽略',
        generateQueued: '确认完成，已开始生成。',
        resolveFailed: '确认项处理失败',
        batchIgnoreFailed: '批量忽略失败',
      },
    },
    chunks: {
      title: '内容切片',
      notReadyTitle: '切片尚未就绪',
      notReadyDescription: '解析、确认和向量生成完成后，才可以查看切片。',
      loadErrorTitle: '切片加载失败',
      loadErrorDescription: '当前文件的切片结果可能尚未就绪。',
      total: '{count} 个切片',
      generationNo: '生成批次 {value}',
      emptyTitle: '暂无切片',
      emptyDescription: '当前文件还没有可用的切片结果。',
      chunkTitle: '切片 {position}',
      parent: '父切片',
      leaf: '叶子切片',
      enabled: '启用',
      disabled: '停用',
      save: '保存',
      toasts: {
        updated: '切片已更新',
        updateFailed: '切片更新失败',
      },
    },
    index: {
      title: '索引信息',
      description: '文件加入知识库之前已经生成的文件级切片和 embedding 资产。',
      notReadyTitle: '索引信息尚未就绪',
      notReadyDescription: '切片和 embedding 生成开始后，才会展示索引元数据。',
    },
    reparse: {
      action: '重新解析',
      confirmTitle: '重新解析这个文件？',
      confirmDescription: '重新解析期间，当前可检索资产会不可用，文件会重新经历解析、确认和索引流程。',
      confirm: '重新解析',
      toasts: {
        started: '已提交重新解析请求',
        failed: '重新解析请求提交失败',
      },
    },
    views: {
      storedOnly: {
        title: '仅存储',
        description: '原文件已保存。后续可以发起解析，生成切片和向量。',
      },
      processing: {
        title: '解析中',
        description: '系统正在提取文档内容，页面会每 2 秒自动刷新。',
      },
      confirming: {
        title: '需要解析确认',
        description: '请先确认解析元素，然后再生成切片和向量。',
      },
      generating: {
        title: '正在生成切片和向量',
        description: '系统正在切块并写入 embedding，页面会每 2 秒自动刷新。',
      },
      ready: {
        title: '可检索资产已就绪',
        description: '切片和向量已经生成，接下来可以查看或编辑叶子切片。',
      },
      failed: {
        title: '处理失败',
        description: '请查看错误信息，后续重试入口接入后可重新处理。',
      },
    },
  },

  // Messages
  messages: {
    uploadSuccess: '文件上传成功',
    uploadFailed: '文件上传失败',
    deleteSuccess: '文件删除成功',
    deleteFailed: '文件删除失败',
    deleteConfirm: '确定要删除文件 "{name}" 吗？',
    deleteConfirmDesc: '此操作无法撤销。',
    noFiles: '暂无文件',
    noFilesDescWithUpload: '上传文档可加入知识库，上传表格可导入数据库，也可在对话工作流中引用。',
    noFilesDescWithUploadInSelector: '当前还没有文件，可以使用左侧栏中的上传入口添加文件。',
    noFilesDescWithoutUploadPermission:
      '当前工作空间暂无文件，且你没有该空间的上传权限。你可以联系管理员开通权限，或切换到其他工作空间。',
    empty: '这里什么都没有',
  },

  // Toast messages for hooks
  toast: {
    loadFilesError: '文件加载失败',
    storageUsageError: '存储使用情况加载失败',
    downloadSuccess: '文件下载成功',
    downloadError: '文件下载失败',
    deleteSuccess: '文件删除成功',
    deleteError: '文件删除失败',
    addFavoriteSuccess: '已加入收藏',
    addFavoriteError: '加入收藏失败',
    removeFavoriteSuccess: '已取消收藏',
    removeFavoriteError: '取消收藏失败',
    foldersLoadError: '文件夹加载失败',
    childFoldersLoadError: '子文件夹加载失败',
    uploadSuccess: '文件上传成功',
    uploadError: '文件上传失败',
    createFolderSuccess: '文件夹创建成功',
    createFolderError: '文件夹创建失败',
    updateFolderSuccess: '文件夹更新成功',
    updateFolderError: '文件夹更新失败',
    deleteFolderSuccess: '文件夹删除成功',
    deleteFolderError: '文件夹删除失败',
    createTextFileSuccess: '文本文件创建成功',
    createTextFileError: '文本文件创建失败',
  },

  // File selector
  selectFiles: '选择文件',
  selectedCount: '已选择 {count} 个',
  selectedCountWithMax: '已选择 {count}/{max} 个',
  selectedSingle: '已选择 1 个文件',
  confirmSelect: '确认选择 ({count})',
  confirmSelectSingle: '确认选择',
  maxCountExceeded: '最多只能选择 {max} 个文件',
  selectorContext: {
    title: '当前上下文',
    action: '切换空间',
    dialogTitle: '切换浏览空间',
    dialogDescription: '选择你要浏览的组织和工作空间。切换后，文件列表和上传入口会自动同步更新。',
    tipTitle: '关于上传权限',
    tipDescription: '只有当前工作空间具备文件上传权限时，左侧栏才会显示上传入口。',
  },
  mobileSelector: {
    browse: '浏览目录',
    browseAndUpload: '目录与上传',
    switchSpace: '切换空间',
    emptyDescriptionWithUpload:
      '当前还没有文件。点击上方“目录与上传”，然后使用其中的上传文件入口添加文件。',
    emptyDescriptionWithoutUpload:
      '当前工作空间暂无文件，且你没有该空间的上传权限。你可以联系管理员开通权限，或切换到其他工作空间。',
  },
  selectorEmptyState: {
    badge: '空状态',
    title: '当前无文件可选',
    description: '组织视图下当前没有可选文件。如需上传文件，请先选择所属工作空间。',
    noticeTitle: '上传前需要选择工作空间',
    noticeDescription: '系统文件会绑定到工作空间存储，上传前请选择一个工作空间。',
    quickActionTitle: '选择工作空间',
    quickActionDescription: '先在下方选择工作空间，然后上传到该工作空间。',
    noWorkspaces: '当前没有可切换的工作空间。',
  },

  // Resource types
  resourceTypes: {
    dataset: '数据集',
    agent: '智能体',
    workflow: '工作流',
    unknown: '未知',
  },

  // Upload dialog
  upload: {
    selectSource: '选择来源',
    workspaceLabel: '所属工作空间',
    workspacePlaceholder: '请选择所属工作空间',
    workspaceRequired: '请选择所属工作空间',
    storageLocation: '存储位置',
    selectFolder: '选择文件夹',
    defaultFolder: '默认文件夹',
    sourceType: '来源类型',
    processingMode: '处理方式',
    processingModes: {
      processNow: {
        title: '上传并解析',
        desc: '上传后立即解析、切片并建立索引。',
      },
      storeOnly: {
        title: '仅存储',
        desc: '先保存原始文件，之后可在文件详情中再解析。',
      },
    },
    processingHintTitle: '文档解析后才会进入可检索状态',
    processingHintDescription:
      '图片、图标、临时文件和暂不支持的格式会按仅存储处理，不进入文档处理链路。',
    uploadFiles: '上传文件',
    confirmUpload: '确认上传',
  },

  // Documents section (for compatibility)
  documents: {
    addDocument: '添加文档',
  },

  // Folder management
  folder: {
    createFolder: '新建文件夹',
    folderName: '文件类名称',
    folderNamePlaceholder: '请输入文件类名称',
    workspaceLabel: '所属工作空间',
    workspacePlaceholder: '请选择所属工作空间',
    workspaceRequired: '请选择所属工作空间',
    parentFolder: '父文件夹',
    selectParentFolder: '选择父文件夹',
    folderLabel: '文件夹：',
    rootFolder: '根目录',
  },

  // Text file creation
  text: {
    createTitle: '添加文本文件',
    fileNameLabel: '文件名称',
    fileNamePlaceholder: '请输入文件名称（不需要扩展名）',
    contentLabel: '文本内容',
    contentPlaceholder: '请输入文本内容',
    charCount: '字符数',
    fileSize: '附件大小',
    storageLocation: '存储位置',
    saveFile: '保存文件',
  },
};

export default messages;
