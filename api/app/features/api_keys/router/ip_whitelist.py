from typing import List
from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session

from app.core.database import get_db
from app.features.api_keys.schemas.ip_whitelist import (
    IPWhitelistCreate,
    IPWhitelistUpdate,
    IPWhitelistResponse
)
from app.features.api_keys.service.ip_whitelist import (
    create_ip_whitelist,
    get_ip_whitelist,
    get_ip_whitelist_by_api_key,
    update_ip_whitelist,
    delete_ip_whitelist,
    disable_ip_whitelist
)

router = APIRouter(prefix="/api/v1/ip-whitelists", tags=["IP Whitelists"])

@router.post("/", response_model=IPWhitelistResponse)
def create_ip_whitelist_endpoint(
    ip_whitelist: IPWhitelistCreate,
    db: Session = Depends(get_db)
):
    """Create a new IP whitelist entry"""
    return create_ip_whitelist(
        db=db,
        api_key_id=ip_whitelist.api_key_id,
        allowed_ips=ip_whitelist.allowed_ips
    )

@router.get("/{ip_whitelist_id}", response_model=IPWhitelistResponse)
def get_ip_whitelist_endpoint(ip_whitelist_id: int, db: Session = Depends(get_db)):
    """Get an IP whitelist entry by ID"""
    ip_whitelist = get_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist_id)
    if not ip_whitelist:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="IP whitelist not found"
        )
    return ip_whitelist

@router.get("/api-key/{api_key_id}", response_model=IPWhitelistResponse)
def get_ip_whitelist_by_api_key_endpoint(api_key_id: int, db: Session = Depends(get_db)):
    """Get IP whitelist entry for an API key"""
    ip_whitelist = get_ip_whitelist_by_api_key(db=db, api_key_id=api_key_id)
    if not ip_whitelist:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="IP whitelist not found"
        )
    return ip_whitelist

@router.patch("/{ip_whitelist_id}", response_model=IPWhitelistResponse)
def update_ip_whitelist_endpoint(
    ip_whitelist_id: int,
    ip_whitelist: IPWhitelistUpdate,
    db: Session = Depends(get_db)
):
    """Update an IP whitelist entry"""
    updated = update_ip_whitelist(
        db=db,
        ip_whitelist_id=ip_whitelist_id,
        allowed_ips=ip_whitelist.allowed_ips
    )
    if not updated:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="IP whitelist not found"
        )
    return updated

@router.delete("/{ip_whitelist_id}", status_code=status.HTTP_204_NO_CONTENT)
def delete_ip_whitelist_endpoint(ip_whitelist_id: int, db: Session = Depends(get_db)):
    """Delete an IP whitelist entry"""
    ip_whitelist = get_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist_id)
    if not ip_whitelist:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="IP whitelist not found"
        )
    delete_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist_id)

@router.post("/{ip_whitelist_id}/disable", response_model=IPWhitelistResponse)
def disable_ip_whitelist_endpoint(ip_whitelist_id: int, db: Session = Depends(get_db)):
    """Disable an IP whitelist entry"""
    disabled = disable_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist_id)
    if not disabled:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="IP whitelist not found"
        )
    return disabled
