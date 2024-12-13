from typing import List
from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession
from app.db.session import get_db
from app.features.providers.schemas.model import (
    ModelCreate,
    ModelUpdate,
    ModelResponse,
    ModelFilter
)
from app.features.providers.service.model import ModelService

router = APIRouter(prefix="/v1/models", tags=["models"])

@router.post("", response_model=ModelResponse)
async def create_model(
    model: ModelCreate,
    db: AsyncSession = Depends(get_db)
):
    """Create a new model"""
    return await ModelService.create_model(db, model)

@router.get("", response_model=List[ModelResponse])
async def list_models(
    skip: int = Query(0, ge=0),
    limit: int = Query(100, ge=1, le=100),
    filter_params: ModelFilter = Depends(),
    db: AsyncSession = Depends(get_db)
):
    """List all models with filtering"""
    return await ModelService.get_models(db, filter_params, skip, limit)

@router.get("/{model_id}", response_model=ModelResponse)
async def get_model(
    model_id: int,
    db: AsyncSession = Depends(get_db)
):
    """Get a specific model by ID"""
    model = await ModelService.get_model(db, model_id)
    if not model:
        raise HTTPException(status_code=404, detail="Model not found")
    return model

@router.put("/{model_id}", response_model=ModelResponse)
async def update_model(
    model_id: int,
    model_update: ModelUpdate,
    db: AsyncSession = Depends(get_db)
):
    """Update a model"""
    model = await ModelService.update_model(db, model_id, model_update)
    if not model:
        raise HTTPException(status_code=404, detail="Model not found")
    return model

@router.delete("/{model_id}", response_model=bool)
async def delete_model(
    model_id: int,
    db: AsyncSession = Depends(get_db)
):
    """Soft delete a model"""
    success = await ModelService.delete_model(db, model_id)
    if not success:
        raise HTTPException(status_code=404, detail="Model not found")
    return success
