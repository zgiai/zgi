# ZGI - 一体化平台
# AGI 开发平台

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh-CN.md">简体中文</a> |
  <a href="./README_zh-TW.md">繁體中文</a> |
  <a href="./README_ja.md">日本語</a>
</p>

<p align="center">
  <em>ZGI是一个直观的LLM平台，支持多个LLM提供商，集成各种插件，提供个性化的AI助手体验。</em>
</p>

<p align="center">
   <a href="https://github.com/zgiai/zgi/blob/main/LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/zgiai/zgi">
  </a>
</p>

## 🌟 主要特性

- **👔 企业级应用：** 完善的权限管理系统，组织和项目管理功能，专为企业级大模型应用设计。
- **🔗 灵活模型集成：** 轻松对接各大模型厂商，自由设置模型使用限制和约束条件。
- **📊 深度统计分析：** 全方位的使用统计和分析功能，实时监控模型性能和用户交互。
- **🧠 多模型支持：** 
  - OpenAI (GPT-3.5, GPT-4)
  - Anthropic Claude
  - DeepSeek
  - 百度文心一言
  - 智谱 ChatGLM
  - 零一万物 Yi
  - 月之暗面 Moonshot
  - 百川
  - 讯飞星火
  - 商汤日日新
  - *更多模型持续接入中...*
- **🔌 插件生态系统：** 通过丰富的第三方插件增强平台功能，包括用于高级交互的函数调用。
- **📄 RAG增强检索：** 与各种文件格式（PDF、Markdown、JSON、Word、Excel、图像）交互，构建强大的信息检索系统。
- **🤖 自定义AI代理：** 创建和定制特定任务的AI代理，提供完全符合您需求的解决方案。
- **🗣️ 文字转语音：** 将AI生成的文本转换为语音，实现免提体验。
- **🎙️ 语音转文字（即将推出）：** 使用语音输入自然高效地与AI交互。
- **💾 本地存储：** 使用浏览器内IndexedDB安全地存储数据，确保隐私和更快的访问速度。
- **📤📥 轻松导入/导出：** 通过强大的数据可移植性轻松移动文档，实现平滑迁移和备份。
- **📚 知识空间（即将推出）：** 构建自定义知识库，存储和访问适合您兴趣的信息。
- **👤 个性化：** 利用记忆插件获得更具上下文感知的个性化AI响应，适应您的独特工作流程。

## 🚀 快速开始

### 前置条件
- 确保已安装 **yarn** 或 **bun**

### 安装步骤

1. **克隆仓库：**
   ```bash
   git clone https://github.com/zgiai/zgi.git
   cd zgi
   ```

2. **安装依赖：**
   ```bash
   yarn install
   # 或
   bun install
   ```

3. **启动API服务器：**
   ```bash
   cd api
   python run.py
   ```

4. **启动开发服务器：**
   ```bash
   yarn dev
   # 或
   bun dev
   ```

## 🏗️ 项目结构

```
zgi/
├── api/          # 后端API服务
├── frontend/     # Web界面
├── desktop/      # 桌面应用
├── sdks/         # 软件开发工具包
├── docs/         # 文档
├── examples/     # 使用示例
└── docker/       # Docker配置文件
```

## 🗺 路线图

- **🎙️ PDF对话：** 即将推出
- **📚 知识空间：** 即将推出
- **🌐 多语言支持：** 扩展语言支持
- **📱 移动应用：** 原生移动应用程序

## 🤝 贡献

我们欢迎贡献！请查看我们的[贡献指南](./docs/CONTRIBUTING.md)了解详情。

## 贡献者

<a href="https://github.com/zgiai/zgi/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=zgiai/zgi" />
</a>

## Star 历史

[![Star History Chart](https://api.star-history.com/svg?repos=zgiai/zgi&type=Date)](https://star-history.com/#zgiai/zgi&Date)

## 安全披露

为了保护您的隐私，请避免在 GitHub 上发布安全问题。请将您的问题发送至 security@zgi.ai，我们将为您提供更详细的答复。

## 开源协议

本仓库采用 [MIT 许可证](LICENSE)。查看 [LICENSE](LICENSE) 文件了解更多信息。

## 🙏 致谢

特别感谢所有贡献者和开源社区，使这个项目成为可能。
