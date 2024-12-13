from datetime import datetime
from typing import Optional, List
from pydantic import BaseModel, Field

class CategoryBase(BaseModel):
    """Base schema for category data"""
    category_name: str = Field(..., max_length=100, description="Category name")
    parent_id: Optional[int] = Field(None, description="Parent category ID or NULL")
    description: Optional[str] = Field(None, max_length=255, description="Optional category description")

class CategoryCreate(CategoryBase):
    """Schema for creating a new category"""
    pass

class CategoryUpdate(BaseModel):
    """Schema for updating an existing category"""
    category_name: Optional[str] = Field(None, max_length=100)
    parent_id: Optional[int] = None
    description: Optional[str] = Field(None, max_length=255)

class CategoryInDB(CategoryBase):
    """Schema for category data as stored in database"""
    id: int
    created_at: datetime
    updated_at: datetime
    deleted_at: Optional[datetime] = None

    class Config:
        from_attributes = True

class CategoryResponse(CategoryInDB):
    """Schema for category response data with children"""
    children: Optional[List['CategoryResponse']] = None

CategoryResponse.model_rebuild()  # Required for self-referencing models

class CategoryTree(BaseModel):
    """Schema for hierarchical category tree response"""
    categories: List[CategoryResponse]
