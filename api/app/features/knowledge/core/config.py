from typing import Dict, Any, Optional
from pydantic_settings import BaseSettings
from functools import lru_cache

class VectorDBSettings(BaseSettings):
    """Vector database settings"""
    PROVIDER: str = "mock"  # mock for testing, change to real provider in production
    API_KEY: Optional[str] = None
    ENVIRONMENT: Optional[str] = None
    INDEX_NAME: Optional[str] = None
    URL: Optional[str] = None
    
    @property
    def provider_config(self) -> Dict[str, Any]:
        """Get provider specific configuration"""
        return {
            "api_key": self.API_KEY,
            "environment": self.ENVIRONMENT,
            "index_name": self.INDEX_NAME,
            "url": self.URL
        }

    class Config:
        env_prefix = 'VECTOR_DB_SETTINGS_'

class EmbeddingSettings(BaseSettings):
    """Embedding service settings"""
    PROVIDER: str = "mock"  # mock for testing, change to real provider in production
    API_KEY: Optional[str] = None
    MODEL: str = "text-embedding-3-small"
    BATCH_SIZE: int = 100
    MAX_RETRIES: int = 3
    TIMEOUT: int = 30
    API_BASE: str = "http://localhost:11434"
    
    @property
    def provider_config(self) -> Dict[str, Any]:
        """Get provider specific configuration"""
        return {
            "api_key": self.API_KEY,
            "model": self.MODEL,
            "batch_size": self.BATCH_SIZE,
            "max_retries": self.MAX_RETRIES,
            "timeout": self.TIMEOUT,
            "api_base": self.API_BASE
        }

    class Config:
        env_prefix = 'EMBEDDING_SETTINGS_'

class DocumentSettings(BaseSettings):
    """Document processing settings"""
    UPLOAD_DIR: str = "/tmp/knowledge/uploads"
    MAX_FILE_SIZE: int = 10 * 1024 * 1024  # 10MB
    SUPPORTED_TYPES: list = ["pdf", "txt", "doc", "docx"]
    CHUNK_SIZE: int = 1000
    CHUNK_OVERLAP: int = 200
    MAX_CHUNKS_PER_DOC: int = 1000

    class Config:
        env_prefix = 'DOCUMENT_SETTINGS_'
    
class KnowledgeBaseSettings(BaseSettings):
    """Knowledge base settings"""
    DEFAULT_EMBEDDING_MODEL: str = "text-embedding-3-small"
    DEFAULT_EMBEDDING_DIMENSION: int = 1536
    MAX_DOCUMENTS: int = 1000
    MAX_TOTAL_TOKENS: int = 1000000
    ENABLE_CACHE: bool = True
    CACHE_TTL: int = 3600

    class Config:
        env_prefix = 'KNOWLEDGE_BASE_SETTINGS_'

@lru_cache()
def get_vector_db_settings() -> VectorDBSettings:
    """Get vector database settings singleton"""
    return VectorDBSettings()

@lru_cache()
def get_embedding_settings() -> EmbeddingSettings:
    """Get embedding settings singleton"""
    return EmbeddingSettings()

@lru_cache()
def get_document_settings() -> DocumentSettings:
    """Get document settings singleton"""
    return DocumentSettings()

@lru_cache()
def get_knowledge_base_settings() -> KnowledgeBaseSettings:
    """Get knowledge base settings singleton"""
    return KnowledgeBaseSettings()
