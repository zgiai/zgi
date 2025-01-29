from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.future import select
from sqlalchemy.orm import joinedload
from .model import Flow, FlowVersion
from .schemas import FlowCreate, FlowUpdate, FlowVersionCreate, FlowVersionUpdate, Flow as FlowSchema
import uuid

class FlowService:
    def __init__(self, db: AsyncSession):
        self.db = db

    async def create_flow(self, flow_in: FlowCreate, user_id: int) -> FlowSchema:
        # Check for duplicate flow name
        existing_flow = await self.db.execute(
            select(Flow).where(Flow.name == flow_in.name, Flow.user_id == user_id, Flow.is_delete == 0)
        )
        if existing_flow.scalars().first():
            raise ValueError("Flow name already exists for this user.")

        flow_id = str(uuid.uuid4())
        db_flow = Flow(id=flow_id, user_id=user_id, **flow_in.dict())
        self.db.add(db_flow)
        self.db.commit()
        self.db.refresh(db_flow)
        return FlowSchema.from_orm(db_flow)

    async def list_flows(self, user_id: int, is_admin: bool):
        query = select(Flow).where(Flow.is_delete == 0)
        if not is_admin:
            query = query.where(Flow.user_id == user_id)
        result = await self.db.execute(query)
        return result.scalars().all()

    async def get_flow(self, flow_id: str, user_id: int) -> FlowSchema:
        result = await self.db.execute(
            select(Flow).options(joinedload(Flow.versions)).where(Flow.id == flow_id, Flow.user_id == user_id)
        )
        db_flow = result.scalars().first()
        if db_flow:
            return FlowSchema.from_orm(db_flow)
        return None

    async def update_flow(self, flow_id: str, flow_in: FlowUpdate, user_id: int, is_admin: bool) -> FlowSchema:
        db_flow = await self.get_flow(flow_id, user_id)
        if db_flow:
            if not is_admin and db_flow.user_id != user_id:
                raise ValueError("Permission denied.")
            # Check for duplicate flow name, excluding the current flow
            existing_flow = await self.db.execute(
                select(Flow).where(Flow.name == flow_in.name, Flow.user_id == user_id, Flow.is_delete == 0, Flow.id != flow_id)
            )
            if existing_flow.scalars().first():
                raise ValueError("Flow name already exists for this user.")
            for key, value in flow_in.dict(exclude_unset=True).items():
                setattr(db_flow, key, value)
            await self.db.commit()
            await self.db.refresh(db_flow)
        return FlowSchema.from_orm(db_flow) if db_flow else None

    async def delete_flow(self, flow_id: str, user_id: int, is_admin: bool):
        db_flow = await self.get_flow(flow_id, user_id)
        if db_flow:
            if not is_admin and db_flow.user_id != user_id:
                raise ValueError("Permission denied.")
            db_flow.is_delete = 1  # Mark as deleted
            await self.db.commit()
