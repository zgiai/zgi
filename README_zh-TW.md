# ZGI - 一體化平台
# AGI 開發平台

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh-CN.md">简体中文</a> |
  <a href="./README_zh-TW.md">繁體中文</a> |
  <a href="./README_ja.md">日本語</a>
</p>

<p align="center">
  <em>ZGI是一個直觀的LLM平台，支持多個LLM提供商，集成各種插件，提供個性化的AI助手體驗。</em>
</p>

<p align="center">
   <a href="https://github.com/zgiai/zgi/blob/main/LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/zgiai/zgi">
  </a>
</p>

## 🌟 主要特性

- **👔 企業級應用：** 完善的權限管理系統，組織和項目管理功能，專為企業級大模型應用設計。
- **🔗 靈活模型集成：** 輕鬆對接各大模型廠商，自由設置模型使用限制和約束條件。
- **📊 深度統計分析：** 全方位的使用統計和分析功能，實時監控模型性能和用戶交互。
- **🧠 多模型支持：** 
  - OpenAI (GPT-3.5, GPT-4)
  - Anthropic Claude
  - DeepSeek
  - 百度文心一言
  - 智譜 ChatGLM
  - 零一萬物 Yi
  - 月之暗面 Moonshot
  - 百川
  - 訊飛星火
  - 商湯日日新
  - *更多模型持續接入中...*
- **🔌 插件生態系統：** 通過豐富的第三方插件增強平台功能，包括用於高級交互的函數調用。
- **📄 RAG增強檢索：** 與各種文件格式（PDF、Markdown、JSON、Word、Excel、圖像）交互，構建強大的信息檢索系統。
- **🤖 自定義AI代理：** 創建和定制特定任務的AI代理，提供完全符合您需求的解決方案。
- **🗣️ 文字轉語音：** 將AI生成的文本轉換為語音，實現免提體驗。
- **🎙️ 語音轉文字（即將推出）：** 使用語音輸入自然高效地與AI交互。
- **💾 本地存儲：** 使用瀏覽器內IndexedDB安全地存儲數據，確保隱私和更快的訪問速度。
- **📤📥 輕鬆導入/導出：** 通過強大的數據可移植性輕鬆移動文檔，實現平滑遷移和備份。
- **📚 知識空間（即將推出）：** 構建自定義知識庫，存儲和訪問適合您興趣的信息。
- **👤 個性化：** 利用記憶插件獲得更具上下文感知的個性化AI響應，適應您的獨特工作流程。

## 🚀 快速開始

### 前置條件
- 確保已安裝 **yarn** 或 **bun**

### 安裝步驟

1. **克隆倉庫：**
   ```bash
   git clone https://github.com/zgiai/zgi.git
   cd zgi
   ```

2. **安裝依賴：**
   ```bash
   yarn install
   # 或
   bun install
   ```

3. **啟動API服務器：**
   ```bash
   cd api
   python run.py
   ```

4. **啟動開發服務器：**
   ```bash
   yarn dev
   # 或
   bun dev
   ```

## 🏗️ 項目結構

```
zgi/
├── api/          # 後端API服務
├── frontend/     # Web界面
├── desktop/      # 桌面應用
├── sdks/         # 軟件開發工具包
├── docs/         # 文檔
├── examples/     # 使用示例
└── docker/       # Docker配置文件
```

## 🗺 路線圖

- **🎙️ PDF對話：** 即將推出
- **📚 知識空間：** 即將推出
- **🌐 多語言支持：** 擴展語言支持
- **📱 移動應用：** 原生移動應用程序

## 🤝 貢獻

我們歡迎貢獻！請查看我們的[貢獻指南](./docs/CONTRIBUTING.md)了解詳情。

## 貢獻者

<a href="https://github.com/zgiai/zgi/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=zgiai/zgi" />
</a>

## Star 歷史

[![Star History Chart](https://api.star-history.com/svg?repos=zgiai/zgi&type=Date)](https://star-history.com/#zgiai/zgi&Date)

## 安全披露

為了保護您的隱私，請避免在 GitHub 上發布安全問題。請將您的問題發送至 security@zgi.ai，我們將為您提供更詳細的答覆。

## 🙏 致謝

特別感謝所有貢獻者和開源社區，使這個項目成為可能。
