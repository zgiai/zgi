# ZGI - All-in-One Platform
# For AGI Development

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh-CN.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a>
</p>

<p align="center">
  <em>ZGI is an intuitive LLM platform that supports multiple LLM providers, integrates various plugins, and provides a personalized AI assistant experience.</em>
</p>

<p align="center">
   <a href="https://github.com/zgiai/zgi/blob/main/LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/zgiai/zgi">
  </a>
</p>

## 🌟 Key Features

- **👔 Enterprise-Ready:** Comprehensive permission management, organization and project management capabilities, designed for enterprise-level LLM applications.
- **🔗 Flexible Model Integration:** Easily connect with various LLM providers and freely set model usage limits and constraints.
- **📊 Advanced Analytics:** In-depth usage statistics and analytics for monitoring model performance and user interactions.
- **🧠 Multi-Model Support:** 
  - OpenAI (GPT-3.5, GPT-4)
  - Anthropic Claude
  - DeepSeek
  - Baidu ERNIE Bot
  - Zhipu ChatGLM
  - 01.AI Yi
  - Moonshot
  - Baichuan
  - Xunfei Spark
  - SenseNova
  - *More models coming soon...*
- **🔌 Plugin Ecosystem:** Enhance platform capabilities with a wide range of third-party plugins, including function calling for advanced interactions.
- **📄 RAG-Enhanced Retrieval:** Interact with various file formats (PDF, Markdown, JSON, Word, Excel, images) to build a powerful information retrieval system.
- **🤖 Custom AI Agents:** Create and tailor AI agents for specific tasks, providing solutions perfectly suited to your needs.
- **🗣️ Text-to-Speech:** Convert AI-generated text into speech for a hands-free experience.
- **🎙️ Speech-to-Text (Coming Soon):** Use voice input to interact with AI naturally and efficiently.
- **💾 Local Storage:** Securely store data locally using in-browser IndexedDB, ensuring privacy and faster access.
- **📤📥 Easy Import/Export:** Effortlessly move documents with robust data portability for smooth migration and backups.
- **📚 Knowledge Spaces (Coming Soon):** Build custom knowledge bases to store and access information tailored to your interests.
- **👤 Personalization:** Utilize the memory plugin for more contextual, personalized AI responses that adapt to your unique workflow.

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