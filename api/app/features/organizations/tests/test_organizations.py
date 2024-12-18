import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session
import uuid

from app.main import app
from app.features.organizations.models import Organization, OrganizationMember, OrganizationRole
from app.features.users.models import User
from app.core.security.auth import create_access_token

@pytest.fixture
def client():
    return TestClient(app)

@pytest.fixture
def db(client):
    from app.core.database import SessionLocal
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()

@pytest.fixture
def test_user(db: Session):
    # 创建一个基本的用户，不包含关系
    user = User(
        email=f"test_org_{uuid.uuid4().hex[:8]}@example.com",
        username=f"testuser_org_{uuid.uuid4().hex[:8]}",
        full_name="Test User Org 1",
        hashed_password="hashed_password",
        is_active=True,
        is_verified=True
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return user

@pytest.fixture
def test_user2(db: Session):
    # 创建第二个测试用户
    user = User(
        email=f"test_org_{uuid.uuid4().hex[:8]}@example.com",
        username=f"testuser_org_{uuid.uuid4().hex[:8]}",
        full_name="Test User Org 2",
        hashed_password="hashed_password",
        is_active=True,
        is_verified=True
    )
    db.add(user)
    db.commit()
    db.refresh(user)
    return user

@pytest.fixture
def test_user_token(test_user: User):
    return create_access_token({"sub": str(test_user.id)})

@pytest.fixture
def auth_headers(test_user_token: str):
    return {"Authorization": f"Bearer {test_user_token}"}

def test_create_organization(client: TestClient, auth_headers: dict, db: Session):
    response = client.post(
        "/v1/organizations/",
        headers=auth_headers,
        json={
            "name": "Test Organization",
            "description": "Test Description"
        }
    )
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == "Test Organization"
    assert data["description"] == "Test Description"
    
    # 验证数据库记录
    db.rollback()  # 回滚当前事务
    db.expire_all()  # 刷新会话
    org = db.query(Organization).filter(Organization.id == data["id"]).first()
    assert org is not None
    assert org.name == "Test Organization"
    
    # 验证创建者是否被设置为所有者
    member = db.query(OrganizationMember).filter(
        OrganizationMember.organization_id == org.id
    ).first()
    assert member is not None
    assert member.role == OrganizationRole.OWNER

def test_list_organizations(client: TestClient, auth_headers: dict, db: Session, test_user: User, test_user2: User):
    # 创建测试组织
    org = Organization(name="Test Org 1", description="Test Description 1")
    db.add(org)
    db.flush()
    
    member = OrganizationMember(
        organization_id=org.id,
        user_id=test_user.id,  # 使用 test_user 而不是 test_user2
        role=OrganizationRole.MEMBER
    )
    db.add(member)
    db.commit()

    response = client.get("/v1/organizations/", headers=auth_headers)
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 1
    assert data[0]["name"] == "Test Org 1"

def test_update_organization(client: TestClient, auth_headers: dict, db: Session, test_user: User):
    # First create an organization
    org = Organization(
        name="Test Org Update",
        description="Test Description Update",
        created_by=test_user.id
    )
    db.add(org)
    db.flush()
    
    # Add test user as owner
    member = OrganizationMember(
        organization_id=org.id,
        user_id=test_user.id,
        role=OrganizationRole.OWNER
    )
    db.add(member)
    db.commit()
    
    # Update organization
    update_data = {
        "name": "Updated Org Name",
        "description": "Updated Description"
    }
    response = client.put(
        f"/v1/organizations/{org.uuid}",
        headers=auth_headers,
        json=update_data
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == update_data["name"]
    assert data["description"] == update_data["description"]
    
    # Verify database update
    db.rollback()  # Rollback any pending transaction
    db.expire_all()  # Expire all objects to force reload
    org = db.query(Organization).filter(Organization.id == org.id).first()  # Get fresh data
    assert org.name == update_data["name"]
    assert org.description == update_data["description"]

def test_update_organization_not_owner(client: TestClient, auth_headers: dict, db: Session, test_user: User, test_user2: User):
    # Create organization with test_user as owner
    org = Organization(
        name="Test Org Update",
        description="Test Description Update",
        created_by=test_user.id
    )
    db.add(org)
    db.flush()
    
    # Add test_user as owner
    member = OrganizationMember(
        organization_id=org.id,
        user_id=test_user.id,
        role=OrganizationRole.OWNER
    )
    db.add(member)
    
    # Add test_user2 as member
    member2 = OrganizationMember(
        organization_id=org.id,
        user_id=test_user2.id,
        role=OrganizationRole.MEMBER
    )
    db.add(member2)
    db.commit()
    
    # Get token for test_user2
    test_user2_token = create_access_token({"sub": str(test_user2.id)})
    test_user2_headers = {"Authorization": f"Bearer {test_user2_token}"}
    
    # Try to update organization as non-owner
    update_data = {
        "name": "Updated By Non-Owner",
        "description": "This update should fail"
    }
    response = client.put(
        f"/v1/organizations/{org.uuid}",
        headers=test_user2_headers,
        json=update_data
    )
    
    assert response.status_code == 403
    
    # Verify organization was not updated
    db.refresh(org)
    assert org.name == "Test Org Update"
    assert org.description == "Test Description Update"
