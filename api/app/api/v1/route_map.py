from fastapi import APIRouter
from app.api.v1.endpoints import auth
from app.api.v1 import flows

def setup_routes(app: APIRouter):
    app.include_router(
        router=auth.router,
        prefix="/auth",
        tags=["auth"]
    )

def setup_flows_routes(app: APIRouter):
    app.include_router(
        router=flows.router,
        prefix="/flows",
        tags=["flows"]
    )