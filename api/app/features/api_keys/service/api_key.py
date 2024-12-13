from typing import Optional
from sqlalchemy.orm import Session
from fastapi import HTTPException, status
import secrets
import string

from app.features.api_keys.models import APIKey
from app.features.api_keys.schemas import APIKeyCreate

def generate_api_key() -> str:
    """Generate a secure API key"""
    alphabet = string.ascii_letters + string.digits
    return ''.join(secrets.choice(alphabet) for _ in range(32))

def create_api_key(db: Session, api_key: APIKeyCreate, user_id: int) -> APIKey:
    """Create a new API key"""
    # Generate a secure API key
    key = generate_api_key()

    # Create the API key record
    db_api_key = APIKey(
        name=api_key.name,
        key=key,
        project_id=api_key.project_id,
        created_by=user_id
    )
    db.add(db_api_key)
    db.commit()
    db.refresh(db_api_key)
    return db_api_key

def get_api_key_by_key(db: Session, key: str) -> Optional[APIKey]:
    """Get API key by key"""
    return db.query(APIKey).filter(APIKey.key == key).first()

def get_api_key_by_uuid(db: Session, uuid: str) -> Optional[APIKey]:
    """Get API key by UUID"""
    return db.query(APIKey).filter(APIKey.uuid == uuid).first()

def disable_api_key(db: Session, uuid: str) -> APIKey:
    """Disable an API key"""
    api_key = get_api_key_by_uuid(db, uuid)
    if not api_key:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="API key not found"
        )
    
    api_key.is_active = False
    db.commit()
    db.refresh(api_key)
    return api_key

def delete_api_key(db: Session, uuid: str) -> None:
    """Delete an API key"""
    api_key = get_api_key_by_uuid(db, uuid)
    if not api_key:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="API key not found"
        )
    
    db.delete(api_key)
    db.commit()
