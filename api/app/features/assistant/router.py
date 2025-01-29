from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy.ext.asyncio import AsyncSession
from typing import List, Optional
from app.core.base import resp_200
from app.core.database import get_db
from app.core.deps import get_current_user
from . import services, schemas, model

router = APIRouter(tags=["Assistant"])


def get_assistant_service(db: AsyncSession = Depends(get_db)) -> services.AssistantService:
    return services.AssistantService(db)


@router.post("/assistants/", summary="Create assistant")
async def create_assistant(
    assistant_in: schemas.AssistantCreate,
    service: services.AssistantService = Depends(get_assistant_service),
    current_user=Depends(get_current_user)
):
    assistant = await service.create_assistant(assistant_in, current_user.id) 
    return resp_200(assistant)


@router.get("/assistants/", summary="List assistants")
async def list_assistants(
    page_num: int = Query(1, alias="page_num", description="Page number"),
    page_size: int = Query(10, alias="page_size", description="Page size"),
    name: Optional[str] = Query(None, alias="name", description="Search by assistant name"),
    service: services.AssistantService = Depends(get_assistant_service),
    current_user=Depends(get_current_user)
):
    assistants = await service.list_assistants(current_user.id, page_num, page_size, name)
    assistants_data = [assistant.dict() for assistant in assistants]
    return resp_200(assistants_data)


@router.get("/assistants/{assistant_id}", summary="Get assistant")
async def read_assistant(
    assistant_id: str,
    service: services.AssistantService = Depends(get_assistant_service),
    current_user=Depends(get_current_user)
):
    assistant = await service.get_assistant(assistant_id, current_user.id)
    if not assistant:
        raise HTTPException(status_code=404, detail="Assistant not found")
    return resp_200(assistant)


@router.put("/assistants/{assistant_id}", summary="Update assistant")
async def update_assistant(
    assistant_id: str,
    assistant_in: schemas.AssistantUpdate,
    service: services.AssistantService = Depends(get_assistant_service),
    current_user=Depends(get_current_user)
):
    assistant = await service.update_assistant(assistant_id, assistant_in, current_user.id)
    return resp_200(assistant)


@router.delete("/assistants/{assistant_id}", summary="Delete assistant")
async def delete_assistant(
    assistant_id: str,
    service: services.AssistantService = Depends(get_assistant_service),
    current_user=Depends(get_current_user)
):
    assistant = await service.delete_assistant(assistant_id, current_user.id)
    return resp_200()