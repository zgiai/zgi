import pytest
from sqlalchemy import select
from app.features.providers.models.provider import ModelProvider
from app.features.providers.models.model import ProviderModel
from app.features.providers.models.category import ModelCategory

pytestmark = pytest.mark.asyncio

async def test_provider_model_relationship(db, test_provider, test_model):
    # Test provider-to-model relationship
    result = await db.execute(
        select(ModelProvider).filter_by(id=test_provider.id)
    )
    provider = result.scalar_one()
    assert len(provider.models) >= 1
    assert any(m.id == test_model.id for m in provider.models)
    
    # Test model-to-provider relationship
    result = await db.execute(
        select(ProviderModel).filter_by(id=test_model.id)
    )
    model = result.scalar_one()
    assert model.provider_id == test_provider.id
    assert model.provider.id == test_provider.id

async def test_model_category_relationship(db, test_model, test_category):
    # Test model-to-category relationship
    result = await db.execute(
        select(ProviderModel).filter_by(id=test_model.id)
    )
    model = result.scalar_one()
    assert len(model.categories) >= 1
    assert any(c.id == test_category.id for c in model.categories)
    
    # Test category-to-model relationship
    result = await db.execute(
        select(ModelCategory).filter_by(id=test_category.id)
    )
    category = result.scalar_one()
    assert len(category.models) >= 1
    assert any(m.id == test_model.id for m in category.models)

async def test_cascade_delete_provider(db, test_provider, test_model):
    # Test that deleting a provider soft-deletes its models
    await db.execute(
        select(ModelProvider).filter_by(id=test_provider.id)
    )
    provider = await db.get(ModelProvider, test_provider.id)
    provider.deleted_at = datetime.utcnow()
    await db.commit()
    
    # Verify model is still accessible but marked as deleted
    result = await db.execute(
        select(ProviderModel).filter_by(id=test_model.id)
    )
    model = result.scalar_one()
    assert model.deleted_at is not None

async def test_model_category_many_to_many(db, test_model, test_category):
    # Create additional categories
    new_category = ModelCategory(
        name="Another Category",
        description="Another category description"
    )
    db.add(new_category)
    await db.commit()
    
    # Add model to multiple categories
    test_model.categories.append(new_category)
    await db.commit()
    
    # Verify model belongs to multiple categories
    result = await db.execute(
        select(ProviderModel).filter_by(id=test_model.id)
    )
    model = result.scalar_one()
    assert len(model.categories) == 2
    category_ids = {c.id for c in model.categories}
    assert test_category.id in category_ids
    assert new_category.id in category_ids

async def test_category_model_many_to_many(db, test_model, test_category):
    # Create additional model
    new_model = ProviderModel(
        provider_id=test_model.provider_id,
        model_name="another-model",
        model_version="1.0",
        description="Another test model",
        type="LLM"
    )
    db.add(new_model)
    await db.commit()
    
    # Add category to multiple models
    new_model.categories.append(test_category)
    await db.commit()
    
    # Verify category has multiple models
    result = await db.execute(
        select(ModelCategory).filter_by(id=test_category.id)
    )
    category = result.scalar_one()
    assert len(category.models) == 2
    model_ids = {m.id for m in category.models}
    assert test_model.id in model_ids
    assert new_model.id in model_ids
