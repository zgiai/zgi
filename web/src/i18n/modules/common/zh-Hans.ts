import type { CommonMessages } from './en-US';

const messages: CommonMessages = {
  search: '搜索',
  filter: '筛选',
  actions: '操作',
  edit: '编辑',
  delete: '删除',
  cancel: '取消',
  confirm: '确认',
  close: '关闭',
  save: '保存',
  loading: '加载中...',
  error: '错误',
  success: '成功',
  add: '添加',
  create: '创建',
  update: '更新',
  view: '查看',
  accessDenied: '访问受限',
  unauthorizedDescription: '您没有权限访问此页面，请联系管理员。',
  name: '名称',
  description: '简介',
  noDescription: '暂无描述',
  model: '模型',
  status: '状态',
  active: '活跃',
  inactive: '不活跃',
  pending: '待处理',
  banned: '已禁用',
  createdAt: '创建时间',
  updatedAt: '更新时间',
  all: '全部',
  yes: '是',
  no: '否',
  enabled: '已启用',
  disabled: '已禁用',
  next: '下一步',
  previous: '上一步',
  more: '更多',
  comingSoon: '即将推出',
  back: '返回',
  items: '项',
  characters: '字符',
  selected: '已选择',
  reset: '重置',
  resetFilters: '取消筛选',
  tokens: 'tokens',
  refresh: '刷新',
  refreshSuccess: '刷新成功',
  saveSuccess: '保存成功',
  saveFailed: '保存失败',
  clear: '清空',
  copy: '复制',
  copyOutput: '复制输出',
  paste: '粘贴',
  results: '结果',

  // Network error
  networkError: '网络连接失败，请检查网络后重试',
  requestErrors: {
    generic: '操作没有成功，请稍后重试。',
    forbidden: '您没有权限执行此操作。',
    notFound: '未找到对应资源，请刷新后重试。',
    rateLimited: '当前请求过于频繁，请稍后再试。',
    serverBusy: '服务暂时不可用，请稍后再试。',
    timeout: '请求超时，请稍后重试。',
    sessionExpired: '登录态已失效，请重新登录。',
    passwordValidation: '密码不符合规则，请检查后重试。',
  },

  // Pagination
  pagination: {
    prev: '上一页',
    next: '下一页',
    info: '显示 {start} - {end} 项，共 {total} 项',
    jumpPlaceholder: '页码',
    jump: '跳转',
  },

  // Icon input
  iconInput: {
    uploadImage: '上传图片',
    aiGenerate: 'AI生成',
    textIcon: '文字图标',
    supportedFormats: '支持jpg、png格式,大小不超过2M',
    defaultIcon: 'Z',
    dropToUpload: '松开鼠标上传图片',
    textIconDialog: {
      title: '编辑文字图标',
      iconText: '图标文字',
      iconTextPlaceholder: '请输入图标文字（最多2个字符）',
      backgroundColor: '背景颜色',
      cancel: '取消',
      save: '保存',
    },
    imageCropDialog: {
      title: '上传并裁切图片',
      uploadText: '拖拽图片到此处，或点击选择',
      selectFile: '选择文件',
      cancel: '取消',
      crop: '裁切',
    },
  },

  // Image Cropper
  imageCropper: {
    title: '上传并裁切图片',
    uploadText: '拖拽图片到此处，或点击选择',
    dropHere: '松开鼠标上传图片',
    supportedFormats: '支持jpg、png格式,大小不超过2M',
    uploadProgress: '上传进度',
    cancel: '取消',
    crop: '裁切',
  },

  // Workspace Selector
  workspaceSelector: {
    placeholder: '选择工作空间',
    search: '搜索工作空间',
    loading: '加载中...',
    noResults: '没有找到匹配的工作空间',
    noWorkspaces: '没有可用的工作空间',
  },

  assetMove: {
    title: '移动到工作空间',
    description: '选择目标工作空间，并在确认前查看移动检查结果。',
    descriptionWithName: '将“{name}”移动到其他工作空间，确认前请查看移动检查结果。',
    targetWorkspace: '目标工作空间',
    targetWorkspacePlaceholder: '选择目标工作空间',
    previewing: '正在检查移动...',
    unknownWorkspace: '未知工作空间',
    blockersTitle: '无法移动',
    warningsTitle: '请确认提示',
    confirm: '移动',
    previewFailed: '移动检查失败',
    moveSuccess: '移动成功',
    moveFailed: '移动失败',
  },

  // Error Boundary
  errorBoundary: {
    somethingWentWrong: '出错了',
    unexpectedError: '应用程序发生了意外错误。',
    tryAgain: '重试',
    configurationError: '配置错误',
    configurationErrorMessage: '加载配置表单时出错，请尝试刷新页面。',
  },

  // Error Pages
  errorPages: {
    notFound: {
      title: '404',
      subtitle: '页面未找到',
      description: '抱歉，您访问的页面不存在或已被移除。',
      backHome: '返回主页',
    },
    error: {
      title: '出错了',
      description: '当前页面发生了前端渲染错误，已停止继续渲染以避免影响数据。',
      recoveryHint: '请先重试；如果仍然失败，复制诊断信息发给管理员定位问题。',
      diagnostics: '诊断信息',
      retry: '重试',
      reload: '刷新页面',
      back: '返回上一页',
      home: '返回控制台',
      copy: '复制诊断信息',
      copied: '已复制',
      refresh: '刷新页面',
    },
  },

  modelMultiSelector: {
    placeholder: '请选择模型…',
    selectProviderFirst: '请先选择提供商以加载可用模型',
    selectedTitle: '已选择的模型',
    remove: '移除 {name}',
    selectAll: '全选',
    clear: '清空',
    searchPlaceholder: '搜索模型…',
    noModels: '暂无可用模型',
    loading: '加载中...',
    scrollForMore: '滚动加载更多',
  },

  // Organization View empty states
  personalSpaceEmpty: {
    agents: '无可用智能体',
    datasets: '无可用知识库',
    databases: '无可用数据库',
    files: '无可用文件',
    description: '在组织视图中，您可以浏览组织资源。如需执行工作空间内操作，请切换到具体工作空间。',
    startCreating: '开始创建',
    selectWorkspaceHint: '请选择一个工作空间后继续',
    overlayHint: '点击高亮区域选择工作空间，或点击其他位置关闭',
    noWorkspacesHint: '您尚未加入任何工作空间，请联系管理员邀请您加入。',
  },

  // Workspace Mismatch Guard
  workspaceMismatch: {
    title: '工作空间不匹配',
    description: '此资源属于工作空间“{workspaceName}”，您当前处于“{currentWorkspaceName}”。',
    descriptionInOrg: '此资源属于工作空间“{workspaceName}”，您当前处于组织视图。',
    switchButton: '切换到该工作空间',
    actionHint: '请切换工作空间后重试',
  },

  organization: {
    fetchOrgFailed: '获取组织信息失败',
    switchOrgFailed: '切换组织失败',
    switchOrgSuccess: '切换组织成功',
  },

  // Status labels
  statusLabels: {
    loading: '加载中...',
    success: '成功',
    failed: '失败',
    error: '错误',
    processing: '处理中',
    completed: '已完成',
    enabled: '已启用',
    disabled: '已禁用',
    waiting: '等待中',
    paused: '已暂停',
    saving: '保存中...',
    creating: '创建中...',
    deleting: '删除中...',
    updating: '更新中...',
  },

  // Toast messages
  toasts: {
    createSuccess: '创建成功',
    createFailed: '创建失败',
    updateSuccess: '更新成功',
    updateFailed: '更新失败',
    deleteSuccess: '删除成功',
    deleteFailed: '删除失败',
    saveSuccess: '保存成功',
    saveFailed: '保存失败',
    copySuccess: '已复制到剪贴板',
    copyFailed: '复制失败',
    operationSuccess: '操作成功',
    operationFailed: '操作失败',
  },

  // Table headers
  table: {
    name: '名称',
    status: '状态',
    actions: '操作',
    createdAt: '创建时间',
    updatedAt: '更新时间',
    description: '描述',
    type: '类型',
    size: '大小',
  },

  // Confirmation dialogs
  confirmDialog: {
    deleteTitle: '确认删除',
    deleteDescription: '确定要删除 "{name}" 吗？',
    irreversibleWarning: '此操作无法撤销。',
    confirmButton: '确认',
    cancelButton: '取消',
  },

  sensitiveOutput: {
    blocked: '抱歉，我无法处理这个问题，请换一个试试吧。',
  },

  notificationSms: {
    fields: {
      recipients: '手机号',
      template: '短信模板',
    },
    setup: {
      title: '短信服务未配置',
      description: '请先完成短信服务商、短信签名和短信模板配置，再使用短信通知。',
      templatePlaceholder: '短信模板未配置',
    },
    templates: {
      pendingActionNotification: '待办通知',
      workflowAlert: '工作流告警',
    },
    params: {
      notificationTitle: '通知标题',
      linkCode: '链接参数',
      remark: '备注',
      summary: '摘要',
    },
    placeholders: {
      recipient: '手机号 {index}',
      recipientSingle: '手机号，多个用英文逗号分隔',
      notificationTitle: '您有一项新的任务待处理',
      linkCode: '任务或通知链接参数',
      remark: '请输入备注',
      summary: '请输入摘要',
      param: '请输入{label}',
    },
    actions: {
      addRecipient: '添加手机号',
      removeRecipient: '移除手机号 {index}',
    },
    help: {
      recipients: '支持单个手机号、英文逗号分隔的多个手机号，也支持可解析为手机号的变量。',
      linkCode: '建议填写短码，例如 abc123；不要包含 -、_、中文或完整链接。',
    },
    validation: {
      paramRequired: '请填写{label}。',
      paramInvalid: '{label}格式不正确。',
      paramTooLong: '{label}不能超过 {max} 个字符。',
    },
    preview: '模板预览',
    previewHint: '实际短信内容以后端配置的服务商审核模板为准。',
    previewUnavailable: '当前模板没有配置预览文案。',
  },

  // Form elements
  form: {
    required: '必填',
    optional: '可选',
    selectPlaceholder: '请选择...',
    inputPlaceholder: '请输入...',
    searchPlaceholder: '搜索...',
  },
};

export default messages;
