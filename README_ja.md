# ZGI - オールインワンプラットフォーム
# AGI 開発プラットフォーム

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh-CN.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a>
</p>

<p align="center">
  <em>ZGIは、複数のLLMプロバイダーをサポートし、様々なプラグインを統合し、パーソナライズされたAIアシスタント体験を提供する直感的なLLMプラットフォームです。</em>
</p>

<p align="center">
   <a href="https://github.com/zgiai/zgi/blob/main/LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/zgiai/zgi">
  </a>
</p>

## 🌟 主な機能

- **👔 エンタープライズ対応：** 包括的な権限管理、組織とプロジェクト管理機能を備え、エンタープライズレベルのLLMアプリケーション向けに設計。
- **🔗 柔軟なモデル統合：** 各種LLMプロバイダーとの簡単な接続、モデル使用制限と制約の自由な設定が可能。
- **📊 高度な分析：** モデルのパフォーマンスとユーザーインタラクションを監視する詳細な使用統計と分析機能。
- **🧠 マルチモデルサポート：** 
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
  - *より多くのモデルを順次追加予定...*
- **📄 RAG強化検索：** PDF、Markdown、JSON、Word、Excel、画像など様々なファイル形式と連携し、強力な情報検索システムを構築。
- **🤖 カスタムAIエージェント：** 特定のタスクに合わせてAIエージェントを作成・調整し、ニーズに完全に適合したソリューションを提供。
- **🗣️ テキスト読み上げ：** AIが生成したテキストを音声に変換し、ハンズフリーでの体験を実現。
- **🎙️ 音声認識（近日公開）：** 音声入力を使用して自然かつ効率的にAIと対話。
- **💾 ローカルストレージ：** ブラウザ内のIndexedDBを使用してデータをローカルに安全に保存し、プライバシーと高速アクセスを確保。
- **📤📥 簡単なインポート/エクスポート：** 堅牢なデータポータビリティにより、スムーズな移行とバックアップのためのドキュメント移動が容易。
- **📚 ナレッジスペース（近日公開）：** 興味に合わせた情報を保存・アクセスするためのカスタムナレッジベースを構築。
- **👤 パーソナライゼーション：** メモリプラグインを活用して、より文脈に応じた個人化されたAIレスポンスを実現し、独自のワークフローに適応。

## 🚀 クイックスタート

### 前提条件
- **yarn** または **bun** がインストールされていること。

### インストール

1. **リポジトリのクローン：**
   ```bash
   git clone https://github.com/zgiai/zgi.git
   cd zgi
   ```

2. **依存関係のインストール：**
   ```bash
   yarn install
   # または
   bun install
   ```

3. **APIサーバーの起動：**
   ```bash
   cd api
   python run.py
   ```

4. **開発サーバーの起動：**
   ```bash
   yarn dev
   # または
   bun dev
   ```

## 🏗️ プロジェクト構造

```
zgi/
├── api/          # バックエンドAPIサービス
├── frontend/     # Webインターフェース
├── desktop/      # デスクトップアプリケーション
├── sdks/         # ソフトウェア開発キット
├── docs/         # ドキュメント
├── examples/     # 使用例
└── docker/       # Docker設定ファイル
```

## 🗺 ロードマップ

- **🎙️ PDFとのチャット：** 近日公開
- **📚 ナレッジスペース：** 近日公開
- **🌐 多言語サポート：** 言語サポートの拡大
- **📱 モバイルアプリ：** ネイティブモバイルアプリケーション

## 🤝 貢献

貢献を歓迎します！詳細は[貢献ガイドライン](./docs/CONTRIBUTING.md)をご覧ください。

## コントリビューター

<a href="https://github.com/zgiai/zgi/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=zgiai/zgi" />
</a>

## スター履歴

[![Star History Chart](https://api.star-history.com/svg?repos=zgiai/zgi&type=Date)](https://star-history.com/#zgiai/zgi&Date)

## セキュリティ開示

プライバシー保護のため、GitHubでのセキュリティ問題の投稿はお控えください。代わりに、security@zgi.aiまでご質問をお送りください。より詳細な回答を提供させていただきます。

## 🙏 謝辞

このプロジェクトを可能にしてくれたすべての貢献者とオープンソースコミュニティに特別な感謝を捧げます。
