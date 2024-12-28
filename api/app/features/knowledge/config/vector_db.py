from typing import Dict, Any
from pydantic import BaseSettings, Field

class VectorDBSettings(BaseSettings):
    """Vector database settings"""
    
    # Default provider
    PROVIDER: str = Field(
        default="pinecone",
        description="Vector database provider (pinecone or weaviate)"
    )
    
    # Pinecone settings
    PINECONE_API_KEY: str = Field(
        default="",
        description="Pinecone API key"
    )
    PINECONE_ENVIRONMENT: str = Field(
        default="",
        description="Pinecone environment"
    )
    
    # Weaviate settings
    WEAVIATE_URL: str = Field(
        default="",
        description="Weaviate server URL"
    )
    WEAVIATE_API_KEY: str = Field(
        default="",
        description="Weaviate API key"
    )
    
    @property
    def provider_config(self) -> Dict[str, Any]:
        """Get provider specific configuration
        
        Returns:
            Dict[str, Any]: Provider configuration
        """
        configs = {
            'pinecone': {
                'api_key': self.PINECONE_API_KEY,
                'environment': self.PINECONE_ENVIRONMENT
            },
            'weaviate': {
                'url': self.WEAVIATE_URL,
                'api_key': self.WEAVIATE_API_KEY
            }
        }
        return configs[self.PROVIDER]
    
    class Config:
        env_prefix = "VECTOR_DB_"
        case_sensitive = True
