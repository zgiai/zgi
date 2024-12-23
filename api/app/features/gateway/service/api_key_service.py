"""Service for managing API key mappings"""
from typing import Dict, Optional
import os
import logging
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

class APIKeyService:
    """Service for managing API key mappings"""
    
    @staticmethod
    def get_provider_key(provider: str) -> Optional[str]:
        """Get provider API key from environment variables.
        
        Args:
            provider: Provider name (e.g., 'openai', 'anthropic', 'deepseek')
            
        Returns:
            Provider API key if found, None otherwise
        """
        env_key = f"{provider.upper()}_API_KEY"
        api_key = os.getenv(env_key)
        
        if not api_key:
            raise ValueError(f"No API key found for provider {provider}. Please set {env_key} in your .env file")
            
        return api_key
    
    @staticmethod
    def create_mapping(
        api_key: str,
        provider_keys: Dict[str, str]
    ) -> None:
        """Create a new API key mapping by setting environment variables.
        
        Args:
            api_key: User's API key
            provider_keys: Dictionary mapping provider names to their API keys
        """
        for provider, key in provider_keys.items():
            env_key = f"{provider.upper()}_API_KEY"
            os.environ[env_key] = key
    
    @staticmethod
    def update_mapping(
        provider_keys: Dict[str, str]
    ) -> None:
        """Update an existing API key mapping by updating environment variables.
        
        Args:
            provider_keys: Dictionary mapping provider names to their API keys
        """
        for provider, key in provider_keys.items():
            env_key = f"{provider.upper()}_API_KEY"
            os.environ[env_key] = key
