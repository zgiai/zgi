# ZGI - Enterprise AI Agent & RAG Orchestration Platform

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh-CN.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a>
</p>

<p align="center">
  <em>ZGI is a powerful enterprise-grade AI development platform focused on visual Agent workflow orchestration, advanced RAG systems, and multi-Agent collaboration.</em>
</p>

<p align="center">
   <a href="https://github.com/zgiai/zgi/blob/main/LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/zgiai/zgi">
  </a>
</p>

## 🌟 Core Features

### 💬 Intelligent Chat Interaction
- Multi-model dialogue support with parallel model comparison
- AI Agent integration via @mentions for personalized interactions
- RAG-enhanced knowledge base conversations
- Voice interaction and speech synthesis
- Image recognition and analysis
- File Q&A for PDF, Word, Excel, and more
- Visual debugging with response time and behavior analysis

### 🔍 Advanced RAG System
- State-of-the-art Retrieval Augmented Generation (RAG)
- Multiple vector database support (FAISS, Milvus, Weaviate, Qdrant)
- Customizable recall rate and search parameters
- Rich API ecosystem with RESTful and GraphQL support
- High-availability distributed architecture
- Granular data access control
- High-performance knowledge management

### 🤖 Multi-Agent Orchestration
- No-code visual workflow editor
- Flexible node-based design for parallel/serial task execution
- Domain-specific LLM adaptation
- Real-time monitoring and debugging
- Configurable inter-Agent communication

### 🔗 Enterprise API Integration
- Comprehensive RESTful APIs with Webhook support
- Detailed documentation, SDKs, and example code
- Modular architecture for custom extensions
- Microservices compatibility
- Multi-environment deployment options
- API-level access control
- High concurrency support

### 🔥 LLMS Gateway: 1000+ Models
- OpenAI SDK-compatible interface
- Built-in support for OpenAI, Claude, Gemini, LLaMA, Mistral, Command R+
- Multi-model comparison and switching
- On-premise deployment options
- Real-time token tracking and cost management
- Rate limiting and access control
- Comprehensive logging and auditing

### 🔐 Enterprise Security
- Hierarchical organization and project management
- Fine-grained permission control
- Data isolation and encryption
- SSO support (OAuth, LDAP, SAML)
- Audit logging and compliance
- Private deployment options
- Backup and recovery mechanisms

## 🚀 Why Choose ZGI?

✅ **Enterprise-Ready**: Complete organization, project, and permission management with private deployment options
✅ **Smart Knowledge Management**: Advanced RAG system with multimodal knowledge base and semantic search
✅ **Visual AI Workflows**: No-code Agent orchestration with multi-Agent collaboration
✅ **Extensive Model Support**: 1000+ models via LLMS Gateway with OpenAI SDK compatibility
✅ **Powerful API Ecosystem**: Standardized APIs with comprehensive documentation
✅ **Security-First**: Fine-grained access control with end-to-end encryption
✅ **High Performance**: Cloud-native architecture with distributed storage

## 🚀 Quick Start

### Prerequisites
- Ensure you have **yarn** or **bun** installed.

### Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/zgiai/zgi.git
   cd zgi
   ```

2. **Install dependencies:**
   ```bash
   yarn install
   # or
   bun install
   ```

3. **Start the API server:**
   ```bash
   cd api
   python run.py
   ```

4. **Start the development server:**
   ```bash
   yarn dev
   # or
   bun dev
   ```

## 🏗️ Project Structure

```
zgi/
├── api/          # Backend API services
├── frontend/     # Web interface
├── desktop/      # Desktop application
├── sdks/         # Software Development Kits
├── docs/         # Documentation
├── examples/     # Usage examples
└── docker/       # Docker configuration files
```

## 🗺 Roadmap

- **🎙️ Chat with PDF:** Coming soon
- **📚 Knowledge Spaces:** Coming soon
- **🌐 Multi-language Support:** Expanding language support
- **📱 Mobile App:** Native mobile applications

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guidelines](./docs/CONTRIBUTING.md) for details.

## Contributors

<a href="https://github.com/zgiai/zgi/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=zgiai/zgi" />
</a>

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=zgiai/zgi&type=Date)](https://star-history.com/#zgiai/zgi&Date)

## Security Disclosure

To protect your privacy, please avoid posting security issues on GitHub. Instead, send your questions to security@zgi.ai and we will provide you with a more detailed answer.

## License

This repository is available under the [MIT License](LICENSE). See the [LICENSE](LICENSE) file for more info.

## 🙏 Acknowledgments

Special thanks to all contributors and the open-source community for making this project possible.