import pytest
from datetime import datetime
from app.features.providers.service.provider import ProviderService
from app.features.providers.schemas.provider import ProviderCreate, ProviderUpdate
from app.features.providers.models.provider import ModelProvider

pytestmark = pytest.mark.asyncio

async def test_create_provider(db):
    provider_data = ProviderCreate(
        name="New Provider",
        description="New provider description",
        base_url="https://api.new-provider.com",
        api_version="v1",
        enabled=True
    )
    
    provider = await ProviderService.create_provider(db, provider_data)
    assert provider.name == provider_data.name
    assert provider.description == provider_data.description
    assert provider.base_url == provider_data.base_url
    assert provider.api_version == provider_data.api_version
    assert provider.enabled == provider_data.enabled
    assert provider.deleted_at is None

async def test_get_provider(db, test_provider):
    provider = await ProviderService.get_provider(db, test_provider.id)
    assert provider is not None
    assert provider.id == test_provider.id
    assert provider.name == test_provider.name

async def test_get_nonexistent_provider(db):
    provider = await ProviderService.get_provider(db, 999)
    assert provider is None

async def test_get_providers(db, test_provider):
    providers = await ProviderService.get_providers(db)
    assert len(providers) >= 1
    assert any(p.id == test_provider.id for p in providers)

async def test_get_providers_enabled_only(db, test_provider):
    # Create a disabled provider
    disabled_provider = ModelProvider(
        name="Disabled Provider",
        description="Disabled provider description",
        base_url="https://api.disabled-provider.com",
        api_version="v1",
        enabled=False
    )
    db.add(disabled_provider)
    await db.commit()

    providers = await ProviderService.get_providers(db, enabled_only=True)
    assert all(p.enabled for p in providers)
    assert any(p.id == test_provider.id for p in providers)
    assert not any(p.id == disabled_provider.id for p in providers)

async def test_update_provider(db, test_provider):
    update_data = ProviderUpdate(
        name="Updated Provider",
        description="Updated description"
    )
    
    updated_provider = await ProviderService.update_provider(db, test_provider.id, update_data)
    assert updated_provider is not None
    assert updated_provider.name == update_data.name
    assert updated_provider.description == update_data.description
    assert updated_provider.base_url == test_provider.base_url  # Unchanged field

async def test_update_nonexistent_provider(db):
    update_data = ProviderUpdate(name="Updated Provider")
    updated_provider = await ProviderService.update_provider(db, 999, update_data)
    assert updated_provider is None

async def test_delete_provider(db, test_provider):
    success = await ProviderService.delete_provider(db, test_provider.id)
    assert success is True
    
    # Verify provider is soft deleted
    deleted_provider = await db.get(ModelProvider, test_provider.id)
    assert deleted_provider.deleted_at is not None
    
    # Verify provider doesn't appear in get_providers results
    providers = await ProviderService.get_providers(db)
    assert not any(p.id == test_provider.id for p in providers)

async def test_delete_nonexistent_provider(db):
    success = await ProviderService.delete_provider(db, 999)
    assert success is False

async def test_search_providers(db, test_provider):
    # Test search by name
    providers = await ProviderService.search_providers(db, test_provider.name)
    assert len(providers) >= 1
    assert any(p.id == test_provider.id for p in providers)
    
    # Test search by description
    providers = await ProviderService.search_providers(db, test_provider.description)
    assert len(providers) >= 1
    assert any(p.id == test_provider.id for p in providers)
    
    # Test search with no results
    providers = await ProviderService.search_providers(db, "nonexistent")
    assert len(providers) == 0

async def test_search_providers_pagination(db):
    # Create multiple providers
    for i in range(5):
        provider = ModelProvider(
            name=f"Test Provider {i}",
            description=f"Description {i}",
            base_url=f"https://api.test-{i}.com",
            api_version="v1",
            enabled=True
        )
        db.add(provider)
    await db.commit()
    
    # Test pagination
    providers = await ProviderService.search_providers(db, "Test Provider", skip=0, limit=2)
    assert len(providers) == 2
    
    providers = await ProviderService.search_providers(db, "Test Provider", skip=2, limit=2)
    assert len(providers) == 2
    
    providers = await ProviderService.search_providers(db, "Test Provider", skip=4, limit=2)
    assert len(providers) == 1
