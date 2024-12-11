from typing import Optional
from fastapi import Request, HTTPException
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import Response

from app.core.security import verify_token
from app.core.cache import Cache
from app.core.deps import get_db
from app.features.users.service import UserService


class AuthMiddleware(BaseHTTPMiddleware):
    def __init__(self, app, cache: Cache):
        super().__init__(app)
        self.cache = cache
        self.security = HTTPBearer()
        self.public_paths = {
            "/docs",
            "/redoc",
            "/openapi.json",
            "/api/v1/auth/login",
            "/api/v1/auth/register",
            "/api/v1/auth/password-reset/request",
            "/api/v1/auth/password-reset/confirm",
        }

    async def dispatch(self, request: Request, call_next) -> Response:
        # Skip authentication for public paths
        if self._is_public_path(request.url.path):
            return await call_next(request)

        try:
            # Get token from header
            credentials: Optional[HTTPAuthorizationCredentials] = await self.security(request)
            if not credentials:
                raise HTTPException(status_code=401, detail="Missing authentication token")

            # Verify token
            token_data = verify_token(credentials.credentials)
            if not token_data:
                raise HTTPException(status_code=401, detail="Invalid authentication token")

            # Check token in cache (for revocation)
            if await self._is_token_revoked(credentials.credentials):
                raise HTTPException(status_code=401, detail="Token has been revoked")

            # Get user from cache or database
            user = await self._get_user(token_data.sub)
            if not user:
                raise HTTPException(status_code=401, detail="User not found")

            if not user.is_active:
                raise HTTPException(status_code=401, detail="User is inactive")

            # Add user to request state
            request.state.user = user

            # Log API metrics
            await self._log_api_metrics(request, user.id)

            response = await call_next(request)
            return response

        except HTTPException as e:
            return Response(
                content={"detail": e.detail},
                status_code=e.status_code,
                media_type="application/json"
            )
        except Exception as e:
            return Response(
                content={"detail": "Internal server error"},
                status_code=500,
                media_type="application/json"
            )

    def _is_public_path(self, path: str) -> bool:
        return any(path.startswith(public_path) for public_path in self.public_paths)

    async def _is_token_revoked(self, token: str) -> bool:
        return await self.cache.get(f"revoked_token:{token}") is not None

    async def _get_user(self, user_id: int):
        # Try to get from cache first
        cache_key = f"user:{user_id}"
        if user := await self.cache.get(cache_key):
            return user

        # Get from database
        db = next(get_db())
        service = UserService(db, self.cache, None)  # Email service not needed here
        user = await service.get_user_profile(user_id)
        if user:
            # Cache for 5 minutes
            await self.cache.set(cache_key, user, expire=300)
        return user

    async def _log_api_metrics(self, request: Request, user_id: int):
        # Log API call metrics asynchronously
        metrics = {
            "endpoint": request.url.path,
            "method": request.method,
            "user_id": user_id,
            "ip_address": request.client.host,
            "user_agent": request.headers.get("user-agent"),
        }
        # This should be handled by your metrics collection system
        # For example, send to a message queue for async processing
