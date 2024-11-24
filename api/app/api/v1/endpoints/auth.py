from fastapi import APIRouter

router = APIRouter()

@router.get("/")
async def read_auth():
    return {"message": "This is the auth endpoint"}
