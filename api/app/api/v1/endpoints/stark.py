from fastapi import APIRouter

router = APIRouter()

@router.get("/stark")
async def stark_endpoint():
    return {
        "code": 0,
        "data": "hi stark",
        "message": "hello stark v1"
    }