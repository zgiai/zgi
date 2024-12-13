from typing import List, Optional
from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession
from app.db.session import get_db
from app.features.providers.schemas.provider import (
    ProviderCreate,
    ProviderUpdate,
    ProviderResponse
)
from app.features.providers.service.provider import ProviderService

router = APIRouter(prefix="/v1/providers", tags=["providers"])

@router.post("", response_model=ProviderResponse)
async def create_provider(
    provider: ProviderCreate,
    db: AsyncSession = Depends(get_db)
):
    """Create a new model provider"""
    return await ProviderService.create_provider(db, provider)

@router.get("", response_model=List[ProviderResponse])
async def list_providers(
    skip: int = Query(0, ge=0),
    limit: int = Query(100, ge=1, le=100),
    enabled_only: bool = Query(False),
    search: Optional[str] = Query(None),
    db: AsyncSession = Depends(get_db)
):
    """List all providers with optional filtering"""
    if search:
        providers = await ProviderService.search_providers(
            db, search, skip, limit, enabled_only
        )
    else:
        providers = await ProviderService.get_providers(
            db, skip, limit, enabled_only
        )
    return providers

@router.get("/{provider_id}", response_model=ProviderResponse)
async def get_provider(
    provider_id: int,
    db: AsyncSession = Depends(get_db)
):
    """Get a specific provider by ID"""
    provider = await ProviderService.get_provider(db, provider_id)
    if not provider:
        raise HTTPException(status_code=404, detail="Provider not found")
    return provider

@router.put("/{provider_id}", response_model=ProviderResponse)
async def update_provider(
    provider_id: int,
    provider_update: ProviderUpdate,
    db: AsyncSession = Depends(get_db)
):
    """Update a provider"""
    provider = await ProviderService.update_provider(db, provider_id, provider_update)
    if not provider:
        raise HTTPException(status_code=404, detail="Provider not found")
    return provider

@router.delete("/{provider_id}", response_model=bool)
async def delete_provider(
    provider_id: int,
    db: AsyncSession = Depends(get_db)
):
    """Soft delete a provider"""
    success = await ProviderService.delete_provider(db, provider_id)
    if not success:
        raise HTTPException(status_code=404, detail="Provider not found")
    return True
