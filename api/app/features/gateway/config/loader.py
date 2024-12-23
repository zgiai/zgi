"""Configuration loader for provider system."""
import os
from pathlib import Path
import yaml
import logging
from typing import Dict, Any, Optional, Tuple

logger = logging.getLogger(__name__)

class ConfigLoader:
    """Load and manage provider configurations."""
    
    def __init__(self, config_path: Optional[str] = None):
        """Initialize config loader.
        
        Args:
            config_path: Path to config file, defaults to providers.yaml in same directory
        """
        if config_path is None:
            config_path = Path(__file__).parent / "providers.yaml"
            
        self.config_path = Path(config_path)
        self._config_cache = None
        
    def _load_config(self) -> Dict[str, Any]:
        """Load configuration from file."""
        if self._config_cache is None:
            try:
                with open(self.config_path, "r") as f:
                    self._config_cache = yaml.safe_load(f)
            except Exception as e:
                logger.error(f"Error loading config from {self.config_path}: {str(e)}")
                self._config_cache = {}
                
        return self._config_cache
        
    def get_provider_config(self, provider_name: str) -> Optional[Dict[str, Any]]:
        """Get configuration for a specific provider.
        
        Args:
            provider_name: Name of the provider
            
        Returns:
            Provider configuration if found, None otherwise
        """
        config = self._load_config()
        provider_config = config.get("providers", {}).get(provider_name)
        
        if not provider_config:
            logger.warning(f"No configuration found for provider: {provider_name}")
            return None
            
        # Inject environment variables for API keys
        api_key_var = f"{provider_name.upper()}_API_KEY"
        api_key = os.environ.get(api_key_var)
        
        if not api_key:
            logger.error(f"Missing API key environment variable: {api_key_var}")
            return None
            
        # Create auth config
        provider_config["auth"] = {
            "type": provider_config.get("auth_type", "bearer"),
            "api_key": api_key
        }
        
        return provider_config

    def get_matching_provider(self, model_name: str) -> Optional[Tuple[str, Dict[str, Any]]]:
        """Find matching provider for a model name.
        
        Args:
            model_name: Name of the model
            
        Returns:
            Tuple of (provider_name, provider_config) if found, None otherwise
            
        Raises:
            ValueError: If no matching provider found
        """
        import re
        config = self._load_config()
        
        for provider_name, provider in config.get("providers", {}).items():
            patterns = provider.get("patterns", [])
            for pattern in patterns:
                if re.match(pattern, model_name):
                    return provider_name, self.get_provider_config(provider_name)
                    
        logger.error(f"No matching provider found for model: {model_name}")
        return None

# Singleton instance
_config_loader = None

def get_config_loader(config_path: Optional[str] = None) -> ConfigLoader:
    """Get singleton instance of config loader."""
    global _config_loader
    if _config_loader is None:
        _config_loader = ConfigLoader(config_path)
    return _config_loader
