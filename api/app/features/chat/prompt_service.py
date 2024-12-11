from typing import Optional, List, Dict, Any
from sqlalchemy.orm import Session
from sqlalchemy import or_
from fastapi import HTTPException

from app.models.prompt import Prompt
from app.features.chat.prompt_schemas import (
    PromptCreate,
    PromptUpdate,
    PromptListParams,
    PromptPreviewRequest
)
from app.features.chat.service import ChatService

class PromptService:
    def __init__(self, db: Session):
        self.db = db
        self.chat_service = ChatService(db)

    def create_prompt(self, user_id: int, prompt_data: PromptCreate) -> Prompt:
        """Create a new prompt"""
        db_prompt = Prompt(
            user_id=user_id,
            **prompt_data.model_dump()
        )
        self.db.add(db_prompt)
        self.db.commit()
        self.db.refresh(db_prompt)
        return db_prompt

    def get_prompt(self, prompt_id: int, user_id: int) -> Optional[Prompt]:
        """Get a prompt by ID"""
        return self.db.query(Prompt).filter(
            Prompt.id == prompt_id,
            or_(
                Prompt.user_id == user_id,
                Prompt.is_template == True
            )
        ).first()

    def update_prompt(self, prompt_id: int, user_id: int, prompt_data: PromptUpdate) -> Optional[Prompt]:
        """Update an existing prompt"""
        prompt = self.get_prompt(prompt_id, user_id)
        if not prompt:
            return None
        
        if prompt.is_template and prompt.user_id != user_id:
            raise HTTPException(status_code=403, detail="Cannot modify system templates")

        for field, value in prompt_data.model_dump(exclude_unset=True).items():
            setattr(prompt, field, value)

        self.db.commit()
        self.db.refresh(prompt)
        return prompt

    def delete_prompt(self, prompt_id: int, user_id: int) -> bool:
        """Delete a prompt"""
        prompt = self.get_prompt(prompt_id, user_id)
        if not prompt or (prompt.is_template and prompt.user_id != user_id):
            return False

        self.db.delete(prompt)
        self.db.commit()
        return True

    def list_prompts(self, user_id: int, params: PromptListParams) -> tuple[List[Prompt], int]:
        """List prompts with filtering and pagination"""
        query = self.db.query(Prompt).filter(
            or_(
                Prompt.user_id == user_id,
                Prompt.is_template == params.include_templates
            )
        )

        if params.scenario:
            query = query.filter(Prompt.scenario == params.scenario)

        if params.search:
            search = f"%{params.search}%"
            query = query.filter(
                or_(
                    Prompt.title.ilike(search),
                    Prompt.content.ilike(search),
                    Prompt.description.ilike(search)
                )
            )

        total = query.count()
        prompts = query.offset((params.page - 1) * params.page_size)\
                      .limit(params.page_size)\
                      .all()

        return prompts, total

    async def preview_prompt(self, user_id: int, preview_request: PromptPreviewRequest) -> Dict[str, Any]:
        """Generate a preview of the prompt using the chat model"""
        if preview_request.prompt_id:
            prompt = self.get_prompt(preview_request.prompt_id, user_id)
            if not prompt:
                raise HTTPException(status_code=404, detail="Prompt not found")
            content = prompt.content
        elif preview_request.content:
            content = preview_request.content
        else:
            raise HTTPException(status_code=400, detail="Either prompt_id or content must be provided")

        # Replace variables in the prompt
        for key, value in preview_request.variables.items():
            content = content.replace(f"{{{key}}}", value)

        # Use chat service to generate preview
        messages = [{"role": "system", "content": content}]
        response = await self.chat_service.generate_completion(messages)

        return {
            "preview": response["content"],
            "tokens_used": response.get("total_tokens", 0),
            "model_used": response.get("model", "unknown")
        }

    def increment_usage(self, prompt_id: int, user_id: int) -> None:
        """Increment the usage count of a prompt"""
        prompt = self.get_prompt(prompt_id, user_id)
        if prompt:
            prompt.usage_count += 1
            self.db.commit()
