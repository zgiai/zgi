import pytest
import asyncio
from datetime import datetime, timedelta
from decimal import Decimal
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select
from app.features.providers.models.model import ProviderModel
from app.features.providers.schemas.model import ModelCreate, ModelUpdate, ModelFilter
from app.features.providers.service.model import ModelService

@pytest.fixture
async def sample_model_data(test_provider):
    return {
        "provider_id": test_provider.id,
        "model_name": "GPT-4",
        "model_version": "1.0",
        "description": "Advanced language model",
        "type": "LLM",
        "modalities": {"text": True},
        "max_context_length": 8192,
        "supports_streaming": True,
        "supports_function_calling": True,
        "price_per_1k_tokens": Decimal("0.03"),
        "api_call_name": "gpt-4"
    }

async def test_create_model(db: AsyncSession, sample_model_data):
    """Test model creation with proper timestamps"""
    model_create = ModelCreate(**sample_model_data)
    model = await ModelService.create_model(db, model_create)
    
    assert model.model_name == sample_model_data["model_name"]
    assert model.provider_id == sample_model_data["provider_id"]
    assert model.type == sample_model_data["type"]
    assert isinstance(model.created_at, datetime)
    assert isinstance(model.updated_at, datetime)
    assert model.deleted_at is None
    assert model.created_at == model.updated_at

async def test_get_model(db: AsyncSession, sample_model_data):
    """Test retrieving a model"""
    model_create = ModelCreate(**sample_model_data)
    created_model = await ModelService.create_model(db, model_create)
    
    model = await ModelService.get_model(db, created_model.id)
    assert model is not None
    assert model.id == created_model.id
    assert model.model_name == sample_model_data["model_name"]
    assert isinstance(model.created_at, datetime)
    assert isinstance(model.updated_at, datetime)

async def test_get_nonexistent_model(db: AsyncSession):
    """Test retrieving a non-existent model"""
    model = await ModelService.get_model(db, 999)
    assert model is None

async def test_update_model(db: AsyncSession, sample_model_data):
    """Test model update with timestamp changes"""
    model_create = ModelCreate(**sample_model_data)
    model = await ModelService.create_model(db, model_create)
    
    # Store original timestamps
    original_created_at = model.created_at
    original_updated_at = model.updated_at
    
    # Wait briefly to ensure timestamp difference
    await asyncio.sleep(0.1)
    
    # Update the model
    update_data = ModelUpdate(
        model_name="GPT-4 Turbo",
        max_context_length=128000
    )
    updated_model = await ModelService.update_model(db, model.id, update_data)
    
    assert updated_model is not None
    assert updated_model.model_name == "GPT-4 Turbo"
    assert updated_model.max_context_length == 128000
    assert updated_model.created_at == original_created_at
    assert updated_model.updated_at > original_updated_at

async def test_delete_model(db: AsyncSession, sample_model_data):
    """Test soft delete functionality"""
    model_create = ModelCreate(**sample_model_data)
    model = await ModelService.create_model(db, model_create)
    
    # Store original timestamps
    original_created_at = model.created_at
    original_updated_at = model.updated_at
    
    # Delete the model
    success = await ModelService.delete_model(db, model.id)
    assert success is True
    
    # Try to get the deleted model directly from DB to check deleted_at
    result = await db.execute(
        select(ProviderModel).where(ProviderModel.id == model.id)
    )
    deleted_model = result.scalar_one()
    assert deleted_model is not None
    assert isinstance(deleted_model.deleted_at, datetime)
    assert deleted_model.created_at == original_created_at
    assert deleted_model.updated_at == original_updated_at
    
    # Verify the model is not retrieved through normal get
    model_from_service = await ModelService.get_model(db, model.id)
    assert model_from_service is None

async def test_filter_models(db: AsyncSession, test_provider):
    """Test model filtering with various parameters"""
    # Create multiple models with different attributes
    models_data = [
        ModelCreate(
            provider_id=test_provider.id,
            model_name="GPT-4",
            type="LLM",
            supports_streaming=True,
            price_per_1k_tokens=Decimal("0.03"),
            max_context_length=8192,
            description="Advanced model"
        ),
        ModelCreate(
            provider_id=test_provider.id,
            model_name="DALL-E",
            type="Image",
            supports_streaming=False,
            price_per_1k_tokens=Decimal("0.02"),
            max_context_length=4096,
            description="Image generation model"
        ),
        ModelCreate(
            provider_id=test_provider.id,
            model_name="Whisper",
            type="Audio",
            supports_streaming=True,
            price_per_1k_tokens=Decimal("0.01"),
            max_context_length=2048,
            description="Speech recognition model"
        )
    ]
    
    created_models = []
    for model_data in models_data:
        model = await ModelService.create_model(db, model_data)
        created_models.append(model)
    
    # Test various filters
    
    # Filter by type
    filter_params = ModelFilter(type="LLM")
    filtered_models = await ModelService.get_models(db, filter_params)
    assert len(filtered_models) == 1
    assert filtered_models[0].type == "LLM"
    
    # Filter by price
    filter_params = ModelFilter(max_price_per_1k_tokens=Decimal("0.02"))
    filtered_models = await ModelService.get_models(db, filter_params)
    assert len(filtered_models) == 2
    assert all(m.price_per_1k_tokens <= Decimal("0.02") for m in filtered_models)
    
    # Filter by context length
    filter_params = ModelFilter(min_context_length=4096)
    filtered_models = await ModelService.get_models(db, filter_params)
    assert len(filtered_models) == 2
    assert all(m.max_context_length >= 4096 for m in filtered_models)
    
    # Search by name
    filter_params = ModelFilter(search="GPT")
    filtered_models = await ModelService.get_models(db, filter_params)
    assert len(filtered_models) == 1
    assert "GPT" in filtered_models[0].model_name
    
    # Search by description
    filter_params = ModelFilter(search="speech")
    filtered_models = await ModelService.get_models(db, filter_params)
    assert len(filtered_models) == 1
    assert "speech" in filtered_models[0].description.lower()

async def test_model_relationships(db: AsyncSession, sample_model_data, test_category):
    """Test model relationships with categories"""
    # Create a model
    model_create = ModelCreate(**sample_model_data)
    model = await ModelService.create_model(db, model_create)
    
    # Add model to category
    test_category.models.append(model)
    await db.commit()
    await db.refresh(model)
    
    # Verify relationship
    assert len(model.categories) == 1
    assert model.categories[0].id == test_category.id
    
    # Remove model from category
    test_category.models.remove(model)
    await db.commit()
    await db.refresh(model)
    
    # Verify relationship removed
    assert len(model.categories) == 0
