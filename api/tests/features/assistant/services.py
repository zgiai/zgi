from sqlalchemy.orm import Session
from app.models import Assistant
from .schemas import AssistantCreate, AssistantUpdate


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
