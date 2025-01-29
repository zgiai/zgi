"""Service for managing API key mappings"""
import datetime
from typing import Dict, Optional, List
import os
import logging
from dotenv import load_dotenv
from fastapi import HTTPException
from sqlalchemy.orm import Session

from app.features import User
from app.features.gateway.models.api_key import LLMModel, LLMProviderName, LLMProvider, LLMConfig
from app.features.gateway.schemas.api_key import LLMModelCreate, LLMModelUpdate, LLMProviderCreate, LLMProviderUpdate, \
    LLMConfigCreate, LLMConfigUpdate

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
        lm = db.query(LLMModel).filter(LLMModel.provider_id == llm_model.provider_id,
                                       LLMModel.name == llm_model.name).first()
        if lm:
            raise ValueError(f"LLMModel with name {llm_model.name} in LLMProvider {provider.name} already exists")
        if llm_model.model_type not in LLMModelType.get_values():
            raise ValueError(f"LLMModel config model_type must be one of the following: {LLMModelType.get_values()}")
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
        if llm_model_update.model_type not in LLMModelType.get_values():
            raise ValueError(f"LLMModel config model_type must be one of the following: {LLMModelType.get_values()}")
        db_llm_model = db.query(LLMModel).filter(LLMModel.id == llm_model_id).first()
        if db_llm_model is None:
            raise HTTPException(status_code=404, detail="LLMModel not found")
        if llm_model_update.name:
            if llm_model_update.name == db_llm_model.name:
                raise ValueError(f"Cannot update LLMModel name. New name cannot be the same as the existing name.")
            exist_llm_model = db.query(LLMModel).filter(LLMModel.provider_id == db_llm_model.provider_id,
                                       LLMModel.name == llm_model_update.name).first()
            if exist_llm_model:
                raise ValueError(f"LLMModel with name {llm_model_update.name} already exists")
        update_data = llm_model_update.model_dump(exclude_unset=True)
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
    def get_llm_models(db: Session, provider_id: Optional[int], page_size: int, page_num: int) -> List[LLMModel]:
        """Get a list of LLMModels with pagination."""
        query = db.query(LLMModel)
        if provider_id is not None:
            query = query.filter(LLMModel.provider_id == provider_id)
        offset = (page_num - 1) * page_size
        return query.offset(offset).limit(page_size).all()

    @staticmethod
    def count_llm_models(db: Session, provider_id: Optional[int]) -> int:
        """Count the total number of LLMModels."""
        query = db.query(LLMModel)
        if provider_id is not None:
            query = query.filter(LLMModel.provider_id == provider_id)
        return query.count()

    @staticmethod
    def get_llm_models_by_type(db: Session, model_type: str) -> List[LLMModel]:
        """Get LLM models by type and include provider information."""
        if model_type not in LLMModelType.get_values():
            raise ValueError(f"Invalid model_type: {model_type}. Must be one of {LLMModelType.get_values()}")
        query = (
            db.query(LLMModel)
            .join(LLMProvider, LLMModel.provider_id == LLMProvider.id)
            .filter(LLMModel.model_type == model_type,
                    LLMModel.status == 1)
        )
        llm_models = query.all()
        # fake data, todo replace with real data
        # if model_type == "embedding":
        #     datetime_now = datetime.datetime.utcnow()
        #     fake_data = [
        #         {
        #             "id": 1,
        #             "name": "embedding_model_1",
        #             "model_type": "embedding",
        #             "model_name": "text-embedding-3-large",
        #             "provider_id": 1,
        #             "status": 1,
        #             "user_id": 1,
        #             "create_time": datetime_now,
        #             "update_time": datetime_now
        #         },
        #         {
        #             "id": 2,
        #             "name": "embedding_model_2",
        #             "model_type": "embedding",
        #             "model_name": "text-embedding-3-small",
        #             "provider_id": 1,
        #             "status": 1,
        #             "user_id": 1,
        #             "create_time": datetime_now,
        #             "update_time": datetime_now
        #         },
        #         {
        #             "id": 3,
        #             "name": "embedding_model_3",
        #             "model_type": "embedding",
        #             "model_name": "text-embedding-ada-002",
        #             "provider_id": 1,
        #             "status": 1,
        #             "user_id": 1,
        #             "create_time": datetime_now,
        #             "update_time": datetime_now
        #         }
        #     ]
        #     llm_models = [LLMModel(**item) for item in fake_data]
        return llm_models


    @staticmethod
    def create_llm_config(db: Session, llm_config: LLMConfigCreate, current_user: User) -> LLMConfig:
        """Create a new LLMConfig."""
        # Check if the LLMModel exists
        model = db.query(LLMModel).filter(LLMModel.id == llm_config.llm_model_id).first()
        if not model:
            raise HTTPException(status_code=404, detail="LLMModel not found")
        config = db.query(LLMConfig).filter(LLMConfig.config_key == llm_config.config_key).first()
        if config:
            raise ValueError(f"LLMConfig with config_key {llm_config.config_key} already exists")
        db_llm_config = LLMConfig(**llm_config.model_dump())
        db_llm_config.user_id = current_user.id
        db.add(db_llm_config)
        db.commit()
        db.refresh(db_llm_config)
        return db_llm_config

    @staticmethod
    def get_llm_config(db: Session, llm_config_id: int) -> Optional[LLMConfig]:
        """Get an LLMConfig by ID."""
        return db.query(LLMConfig).filter(LLMConfig.id == llm_config_id).first()

    @staticmethod
    def update_llm_config(db: Session, llm_config_id: int, llm_config_update: LLMConfigUpdate) -> Optional[LLMConfig]:
        """Update an LLMConfig by ID."""
        db_llm_config = db.query(LLMConfig).filter(LLMConfig.id == llm_config_id).first()
        if db_llm_config is None:
            return None
        update_data = llm_config_update.model_dump(exclude_unset=True)
        for key, value in update_data.items():
            setattr(db_llm_config, key, value)
        db.add(db_llm_config)
        db.commit()
        db.refresh(db_llm_config)
        return db_llm_config

    @staticmethod
    def delete_llm_config(db: Session, llm_config_id: int) -> Optional[LLMConfig]:
        """Delete an LLMConfig by ID."""
        db_llm_config = db.query(LLMConfig).filter(LLMConfig.id == llm_config_id).first()
        if db_llm_config is None:
            return None
        db.delete(db_llm_config)
        db.commit()
        return db_llm_config

    @staticmethod
    def get_llm_configs(db: Session, page_size: int, page_num: int) -> List[LLMConfig]:
        """Get a list of LLMConfigs with pagination."""
        offset = (page_num - 1) * page_size
        return db.query(LLMConfig).offset(offset).limit(page_size).all()

    @staticmethod
    def count_llm_configs(db: Session) -> int:
        """Count the total number of LLMConfigs."""
        return db.query(LLMConfig).count()
