import logging
from typing import Callable

from fastapi import Request, Response
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.types import ASGIApp

logger = logging.getLogger(__name__)

class APILoggingMiddleware(BaseHTTPMiddleware):
    def __init__(
        self,
        app: ASGIApp,
        *,
        logger: logging.Logger = logger,
        exclude_paths: list[str] = None,
    ):
        super().__init__(app)
        self.logger = logger
        self.exclude_paths = exclude_paths or []

    async def dispatch(
        self, request: Request, call_next: RequestResponseEndpoint
    ) -> Response:
        if any(request.url.path.startswith(path) for path in self.exclude_paths):
            return await call_next(request)

        try:
            response = await call_next(request)
            return response
        except Exception as e:
            self.logger.error(f"Error processing request: {str(e)}")
            raise
