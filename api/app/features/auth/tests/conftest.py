import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session

from app.main import app
from app.core.database import Base, engine, get_db
from app.features.users.models import User


@pytest.fixture(scope="session")
def db():
    Base.metadata.create_all(bind=engine)
    try:
        db = next(get_db())
        yield db
    finally:
        Base.metadata.drop_all(bind=engine)


@pytest.fixture(scope="module")
def client():
    with TestClient(app) as c:
        yield c


@pytest.fixture(scope="function")
def db_session():
    """Returns an sqlalchemy session, and after the test tears down everything properly."""
    session = next(get_db())
    # Clear all tables before each test
    for table in reversed(Base.metadata.sorted_tables):
        session.execute(table.delete())
    session.commit()
    yield session
    session.close()
