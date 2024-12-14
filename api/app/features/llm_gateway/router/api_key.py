"""API key management router"""
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from sqlalchemy.future import select
from typing import Dict
from app.core.database import get_db
from app.core.auth import get_api_key
from ..service.api_key_service import APIKeyService
from ..models.api_key import APIKeyMapping
from pydantic import BaseModel

router = APIRouter()

class APIKeyMappingCreate(BaseModel):
    """API key mapping creation schema"""
    provider_keys: Dict[str, str]

class APIKeyMappingResponse(BaseModel):
    """API key mapping response schema"""
    api_key: str
    provider_keys: Dict[str, str]

@router.post("/api-keys", response_model=APIKeyMappingResponse)
async def create_api_key_mapping(
    mapping: APIKeyMappingCreate,
    api_key: str = Depends(get_api_key),
    db: Session = Depends(get_db)
):
    """Create a new API key mapping"""
    try:
        result = await APIKeyService.create_mapping(
            db=db,
            api_key=api_key,
            provider_keys=mapping.provider_keys
        )
        return APIKeyMappingResponse(
            api_key=result.api_key,
            provider_keys=result.provider_keys
        )
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))

@router.get("/api-keys/current", response_model=APIKeyMappingResponse)
async def get_current_api_key_mapping(
    api_key: str = Depends(get_api_key),
    db: Session = Depends(get_db)
):
    """Get current API key mapping"""
    try:
        stmt = select(APIKeyMapping).where(APIKeyMapping.api_key == api_key)
        result = await db.execute(stmt)
        mapping = result.scalar_one_or_none()
        
        if not mapping:
            raise HTTPException(status_code=404, detail="API key mapping not found")
            
        return APIKeyMappingResponse(
            api_key=mapping.api_key,
            provider_keys=mapping.provider_keys
        )
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))

@router.put("/api-keys", response_model=APIKeyMappingResponse)
async def update_api_key_mapping(
    mapping: APIKeyMappingCreate,
    api_key: str = Depends(get_api_key),
    db: Session = Depends(get_db)
):
    """Update API key mapping"""
    try:
        result = await APIKeyService.update_mapping(
            db=db,
            api_key=api_key,
            provider_keys=mapping.provider_keys
        )
        
        if not result:
            raise HTTPException(status_code=404, detail="API key mapping not found")
            
        return APIKeyMappingResponse(
            api_key=result.api_key,
            provider_keys=result.provider_keys
        )
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))
