from typing import List
from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.orm import Session
from app.db.session import get_db
from app.features.providers.schemas.category import (
    CategoryCreate,
    CategoryUpdate,
    CategoryResponse,
    CategoryTree
)
from app.features.providers.service.category import CategoryService

router = APIRouter(prefix="/v1/categories", tags=["categories"])

@router.post("", response_model=CategoryResponse)
async def create_category(
    category: CategoryCreate,
    db: Session = Depends(get_db)
):
    """Create a new category"""
    return await CategoryService.create_category(db, category)

@router.get("", response_model=List[CategoryResponse])
async def list_categories(
    skip: int = Query(0, ge=0),
    limit: int = Query(100, ge=1, le=100),
    db: Session = Depends(get_db)
):
    """List all categories"""
    return await CategoryService.get_categories(db, skip, limit)

@router.get("/tree", response_model=CategoryTree)
async def get_category_tree(
    db: Session = Depends(get_db)
):
    """Get hierarchical category tree"""
    categories = await CategoryService.get_category_tree(db)
    return CategoryTree(categories=categories)

@router.get("/{category_id}", response_model=CategoryResponse)
async def get_category(
    category_id: int,
    db: Session = Depends(get_db)
):
    """Get a specific category by ID"""
    category = await CategoryService.get_category(db, category_id)
    if not category:
        raise HTTPException(status_code=404, detail="Category not found")
    return category

@router.put("/{category_id}", response_model=CategoryResponse)
async def update_category(
    category_id: int,
    category_update: CategoryUpdate,
    db: Session = Depends(get_db)
):
    """Update a category"""
    category = await CategoryService.update_category(db, category_id, category_update)
    if not category:
        raise HTTPException(status_code=404, detail="Category not found")
    return category

@router.delete("/{category_id}", response_model=bool)
async def delete_category(
    category_id: int,
    db: Session = Depends(get_db)
):
    """Soft delete a category"""
    success = await CategoryService.delete_category(db, category_id)
    if not success:
        raise HTTPException(status_code=404, detail="Category not found")
    return True

@router.post("/{category_id}/models/{model_id}")
async def add_model_to_category(
    category_id: int,
    model_id: int,
    db: Session = Depends(get_db)
):
    """Add a model to a category"""
    result = await CategoryService.add_model_to_category(db, model_id, category_id)
    if not result:
        raise HTTPException(status_code=400, detail="Failed to add model to category")
    return {"status": "success"}

@router.delete("/{category_id}/models/{model_id}")
async def remove_model_from_category(
    category_id: int,
    model_id: int,
    db: Session = Depends(get_db)
):
    """Remove a model from a category"""
    success = await CategoryService.remove_model_from_category(db, model_id, category_id)
    if not success:
        raise HTTPException(status_code=404, detail="Model-category association not found")
    return {"status": "success"}
