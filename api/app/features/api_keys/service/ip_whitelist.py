from typing import List, Optional
from sqlalchemy.orm import Session

from app.features.api_keys.models import IPWhitelist

def create_ip_whitelist(
    db: Session,
    api_key_id: int,
    allowed_ips: List[str]
) -> IPWhitelist:
    """Create a new IP whitelist entry"""
    ip_whitelist = IPWhitelist(
        api_key_id=api_key_id,
        allowed_ips=allowed_ips
    )
    db.add(ip_whitelist)
    db.commit()
    db.refresh(ip_whitelist)
    return ip_whitelist

def get_ip_whitelist(db: Session, ip_whitelist_id: int) -> Optional[IPWhitelist]:
    """Get an IP whitelist entry by ID"""
    return db.query(IPWhitelist).filter(IPWhitelist.id == ip_whitelist_id).first()

def get_ip_whitelist_by_api_key(db: Session, api_key_id: int) -> Optional[IPWhitelist]:
    """Get IP whitelist entry for an API key"""
    return db.query(IPWhitelist).filter(IPWhitelist.api_key_id == api_key_id).first()

def update_ip_whitelist(
    db: Session,
    ip_whitelist_id: int,
    allowed_ips: List[str]
) -> Optional[IPWhitelist]:
    """Update an IP whitelist entry"""
    ip_whitelist = get_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist_id)
    if ip_whitelist:
        ip_whitelist.allowed_ips = allowed_ips
        db.commit()
        db.refresh(ip_whitelist)
    return ip_whitelist

def delete_ip_whitelist(db: Session, ip_whitelist_id: int) -> None:
    """Delete an IP whitelist entry"""
    ip_whitelist = get_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist_id)
    if ip_whitelist:
        db.delete(ip_whitelist)
        db.commit()

def disable_ip_whitelist(db: Session, ip_whitelist_id: int) -> Optional[IPWhitelist]:
    """Disable an IP whitelist entry"""
    ip_whitelist = get_ip_whitelist(db=db, ip_whitelist_id=ip_whitelist_id)
    if ip_whitelist:
        ip_whitelist.is_active = False
        db.commit()
        db.refresh(ip_whitelist)
    return ip_whitelist
