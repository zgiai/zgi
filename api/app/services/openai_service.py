from openai import AsyncOpenAI
from app.core.config import settings
from typing import List, Dict, Any, AsyncGenerator
import asyncio
import logging
from fastapi import HTTPException
import json
import time
import uuid

class OpenAIService:
    def __init__(self):
        self.client = AsyncOpenAI(api_key=settings.OPENAI_API_KEY, base_url=settings.OPENAI_API_BASE)

    async def stream_chat_completion(
        self,
        messages,
        model="gpt-3.5-turbo",
        temperature=1.0,
        top_p=1.0,
        n=1,
        max_tokens=None
    ):
        print(messages,'>>>>>messages')
        try:
            response = await self.client.chat.completions.create(
                model=model,
                messages=[{"role": m.role, "content": m.content} for m in messages],
                temperature=temperature,
                top_p=top_p,
                n=n,
                max_tokens=max_tokens,
                stream=True
            )
            
            async for chunk in response:
                if chunk.choices[0].delta.content is not None:
                    chunk_data = {
                        'id': f'chatcmpl-{uuid.uuid4().hex[:10]}',
                        'object': 'chat.completion.chunk',
                        'created': int(time.time()),
                        'model': model,
                        'system_fingerprint': f'fp_{uuid.uuid4().hex[:10]}',
                        'choices': [{
                            'index': 0,
                            'delta': {
                                'content': chunk.choices[0].delta.content
                            },
                            'logprobs': None,
                            'finish_reason': None
                        }]
                    }
                    yield f"data: {json.dumps(chunk_data)}\n\n"
            
            yield "data: [DONE]\n\n"
            
        except Exception as e:
            yield f"data: {json.dumps({'error': str(e)})}\n\n"

    async def create_chat_completion(
        self,
        messages,
        model="gpt-3.5-turbo",
        temperature=1.0,
        top_p=1.0,
        n=1,
        max_tokens=None
    ):
        try:
            response = await self.client.chat.completions.create(
                model=model,
                messages=[{"role": m.role, "content": m.content} for m in messages],
                temperature=temperature,
                top_p=top_p,
                n=n,
                max_tokens=max_tokens
            )
            
            return {
                "id": f"chatcmpl-{uuid.uuid4().hex[:10]}",
                "object": "chat.completion",
                "created": int(time.time()),
                "model": model,
                "system_fingerprint": f"fp_{uuid.uuid4().hex[:10]}",
                "choices": [{
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": response.choices[0].message.content,
                    },
                    "logprobs": None,
                    "finish_reason": "stop"
                }],
                "usage": {
                    "prompt_tokens": response.usage.prompt_tokens,
                    "completion_tokens": response.usage.completion_tokens,
                    "total_tokens": response.usage.total_tokens,
                    "completion_tokens_details": {
                        "reasoning_tokens": 0
                    }
                }
            }
            
        except Exception as e:
            raise Exception(f"OpenAI API error: {str(e)}")
