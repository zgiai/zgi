# File operations API endpoints

from fastapi import APIRouter, UploadFile, File, Query
from fastapi.responses import JSONResponse, StreamingResponse
from app.services.rag_service import rag_service
from app.services.openai_service import OpenAIService
from app.core.config import settings
import uuid
import time
from app.core.config import settings
import numpy as np
from app.utils.vector_store import VectorStore
from pydantic import BaseModel, Field
from typing import List, Optional

router = APIRouter()

# Create an instance of the service
openai_service = OpenAIService()
vector_service =VectorStore()
# 定义请求体模型
class Message(BaseModel):
    role: str
    content: str

class ChatCompletionRequest(BaseModel):
    model: str = Field(..., description="The model to use for completion")
    messages: List[Message]
    stream: bool = Field(default=False)
    temperature: Optional[float] = Field(default=1.0)
    top_p: Optional[float] = Field(default=1.0)
    n: Optional[int] = Field(default=1)
    max_tokens: Optional[int] = None

@router.get("/completionsx")
async def chat(query: str,stream: bool = False):
    try:
        # 获取向量搜索结果
        results = await vector_service.query_vector_store(query)
        print(results,'>>>>>>>results')
        # 提取相关内容
        context = "\n".join([result["content"] for result in results])
        print(context,'>>>>>>>results')
        # 构建提示
        prompt = f"Based on the following context:\n\n{context}\n\nUser query: {query}\n\nPlease provide a comprehensive and coherent response:"
        print(prompt,'>>>>>>>prompt')
        # 调用OpenAI API进行润色
        response = await rag_service.generate_response(prompt,stream=False)
        
        # 构建符合OpenAI格式的返回值
        return JSONResponse(content={
            "id": f"chatcmpl-{uuid.uuid4().hex[:10]}",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": settings.OPENAI_MODEL,
            "choices": [{
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": response
                },
                "finish_reason": "stop"
            }],
            "usage": {
                "prompt_tokens": len(prompt.split()),
                "completion_tokens": len(response.split()),
                "total_tokens": len(prompt.split()) + len(response.split())
            }
        })
    except Exception as e:
        return JSONResponse(status_code=500, content={"error": str(e)})

async def generate_complete_response(
    messages: List[Message],
    model: str,
    temperature: float = 1.0,
    top_p: float = 1.0,
    n: int = 1,
    max_tokens: Optional[int] = None
):
    return await openai_service.create_chat_completion(
        messages=messages,
        model=model,
        temperature=temperature,
        top_p=top_p,
        n=n,
        max_tokens=max_tokens
    )

@router.post("/completions")
async def chat_completions(request: ChatCompletionRequest):
    sys_message = next((m for m in request.messages if m.role == "system"), None) 
    user_message = next((m for m in request.messages if m.role == "user"), None)  # 查找用户消息
    if user_message:  # 确保找到了用户消息
        results = await vector_service.query_vector_store(user_message.content)  # 使用用户消息的 content 进行查询
        print(results, '>>>>>>>results')
        if results:  # 检查 results 是否存在数据
            # 提取相关内容
            context = "\n".join([result["content"] for result in results])
            print(context, '>>>>>>>results')
            # 构建提示
            prompt = f"Based on the following context:\n\n{context}\n\nUser query: {user_message.content}\n\nPlease provide a comprehensive and coherent response:"
            print(prompt, '>>>>>>>prompt')
            
            # 构建新的请求格式
            new_message = [
                {
                    "role": "system",
                    "content": sys_message.content
                },
                {
                    "role": "user",
                    "content": prompt  # 使用生成的 prompt 替换用户消息的 content
                }
            ]
            messages = [Message(role=m['role'], content=m['content']) for m in new_message]
        else:  # 如果 results 没有数据
            print("No results found, using original messages.")
            messages = request.messages  # 使用原始请求的 messages

        
        print(messages, '>>>>>>>new_message')
    try:
        if request.stream:
            return StreamingResponse(
                openai_service.stream_chat_completion(
                    messages=messages,
                    model=request.model,
                    temperature=request.temperature,
                    top_p=request.top_p,
                    n=request.n,
                    max_tokens=request.max_tokens
                ), 
                media_type="text/event-stream"
            )
        else:
            return await generate_complete_response(
                messages=messages,
                model=request.model,
                temperature=request.temperature,
                top_p=request.top_p,
                n=request.n,
                max_tokens=request.max_tokens
            )
    except Exception as e:
        return JSONResponse(status_code=500, content={"error": str(e)})
