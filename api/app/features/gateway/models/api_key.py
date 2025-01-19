"""API key mapping models"""
from enum import Enum

from sqlalchemy import Column, Integer, String, DateTime, Float, JSON, Boolean, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
from app.core.database import Base


class LLMProviderName(Enum):
    OPENAI = 'openai'
    DEEPSEEK = 'deepseek'
    QWEN = 'qwen'
    ANTHROPIC = 'anthropic'
    OLLAMA = 'ollama'
    # AZURE_OPENAI = 'azure_openai'
    # XINFERENCE = 'xinference'
    # LLAMACPP = 'llamacpp'
    # VLLM = 'vllm'
    # QIAN_FAN = 'qianfan'
    # ZHIPU = 'zhipu'
    # MINIMAX = 'minimax'
    # SPARK = 'spark'

    @classmethod
    def get_values(cls) -> list[str]:
        """Get enum values"""
        return [enum.value for enum in cls]


class LLMModelType(Enum):
    LLM = 'llm'
    EMBEDDING = 'embedding'
    RERANK = 'rerank'

    @classmethod
    def get_values(cls) -> list[str]:
        """Get enum values"""
        return [enum.value for enum in cls]


class APIKeyMapping(Base):
    """API key mapping model for storing provider API keys"""
    __tablename__ = "api_key_mappings"

    id = Column(Integer, primary_key=True, index=True)
    api_key = Column(String(255), unique=True, index=True, nullable=False)
    provider_keys = Column(JSON, nullable=False)  # {"openai": "sk-xxx", "deepseek": "sk-yyy"}
    created_at = Column(DateTime, default=func.now())
    updated_at = Column(DateTime, default=func.now(), onupdate=func.now())


class LLMProvider(Base):
    """LLM provider"""
    __tablename__ = "llm_provider"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), unique=True, index=True, nullable=False)
    provider = Column(String(255), index=True, nullable=False)
    description = Column(String(1000))
    api_base = Column(String(255))
    api_key = Column(String(255))
    user_id = Column(Integer, nullable=False)
    create_time = Column(DateTime, default=func.now())
    update_time = Column(DateTime, default=func.now(), onupdate=func.now())

    # Relationships
    llm_models = relationship("LLMModel", back_populates="llm_provider")

class LLMModel(Base):
    """LLM model for storing provider"""
    __tablename__ = "llm_model"

    id = Column(Integer, primary_key=True, index=True)
    name = Column(String(255), unique=True, index=True, nullable=False)
    provider_id = Column(Integer, ForeignKey("llm_provider.id", ondelete="CASCADE"), nullable=False)
    description = Column(String(1000))
    model_name = Column(String(255), nullable=False)
    model_type = Column(String(20), nullable=False)
    status = Column(Integer, nullable=False, default=1)
    user_id = Column(Integer, nullable=False)
    create_time = Column(DateTime, default=func.now())
    update_time = Column(DateTime, default=func.now(), onupdate=func.now())

    # Relationships
    llm_configs = relationship("LLMConfig", back_populates="llm_model")
    llm_provider = relationship("LLMProvider", back_populates="llm_models")

class LLMConfig(Base):
    """API key mapping model for storing provider API keys"""
    __tablename__ = "llm_config"

    id = Column(Integer, primary_key=True, index=True)
    config_key = Column(String(255), unique=True, index=True, nullable=False)
    llm_model_id = Column(Integer, ForeignKey("llm_model.id", ondelete="CASCADE"), nullable=False)
    description = Column(String(1000))
    create_time = Column(DateTime, default=func.now())
    update_time = Column(DateTime, default=func.now(), onupdate=func.now())

    # Relationships
    llm_model = relationship("LLMModel", back_populates="llm_configs")
