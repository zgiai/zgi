import pytest
from datetime import datetime
from sqlalchemy.orm import Session
from app.features.providers.models.category import ModelCategory
from app.features.providers.schemas.category import CategoryCreate, CategoryUpdate
from app.features.providers.service.category import CategoryService
from app.features.providers.service.model import ModelService
from app.features.providers.schemas.model import ModelCreate
from app.features.providers.service.provider import ProviderService
from app.features.providers.schemas.provider import ProviderCreate

@pytest.fixture
async def sample_provider(db: Session):
    provider = await ProviderService.create_provider(
        db,
        ProviderCreate(provider_name="Test Provider", enabled=True)
    )
    return provider

@pytest.fixture
def sample_category():
    return {
        "category_name": "Language Models",
        "description": "Natural language processing models"
    }

@pytest.fixture
async def sample_model(db: Session, sample_provider):
    model = await ModelService.create_model(
        db,
        ModelCreate(
            provider_id=sample_provider.id,
            model_name="Test Model",
            type="LLM"
        )
    )
    return model

@pytest.mark.asyncio
async def test_create_category(db: Session):
    service = CategoryService(db)
    category_data = CategoryCreate(
        category_name="Test Category",
        description="Test Description"
    )
    category = await service.create_category(category_data)
    assert category.category_name == "Test Category"
    assert category.description == "Test Description"
    assert category.created_at is not None
    assert category.updated_at is not None
    assert category.deleted_at is None

@pytest.mark.asyncio
async def test_create_nested_category(db: Session):
    service = CategoryService(db)
    parent_data = CategoryCreate(
        category_name="Parent Category",
        description="Parent Description"
    )
    parent = await service.create_category(parent_data)

    child_data = CategoryCreate(
        category_name="Child Category",
        description="Child Description"
    )
    child = await service.create_category(child_data, parent_id=parent.id)
    
    assert child.parent_id == parent.id

@pytest.mark.asyncio
async def test_get_category(db: Session, sample_category):
    service = CategoryService(db)
    category_create = CategoryCreate(**sample_category)
    created_category = await service.create_category(category_create)
    category = await service.get_category(created_category.id)
    assert category is not None
    assert category.id == created_category.id
    assert category.category_name == sample_category["category_name"]

@pytest.mark.asyncio
async def test_get_category_tree(db: Session):
    service = CategoryService(db)
    # Create parent category
    parent_data = CategoryCreate(
        category_name="Parent Category",
        description="Parent Description"
    )
    parent = await service.create_category(parent_data)

    # Create child categories
    child_data1 = CategoryCreate(
        category_name="Child Category 1",
        description="Child Description 1"
    )
    child_data2 = CategoryCreate(
        category_name="Child Category 2",
        description="Child Description 2"
    )
    await service.create_category(child_data1, parent_id=parent.id)
    await service.create_category(child_data2, parent_id=parent.id)

    # Get tree
    tree = await service.get_category_tree()
    assert len(tree) >= 1
    parent_from_tree = next(cat for cat in tree if cat.id == parent.id)
    assert len(parent_from_tree.children) == 2

@pytest.mark.asyncio
async def test_update_category(db: Session, sample_category):
    service = CategoryService(db)
    category_create = CategoryCreate(**sample_category)
    category = await service.create_category(category_create)
    
    update_data = CategoryUpdate(
        category_name="Updated Category",
        description="Updated Description"
    )
    updated = await service.update_category(category.id, update_data)
    assert updated is not None
    assert updated.category_name == "Updated Category"
    assert updated.description == "Updated Description"
    assert updated.updated_at > category.updated_at

@pytest.mark.asyncio
async def test_delete_category(db: Session, sample_category):
    service = CategoryService(db)
    category_create = CategoryCreate(**sample_category)
    category = await service.create_category(category_create)
    
    result = await service.delete_category(category.id)
    assert result is True
    
    # Verify category is soft deleted
    deleted_category = await service.get_category(category.id)
    assert deleted_category is None

@pytest.mark.asyncio
async def test_add_model_to_category(
    db: Session,
    sample_category,
    sample_model
):
    service = CategoryService(db)
    category_create = CategoryCreate(**sample_category)
    category = await service.create_category(category_create)
    
    result = await service.add_model_to_category(category.id, sample_model.id)
    assert result is True

    # Verify model is in category
    category = await service.get_category(category.id)
    assert sample_model in category.models

@pytest.mark.asyncio
async def test_remove_model_from_category(
    db: Session,
    sample_category,
    sample_model
):
    service = CategoryService(db)
    category_create = CategoryCreate(**sample_category)
    category = await service.create_category(category_create)
    
    await service.add_model_to_category(category.id, sample_model.id)
    
    # Then remove it
    result = await service.remove_model_from_category(category.id, sample_model.id)
    assert result is True

    # Verify model is removed from category
    category = await service.get_category(category.id)
    assert sample_model not in category.models
