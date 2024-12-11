from typing import List, Optional
from ipaddress import ip_network, ip_address, IPv4Network, IPv4Address
from sqlalchemy.orm import Session
from fastapi import Request, HTTPException
from starlette.middleware.base import BaseHTTPMiddleware

from app.models.security import IPWhitelist
from app.core.database import get_db

class IPWhitelistChecker:
    """Check if IP addresses are allowed"""
    
    @staticmethod
    def is_ip_allowed(ip: str, allowed_ips: List[str]) -> bool:
        """
        Check if an IP is in the allowed list
        Supports both individual IPs and CIDR notation
        """
        try:
            client_ip = ip_address(ip)
            for allowed in allowed_ips:
                try:
                    if '/' in allowed:
                        network = ip_network(allowed, strict=False)
                        if client_ip in network:
                            return True
                    else:
                        if client_ip == ip_address(allowed):
                            return True
                except ValueError:
                    continue
            return False
        except ValueError:
            return False

class IPWhitelistMiddleware(BaseHTTPMiddleware):
    """Middleware to check IP whitelist"""
    
    async def dispatch(self, request: Request, call_next):
        # Skip IP check for certain paths
        if request.url.path.startswith("/docs") or request.url.path.startswith("/openapi"):
            return await call_next(request)

        # Get client IP
        client_ip = request.client.host
        
        # Get database session
        db = next(get_db())
        
        try:
            # Get API key from header
            api_key = request.headers.get("X-API-Key")
            if api_key:
                # Get whitelist for this API key
                whitelist = db.query(IPWhitelist).filter(
                    IPWhitelist.api_key_id == api_key,
                    IPWhitelist.is_active == True
                ).first()
                
                if whitelist and whitelist.allowed_ips:
                    if not IPWhitelistChecker.is_ip_allowed(client_ip, whitelist.allowed_ips):
                        raise HTTPException(
                            status_code=403,
                            detail=f"IP {client_ip} is not in the whitelist"
                        )
        finally:
            db.close()
        
        return await call_next(request)

class IPWhitelistManager:
    """Manage IP whitelist entries"""
    
    def __init__(self, db: Session):
        self.db = db

    def create_whitelist(self, api_key_id: str, allowed_ips: List[str]) -> IPWhitelist:
        """Create a new IP whitelist entry"""
        # Validate IP addresses
        for ip in allowed_ips:
            try:
                if '/' in ip:
                    ip_network(ip, strict=False)
                else:
                    ip_address(ip)
            except ValueError:
                raise ValueError(f"Invalid IP address or CIDR range: {ip}")

        whitelist = IPWhitelist(
            api_key_id=api_key_id,
            allowed_ips=allowed_ips
        )
        self.db.add(whitelist)
        self.db.commit()
        self.db.refresh(whitelist)
        return whitelist

    def update_whitelist(self, api_key_id: str, allowed_ips: List[str]) -> Optional[IPWhitelist]:
        """Update an existing IP whitelist"""
        whitelist = self.db.query(IPWhitelist).filter(
            IPWhitelist.api_key_id == api_key_id
        ).first()
        
        if not whitelist:
            return None

        # Validate IP addresses
        for ip in allowed_ips:
            try:
                if '/' in ip:
                    ip_network(ip, strict=False)
                else:
                    ip_address(ip)
            except ValueError:
                raise ValueError(f"Invalid IP address or CIDR range: {ip}")

        whitelist.allowed_ips = allowed_ips
        self.db.commit()
        self.db.refresh(whitelist)
        return whitelist

    def get_whitelist(self, api_key_id: str) -> Optional[IPWhitelist]:
        """Get IP whitelist for an API key"""
        return self.db.query(IPWhitelist).filter(
            IPWhitelist.api_key_id == api_key_id
        ).first()

    def delete_whitelist(self, api_key_id: str) -> bool:
        """Delete an IP whitelist"""
        whitelist = self.db.query(IPWhitelist).filter(
            IPWhitelist.api_key_id == api_key_id
        ).first()
        
        if not whitelist:
            return False

        self.db.delete(whitelist)
        self.db.commit()
        return True
