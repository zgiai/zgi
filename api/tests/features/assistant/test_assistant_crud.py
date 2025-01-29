from sqlalchemy.orm import Session
from app.models import Assistant
from app.schemas import AssistantCreate, AssistantUpdate
from app.crud import assistant as crud_assistant


def test_create_assistant(db: Session):
    assistant_in = AssistantCreate(
        id="test_id",
        name="Test Assistant",
        logo="test_logo.png",
        desc="Test description",
        system_prompt="Test system prompt",
        prompt="Test prompt",
        guide_word="Test guide word",
        guide_question={},
        model_name="Test model",
        temperature=0.5,
        max_token=100,
        status=1,
        user_id=1,
        is_delete=0
    )
    assistant = crud_assistant.create(db=db, obj_in=assistant_in)
    assert assistant.id == assistant_in.id


def test_get_assistant(db: Session):
    assistant = crud_assistant.get(db=db, id="test_id")
    assert assistant


def test_update_assistant(db: Session):
    assistant_in = AssistantUpdate(
        name="Updated Assistant",
        logo="updated_logo.png"
    )
    assistant = crud_assistant.update(db=db, db_obj=assistant, obj_in=assistant_in)
    assert assistant.name == assistant_in.name


def test_delete_assistant(db: Session):
    assistant = crud_assistant.remove(db=db, id="test_id")
    assert assistant is None
