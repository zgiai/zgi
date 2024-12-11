from fastapi import APIRouter, Depends
from fastapi.security import OAuth2PasswordBearer, OAuth2PasswordRequestForm

from app.core.database import get_db
from app.features.auth.client.service import AuthClientService
from app.features.auth.client.schemas import Token
from app.features.users.models import User

router = APIRouter(prefix="/v1/auth", tags=["auth"])
oauth2_scheme = OAuth2PasswordBearer(tokenUrl="v1/auth/token")


def get_auth_service(db=Depends(get_db)):
    return AuthClientService(db)


def get_current_user(
    token: str = Depends(oauth2_scheme),
    auth_service: AuthClientService = Depends(get_auth_service),
) -> User:
    return auth_service.get_current_user(token)


@router.post("/token", response_model=Token)
async def login_for_access_token(
    form_data: OAuth2PasswordRequestForm = Depends(),
    auth_service: AuthClientService = Depends(get_auth_service),
):
    """Login endpoint to get access token"""
    result = auth_service.login(form_data.username, form_data.password)
    return Token(
        access_token=result["access_token"],
        token_type=result["token_type"]
    )


@router.get("/me", response_model=dict)
async def protected_resource(current_user: User = Depends(get_current_user)):
    """Protected resource endpoint for testing"""
    return {
        "message": "You are authenticated",
        "user_id": current_user.id,
        "email": current_user.email
    }
