from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy.orm import Session
from app import crud, models, schemas
from app.api import deps
from typing import List

router = APIRouter()


@router.post("/assistants/", response_model=schemas.Assistant)
def create_assistant(
    *,
    db: Session = Depends(deps.get_db),
    assistant_in: schemas.AssistantCreate
) -> models.Assistant:
    assistant = crud.assistant.create(db=db, obj_in=assistant_in)
    return assistant


@router.get("/assistants/{assistant_id}", response_model=schemas.Assistant)
def read_assistant(
    *,
    db: Session = Depends(deps.get_db),
    assistant_id: str
) -> models.Assistant:
    assistant = crud.assistant.get(db=db, id=assistant_id)
    if not assistant:
        raise HTTPException(status_code=404, detail="Assistant not found")
    return assistant


@router.get("/assistants/", response_model=List[schemas.Assistant])
def list_assistants(
    *,
    db: Session = Depends(deps.get_db)
) -> List[models.Assistant]:
    assistants = crud.assistant.get_multi(db=db)
    return assistants


@router.put("/assistants/{assistant_id}", response_model=schemas.Assistant)
def update_assistant(
    *,
    db: Session = Depends(deps.get_db),
    assistant_id: str,
    assistant_in: schemas.AssistantUpdate
) -> models.Assistant:
    assistant = crud.assistant.get(db=db, id=assistant_id)
    if not assistant:
        raise HTTPException(status_code=404, detail="Assistant not found")
    assistant = crud.assistant.update(db=db, db_obj=assistant, obj_in=assistant_in)
    return assistant


@router.delete("/assistants/{assistant_id}", response_model=schemas.Assistant)
def delete_assistant(
    *,
    db: Session = Depends(deps.get_db),
    assistant_id: str
) -> models.Assistant:
    assistant = crud.assistant.remove(db=db, id=assistant_id)
    return assistant
