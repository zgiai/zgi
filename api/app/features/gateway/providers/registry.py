from typing import Dict, Type, Optional, List
import yaml
import importlib
from pathlib import Path
from ..models.config import ModelConfig
from .base import BaseProvider
from .openai_provider import OpenAIProvider
from .deepseek_provider import DeepSeekProvider

class ModelRegistry:
    def __init__(self):
        self._models: Dict[str, ModelConfig] = {}
        self._load_configurations()

    def _load_configurations(self):
        """Load model configurations from YAML file"""
        config_path = Path(__file__).parent.parent / "config" / "models.yaml"
        try:
            with open(config_path, 'r') as f:
                config_data = yaml.safe_load(f)
                for model_name, model_data in config_data.get("models", {}).items():
                    self._models[model_name] = ModelConfig.from_dict(model_data)
        except Exception as e:
            raise RuntimeError(f"Failed to load model configurations: {str(e)}")

    def get_model(self, name: str) -> ModelConfig:
        """Get model configuration by name"""
        if name not in self._models:
            raise ValueError(f"Model {name} is not supported")
        return self._models[name]

    def get_provider(self, model_name: str, api_key: str, base_url: Optional[str] = None) -> BaseProvider:
        """
        Return the appropriate provider handler for a given model name.
        
        Args:
            model_name: Name of the model to use
            api_key: API key for the provider
            base_url: Optional base URL for the provider API
        
        Returns:
            An instance of the appropriate provider handler
        
        Raises:
            ValueError: If the model is not supported
        """
        model_config = self.get_model(model_name)
        
        # Import the handler class dynamically
        module_path = f"{__package__}.{model_config.handler.lower()}_provider"
        try:
            module = importlib.import_module(module_path)
            handler_class = getattr(module, model_config.handler)
        except (ImportError, AttributeError) as e:
            raise ValueError(f"Failed to load provider handler: {str(e)}")

        # Use configuration base_url if not provided
        effective_base_url = base_url or model_config.base_url
        
        provider_kwargs = {
            "api_key": api_key,
            "base_url": effective_base_url
        }
        
        # Add model name for providers that need it
        if model_config.model:
            provider_kwargs["model"] = model_config.model
            
        return handler_class(**provider_kwargs)

    def list_models(self) -> List[str]:
        """List all available model names"""
        return list(self._models.keys())

    def get_capabilities(self, model_name: str) -> List[str]:
        """Get capabilities of a specific model"""
        return self.get_model(model_name).capabilities

# Create a global instance
MODEL_REGISTRY = ModelRegistry()

# For backward compatibility
def get_provider(model_name: str, api_key: str, base_url: Optional[str] = None) -> BaseProvider:
    """Backward compatible get_provider function"""
    return MODEL_REGISTRY.get_provider(model_name, api_key, base_url)
