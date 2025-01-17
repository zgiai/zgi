"""Service for managing API key mappings"""
from typing import Dict, Optional, List
import os
import logging
from dotenv import load_dotenv
from fastapi import HTTPException
from sqlalchemy.orm import Session

from app.features import User
from app.features.gateway.models.api_key import LLMModel, LLMProviderName, LLMProvider
from app.features.gateway.schemas.api_key import LLMModelCreate, LLMModelUpdate, LLMProviderCreate, LLMProviderUpdate

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

    @staticmethod
    def create_llm_provider(db: Session, llm_provider: LLMProviderCreate, current_user: User) -> LLMProvider:
        """Create a new LLMProvider."""
        lp = db.query(LLMProvider).filter(LLMProvider.name == llm_provider.name).first()
        if lp:
            raise ValueError(f"LLMProvider with name {llm_provider.name} already exists")
        if llm_provider.provider not in LLMProviderName.get_values():
            raise ValueError(
                f"LLMProvider provider must be one of the following: {LLMProviderName.get_values()}")
        db_llm_provider = LLMProvider(**llm_provider.model_dump())
        db_llm_provider.user_id = current_user.id
        db.add(db_llm_provider)
        db.commit()
        db.refresh(db_llm_provider)
        return db_llm_provider

    @staticmethod
    def get_llm_provider(db: Session, llm_provider_id: int) -> Optional[LLMProvider]:
        """Get an LLMProvider by ID."""
        return db.query(LLMProvider).filter(LLMProvider.id == llm_provider_id).first()

    @staticmethod
    def update_llm_provider(
            db: Session, llm_provider_id: int,
            llm_provider_update: LLMProviderUpdate
    ) -> Optional[LLMProvider]:
        """Update an LLMProvider by ID."""
        db_llm_provider = db.query(LLMProvider).filter(LLMProvider.id == llm_provider_id).first()
        if db_llm_provider is None:
            return None
        if llm_provider_update.provider:
            if llm_provider_update.provider not in LLMProviderName.get_values():
                raise ValueError(
                    f"Cannot update LLMProvider provider,"
                    f"must be one of the following:{LLMProviderName.get_values()}")
        if llm_provider_update.name:
            if llm_provider_update.name == db_llm_provider.name:
                raise ValueError(f"Cannot update LLMProvider name. New name cannot be the same as the existing name.")
            exist_llm_provider = db.query(LLMProvider).filter(LLMProvider.name == llm_provider_update.name).first()
            if exist_llm_provider:
                raise ValueError(f"LLMProvider with name {llm_provider_update.name} already exists")
        update_data = llm_provider_update.model_dump(exclude_unset=True)
        for key, value in update_data.items():
            setattr(db_llm_provider, key, value)
        db.add(db_llm_provider)
        db.commit()
        db.refresh(db_llm_provider)
        return db_llm_provider

    @staticmethod
    def delete_llm_provider(db: Session, llm_provider_id: int) -> Optional[LLMProvider]:
        """Delete an LLMProvider by ID."""
        db_llm_provider = db.query(LLMProvider).filter(LLMProvider.id == llm_provider_id).first()
        if db_llm_provider is None:
            return None
        db.delete(db_llm_provider)
        db.commit()
        return db_llm_provider

    @staticmethod
    def get_llm_providers(db: Session, page_size: int, page_num: int) -> List[LLMProvider]:
        """Get a list of LLMProviders with pagination."""
        offset = (page_num - 1) * page_size
        return db.query(LLMProvider).offset(offset).limit(page_size).all()

    @staticmethod
    def count_llm_providers(db: Session) -> int:
        """Count the total number of LLMProviders."""
        return db.query(LLMProvider).count()

    @staticmethod
    def create_llm_model(db: Session, llm_model: LLMModelCreate, current_user: User) -> LLMModel:
        """Create a new LLMModel."""
        # Check if the LLMProvider exists
        provider = db.query(LLMProvider).filter(LLMProvider.id == llm_model.provider_id).first()
        if not provider:
            raise HTTPException(status_code=404, detail="LLMProvider not found")
        lm = db.query(LLMModel).filter(LLMModel.name == llm_model.name).first()
        if lm:
            raise ValueError(f"LLMModel with name {llm_model.name} already exists")
        config = llm_model.config
        if config is None:
            raise ValueError("LLMModel config cannot be None")
        if llm_model.model_type not in LLMProviderName.get_values():
            raise ValueError(f"LLMModel config model_type must be one of the following: {LLMProviderName.get_values()}")
        db_llm_model = LLMModel(**llm_model.model_dump())
        db_llm_model.user_id = current_user.id
        db.add(db_llm_model)
        db.commit()
        db.refresh(db_llm_model)
        return db_llm_model

    @staticmethod
    def get_llm_model(db: Session, llm_model_id: int) -> Optional[LLMModel]:
        """Get an LLMModel by ID."""
        return db.query(LLMModel).filter(LLMModel.id == llm_model_id).first()

    @staticmethod
    def update_llm_model(db: Session, llm_model_id: int, llm_model_update: LLMModelUpdate) -> Optional[LLMModel]:
        """Update an LLMModel by ID."""
        db_llm_model = db.query(LLMModel).filter(LLMModel.id == llm_model_id).first()
        if db_llm_model is None:
            return None
        update_data = llm_model_update.dict(exclude_unset=True)
        for key, value in update_data.items():
            setattr(db_llm_model, key, value)
        db.add(db_llm_model)
        db.commit()
        db.refresh(db_llm_model)
        return db_llm_model

    @staticmethod
    def delete_llm_model(db: Session, llm_model_id: int) -> Optional[LLMModel]:
        """Delete an LLMModel by ID."""
        db_llm_model = db.query(LLMModel).filter(LLMModel.id == llm_model_id).first()
        if db_llm_model is None:
            return None
        db.delete(db_llm_model)
        db.commit()
        return db_llm_model

    @staticmethod
    def get_llm_models(db: Session, page_size: int, page_num: int) -> List[LLMModel]:
        """Get a list of LLMModels with pagination."""
        offset = (page_num - 1) * page_size
        return db.query(LLMModel).offset(offset).limit(page_size).all()

    @staticmethod
    def count_llm_models(db: Session) -> int:
        """Count the total number of LLMModels."""
        return db.query(LLMModel).count()
