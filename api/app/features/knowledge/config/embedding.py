from typing import Dict, Any
from pydantic import BaseSettings, Field

class EmbeddingSettings(BaseSettings):
    """Embedding service settings"""
    
    # Default provider
    PROVIDER: str = Field(
        default="openai",
        description="Embedding provider (openai or custom)"
    )
    
    # OpenAI settings
    OPENAI_API_KEY: str = Field(
        default="",
        description="OpenAI API key"
    )
    OPENAI_MODEL: str = Field(
        default="text-embedding-3-small",
        description="OpenAI embedding model"
    )
    
    @property
    def provider_config(self) -> Dict[str, Any]:
        """Get provider specific configuration
        
        Returns:
            Dict[str, Any]: Provider configuration
        """
        configs = {
            'openai': {
                'api_key': self.OPENAI_API_KEY,
                'model': self.OPENAI_MODEL
            }
        }
        return configs[self.PROVIDER]
    
    class Config:
        env_prefix = "EMBEDDING_"
        case_sensitive = True
