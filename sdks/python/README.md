# ZGI Python SDK

A Python SDK for interacting with ZGI AI services. This SDK provides a simple and intuitive way to integrate ZGI's AI capabilities into your Python applications.

## Features

- Chat completion with various AI models
- Document management and search
- Knowledge base creation and management
- Built-in rate limiting and error handling
- Comprehensive logging
- Type hints for better IDE support

## Installation

```bash
pip install zgi
```

## Quick Start

```python
from zgi import ZGIClient

# Initialize the client
client = ZGIClient(api_key="your-api-key")

# Chat completion
messages = [{"role": "user", "content": "What is artificial intelligence?"}]
response = client.chat(messages)
print(response["choices"][0]["message"]["content"])

# Upload a document
doc_id = client.upload_document(knowledge_base_id=1, file_path="path/to/document.pdf")

# Search documents
results = client.search_documents(knowledge_base_id=1, query="AI applications")

# Create a knowledge base
kb = client.create_knowledge_base("My Knowledge Base")
```

## Configuration

The SDK can be configured using environment variables or directly through the client:

```python
from zgi import ZGIClient, configure_logging
import logging

# Configure logging
configure_logging(level=logging.DEBUG)

# Initialize client with custom configuration
client = ZGIClient(
    api_key="your-api-key",
    base_url="https://api.zgi.ai",
    default_model="gpt-3.5-turbo",
    timeout=30,
)
```

## Environment Variables

- `ZGI_API_KEY`: Your ZGI API key
- `ZGI_BASE_URL`: Base URL for the ZGI API (optional)
- `ZGI_DEFAULT_MODEL`: Default model to use for chat completion (optional)

## Error Handling

The SDK provides custom exceptions for different types of errors:

```python
from zgi import ZGIClient
from zgi.exceptions import APIError, AuthenticationError, RateLimitError

client = ZGIClient(api_key="your-api-key")

try:
    response = client.chat([{"role": "user", "content": "Hello"}])
except AuthenticationError:
    print("Invalid API key")
except RateLimitError:
    print("Rate limit exceeded")
except APIError as e:
    print(f"API error: {e}")
```

## Development

To set up the development environment:

```bash
# Clone the repository
git clone https://github.com/zgi-ai/zgi-python.git
cd zgi-python

# Install development dependencies
pip install -e ".[dev]"

# Run tests
pytest
```

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

For support, please contact support@zgi.ai or visit our [documentation](https://docs.zgi.ai).
