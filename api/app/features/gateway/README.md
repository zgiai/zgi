# Gateway Service

The Gateway Service is a unified LLM service gateway that manages and routes requests to different LLM Providers (such as OpenAI, Anthropic, DeepSeek, etc.).

## Architecture

### Core Components

1. **Provider Base Class** (`providers/base.py`)
   - Defines the interface that all Providers must implement
   - Provides common utility methods and properties

2. **Router** (`router/router.py`)
   - Routes requests to appropriate Providers based on model ID
   - Manages Provider instance lifecycle
   - Handles configuration loading and API key management

3. **Configuration Management** (`config/models.yaml`)
   - Defines model-to-Provider mappings
   - Manages model versions and configurations

## Adding a New Provider

Follow these steps to add a new LLM Provider:

### 1. Create Provider Class

Create a new Provider class file in the `providers` directory (e.g., `new_provider.py`):

```python
from .base import LLMProvider

class NewProvider(LLMProvider):
    """New provider implementation."""
    
    SUPPORTED_PREFIXES = ["new-model-"]  # Define supported model prefixes
    
    def __init__(self, provider_name: str = "new_provider", api_key: str = None, base_url: str = None):
        """Initialize provider."""
        super().__init__(provider_name, api_key, base_url)
        self.base_url = base_url or "https://api.new-provider.com"
        self.headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json"
        }
        
    async def chat_completion(self, messages, model, temperature=1.0, max_tokens=None, stream=False, **kwargs):
        """Implement chat completion method."""
        # Implement chat completion logic
        pass
```

### 2. Register Provider

Add the new Provider to `PROVIDER_CLASSES` in `router/router.py`:

```python
PROVIDER_CLASSES = {
    "openai": OpenAIProvider,
    "anthropic": AnthropicProvider,
    "deepseek": DeepSeekProvider,
    "new_provider": NewProvider  # Add new Provider
}
```

### 3. Add Model Configuration

Add new model configuration in `config/models.yaml`:

```yaml
new-model-v1:
  provider: new_provider
  model_name: new-model-v1
  description: New model description
  max_tokens: 4096
  supports_streaming: true
```

### 4. Set Environment Variables

Add necessary environment variables:

```bash
export NEW_PROVIDER_API_KEY=your_api_key
export NEW_PROVIDER_BASE_URL=https://api.new-provider.com  # Optional
```

## Best Practices

1. **Error Handling**
   - Handle API errors gracefully in Provider implementations
   - Use custom `ProviderError` exception class

2. **Configuration Management**
   - Use environment variables for sensitive information
   - Explicitly specify model capabilities in configuration files

3. **Code Style**
   - Follow Python type hints
   - Provide comprehensive docstrings
   - Implement necessary unit tests

## Examples

### Usage Example

```python
from gateway.router.router import Router

router = Router()
response = await router.route_request({
    "model": "new-model-v1",
    "messages": [{"role": "user", "content": "Hello!"}],
    "temperature": 0.7,
    "stream": False
})
```

### Test Example

```python
import pytest
from gateway.providers.new_provider import NewProvider

@pytest.mark.asyncio
async def test_new_provider():
    provider = NewProvider(api_key="test_key")
    response = await provider.chat_completion(
        messages=[{"role": "user", "content": "Hello"}],
        model="new-model-v1"
    )
    assert response["choices"][0]["message"]["content"]
```

## API Response Format

All providers should return responses in a standardized format:

```json
{
    "id": "response-id",
    "object": "chat.completion",
    "created": "timestamp",
    "model": "model-name",
    "choices": [{
        "index": 0,
        "message": {
            "role": "assistant",
            "content": "response content"
        },
        "finish_reason": "stop"
    }],
    "usage": {
        "prompt_tokens": 0,
        "completion_tokens": 0,
        "total_tokens": 0
    }
}
```

## Development Guidelines

1. **Provider Implementation**
   - Implement all required methods from the base class
   - Handle both streaming and non-streaming responses
   - Properly manage API rate limits and quotas

2. **Testing**
   - Write unit tests for normal operation
   - Test error conditions and edge cases
   - Mock external API calls in tests

3. **Documentation**
   - Document all public methods and classes
   - Include usage examples
   - Document any provider-specific limitations or features

4. **Security**
   - Never commit API keys or sensitive data
   - Validate all input parameters
   - Handle sensitive data according to security best practices

## Troubleshooting

Common issues and solutions:

1. **API Key Issues**
   - Ensure environment variables are properly set
   - Check API key permissions and quotas

2. **Model Configuration**
   - Verify model exists in `models.yaml`
   - Check model name matches provider's requirements

3. **Response Format**
   - Ensure provider response matches standard format
   - Check for missing required fields

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add your changes
4. Write/update tests
5. Submit a pull request

For more detailed information, please refer to the individual component documentation.
