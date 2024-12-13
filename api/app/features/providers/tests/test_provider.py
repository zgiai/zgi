import pytest
import asyncio
from datetime import datetime
from sqlalchemy.orm import Session
from sqlalchemy import delete
from app.features.providers.models.provider import ModelProvider
from app.features.providers.models.model import ProviderModel
from app.features.providers.schemas.provider import ProviderCreate, ProviderUpdate
from app.features.providers.service.provider import ProviderService

@pytest.fixture
async def clean_db(db: Session):
    """Clean up database before each test"""
    await db.execute(delete(ProviderModel))
    await db.execute(delete(ModelProvider))
    await db.commit()

@pytest.fixture
def sample_provider():
    return {
        "provider_name": "Test Provider",
        "enabled": True,
        "api_key": "test_api_key",
        "org_id": "test_org",
        "base_url": "https://api.test.com"
    }

@pytest.mark.asyncio
async def test_create_provider(db: Session, clean_db, sample_provider):
    provider_create = ProviderCreate(**sample_provider)
    provider = await ProviderService.create_provider(db, provider_create)
    
    # Verify the provider was created
    assert provider.id is not None
    assert provider.provider_name == sample_provider["provider_name"]
    assert provider.enabled == sample_provider["enabled"]
    assert provider.api_key == sample_provider["api_key"]
    assert provider.org_id == sample_provider["org_id"]
    assert provider.base_url == sample_provider["base_url"]
    assert provider.deleted_at is None

@pytest.mark.asyncio
async def test_get_provider(db: Session, clean_db, sample_provider):
    # Create a provider first
    provider_create = ProviderCreate(**sample_provider)
    created_provider = await ProviderService.create_provider(db, provider_create)
    
    # Get the provider
    provider = await ProviderService.get_provider(db, created_provider.id)
    assert provider is not None
    assert provider.provider_name == sample_provider["provider_name"]

@pytest.mark.asyncio
async def test_get_nonexistent_provider(db: Session, clean_db):
    provider = await ProviderService.get_provider(db, 999)
    assert provider is None

@pytest.mark.asyncio
async def test_update_provider(db: Session, clean_db):
    """Test update provider"""
    # Create a test provider
    now = datetime.utcnow()
    provider = ModelProvider(
        provider_name="Test Provider",
        enabled=True,
        created_at=now,
        updated_at=now,
    )
    db.add(provider)
    await db.commit()
    await db.refresh(provider)

    original_updated_at = provider.updated_at

    # Wait a short time to ensure timestamps are different
    await asyncio.sleep(0.1)

    # Update the provider
    provider_update = ProviderUpdate(
        provider_name="Updated Provider",
        enabled=False
    )
    
    updated_provider = await ProviderService.update_provider(
        db, provider.id, provider_update
    )
    
    # Get the updated provider from database
    db_provider = await ProviderService.get_provider(db, provider.id)
    
    # Verify the update
    assert db_provider is not None
    assert db_provider.provider_name == "Updated Provider"
    assert db_provider.enabled is False
    assert db_provider.updated_at > original_updated_at

@pytest.mark.asyncio
async def test_delete_provider(db: Session, clean_db, sample_provider):
    # Create a provider first
    provider_create = ProviderCreate(**sample_provider)
    provider = await ProviderService.create_provider(db, provider_create)
    
    # Delete the provider
    success = await ProviderService.delete_provider(db, provider.id)
    assert success is True
    
    # Try to get the deleted provider
    deleted_provider = await ProviderService.get_provider(db, provider.id)
    assert deleted_provider is None

@pytest.mark.asyncio
async def test_search_providers(db: Session, clean_db):
    """Test search providers"""
    # Create test providers with different cases
    providers = [
        ModelProvider(
            provider_name="Test Provider One",
            enabled=True,
            created_at=datetime.utcnow(),
            updated_at=datetime.utcnow(),
        ),
        ModelProvider(
            provider_name="TEST PROVIDER TWO",
            enabled=True,
            created_at=datetime.utcnow(),
            updated_at=datetime.utcnow(),
        ),
        ModelProvider(
            provider_name="test provider three",
            enabled=False,
            created_at=datetime.utcnow(),
            updated_at=datetime.utcnow(),
        ),
    ]
    
    for provider in providers:
        db.add(provider)
    await db.commit()

    # Test case-insensitive search
    results = await ProviderService.search_providers(db, "TEST PROVIDER")
    assert len(results) == 3

    results = await ProviderService.search_providers(db, "test provider")
    assert len(results) == 3

    # Test with enabled_only flag
    results = await ProviderService.search_providers(db, "test provider", enabled_only=True)
    assert len(results) == 2

    # Test with empty query
    results = await ProviderService.search_providers(db, "")
    assert len(results) == 3

    # Test pagination
    results = await ProviderService.search_providers(db, "", limit=2)
    assert len(results) == 2
