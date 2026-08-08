# ZGI

<p align="center">
  <a href="README.md">English</a> &middot;
  <a href="README.zh-CN.md">简体中文</a> &middot;
  日本語 &middot;
  <a href="README.ko-KR.md">한국어</a>
</p>

<p align="center">
  <em>AIエージェントと実行可能なワークフローを構築・接続・公開・運用するための、ソースコードを利用できるAgent Runtimeワークスペース。</em>
</p>

<p align="center">
  <a href="https://github.com/zgiai/zgi/stargazers"><img src="https://img.shields.io/github/stars/zgiai/zgi?style=for-the-badge&logo=github&label=Stars&labelColor=111827&color=fbbf24" alt="GitHub stars" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-ZGI%20Community%20License-2563eb?style=for-the-badge&labelColor=111827" alt="ZGI Community License" /></a>
  <a href="#クイックスタート"><img src="https://img.shields.io/badge/Run-Docker%20Compose-2496ED?style=for-the-badge&logo=docker&logoColor=white&labelColor=111827" alt="Docker Composeで実行" /></a>
  <a href="web"><img src="https://img.shields.io/badge/Frontend-Next.js-000000?style=for-the-badge&logo=nextdotjs&logoColor=white&labelColor=111827" alt="Next.jsフロントエンド" /></a>
  <a href="api"><img src="https://img.shields.io/badge/Backend-Go-00ADD8?style=for-the-badge&logo=go&logoColor=white&labelColor=111827" alt="Goバックエンド" /></a>
</p>

<p align="center">
  <sub>
    <a href="#zgiを選ぶ理由">ZGIを選ぶ理由</a> &middot;
    <a href="#構築から運用まで">仕組み</a> &middot;
    <a href="#主な機能">主な機能</a> &middot;
    <a href="#クイックスタート">クイックスタート</a> &middot;
    <a href="#開発">開発</a> &middot;
    <a href="#ドキュメント">ドキュメント</a> &middot;
    <a href="#コントリビューション">コントリビューション</a> &middot;
    <a href="#ライセンス">ライセンス</a>
  </sub>
</p>

![ZGIビジュアルワークフローエディター](docs/assets/zgi-workflow-editor-api-enrichment.png)

## ZGIを選ぶ理由

ZGIは、AIアプリケーションにチャットで回答させるだけでなく、実際の業務を実行させたいチーム向けの、ソースコードを利用できるAgent Runtimeプラットフォームです。エージェント設定、ビジュアルワークフローのオーケストレーション、高度な知識検索、構造化データ、モデルルーティング、再利用可能なスキル、サンドボックス実行を、セルフホスト可能な1つのワークスペースに統合します。

エージェントを一度構築し、承認済みのナレッジベース、データベース、スキル、ワークフローに接続すれば、WebApp、社内アプリセンター、API、スケジュール実行、内部呼び出しを通じてユーザーに提供できます。公開後も、権限、ランタイムログ、バッチテストを使用してアプリケーションを継続的に監視・管理できます。

## 構築から運用まで

```text
エージェントとワークフローを構築
        ↓
モデル、ナレッジベース、データベース、ファイル、スキルを接続
        ↓
ツール、コード、知識検索、人が介在するステップを実行
        ↓
WebApp、アプリセンター、API、内部呼び出しで公開
        ↓
権限、ログ、バッチテストを使用して運用
```

## 主な機能

| 分野 | ZGIが提供する機能 |
| --- | --- |
| **エージェントアプリケーション** | 指示、モデル、メモリ、ナレッジベース、ファイル入力、スキル、ワークフロー連携を設定し、すぐに利用できるエージェントアプリケーションとして公開できます。 |
| **実行可能なワークフロー** | ビジュアルキャンバス上で、LLM呼び出し、分岐、ループ、承認、ユーザーへの質問、HTTPリクエスト、データベース操作、コード、ドキュメント、通知、スケジュールタスクをオーケストレーションできます。 |
| **高度な知識検索** | セマンティック検索、全文検索、ハイブリッド検索、ナレッジグラフ検索をリランキングと組み合わせ、エージェントのアクセス範囲を承認済みの知識とデータに限定できます。 |
| **スキルとサンドボックスツール** | ファイル、チャート、レポート、計算、データベース、ワークフロー呼び出しの機能を再利用可能な形でパッケージ化し、分離されたランタイムで実行できます。 |
| **モデルゲートウェイ** | プロバイダー、チャネル、認証情報、デフォルトモデル、ルーティングポリシー、クォータ、料金メタデータを一元管理できます。 |
| **公開とガバナンス** | WebApp、アプリセンター、APIキー、内部呼び出しを通じてエージェントを提供し、ワークスペース権限、ランタイムログ、再利用可能なバッチテストで管理できます。 |
| **セルフホスト型ランタイム** | コンソール、API、サンドボックス、Runner、PostgreSQL、Redisをローカル環境または独自のインフラストラクチャで実行できます。 |

## クイックスタート

ローカル環境ですべてのサービスを起動します：

```bash
make dev-docker
```

`make`がインストールされていない場合は、起動スクリプトを直接実行できます：

```bash
./dev/start-docker
```

コンソールを開きます：

```text
http://localhost:2679
```

初回起動時に、最初の管理者アカウントを作成してください。ZGIにはデフォルトの管理者アカウントは用意されていません。

サービスを停止します：

```bash
make docker-down
```

ログを確認します：

```bash
make docker-logs
```

## 開発

ソースコードから開発するには、以下をインストールしてください：

- DockerおよびDocker Compose
- Make
- Go
- Node.jsおよびpnpm

Webアプリケーションは`pnpm@10.12.1`を使用します。

依存関係を準備します：

```bash
make setup
```

APIとWebアプリケーションを別々のターミナルで起動します：

```bash
make dev-docker
make dev-api
make dev-web
```

## ドキュメント

製品ドキュメントは[`docs.zgi.ai`](https://docs.zgi.ai)でご覧いただけます。

リポジトリ内のその他のREADMEファイルには、主に開発およびコントリビューションに関する情報が記載されています。組み込みシステムスキルカタログなどのデプロイ動作については、[`docker/README.md`](docker/README.md#system-skill-catalog)を参照してください。

## コントリビューション

コントリビューションを歓迎します。Pull Requestを作成する前に、[`CONTRIBUTING.md`](CONTRIBUTING.md)をお読みください。

コミュニティの行動規範については、[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md)をご覧ください。

セキュリティに関する問題を報告する場合は、[`SECURITY.md`](SECURITY.md)の手順に従ってください。

## ライセンス

ZGIのソースコードは、Apache License 2.0をベースに追加条件を含むZGI Community Licenseの下で提供されています。ZGIは、個人、研究、教育、および組織内部での利用については無料です。ホスト型マルチテナントサービス、ホワイトラベルでの配布、ZGI公式ブランドの削除には商用ライセンスが必要です。このライセンスはOSI承認のオープンソースライセンスではありません。詳細は[`LICENSE`](LICENSE)をご覧ください。

ZGI Community Licenseが参照するApache License 2.0の全文は、[`LICENSE-APACHE`](LICENSE-APACHE)に収録されています。
