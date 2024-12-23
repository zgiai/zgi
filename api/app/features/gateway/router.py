"""Router for LLM requests."""
import os
import yaml
from typing import Dict, Any, Optional, Tuple
from pathlib import Path

from .providers.factory import create_provider, load_config

class Router:
    """Router for LLM requests."""
    
    def __init__(self):
        """Initialize the router."""
        self.config = load_config()
        self._provider_cache = {}
        
    def get_model_info(self, model_id: str) -> Tuple[str, str]:
        """Get provider and model name for a given model ID.
        
        Args:
            model_id: ID of the model (e.g., "claude-3-opus-20240229")
            
        Returns:
            Tuple of (provider_name, model_name)
            
        Raises:
            ValueError: If model is not found in config
        """
        # First try exact match
        if model_id in self.config:
            model_config = self.config[model_id]
            return model_config["provider"], model_config["model_name"]
            
        # Then try prefix match (for versioned models)
        model_prefix = "-".join(model_id.split("-")[:-1])  # Remove version suffix
        for config_id, config in self.config.items():
            if config_id.startswith(model_prefix):
                return config["provider"], config["model_name"]
                
        raise ValueError(f"Model not found: {model_id}")
        
    def get_provider(self, model_id: str):
        """Get or create provider for a given model.
        
        Args:
            model_id: ID of the model
            
        Returns:
            Provider instance
        """
        provider_name, _ = self.get_model_info(model_id)
        
        if provider_name not in self._provider_cache:
            api_key_var = f"{provider_name.upper()}_API_KEY"
            api_key = os.environ.get(api_key_var)
            if not api_key:
                raise ValueError(f"Missing API key: {api_key_var}")
                
            self._provider_cache[provider_name] = create_provider(
                provider_name=provider_name,
                api_key=api_key
            )
            
        return self._provider_cache[provider_name]
        
    async def route_request(self, request_params: Dict[str, Any]):
        """Route a request to the appropriate provider.
        
        Args:
            request_params: Request parameters
            
        Returns:
            Provider response
        """
        model_id = request_params["model"]
        provider = self.get_provider(model_id)
        
        # Get the actual model name from config
        _, model_name = self.get_model_info(model_id)
        request_params["model"] = model_name
        
        return await provider.create_chat_completion(request_params)
