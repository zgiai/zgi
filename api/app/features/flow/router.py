from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession
from app.core.database import get_db
from app.core.deps import get_current_user
from . import schemas, model
from .services import FlowService
from app.core.base import resp_200

router = APIRouter()

@router.post("/flows/", summary="Create a new flow")
async def create_flow(
    flow_in: schemas.FlowCreate,
    db: AsyncSession = Depends(get_db),
    current_user=Depends(get_current_user)
):
    service = FlowService(db)
    flow = await service.create_flow(flow_in, current_user.id)
    return resp_200(flow)

@router.get("/flows/", summary="List all flows")
async def list_flows(
    db: AsyncSession = Depends(get_db),
    current_user=Depends(get_current_user)
):
    service = FlowService(db)
    flows = await service.list_flows(current_user.id)
    return resp_200(flows)

@router.get("/flows/{flow_id}", summary="Get a flow by ID")
async def get_flow(
    flow_id: str,
    db: AsyncSession = Depends(get_db),
    current_user=Depends(get_current_user)
):
    service = FlowService(db)
    flow = await service.get_flow(flow_id, current_user.id)
    if not flow:
        raise HTTPException(status_code=404, detail="Flow not found")
    return resp_200(flow)

@router.put("/flows/{flow_id}", summary="Update a flow")
async def update_flow(
    flow_id: str,
    flow_in: schemas.FlowUpdate,
    db: AsyncSession = Depends(get_db),
    current_user=Depends(get_current_user)
):
    service = FlowService(db)
    flow = await service.update_flow(flow_id, flow_in, current_user.id)
    return resp_200(flow)

@router.delete("/flows/{flow_id}", summary="Delete a flow")
async def delete_flow(
    flow_id: str,
    db: AsyncSession = Depends(get_db),
    current_user=Depends(get_current_user)
):
    service = FlowService(db)
    await service.delete_flow(flow_id, current_user.id)
    return resp_200({"message": "Flow deleted"})
