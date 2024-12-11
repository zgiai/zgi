# File operations API endpoints

from fastapi import APIRouter, UploadFile, File,Form
from fastapi.responses import JSONResponse
from app.services import file_service
from app.services.rag_service import rag_service

router = APIRouter()

@router.post("/upload")
async def upload_file(file: UploadFile = File(...),saveToData:bool=Form(0)):
    try:
        result = await file_service.upload_file(file,saveToData)
        return JSONResponse(content=result)
    except Exception as e:
        return JSONResponse(status_code=500, content={"error": str(e)})

@router.post("/process")
async def process_file(file: UploadFile = File(...)):
    try:
        document = await file_service.read_file(file)
        await rag_service.process_document(document)
        return JSONResponse(content={"message": "File processed successfully"})
    except Exception as e:
        return JSONResponse(status_code=500, content={"error": str(e)})

@router.get("/search")
async def search_similar(query: str):
    try:
        query_vector = rag_service.vectorize_chunks([query])
        results = await rag_service.vector_store.search_vectors(query_vector)
        return JSONResponse(content={"results": results})
    except Exception as e:
        return JSONResponse(status_code=500, content={"error": str(e)})

@router.get("/list")
async def list_files():
    try:
        files = await file_service.list_files()
        return JSONResponse(content={"files": files})
    except Exception as e:
        return JSONResponse(status_code=400, content={"error": str(e)})

@router.post("/read_file")
async def read_file(file: UploadFile = File(...)):
    try:
        document = await file_service.process_and_vectorize_file(file)
        print("document", document)
        return document
    except Exception as e:
        return JSONResponse(status_code=400, content={"error": str(e)})

@router.get("/test")
async def test():
    return {"message": "Test successful"}