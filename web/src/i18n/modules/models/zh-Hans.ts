import type { ModelsMessages } from './en-US';

const messages: ModelsMessages = {
  modelParameters: '模型参数',
  unsavedChanges: {
    title: '未保存的参数变更',
    description: '存在未保存的参数修改，是否保存？',
    discard: '不保存',
    save: '保存',
  },
  configParameters: {
    presets: {
      title: '预设',
      description: '按当前模型 schema 自动应用一组更专业的对话参数，不支持的项会自动跳过。',
      placeholder: '选择预设',
      defaultLabel: '均衡',
      items: {
        precise: {
          label: '精准',
          description: '降低随机性，适合翻译、抽取、改写、分类等高一致性任务。',
        },
        balanced: {
          label: '均衡',
          description: '适合作为通用默认值，兼顾稳定性、自然度与表达流畅性。',
        },
        creative: {
          label: '创作',
          description: '提高表达多样性，适合脑暴、文案、故事和更具表现力的写作。',
        },
      },
    },
    states: {
      empty: {
        badge: '默认配置',
        title: '当前模型使用默认参数',
        description: '该模型没有开放可调的运行时参数，因此这里会直接沿用提供商默认配置。',
        hint: '如果需要更细的参数控制，可以切换到支持高级参数调节的模型。',
      },
      loadFailed: {
        badge: '加载失败',
        title: '参数定义加载失败',
        description: '暂时无法加载该模型的参数 schema，请尝试重新打开弹窗或重新选择模型。',
        hint: '如果问题持续存在，可能是当前提供商没有返回有效的参数 schema。',
      },
      notFound: {
        badge: 'Schema 不可用',
        title: '参数定义不可用',
        description: '当前模型没有提供参数 schema，因此这里暂时无法配置运行时参数。',
        hint: '你仍然可以继续使用该模型，只是当前选择下暂不支持参数调节。',
      },
    },
    templates: {
      temperature: {
        label: '温度',
        help: '控制输出的随机性，值越低越稳定。',
      },
      top_p: {
        label: 'Top P',
        help: '将采样限制在累计概率阈值内的候选 token 范围。',
      },
      presence_penalty: {
        label: '存在惩罚',
        help: '鼓励模型引入新内容，减少重复已有主题。',
      },
      frequency_penalty: {
        label: '频率惩罚',
        help: '根据 token 重复频率施加惩罚，减少重复输出。',
      },
      logit_bias: {
        label: 'Logit Bias',
        help: '在采样前调整指定 token 的概率分布。',
      },
      seed: {
        label: '随机种子',
        help: '使用固定随机种子，提高多次请求结果的一致性。',
      },
      stop: {
        label: '停止序列',
        help: '命中设定的停止序列后结束生成。',
      },
      max_tokens: {
        label: '最大 Tokens',
        help: '限制本次生成可输出的最大 token 数。',
      },
    },
  },
  selector: {
    placeholder: '选择模型',
    searchPlaceholder: '搜索模型...',
    refreshSuccess: '刷新成功',
    noResults: '未找到匹配的模型',
    noModelsAvailable: '暂无可用模型',
    types: {
      llm: '大语言模型',
      'text-embedding': '向量模型',
      rerank: '重排序',
      moderation: '内容审核',
      speech2text: '语音转文字',
      tts: '文字转语音',
    },
    usecases: {
      'text-chat': '文本对话',
      vision: '图像理解',
      'image-gen': '图像生成',
      embedding: '文本向量',
      rerank: '搜索重排',
      'speech-to-text': '语音识别',
      'text-to-speech': '语音转换',
      'realtime-audio': '实时语音',
      'video-gen': '视频生成',
      moderation: '内容审核',
      reasoning: '深度推理',
      'function-calling': '函数调用',
    },
    empty: {
      noModelsTitle: '需要先配置模型',
      noResults: '没有符合搜索条件的模型',
      noModels: '暂无可用的{type}',
      contactAdmin: '请联系管理员为当前工作空间启用模型。',
      configureDescription: '请先配置至少一个可用的{type}，然后再继续使用当前工作流或智能体。',
      configure: '去配置',
      clearSearch: '清空搜索',
      refresh: '刷新模型',
    },
    expandAll: '展开全部',
    collapseAll: '收起全部',
    modelCount: {
      single: '个模型',
      multiple: '个模型',
    },
    tooltip: {
      modelId: '模型ID：',
      context: '上下文',
      useCases: '使用场景',
      features: '功能',
      description: '描述：',
      unknown: '未知',
    },
  },
  messages: {
    loadFailed: '加载模型失败',
    createSuccess: '自定义模型创建成功',
    createFailed: '创建自定义模型失败',
    updateSuccess: '自定义模型更新成功',
    updateFailed: '更新自定义模型失败',
    deleteSuccess: '自定义模型删除成功',
    deleteFailed: '删除自定义模型失败',
  },
};

export default messages;
