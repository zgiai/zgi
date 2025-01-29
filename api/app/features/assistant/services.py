from sqlalchemy.orm import Session
from sqlalchemy.ext.asyncio import AsyncSession
from .model import Assistant
from .schemas import AssistantCreate, AssistantUpdate, Assistant as AssistantSchema
from typing import List, Optional
from sqlalchemy import select
import uuid


def create_assistant(db: Session, assistant_in: AssistantCreate) -> Assistant:
    db_assistant = Assistant(**assistant_in.dict())
    db.add(db_assistant)
    db.commit()
    db.refresh(db_assistant)
    return db_assistant


def get_assistant(db: Session, assistant_id: str) -> Assistant:
    return db.query(Assistant).filter(Assistant.id == assistant_id).first()


def update_assistant(db: Session, db_assistant: Assistant, assistant_in: AssistantUpdate) -> Assistant:
    for key, value in assistant_in.dict(exclude_unset=True).items():
        setattr(db_assistant, key, value)
    db.commit()
    db.refresh(db_assistant)
    return db_assistant


def delete_assistant(db: Session, assistant_id: str) -> Assistant:
    db_assistant = db.query(Assistant).filter(Assistant.id == assistant_id).first()
    db.delete(db_assistant)
    db.commit()
    return db_assistant


class AssistantService:
    def __init__(self, db: AsyncSession):
        self.db = db

    async def create_assistant(self, assistant_in: AssistantCreate, user_id: int) -> AssistantSchema:
        # Check for duplicate assistant name
        existing_assistant = await self.db.execute(
            select(Assistant).where(Assistant.name == assistant_in.name, Assistant.user_id == user_id)
        )
        if existing_assistant.scalars().first():
            raise ValueError("Assistant name already exists for this user.")

        # Convert lists to comma-separated strings
        knowledges = ','.join(assistant_in.knowledge_list) if assistant_in.knowledge_list else ''
        tools = ','.join(assistant_in.tool_list) if assistant_in.tool_list else ''

        # Generate a unique ID for the assistant
        assistant_id = str(uuid.uuid4())
        db_assistant = Assistant(id=assistant_id, knowledges=knowledges, tools=tools, **assistant_in.dict(exclude={'user_id', 'knowledge_list', 'tool_list'}), user_id=user_id)
        self.db.add(db_assistant)
        await self.db.commit()
        await self.db.refresh(db_assistant)
        # Convert to Pydantic schema
        return AssistantSchema.from_orm(db_assistant) 

    async def get_assistant(self, assistant_id: str, user_id: int) -> Assistant:
        result = await self.db.execute(
            select(Assistant).where(Assistant.id == assistant_id, Assistant.user_id == user_id)
        )
        return result.scalars().first()

    async def list_assistants(self, user_id: int, page_num: int, page_size: int, name: Optional[str]) -> List[AssistantSchema]:
        query = select(Assistant).where(Assistant.user_id == user_id, Assistant.is_delete == 0)
        if name:
            query = query.where(Assistant.name.ilike(f"%{name}%"))
        query = query.offset((page_num - 1) * page_size).limit(page_size)
        result = await self.db.execute(query)
        db_assistants = result.scalars().all()
        return [AssistantSchema.from_orm(db_assistant) for db_assistant in db_assistants]

    async def update_assistant(self, assistant_id: str, assistant_in: AssistantUpdate, user_id: int) -> AssistantSchema:
        db_assistant = await self.get_assistant(assistant_id, user_id)
        if db_assistant:
            # Convert lists to comma-separated strings
            if assistant_in.knowledge_list is not None:
                db_assistant.knowledges = ','.join(assistant_in.knowledge_list)
            if assistant_in.tool_list is not None:
                db_assistant.tools = ','.join(assistant_in.tool_list)
            for key, value in assistant_in.dict(exclude_unset=True, exclude={'knowledge_list', 'tool_list'}).items():
                setattr(db_assistant, key, value)
            await self.db.commit()
            await self.db.refresh(db_assistant)
            return AssistantSchema.from_orm(db_assistant)
        return None

    async def delete_assistant(self, assistant_id: str, user_id: int) -> Optional[AssistantSchema]:
        db_assistant = await self.get_assistant(assistant_id, user_id)
        if db_assistant:
            db_assistant.is_delete = 1  # Mark as deleted
            await self.db.commit()
            await self.db.refresh(db_assistant)
            return AssistantSchema.from_orm(db_assistant)
        return None
