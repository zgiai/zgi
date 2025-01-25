"""API key management router"""
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from sqlalchemy.future import select
from typing import Dict, Optional

from app.core.base import resp_200, UnifiedResponseModel
from app.core.database import get_db, get_sync_db
from app.core.auth import get_api_key, get_current_user
from ..schemas.api_key import LLMModelResponse, LLMModelUpdate, LLMModelCreate, LLMModelListResponse, \
    LLMProviderResponse, LLMProviderUpdate, LLMProviderListResponse, LLMProviderCreate, LLMConfigResponse, \
    LLMConfigCreate, LLMConfigListResponse, LLMConfigUpdate
from ..service.api_key_service import APIKeyService
from ..models.api_key import APIKeyMapping
from pydantic import BaseModel

from ... import User

router = APIRouter()


class APIKeyMappingCreate(BaseModel):
    """API key mapping creation schema"""
    provider_keys: Dict[str, str]


class APIKeyMappingResponse(BaseModel):
    """API key mapping response schema"""
    api_key: str
    provider_keys: Dict[str, str]


@router.post("/api-keys", response_model=UnifiedResponseModel[APIKeyMappingResponse])
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
        return resp_200(APIKeyMappingResponse(
            api_key=result.api_key,
            provider_keys=result.provider_keys
        ))
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


@router.post("/llm-providers", response_model=UnifiedResponseModel[LLMProviderResponse])
async def create_llm_provider(
        llm_provider: LLMProviderCreate,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Create a new LLMProvider."""
    try:
        result = APIKeyService.create_llm_provider(db=db, llm_provider=llm_provider, current_user=current_user)
        return resp_200(LLMProviderResponse.model_validate(result))
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.get("/llm-providers", response_model=UnifiedResponseModel[LLMProviderListResponse])
async def get_llm_providers(
        page_size: Optional[int] = 10,
        page_num: Optional[int] = 1,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Get a list of LLMProviders with pagination."""
    try:
        total = APIKeyService.count_llm_providers(db=db)
        llm_providers = APIKeyService.get_llm_providers(db=db, page_size=page_size, page_num=page_num)
        return resp_200(LLMProviderListResponse(
            total=total,
            page_size=page_size,
            page_num=page_num,
            data=[LLMProviderResponse.model_validate(llm_provider) for llm_provider in llm_providers]
        ))
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.get("/llm-providers/{llm_provider_id}", response_model=UnifiedResponseModel[LLMProviderResponse])
async def get_llm_provider(
        llm_provider_id: int,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Get an LLMProvider by ID."""
    try:
        result = APIKeyService.get_llm_provider(db=db, llm_provider_id=llm_provider_id)
        if not result:
            raise HTTPException(status_code=404, detail="LLMProvider not found")
        return resp_200(LLMProviderResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.put("/llm-providers/{llm_provider_id}", response_model=UnifiedResponseModel[LLMProviderResponse])
async def update_llm_provider(
        llm_provider_id: int,
        llm_provider_update: LLMProviderUpdate,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Update an LLMProvider by ID."""
    try:
        result = APIKeyService.update_llm_provider(
            db=db,
            llm_provider_id=llm_provider_id,
            llm_provider_update=llm_provider_update
        )
        if not result:
            raise HTTPException(status_code=404, detail="LLMProvider not found")
        return resp_200(LLMProviderResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.delete("/llm-providers/{llm_provider_id}", response_model=UnifiedResponseModel[LLMProviderResponse])
async def delete_llm_provider(
        llm_provider_id: int,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Delete an LLMProvider by ID."""
    try:
        result = APIKeyService.delete_llm_provider(db=db, llm_provider_id=llm_provider_id)
        if not result:
            raise HTTPException(status_code=404, detail="LLMProvider not found")
        return resp_200(LLMProviderResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/llm-models", response_model=UnifiedResponseModel[LLMModelResponse])
async def create_llm_model(
        llm_model: LLMModelCreate,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Create a new LLMModel."""
    try:
        result = APIKeyService.create_llm_model(db=db, llm_model=llm_model, current_user=current_user)
        return resp_200(LLMModelResponse.model_validate(result))
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.get("/llm-models", response_model=UnifiedResponseModel[LLMModelListResponse])
async def get_llm_models(
        provider_id: Optional[int] = None,
        page_size: Optional[int] = 10,
        page_num: Optional[int] = 1,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Get a list of LLMModels with pagination."""
    try:
        total = APIKeyService.count_llm_models(db=db, provider_id=provider_id)
        llm_models = APIKeyService.get_llm_models(
            db=db, provider_id=provider_id, page_size=page_size, page_num=page_num)
        return resp_200(LLMModelListResponse(
            total=total,
            page_size=page_size,
            page_num=page_num,
            data=[LLMModelResponse.model_validate(llm_model) for llm_model in llm_models]
        ))
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.get("/llm-models/{llm_model_id}", response_model=UnifiedResponseModel[LLMModelResponse])
async def get_llm_model(
        llm_model_id: int,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Get an LLMModel by ID."""
    try:
        result = APIKeyService.get_llm_model(db=db, llm_model_id=llm_model_id)
        if not result:
            raise HTTPException(status_code=404, detail="LLMModel not found")
        return resp_200(LLMModelResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))

@router.get("/llm-models-by-type/{model_type}", response_model=UnifiedResponseModel[LLMModelListResponse])
async def get_llm_models_by_type(
        model_type: str,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Get a list of LLMModels by type with pagination and provider information."""
    try:
        llm_models = APIKeyService.get_llm_models_by_type(db=db, model_type=model_type)
        models_result = []
        for model in llm_models:
            models_result.append(LLMModelResponse.model_validate(model))
        return resp_200(models_result)
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))

@router.put("/llm-models/{llm_model_id}", response_model=UnifiedResponseModel[LLMModelResponse])
async def update_llm_model(
        llm_model_id: int,
        llm_model_update: LLMModelUpdate,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Update an LLMModel by ID."""
    try:
        result = APIKeyService.update_llm_model(db=db, llm_model_id=llm_model_id, llm_model_update=llm_model_update)
        if not result:
            raise HTTPException(status_code=404, detail="LLMModel not found")
        return resp_200(LLMModelResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.delete("/llm-models/{llm_model_id}", response_model=UnifiedResponseModel[LLMModelResponse])
async def delete_llm_model(
        llm_model_id: int,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Delete an LLMModel by ID."""
    try:
        result = APIKeyService.delete_llm_model(db=db, llm_model_id=llm_model_id)
        if not result:
            raise HTTPException(status_code=404, detail="LLMModel not found")
        return resp_200(LLMModelResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/llm-configs")
async def create_llm_config(
        llm_config: LLMConfigCreate,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Create a new LLMConfig."""
    try:
        result = APIKeyService.create_llm_config(db=db, llm_config=llm_config, current_user=current_user)
        return resp_200(LLMConfigResponse.model_validate(result))
    except HTTPException as e:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.get("/llm-configs")
async def get_llm_configs(
        page_size: Optional[int] = 10,
        page_num: Optional[int] = 1,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Get a list of LLMConfigs with pagination."""
    try:
        total = APIKeyService.count_llm_configs(db=db)
        llm_configs = APIKeyService.get_llm_configs(db=db, page_size=page_size, page_num=page_num)
        return resp_200(LLMConfigListResponse(
            total=total,
            page_size=page_size,
            page_num=page_num,
            data=[LLMConfigResponse.model_validate(llm_config).model_dump() for llm_config in llm_configs]
        ))
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.get("/llm-configs/{llm_config_id}")
async def get_llm_config(
        llm_config_id: int,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Get an LLMConfig by ID."""
    try:
        result = APIKeyService.get_llm_config(db=db, llm_config_id=llm_config_id)
        if not result:
            raise HTTPException(status_code=404, detail="LLMConfig not found")
        return resp_200(LLMConfigResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.put("/llm-configs/{llm_config_id}")
async def update_llm_config(
        llm_config_id: int,
        llm_config_update: LLMConfigUpdate,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Update an LLMConfig by ID."""
    try:
        result = APIKeyService.update_llm_config(
            db=db,
            llm_config_id=llm_config_id,
            llm_config_update=llm_config_update
        )
        if not result:
            raise HTTPException(status_code=404, detail="LLMConfig not found")
        return resp_200(LLMConfigResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.delete("/llm-configs/{llm_config_id}")
async def delete_llm_config(
        llm_config_id: int,
        db: Session = Depends(get_sync_db),
        current_user: User = Depends(get_current_user)
):
    """Delete an LLMConfig by ID."""
    try:
        result = APIKeyService.delete_llm_config(db=db, llm_config_id=llm_config_id)
        if not result:
            raise HTTPException(status_code=404, detail="LLMConfig not found")
        return resp_200(LLMConfigResponse.model_validate(result))
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))
