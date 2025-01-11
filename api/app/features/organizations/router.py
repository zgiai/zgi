from datetime import datetime, timedelta

from fastapi import APIRouter, Depends, HTTPException, Query, Body, Request
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.orm import Session
from typing import List, Optional, Annotated

from app.core.auth import require_admin
from app.core.base import resp_200
from app.core.database import get_sync_db
from app.core.security import create_access_token, verify_token
from app.core.security.auth import get_current_user
from app.core.logging.api_logger import logger
from app.features import Project
from app.features.organizations.schemas import (
    OrganizationResponse,
    OrganizationList,
    MemberList,
    RoleResponse,
    RoleBase,
    RoleResponseBase,
    MemberResponse,
    SearchResponse,
)
from app.features.organizations.service import (
    organization_require_member_admin,
    organization_body_require_admin,
    organization_params_require_admin,
    role_params_require_admin,
)
from app.features.projects.models import ProjectStatus
from app.features.users.models import User
from app.features.organizations.models import (
    Organization,
    OrganizationMember,
    OrganizationRole,
    Role,
    organization_member_roles,
)
from app.features.organizations import schemas

router = APIRouter(tags=["organizations"])


@router.post("/create")
def create_organization(
    org_data: schemas.OrganizationCreate,
    current_user: User = Depends(require_admin),
    db: Session = Depends(get_sync_db),
):
    """Create a new organization"""
    org = Organization(
        name=org_data.name, description=org_data.description, created_by=current_user.id
    )
    db.add(org)
    # db.flush()

    # # Add creator as owner
    # member = OrganizationMember(
    #     organization_id=org.id,
    #     user_id=current_user.id
    # )
    # db.add(member)
    db.commit()
    db.refresh(org)
    resp_data = OrganizationResponse.model_validate(org)
    if org_data.project:
        project = Project(
            name=org_data.project.name,
            description=org_data.project.description,
            organization_id=org.id,
            created_by=current_user.id,
        )
        db.add(project)
        db.commit()
        db.refresh(project)
        resp_data.projects = [project]
    return resp_200(resp_data)


@router.get("/list")
def list_organizations(
    user_id: Optional[int] = None,
    page_size: Optional[int] = 10,
    page_num: Optional[int] = 1,
    is_active: Optional[bool] = None,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_sync_db),
):
    """List organizations for current user"""
    query = db.query(Organization)
    if is_active is not None:
        query = query.filter(Organization.is_active == is_active)
    if not current_user.is_superuser and current_user.user_type == 0:
        query = query.join(OrganizationMember).filter(
            OrganizationMember.user_id == current_user.id
        )
    else:
        if user_id:
            query = query.join(OrganizationMember).filter(
                OrganizationMember.user_id == user_id
            )
    total = query.count()
    if page_size and page_num:
        query = query.offset((page_num - 1) * page_size).limit(page_size)
    organization_list = query.all()
    resp_data_list = []
    for org in organization_list:
        org_resp = OrganizationResponse.model_validate(org)
        org.projects = (
            db.query(Project)
            .filter(
                Project.organization_id == org.id,
                Project.status == ProjectStatus.ACTIVE,
            )
            .all()
        )
        resp_data_list.append(org_resp)
    return resp_200(data=OrganizationList(organizations=resp_data_list, total=total))


@router.get("/info")
def get_organization(
    organization_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_sync_db),
):
    """Get organization details"""
    org = db.query(Organization).filter(Organization.id == organization_id).first()
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")
    resp_data = OrganizationResponse.model_validate(org)
    resp_data.projects = (
        db.query(Project)
        .filter(
            Project.organization_id == org.id, Project.status == ProjectStatus.ACTIVE
        )
        .all()
    )
    return resp_200(data=resp_data)


@router.put("/update")
def update_organization(
    organization_id: int,
    org_data: schemas.OrganizationUpdate,
    current_user: User = Depends(organization_params_require_admin),
    db: Session = Depends(get_sync_db),
):
    """Update organization details"""
    # First check if user is a member of the organization
    org = db.query(Organization).filter(Organization.id == organization_id).first()
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")

    # Update organization
    update_data = org_data.model_dump(exclude_unset=True, exclude_none=True)
    if "is_active" not in update_data:
        update_data["is_active"] = org.is_active

    # Handle datetime fields separately
    update_data.pop("created_at", None)
    update_data.pop("updated_at", None)

    for field, value in update_data.items():
        if hasattr(org, field):
            if isinstance(value, bytes):
                value = value.decode()
            setattr(org, field, value)

    db.commit()
    db.refresh(org)

    # Convert to Pydantic model for proper serialization
    return resp_200(data=OrganizationResponse.model_validate(org))


@router.delete("/delete")
def delete_organization(
    organization_id: int,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(require_admin),
):
    """Soft delete a project"""
    org = db.query(Organization).filter(Organization.id == organization_id).first()

    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")
    if not org.is_active:
        raise HTTPException(status_code=400, detail="Organization is already deleted")

    # Soft delete the Organization
    org.is_active = False

    try:
        db.add(org)
        db.commit()
        db.refresh(org)
        return resp_200(data=OrganizationResponse.model_validate(org))
    except SQLAlchemyError as e:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/set_admins")
def set_admins(
    organization_id: Annotated[int, Body(embed=True)],
    user_ids: Annotated[List[int], Body(embed=True)],
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(require_admin),
):
    """Set admins for an organization"""
    for user_id in user_ids:
        org = db.query(Organization).filter(Organization.id == organization_id).first()
        if not org:
            raise HTTPException(status_code=404, detail="Organization not found")
        if not org.is_active:
            raise HTTPException(
                status_code=400, detail="Organization is already deleted"
            )
        user = db.query(User).filter(User.id == user_id).first()
        if not user:
            raise HTTPException(status_code=404, detail=f"user_id {user_id} not found")
        member = (
            db.query(OrganizationMember)
            .filter(
                OrganizationMember.organization_id == organization_id,
                OrganizationMember.user_id == user_id,
            )
            .first()
        )
        if member:
            member.is_admin = True
            db.add(member)
            db.commit()
        else:
            new_member = OrganizationMember(
                organization_id=organization_id, user_id=user_id, is_admin=True
            )
            db.add(new_member)
            db.commit()
    return resp_200("Admins updated successfully")


@router.post("/unset_admins")
def unset_admins(
    organization_id: Annotated[int, Body(embed=True)],
    user_ids: Annotated[List[int], Body(embed=True)],
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(require_admin),
):
    """Unset multiple admins for an organization"""
    for user_id in user_ids:
        member = (
            db.query(OrganizationMember)
            .filter(
                OrganizationMember.organization_id == organization_id,
                OrganizationMember.user_id == user_id,
            )
            .first()
        )
        if not member:
            raise HTTPException(status_code=404, detail="Member not found")
        if not member.is_admin:
            raise HTTPException(status_code=400, detail="User is not an admin")
        member.is_admin = False
        db.add(member)
    db.commit()
    return resp_200(message="Admins unset successfully")


@router.post("/set_members")
def set_members(
    organization_id: Annotated[int, Body(embed=True)],
    user_ids: Annotated[List[int], Body(embed=True)],
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(organization_body_require_admin),
):
    """Set members for an organization"""
    for user_id in user_ids:
        org = db.query(Organization).filter(Organization.id == organization_id).first()
        if not org:
            raise HTTPException(status_code=404, detail="Organization not found")
        if not org.is_active:
            raise HTTPException(
                status_code=400, detail="Organization is already deleted"
            )
        user = db.query(User).filter(User.id == user_id).first()
        if not user:
            raise HTTPException(status_code=404, detail=f"user_id {user_id} not found")
        member = (
            db.query(OrganizationMember)
            .filter(
                OrganizationMember.organization_id == organization_id,
                OrganizationMember.user_id == user_id,
            )
            .first()
        )
        if member:
            logger.info(f"User {user_id} is already a member of the organization")
            continue
        else:
            new_member = OrganizationMember(
                organization_id=organization_id, user_id=user_id, is_admin=False
            )
            db.add(new_member)
            db.commit()
    return resp_200(message="Members updated successfully")


@router.post("/unset_members")
def unset_members(
    organization_id: Annotated[int, Body(embed=True)],
    user_ids: Annotated[List[int], Body(embed=True)],
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(organization_body_require_admin),
):
    """Unset multiple members from an organization"""
    for user_id in user_ids:
        member = (
            db.query(OrganizationMember)
            .filter(
                OrganizationMember.organization_id == organization_id,
                OrganizationMember.user_id == user_id,
            )
            .first()
        )
        if not member:
            raise HTTPException(status_code=404, detail="Member not found")
        if member.is_admin:
            raise HTTPException(status_code=400, detail="User is an admin")
        db.delete(member)
    db.commit()
    return resp_200(message="Members unset successfully")


@router.post("/invite")
def create_invitation(
    organization_id: Annotated[int, Body(embed=True)],
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(organization_body_require_admin),
):
    """Create an invitation for an organization"""
    expires_delta = timedelta(minutes=30)
    expires_at = datetime.now() + expires_delta
    invite_token = create_access_token(
        {"org_id": organization_id}, expires_delta=expires_delta
    )
    return resp_200(data={"invite_token": invite_token, "expires_at": expires_at})


@router.post("/verify_invite")
def verify_invite(invite_token: Annotated[str, Body(embed=True)],
                  db: Session = Depends(get_sync_db),
                  current_user: User = Depends(get_current_user)):
    decoded_token = verify_token(invite_token)
    if not decoded_token:
        raise HTTPException(status_code=400, detail="Invalid invite token")
    org_id = decoded_token['org_id']
    if not org_id:
        raise HTTPException(status_code=400, detail="Token does not contain organization_id")
    organization = db.query(Organization).filter(Organization.id == org_id).first()
    if not organization:
        raise HTTPException(status_code=404, detail="Organization not found")
    resp_data = OrganizationResponse.model_validate(organization)
    return resp_200(resp_data)

@router.post("/accept_invite")
def accept_invite(
    invite_token: Annotated[str, Body(embed=True)],
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(get_current_user),
):
    decoded_token = verify_token(invite_token)
    if not decoded_token:
        raise HTTPException(status_code=400, detail="Invalid invite token")
    org_id = decoded_token["org_id"]
    member = (
        db.query(OrganizationMember)
        .filter(
            OrganizationMember.organization_id == org_id,
            OrganizationMember.user_id == current_user.id,
        )
        .first()
    )
    if member:
        raise HTTPException(
            status_code=400, detail="User already belongs to the organization"
        )
    new_member = OrganizationMember(
        organization_id=org_id, user_id=current_user.id, is_admin=False
    )
    db.add(new_member)
    db.commit()
    return resp_200(message="Invitation accepted successfully")


@router.post("/roles/create")
def create_role(
    request: Request,
    role_data: schemas.RoleCreate = Body(...),
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(organization_body_require_admin),
):
    """Create a new role"""
    raw_body = request.body()
    print(f"Received raw body: {raw_body}")
    existing_role = (
        db.query(Role)
        .filter(
            Role.organization_id == role_data.organization_id,
            Role.name == role_data.name,
        )
        .first()
    )
    if existing_role:
        raise HTTPException(
            status_code=400,
            detail=f"Role with this name {role_data.name} already exists",
        )
    role = Role(
        organization_id=role_data.organization_id,
        name=role_data.name,
        description=role_data.description,
        created_by=current_user.id,
    )
    db.add(role)
    db.commit()
    db.refresh(role)
    return resp_200(data=schemas.RoleResponse.model_validate(role))


@router.get("/roles/list")
def list_roles(
    organization_id: Optional[int] = None,
    page_size: Optional[int] = 10,
    page_num: Optional[int] = 1,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(get_current_user),
):
    """List roles"""
    query = db.query(Role).filter(Role.is_active == True)
    if organization_id:
        query = query.filter(Role.organization_id == organization_id)
    total = query.count()
    if page_size and page_num:
        query = query.offset((page_num - 1) * page_size).limit(page_size)
    role_list = query.all()
    return resp_200(data=schemas.RoleList(roles=role_list, total=total))


@router.get("/roles/info")
def get_role(
    role_id: int,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(get_current_user),
):
    """Get role details"""
    role = db.query(Role).filter(Role.id == role_id, Role.is_active == True).first()
    if not role:
        raise HTTPException(status_code=404, detail="Role not found")
    return resp_200(data=schemas.RoleResponse.model_validate(role))


@router.put("/roles/update")
def update_role(
    role_id: int,
    role_data: schemas.RoleUpdate,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(role_params_require_admin),
):
    """Update role details"""
    role = db.query(Role).filter(Role.id == role_id).first()
    if not role:
        raise HTTPException(status_code=404, detail="Role not found")
    existing_role = (
        db.query(Role)
        .filter(
            Role.organization_id == role.organization_id, Role.name == role_data.name
        )
        .first()
    )
    if existing_role and existing_role.id != role_id:
        raise HTTPException(
            status_code=400,
            detail=f"Role with this name {role_data.name} already exists",
        )

    update_data = role_data.model_dump(exclude_unset=True)
    for field, value in update_data.items():
        if value and hasattr(role, field):
            setattr(role, field, value)

    db.commit()
    db.refresh(role)
    return resp_200(data=schemas.RoleResponse.model_validate(role))


@router.delete("/roles/delete")
def delete_role(
    role_id: int,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(role_params_require_admin),
):
    """Delete a role"""
    role = db.query(Role).filter(Role.id == role_id).first()
    if not role:
        raise HTTPException(status_code=404, detail="Role not found")

    if not role.is_active:
        raise HTTPException(status_code=400, detail="Role is already deleted")

    # Soft delete the role
    role.is_active = False

    try:
        db.commit()
        db.refresh(role)
        return resp_200(message="Role deleted successfully")
    except SQLAlchemyError as e:
        db.rollback()
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/members/update_roles")
def bind_role_to_member(
    member_id: Annotated[int, Body(embed=True)],
    role_ids: Annotated[List[int], Body(embed=True)],
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(organization_require_member_admin),
):
    """Bind a role to an organization member"""
    member = (
        db.query(OrganizationMember).filter(OrganizationMember.id == member_id).first()
    )
    if not member:
        raise HTTPException(status_code=404, detail="Organization member not found")
    all_roles = (
        db.query(Role)
        .filter(Role.organization_id == member.organization_id, Role.is_active == True)
        .all()
    )
    all_role_dict = {role.id: role for role in all_roles}
    role_ids_set = set(role_ids)
    if not role_ids_set.issubset(all_role_dict):
        missing_role_ids = role_ids_set - all_role_dict.keys()
        raise HTTPException(
            status_code=400,
            detail=f"One or more role IDs do not exist in the organization: {missing_role_ids}",
        )

    old_roles_dict = {role.id: role for role in member.roles}
    need_add_role_id_set = role_ids_set - old_roles_dict.keys()
    need_delete_role_id_set = old_roles_dict.keys() - role_ids_set
    for role_id in need_delete_role_id_set:
        role = old_roles_dict[role_id]
        member.roles.remove(role)
    for role_id in need_add_role_id_set:
        role = all_role_dict[role_id]
        member.roles.append(role)
    if need_add_role_id_set or need_delete_role_id_set:
        db.commit()
        db.refresh(member)
    return resp_200(message="Role update to member successfully")


@router.get("/members/list")
def list_members(
    organization_id: int,
    page_size: Optional[int] = 10,
    page_num: Optional[int] = 1,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(get_current_user),
):
    """List all members for an organization"""
    org = db.query(Organization).filter(Organization.id == organization_id).first()
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")
    query = db.query(OrganizationMember).filter(
        OrganizationMember.organization_id == organization_id
    )
    total = query.count()
    if page_size and page_num:
        query = query.offset((page_num - 1) * page_size).limit(page_size)
    member_list = query.all()
    response_members = []
    for member in member_list:
        response_member = {
            "id": member.id,
            "organization_id": member.organization_id,
            "user_id": member.user_id,
            "username": member.user.username,
            "is_admin": member.is_admin,
            "roles": [RoleResponseBase.model_validate(role) for role in member.roles],
            "created_at": member.created_at,
            "updated_at": member.updated_at,
        }
        response_members.append(response_member)
    return resp_200(data=MemberList(members=response_members, total=total))


@router.get("/members/me")
def members_me(
    organization_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_sync_db),
):
    org = db.query(Organization).filter(Organization.id == organization_id).first()
    if not org:
        raise HTTPException(status_code=404, detail="Organization not found")
    member = (
        db.query(OrganizationMember)
        .filter(
            OrganizationMember.organization_id == organization_id,
            OrganizationMember.user_id == current_user.id,
        )
        .first()
    )
    if not member:
        raise HTTPException(status_code=404, detail="Member not found")
    response_member = MemberResponse(
        id=member.id,
        organization_id=member.organization_id,
        user_id=member.user_id,
        username=member.user.username,
        is_admin=member.is_admin,
        roles=[RoleResponseBase.model_validate(role) for role in member.roles],
        created_at=member.created_at,
        updated_at=member.updated_at,
    )
    return resp_200(data=response_member)


@router.post("/members/exit")
def members_exit(
    organization_id: int,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(get_current_user),
):
    """Allow current user to leave an organization"""
    member = (
        db.query(OrganizationMember)
        .filter(
            OrganizationMember.organization_id == organization_id,
            OrganizationMember.user_id == current_user.id,
        )
        .first()
    )
    if not member:
        raise HTTPException(status_code=404, detail="Member not found")

    db.delete(member)
    db.commit()
    return resp_200(message="You have successfully exited the organization")


@router.get("/search_user")
def search_user(
    organization_id: int,
    email: str,
    db: Session = Depends(get_sync_db),
    current_user: User = Depends(organization_params_require_admin),
):
    """Get user by email and organization_id"""
    user = db.query(User).filter(User.email == email).first()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")

    member = (
        db.query(OrganizationMember)
        .filter(
            OrganizationMember.organization_id == organization_id,
            OrganizationMember.user_id == user.id,
        )
        .first()
    )

    search_response = SearchResponse(
        user_id=user.id, username=user.username, email=user.email
    )
    if member:
        search_response.organization_id = member.organization_id
        search_response.roles = [
            RoleResponseBase.model_validate(role) for role in member.roles
        ]

    return resp_200(data=search_response)
