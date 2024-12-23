"""API key management for LLM providers."""
import os
from typing import Optional, Dict, Any
import yaml
from pathlib import Path
from functools import lru_cache
from dotenv import load_dotenv

# Load environment variables from .env file
env_path = Path(__file__).parents[5] / ".env"  # Go up to api directory
load_dotenv(env_path)

class KeyValidationError(Exception):
    """Raised when API key validation fails."""
    pass

class KeyManager:
    """Manager for LLM provider API keys."""
    
    def __init__(self, environment: str = "default"):
        """
        Initialize key manager.
        
        Args:
            environment: Environment to use (default, production, staging)
        """
        self.environment = environment
        self._config = self._load_config()
        
    def _load_config(self) -> Dict[str, Any]:
        """Load API key configuration."""
        config_path = Path(__file__).parent / "config" / "api_keys.yaml"
        if not config_path.exists():
            raise FileNotFoundError(f"API keys config not found at {config_path}")
            
        with open(config_path) as f:
            config = yaml.safe_load(f)
            
        # Get environment specific config or default
        if self.environment == "default":
            return config["default"]
        return config["environments"].get(self.environment, config["default"])
    
    def validate_key(self, provider_id: str, api_key: str) -> bool:
        """
        Validate an API key for a provider.
        
        Args:
            provider_id: Provider ID
            api_key: API key to validate
            
        Returns:
            True if valid, False otherwise
        """
        provider_config = self._config.get(provider_id)
        if not provider_config:
            return False
            
        # Check basic validation rules
        rules = provider_config.get("validation_rules", [])
        for rule in rules:
            rule_type, value = rule.split(":")
            
            if rule_type == "starts_with":
                if not api_key.startswith(value):
                    return False
            elif rule_type == "length":
                if len(api_key) != int(value):
                    return False
                    
        return True
    
    def get_provider_key(self, provider_id: str) -> Optional[str]:
        """
        Get API key for a provider from environment variables.
        
        Args:
            provider_id: Provider ID
            
        Returns:
            API key if found, None otherwise
        """
        provider_config = self._config.get(provider_id)
        if not provider_config:
            return None
            
        # Try to get key from environment variable
        key_env = provider_config.get("key_env")
        if key_env:
            return os.getenv(key_env)
            
        # Could add support for other storage methods here
        # (e.g., HashiCorp Vault, AWS Secrets Manager)
        return None
    
    def get_key_info(self, provider_id: str) -> Optional[Dict[str, Any]]:
        """
        Get API key configuration for a provider.
        
        Args:
            provider_id: Provider ID
            
        Returns:
            Key configuration if found, None otherwise
        """
        return self._config.get(provider_id)

@lru_cache()
def get_key_manager(environment: str = "default") -> KeyManager:
    """
    Get singleton instance of key manager.
    
    Args:
        environment: Environment to use
        
    Returns:
        KeyManager instance
    """
    return KeyManager(environment)
