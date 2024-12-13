import json
import time
import uuid
from typing import Optional, AsyncGenerator, Dict, Any
import httpx
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, desc, and_
from tenacity import retry, stop_after_attempt, wait_exponential

from app.core.config import settings
from app.features.chat.models.chat import ChatSession
from app.features.chat.schemas.chat import (
    ChatCompletionRequest,
    ChatCompletionResponse,
    ChatCompletionStreamResponse
)

class ChatService:
    def __init__(self, db: AsyncSession):
        self.db = db
        self.openai_api_key = settings.OPENAI_API_KEY
        self.openai_api_base = settings.OPENAI_API_BASE or "https://api.openai.com"
        self.timeout = httpx.Timeout(30.0, connect=5.0)
        self.max_retries = 3
        
    async def _create_chat_session(
        self,
        request: ChatCompletionRequest,
        user_id: int,
        request_id: str,
        ip_address: str
    ) -> ChatSession:
        chat_session = ChatSession(
            user_id=user_id,
            conversation_id=str(uuid.uuid4()),
            request_id=request_id,
            question=request.messages[-1].content if request.messages else "",
            model=request.model,
            status=3,  # In progress
            ip_address=ip_address,
            raw_request=request.model_dump_json(),
            parameters={
                "temperature": request.temperature,
                "top_p": request.top_p,
                "n": request.n,
                "stop": request.stop,
                "max_tokens": request.max_tokens,
                "presence_penalty": request.presence_penalty,
                "frequency_penalty": request.frequency_penalty,
            }
        )
        self.db.add(chat_session)
        await self.db.commit()
        await self.db.refresh(chat_session)
        return chat_session

    def _calculate_cost(self, model: str, prompt_tokens: int, completion_tokens: int) -> float:
        model_prices = {
            "gpt-4": {"prompt": 0.03, "completion": 0.06},
            "gpt-4-32k": {"prompt": 0.06, "completion": 0.12},
            "gpt-3.5-turbo": {"prompt": 0.001, "completion": 0.002},
            "gpt-3.5-turbo-16k": {"prompt": 0.003, "completion": 0.004},
        }
        
        if model not in model_prices:
            model = "gpt-3.5-turbo"
        
        prices = model_prices[model]
        cost = (prompt_tokens * prices["prompt"] + completion_tokens * prices["completion"]) / 1000
        return round(cost, 7)

    async def _update_chat_session(
        self,
        chat_session: ChatSession,
        response_data: Dict[str, Any],
        answer: str,
        status: int = 1
    ):
        chat_session.status = status
        chat_session.answer = answer
        chat_session.openai_response_id = response_data.get("id")
        chat_session.openai_created_at = response_data.get("created")
        chat_session.openai_system_fingerprint = response_data.get("system_fingerprint")
        
        if "usage" in response_data:
            usage = response_data["usage"]
            chat_session.prompt_tokens = usage.get("prompt_tokens", 0)
            chat_session.completion_tokens = usage.get("completion_tokens", 0)
            chat_session.cost = self._calculate_cost(
                chat_session.model,
                chat_session.prompt_tokens,
                chat_session.completion_tokens
            )
        
        if "choices" in response_data and response_data["choices"]:
            chat_session.finish_reason = response_data["choices"][0].get("finish_reason")
        
        await self.db.commit()

    @retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=4, max=10))
    async def _make_openai_request(self, request: ChatCompletionRequest) -> httpx.Response:
        headers = {
            "Authorization": f"Bearer {self.openai_api_key}",
            "Content-Type": "application/json",
            "OpenAI-Organization": settings.OPENAI_ORG_ID if settings.OPENAI_ORG_ID else None
        }
        headers = {k: v for k, v in headers.items() if v is not None}
        
        async with httpx.AsyncClient(timeout=self.timeout) as client:
            response = await client.post(
                f"{self.openai_api_base}/v1/chat/completions",
                json=json.loads(request.model_dump_json()),
                headers=headers
            )
            response.raise_for_status()
            return response

    async def create_chat_completion(
        self,
        request: ChatCompletionRequest,
        user_id: int,
        ip_address: str
    ) -> AsyncGenerator[Dict[str, Any], None]:
        request_id = str(uuid.uuid4())
        chat_session = await self._create_chat_session(request, user_id, request_id, ip_address)
        
        try:
            if request.stream:
                chunks = []
                full_response = ""
                async with httpx.AsyncClient(timeout=self.timeout) as client:
                    headers = {
                        "Authorization": f"Bearer {self.openai_api_key}",
                        "Content-Type": "application/json",
                        "OpenAI-Organization": settings.OPENAI_ORG_ID if settings.OPENAI_ORG_ID else None
                    }
                    headers = {k: v for k, v in headers.items() if v is not None}
                    
                    async with client.stream(
                        "POST",
                        f"{self.openai_api_base}/v1/chat/completions",
                        json=json.loads(request.model_dump_json()),
                        headers=headers
                    ) as response:
                        response.raise_for_status()
                        async for line in response.aiter_lines():
                            if line.startswith("data: "):
                                try:
                                    chunk = json.loads(line.removeprefix("data: "))
                                    if chunk == "[DONE]":
                                        continue
                                    chunks.append(chunk)
                                    if "choices" in chunk and chunk["choices"]:
                                        content = chunk["choices"][0].get("delta", {}).get("content", "")
                                        if content:
                                            full_response += content
                                    yield chunk
                                except json.JSONDecodeError:
                                    continue
                
                if chunks:
                    last_chunk = chunks[-1]
                    chat_session.raw_response_chunks = chunks
                    await self._update_chat_session(
                        chat_session,
                        last_chunk,
                        full_response
                    )
            else:
                for attempt in range(self.max_retries):
                    try:
                        response = await self._make_openai_request(request)
                        response_data = response.json()
                        
                        if "choices" in response_data and response_data["choices"]:
                            answer = response_data["choices"][0]["message"]["content"]
                            await self._update_chat_session(
                                chat_session,
                                response_data,
                                answer
                            )
                            yield response_data
                            break
                    except httpx.HTTPError as e:
                        if attempt == self.max_retries - 1:
                            await self._update_chat_session(
                                chat_session,
                                {},
                                str(e),
                                status=2
                            )
                            raise
                        time.sleep(2 ** attempt)  # Exponential backoff
                        
        except Exception as e:
            await self._update_chat_session(
                chat_session,
                {},
                str(e),
                status=2
            )
            raise

    async def get_chat_history(
        self,
        user_id: int,
        conversation_id: Optional[str] = None,
        limit: int = 10,
        offset: int = 0
    ) -> Dict[str, Any]:
        query = select(ChatSession).where(ChatSession.user_id == user_id)
        
        if conversation_id:
            query = query.where(ChatSession.conversation_id == conversation_id)
        
        # Get total count
        count_query = select(func.count()).select_from(query.subquery())
        total = await self.db.scalar(count_query)
        
        # Get paginated results
        query = query.order_by(desc(ChatSession.created_at)).offset(offset).limit(limit)
        result = await self.db.execute(query)
        items = result.scalars().all()
        
        return {
            "total": total,
            "items": items
        }

    async def get_chat_detail(self, chat_id: int, user_id: int) -> Optional[ChatSession]:
        query = select(ChatSession).where(
            and_(
                ChatSession.id == chat_id,
                ChatSession.user_id == user_id
            )
        )
        result = await self.db.execute(query)
        return result.scalar_one_or_none()
