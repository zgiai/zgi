# ZGI - 企业级 AI Agent 与 RAG 编排平台

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh-CN.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a>
</p>

<p align="center">
  <em>ZGI 是一个强大的企业级 AI 开发平台，专注于可视化 Agent 工作流编排、先进的 RAG 系统和多 Agent 协作。</em>
</p>

<p align="center">
   <a href="https://github.com/zgiai/zgi/blob/main/LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/zgiai/zgi">
  </a>
</p>

## 🌟 核心特性

### 💬 智能对话交互
- 支持多模型对话，可并行对比不同模型的回答
- 通过 @提及 集成 AI Agent，实现个性化交互
- RAG 增强的知识库对话
- 语音交互与语音合成
- 图像识别与分析
- 支持 PDF、Word、Excel 等文件的智能问答
- 可视化调试，包含响应时间和行为分析

### 🔍 先进的 RAG 系统
- 先进的检索增强生成（RAG）技术
- 多向量数据库支持（FAISS、Milvus、Weaviate、Qdrant）
- 可自定义的召回率和搜索参数
- 丰富的 API 生态，支持 RESTful 和 GraphQL
- 高可用分布式架构
- 精细的数据访问控制
- 高性能知识管理

### 🤖 多 Agent 编排
- 无代码可视化工作流编辑器
- 灵活的节点式设计，支持并行/串行任务执行
- 领域特定的大模型适配
- 实时监控和调试
- 可配置的 Agent 间通信

### 🔗 企业级 API 集成
- 全面的 RESTful API，支持 Webhook
- 详细的文档、SDK 和示例代码
- 模块化架构，支持自定义扩展
- 微服务兼容性
- 多环境部署选项
- API 级别的访问控制
- 高并发支持

### 🔥 LLMS Gateway：支持 1000+ 模型
- OpenAI SDK 兼容接口
- 内置支持 OpenAI、Claude、Gemini、LLaMA、Mistral、Command R+ 等
- 多模型对比和切换
- 本地部署选项
- 实时 Token 统计和成本管理
- 速率限制和访问控制
- 全面的日志和审计

### 🔐 企业级安全
- 分层的组织和项目管理
- 精细的权限控制
- 数据隔离和加密
- SSO 支持（OAuth、LDAP、SAML）
- 审计日志和合规
- 私有化部署选项
- 备份和恢复机制

## 🚀 为什么选择 ZGI？

✅ **企业级就绪**：完整的组织、项目和权限管理，支持私有化部署
✅ **智能知识管理**：先进的 RAG 系统，结合多模态知识库和语义搜索
✅ **可视化 AI 工作流**：无代码 Agent 编排，支持多 Agent 协作
✅ **广泛的模型支持**：通过 LLMS Gateway 支持 1000+ 模型，兼容 OpenAI SDK
✅ **强大的 API 生态**：标准化 API，配备完整文档
✅ **安全至上**：精细的访问控制，端到端加密
✅ **高性能**：云原生架构，分布式存储

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
