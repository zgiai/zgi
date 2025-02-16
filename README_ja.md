# ZGI - エンタープライズ AI Agent & RAG オーケストレーションプラットフォーム

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh-CN.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a>
</p>

<p align="center">
  <em>ZGIは、ビジュアルなAgentワークフローオーケストレーション、先進的なRAGシステム、マルチエージェントコラボレーションに特化した強力なエンタープライズAI開発プラットフォームです。</em>
</p>

<p align="center">
   <a href="https://github.com/zgiai/zgi/blob/main/LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/zgiai/zgi">
  </a>
</p>

## 🌟 主要機能

### 💬 インテリジェントチャット対話
- マルチモデル対話と並列モデル比較をサポート
- @メンション機能によるAI Agentの統合とパーソナライズされた対話
- RAG強化型ナレッジベース対話
- 音声対話と音声合成
- 画像認識と分析
- PDF、Word、Excel等のファイルに対するインテリジェントQ&A
- レスポンスタイムと動作分析を含む可視化デバッグ

### 🔍 先進的なRAGシステム
- 最先端の検索拡張生成（RAG）技術
- 複数のベクトルデータベース対応（FAISS、Milvus、Weaviate、Qdrant）
- カスタマイズ可能なリコール率と検索パラメータ
- RESTfulとGraphQLをサポートする豊富なAPIエコシステム
- 高可用性分散アーキテクチャ
- きめ細かいデータアクセス制御
- 高性能ナレッジマネジメント

### 🤖 マルチエージェントオーケストレーション
- ノーコードのビジュアルワークフローエディタ
- 並列/直列タスク実行に対応する柔軟なノードベース設計
- ドメイン特化型LLM適応
- リアルタイムモニタリングとデバッグ
- 設定可能なエージェント間通信

### 🔗 エンタープライズAPI統合
- Webhookをサポートする包括的なRESTful API
- 詳細なドキュメント、SDK、サンプルコード
- カスタム拡張可能なモジュラーアーキテクチャ
- マイクロサービス互換性
- マルチ環境デプロイメントオプション
- APIレベルのアクセス制御
- 高並列処理対応

### 🔥 LLMS Gateway：1000+モデル対応
- OpenAI SDK互換インターフェース
- OpenAI、Claude、Gemini、LLaMA、Mistral、Command R+等の内蔵サポート
- マルチモデル比較と切り替え
- オンプレミス展開オプション
- リアルタイムトークン追跡とコスト管理
- レート制限とアクセス制御
- 包括的なログとオーディット

### 🔐 エンタープライズセキュリティ
- 階層的な組織とプロジェクト管理
- きめ細かい権限制御
- データ分離と暗号化
- SSO対応（OAuth、LDAP、SAML）
- 監査ログとコンプライアンス
- プライベート展開オプション
- バックアップと復旧メカニズム

## 🚀 なぜZGIを選ぶのか？

✅ **エンタープライズレディ**：完全な組織、プロジェクト、権限管理とプライベート展開オプション
✅ **スマートナレッジマネジメント**：マルチモーダルナレッジベースとセマンティック検索を備えた先進的なRAGシステム
✅ **ビジュアルAIワークフロー**：ノーコードエージェントオーケストレーションとマルチエージェントコラボレーション
✅ **広範なモデルサポート**：LLMS Gatewayを通じて1000+モデルをサポート、OpenAI SDK互換
✅ **パワフルなAPIエコシステム**：標準化されたAPI、包括的なドキュメント
✅ **セキュリティファースト**：きめ細かいアクセス制御、エンドツーエンド暗号化
✅ **高性能**：クラウドネイティブアーキテクチャ、分散ストレージ

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
