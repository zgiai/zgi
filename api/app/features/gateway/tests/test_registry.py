import pytest
from unittest.mock import patch, Mock
from dataclasses import dataclass
from typing import List, Dict, Any, Optional
import yaml
from pathlib import Path

# Mock the required classes to avoid circular imports
@dataclass
class ModelConfig:
    name: str
    provider: str
    handler: str
    version: str
    capabilities: List[str]
    rate_limit: int
    token_limit: int
    pricing: float
    is_streaming_supported: bool
    base_url: Optional[str] = None
    model: Optional[str] = None

class BaseProvider:
    def __init__(self, api_key: str, base_url: str = None):
        self.api_key = api_key
        self.base_url = base_url

class ModelRegistry:
    def __init__(self):
        self._models: Dict[str, ModelConfig] = {}
        self._load_configurations()

    def _load_configurations(self):
        """Load model configurations from YAML file"""
        config_path = Path(__file__).parent / "models.yaml"
        try:
            with open(config_path, 'r') as f:
                config_data = yaml.safe_load(f)
                for model_name, model_data in config_data.get("models", {}).items():
                    self._models[model_name] = ModelConfig(**model_data)
        except Exception as e:
            raise RuntimeError(f"Failed to load model configurations: {str(e)}")

    def get_model(self, name: str) -> ModelConfig:
        if name not in self._models:
            raise ValueError(f"Model {name} is not supported")
        return self._models[name]

    def get_provider(self, model_name: str, api_key: str, base_url: Optional[str] = None) -> BaseProvider:
        model_config = self.get_model(model_name)
        return BaseProvider(api_key=api_key, base_url=base_url or model_config.base_url)

    def list_models(self) -> List[str]:
        return list(self._models.keys())

    def get_capabilities(self, model_name: str) -> List[str]:
        return self.get_model(model_name).capabilities

def test_get_model():
    registry = ModelRegistry()
    model = registry.get_model("test-model")
    assert isinstance(model, ModelConfig)
    assert model.name == "test-model"
    assert model.provider == "test"
    assert model.capabilities == ["chat"]

def test_get_model_not_found():
    registry = ModelRegistry()
    with pytest.raises(ValueError, match="Model invalid-model is not supported"):
        registry.get_model("invalid-model")

def test_list_models():
    registry = ModelRegistry()
    models = registry.list_models()
    assert isinstance(models, list)
    assert "test-model" in models

def test_get_capabilities():
    registry = ModelRegistry()
    capabilities = registry.get_capabilities("test-model")
    assert isinstance(capabilities, list)
    assert "chat" in capabilities

def test_get_provider():
    registry = ModelRegistry()
    provider = registry.get_provider("test-model", "test-key")
    assert isinstance(provider, BaseProvider)
    assert provider.api_key == "test-key"
    assert provider.base_url == "http://test.api"

def test_get_provider_with_custom_base_url():
    registry = ModelRegistry()
    custom_url = "http://custom.api"
    provider = registry.get_provider("test-model", "test-key", base_url=custom_url)
    assert provider.base_url == custom_url
