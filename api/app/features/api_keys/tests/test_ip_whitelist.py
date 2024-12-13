import pytest
from fastapi import status
from sqlalchemy.orm import Session

from app.features.api_keys.models import APIKey, IPWhitelist
from app.features.api_keys.service.api_key import create_api_key
from app.features.api_keys.service.ip_whitelist import (
    create_ip_whitelist,
    get_ip_whitelist,
    get_ip_whitelist_by_api_key,
    update_ip_whitelist,
    delete_ip_whitelist,
    disable_ip_whitelist,
)
from app.features.api_keys.schemas import APIKeyCreate

@pytest.fixture
def api_key_data(db: Session, test_user, test_project):
    """Fixture to create an API key"""
    api_key_data = APIKeyCreate(
        name="Test API Key",
        project_id=test_project.id,
    )
    api_key = create_api_key(db=db, api_key=api_key_data, user_id=test_user.id)
    return api_key

@pytest.mark.asyncio
async def test_create_ip_whitelist(client, db: Session, test_user, test_project, api_key_data):
    """Test creating an IP whitelist entry"""
    # Create IP whitelist
    response = await client.post(
        f"/api/v1/api-keys/{api_key_data.uuid}/ip-whitelist",
        json={
            "allowed_ips": ["192.168.1.1", "10.0.0.0/24"]
        }
    )
    assert response.status_code == status.HTTP_201_CREATED
    data = response.json()
    assert data["api_key_id"] == api_key_data.id
    assert len(data["allowed_ips"]) == 2
    assert data["is_active"] is True

    # Verify in database
    ip_whitelist = db.query(IPWhitelist).filter(IPWhitelist.api_key_id == api_key_data.id).first()
    assert ip_whitelist is not None
    assert len(ip_whitelist.allowed_ips) == 2

@pytest.mark.asyncio
async def test_get_ip_whitelist(client, db: Session, test_user, test_project, api_key_data):
    """Test retrieving an IP whitelist entry"""
    # Create IP whitelist first
    ip_whitelist = IPWhitelist(
        api_key_id=api_key_data.id,
        allowed_ips=["192.168.1.1", "10.0.0.0/24"]
    )
    db.add(ip_whitelist)
    db.commit()
    db.refresh(ip_whitelist)

    # Get IP whitelist
    response = await client.get(f"/api/v1/api-keys/{api_key_data.uuid}/ip-whitelist")
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["api_key_id"] == api_key_data.id
    assert len(data["allowed_ips"]) == 2

@pytest.mark.asyncio
async def test_update_ip_whitelist(client, db: Session, test_user, test_project, api_key_data):
    """Test updating an IP whitelist entry"""
    # Create IP whitelist first
    ip_whitelist = IPWhitelist(
        api_key_id=api_key_data.id,
        allowed_ips=["192.168.1.1", "10.0.0.0/24"]
    )
    db.add(ip_whitelist)
    db.commit()
    db.refresh(ip_whitelist)

    # Update IP whitelist
    response = await client.put(
        f"/api/v1/api-keys/{api_key_data.uuid}/ip-whitelist",
        json={
            "allowed_ips": ["192.168.1.2", "10.0.1.0/24", "172.16.0.0/16"]
        }
    )
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["api_key_id"] == api_key_data.id
    assert len(data["allowed_ips"]) == 3

    # Verify in database
    updated_ip_whitelist = db.query(IPWhitelist).filter(IPWhitelist.api_key_id == api_key_data.id).first()
    assert updated_ip_whitelist is not None
    assert len(updated_ip_whitelist.allowed_ips) == 3

@pytest.mark.asyncio
async def test_delete_ip_whitelist(client, db: Session, test_user, test_project, api_key_data):
    """Test deleting an IP whitelist entry"""
    # Create IP whitelist first
    ip_whitelist = IPWhitelist(
        api_key_id=api_key_data.id,
        allowed_ips=["192.168.1.1", "10.0.0.0/24"]
    )
    db.add(ip_whitelist)
    db.commit()
    db.refresh(ip_whitelist)

    # Delete IP whitelist
    response = await client.delete(f"/api/v1/api-keys/{api_key_data.uuid}/ip-whitelist")
    assert response.status_code == status.HTTP_204_NO_CONTENT

    # Verify deletion
    deleted_ip_whitelist = db.query(IPWhitelist).filter(IPWhitelist.api_key_id == api_key_data.id).first()
    assert deleted_ip_whitelist is None

@pytest.mark.asyncio
async def test_disable_ip_whitelist(client, db: Session, test_user, test_project, api_key_data):
    """Test disabling an IP whitelist entry"""
    # Create IP whitelist first
    ip_whitelist = IPWhitelist(
        api_key_id=api_key_data.id,
        allowed_ips=["192.168.1.1", "10.0.0.0/24"]
    )
    db.add(ip_whitelist)
    db.commit()
    db.refresh(ip_whitelist)

    # Disable IP whitelist
    response = await client.post(f"/api/v1/api-keys/{api_key_data.uuid}/ip-whitelist/disable")
    assert response.status_code == status.HTTP_200_OK
    data = response.json()
    assert data["is_active"] is False

    # Verify in database
    disabled_ip_whitelist = db.query(IPWhitelist).filter(IPWhitelist.api_key_id == api_key_data.id).first()
    assert disabled_ip_whitelist is not None
    assert disabled_ip_whitelist.is_active is False

def test_create_ip_whitelist(db: Session, api_key: APIKey):
    """Test creating an IP whitelist entry"""
    allowed_ips = ["192.168.1.1", "10.0.0.1"]
    ip_whitelist = create_ip_whitelist(
        db=db,
        api_key_id=api_key.id,
        allowed_ips=allowed_ips
    )
    assert ip_whitelist is not None
    assert ip_whitelist.api_key_id == api_key.id
    assert ip_whitelist.allowed_ips == allowed_ips
    assert ip_whitelist.is_active is True

def test_get_ip_whitelist(db: Session, ip_whitelist: IPWhitelist):
    """Test getting an IP whitelist entry by ID"""
    retrieved = get_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist.id)
    assert retrieved is not None
    assert retrieved.id == ip_whitelist.id
    assert retrieved.api_key_id == ip_whitelist.api_key_id
    assert retrieved.allowed_ips == ip_whitelist.allowed_ips

def test_get_ip_whitelist_by_api_key(db: Session, ip_whitelist: IPWhitelist):
    """Test getting an IP whitelist entry by API key ID"""
    retrieved = get_ip_whitelist_by_api_key(db=db, api_key_id=ip_whitelist.api_key_id)
    assert retrieved is not None
    assert retrieved.id == ip_whitelist.id
    assert retrieved.api_key_id == ip_whitelist.api_key_id
    assert retrieved.allowed_ips == ip_whitelist.allowed_ips

def test_update_ip_whitelist(db: Session, ip_whitelist: IPWhitelist):
    """Test updating an IP whitelist entry"""
    new_allowed_ips = ["192.168.1.2", "10.0.0.2"]
    updated = update_ip_whitelist(
        db=db,
        ip_whitelist_id=ip_whitelist.id,
        allowed_ips=new_allowed_ips
    )
    assert updated is not None
    assert updated.id == ip_whitelist.id
    assert updated.allowed_ips == new_allowed_ips

def test_delete_ip_whitelist(db: Session, ip_whitelist: IPWhitelist):
    """Test deleting an IP whitelist entry"""
    delete_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist.id)
    retrieved = get_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist.id)
    assert retrieved is None

def test_disable_ip_whitelist(db: Session, ip_whitelist: IPWhitelist):
    """Test disabling an IP whitelist entry"""
    disabled = disable_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist.id)
    assert disabled is not None
    assert disabled.is_active is False

def test_get_nonexistent_ip_whitelist(db: Session):
    """Test getting a nonexistent IP whitelist entry"""
    retrieved = get_ip_whitelist(db=db, ip_whitelist_id=999)
    assert retrieved is None

def test_update_nonexistent_ip_whitelist(db: Session):
    """Test updating a nonexistent IP whitelist entry"""
    updated = update_ip_whitelist(
        db=db,
        ip_whitelist_id=999,
        allowed_ips=["192.168.1.1"]
    )
    assert updated is None

def test_disable_nonexistent_ip_whitelist(db: Session):
    """Test disabling a nonexistent IP whitelist entry"""
    disabled = disable_ip_whitelist(db=db, ip_whitelist_id=999)
    assert disabled is None
