from fastapi import APIRouter
from app.api.v1.endpoints import auth

def setup_routes(app: APIRouter):
    app.include_router(
        router=auth.router,
        prefix="/auth",
        tags=["auth"]
    )
