from sqlalchemy.orm import Session
from fastapi.testclient import TestClient
from app.features.users.models import User

def create_test_user(db: Session, email: str = "test@example.com") -> User:
    """Create a test user in the database"""
    user = User(
        email=email,
        hashed_password="$2b$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewKyNhvQNqdWfh4u",  # test123
        is_active=True,
        is_superuser=False
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return user

def get_test_user_token(client: TestClient, user: User) -> str:
    """Get authentication token for test user"""
    response = client.post(
        "/v1/auth/login",
        data={
            "username": user.email,
            "password": "test123"
        }
    )
    return response.json()["access_token"]
