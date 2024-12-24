# LLM Gateway Service

[English](#english) | [中文](#中文) | [日本語](#日本語)

## English

### Overview
The LLM Gateway Service is a unified service gateway that manages and routes requests to different Language Model Providers (OpenAI, Anthropic, DeepSeek, etc.). It provides a consistent interface while handling provider-specific implementations and configurations.

### Key Features
- **Multi-Provider Support**: Seamlessly integrate with multiple LLM providers
- **Unified Interface**: Consistent API regardless of the underlying provider
- **Streaming Support**: Handle both streaming and non-streaming responses
- **Error Handling**: Robust error handling and logging
- **Configuration Management**: Flexible model and provider configuration
- **Rate Limiting**: Built-in rate limiting and quota management
- **Message Conversion**: Automatic message format conversion between providers

### Architecture

#### Core Components
1. **Provider System**
   - Base Provider Class
   - Provider-specific implementations
   - Provider Manager for instance lifecycle

2. **Router System**
   - Request routing based on model
   - API endpoint handling
   - Response streaming

3. **Configuration System**
   - Model configurations
   - Provider settings
   - Environment management

4. **Utility System**
   - HTTP client management
   - Message conversion
   - Response formatting
   - Error handling

### Usage

#### Adding a New Provider
1. Create provider class implementing `LLMProvider`
2. Add model configuration
3. Set environment variables
4. Register provider in `ProviderManager`

#### API Example
```python
from gateway.service.llm_service import LLMService

service = LLMService()
response = await service.create_chat_completion({
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}],
    "temperature": 0.7
})
```

### Development

#### Setup
```bash
# Install dependencies
pip install -r requirements.txt

# Set environment variables
export OPENAI_API_KEY=your_key
export ANTHROPIC_API_KEY=your_key
export DEEPSEEK_API_KEY=your_key
```

#### Testing
```bash
pytest tests/
```

## 中文

### 概述
LLM Gateway Service 是一个统一的服务网关，用于管理和路由不同语言模型提供商（OpenAI、Anthropic、DeepSeek 等）的请求。它提供了一致的接口，同时处理提供商特定的实现和配置。

### 主要特性
- **多提供商支持**：无缝集成多个 LLM 提供商
- **统一接口**：统一的 API，与底层提供商无关
- **流式响应**：支持流式和非流式响应
- **错误处理**：健壮的错误处理和日志记录
- **配置管理**：灵活的模型和提供商配置
- **速率限制**：内置速率限制和配额管理
- **消息转换**：提供商之间的自动消息格式转换

### 架构

#### 核心组件
1. **提供商系统**
   - 基础提供商类
   - 特定提供商实现
   - 提供商管理器（生命周期管理）

2. **路由系统**
   - 基于模型的请求路由
   - API 端点处理
   - 响应流处理

3. **配置系统**
   - 模型配置
   - 提供商设置
   - 环境管理

4. **工具系统**
   - HTTP 客户端管理
   - 消息转换
   - 响应格式化
   - 错误处理

### 使用方法

#### 添加新的提供商
1. 创建实现 `LLMProvider` 的提供商类
2. 添加模型配置
3. 设置环境变量
4. 在 `ProviderManager` 中注册提供商

#### API 示例
```python
from gateway.service.llm_service import LLMService

service = LLMService()
response = await service.create_chat_completion({
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "你好！"}],
    "temperature": 0.7
})
```

## 日本語

### 概要
LLM Gateway Service は、異なる言語モデルプロバイダー（OpenAI、Anthropic、DeepSeek など）へのリクエストを管理・ルーティングする統合サービスゲートウェイです。プロバイダー固有の実装と設定を処理しながら、一貫したインターフェースを提供します。

### 主な機能
- **マルチプロバイダー対応**：複数のLLMプロバイダーとのシームレスな統合
- **統一インターフェース**：基盤となるプロバイダーに関係なく一貫したAPI
- **ストリーミング対応**：ストリーミングおよび非ストリーミングレスポンスの処理
- **エラー処理**：堅牢なエラー処理とログ記録
- **設定管理**：柔軟なモデルとプロバイダーの設定
- **レート制限**：組み込みのレート制限とクォータ管理
- **メッセージ変換**：プロバイダー間の自動メッセージフォーマット変換

### アーキテクチャ

#### コアコンポーネント
1. **プロバイダーシステム**
   - 基本プロバイダークラス
   - プロバイダー固有の実装
   - インスタンスライフサイクル管理

2. **ルーターシステム**
   - モデルベースのリクエストルーティング
   - APIエンドポイント処理
   - レスポンスストリーミング

3. **設定システム**
   - モデル設定
   - プロバイダー設定
   - 環境管理

4. **ユーティリティシステム**
   - HTTPクライアント管理
   - メッセージ変換
   - レスポンスフォーマット
   - エラー処理

### 使用方法

#### 新しいプロバイダーの追加
1. `LLMProvider` を実装するプロバイダークラスを作成
2. モデル設定を追加
3. 環境変数を設定
4. `ProviderManager` にプロバイダーを登録

#### API 例
```python
from gateway.service.llm_service import LLMService

service = LLMService()
response = await service.create_chat_completion({
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "こんにちは！"}],
    "temperature": 0.7
})
```

## Contributing
Please read our [Contributing Guidelines](CONTRIBUTING.md) before submitting pull requests.

## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
