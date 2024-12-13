from datetime import datetime
from typing import List, Optional
from sqlalchemy import select, or_
from sqlalchemy.ext.asyncio import AsyncSession
from app.features.providers.models.model import ProviderModel
from app.features.providers.schemas.model import ModelCreate, ModelUpdate, ModelFilter

class ModelService:
    @staticmethod
    async def create_model(db: AsyncSession, model: ModelCreate) -> ProviderModel:
        """Create a new model"""
        now = datetime.utcnow()
        db_model = ProviderModel(
            **model.model_dump(),
            created_at=now,
            updated_at=now
        )
        db.add(db_model)
        await db.commit()
        await db.refresh(db_model)
        return db_model

    @staticmethod
    async def get_model(db: AsyncSession, model_id: int) -> Optional[ProviderModel]:
        """Get a model by ID"""
        query = select(ProviderModel).where(
            ProviderModel.id == model_id,
            ProviderModel.deleted_at.is_(None)
        )
        result = await db.execute(query)
        return result.scalar_one_or_none()

    @staticmethod
    async def get_models(
        db: AsyncSession,
        filter_params: ModelFilter,
        skip: int = 0,
        limit: int = 100
    ) -> List[ProviderModel]:
        """Get list of models with filtering"""
        query = select(ProviderModel).where(ProviderModel.deleted_at.is_(None))

        # Apply filters
        if filter_params.provider_id is not None:
            query = query.where(ProviderModel.provider_id == filter_params.provider_id)
        if filter_params.type:
            query = query.where(ProviderModel.type == filter_params.type)
        if filter_params.supports_streaming is not None:
            query = query.where(ProviderModel.supports_streaming == filter_params.supports_streaming)
        if filter_params.supports_function_calling is not None:
            query = query.where(ProviderModel.supports_function_calling == filter_params.supports_function_calling)
        if filter_params.supports_roles is not None:
            query = query.where(ProviderModel.supports_roles == filter_params.supports_roles)
        if filter_params.fine_tuning_available is not None:
            query = query.where(ProviderModel.fine_tuning_available == filter_params.fine_tuning_available)
        if filter_params.multi_lang_support is not None:
            query = query.where(ProviderModel.multi_lang_support == filter_params.multi_lang_support)
        if filter_params.max_price_per_1k_tokens is not None:
            query = query.where(ProviderModel.price_per_1k_tokens <= filter_params.max_price_per_1k_tokens)
        if filter_params.min_context_length is not None:
            query = query.where(ProviderModel.max_context_length >= filter_params.min_context_length)
        if filter_params.tags:
            # Note: This assumes tags is stored as a JSON array in the database
            for tag in filter_params.tags:
                query = query.where(ProviderModel.tags.contains([tag]))
        if filter_params.search:
            search_pattern = f"%{filter_params.search}%"
            query = query.where(
                or_(
                    ProviderModel.model_name.ilike(search_pattern),
                    ProviderModel.description.ilike(search_pattern)
                )
            )

        query = query.offset(skip).limit(limit)
        result = await db.execute(query)
        return list(result.scalars().all())

    @staticmethod
    async def update_model(
        db: AsyncSession,
        model_id: int,
        model_update: ModelUpdate
    ) -> Optional[ProviderModel]:
        """Update a model"""
        db_model = await ModelService.get_model(db, model_id)
        if not db_model:
            return None

        update_data = model_update.model_dump(exclude_unset=True)
        for field, value in update_data.items():
            setattr(db_model, field, value)

        db_model.updated_at = datetime.utcnow()
        await db.commit()
        await db.refresh(db_model)
        return db_model

    @staticmethod
    async def delete_model(db: AsyncSession, model_id: int) -> bool:
        """Soft delete a model"""
        db_model = await ModelService.get_model(db, model_id)
        if not db_model:
            return False

        db_model.deleted_at = datetime.utcnow()
        await db.commit()
        return True
