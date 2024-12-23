"""FastAPI server for LLM gateway."""
from fastapi import FastAPI, HTTPException
from fastapi.responses import StreamingResponse
from typing import Dict, Any, AsyncGenerator

from ..router.router import Router

app = FastAPI()
router = Router()

@app.post("/v1/chat/completions")
async def chat_completions(request: Dict[str, Any]):
    """Handle chat completion requests."""
    try:
        response = await router.route_request(request)
        
        # Handle streaming response
        if request.get("stream", False):
            async def stream_generator():
                async for chunk in response:
                    yield f"data: {chunk}\n\n"
            return StreamingResponse(
                stream_generator(),
                media_type="text/event-stream"
            )
            
        return response
        
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
