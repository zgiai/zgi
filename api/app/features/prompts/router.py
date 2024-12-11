from typing import List
from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session

from app.core.database import get_db
from app.core.security import get_current_user
from app.models import User
from app.features.prompts import schemas, service

router = APIRouter(prefix="/v1/console/prompts", tags=["prompts"])

@router.post("/templates", response_model=schemas.PromptTemplateResponse)
def create_template(
    data: schemas.PromptTemplateCreate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Create a new prompt template"""
    prompt_service = service.PromptService(db)
    return prompt_service.create_template(data, current_user.id)

@router.put("/templates/{template_id}", response_model=schemas.PromptTemplateResponse)
def update_template(
    template_id: int,
    data: schemas.PromptTemplateUpdate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Update a prompt template"""
    prompt_service = service.PromptService(db)
    return prompt_service.update_template(template_id, data)

@router.get("/templates/{template_id}", response_model=schemas.PromptTemplateResponse)
def get_template(
    template_id: int,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get a prompt template by ID"""
    prompt_service = service.PromptService(db)
    return prompt_service.get_template(template_id)

@router.get("/applications/{application_id}/templates", response_model=List[schemas.PromptTemplateResponse])
def list_templates(
    application_id: int,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """List all prompt templates for an application"""
    prompt_service = service.PromptService(db)
    return prompt_service.list_templates(application_id)

@router.post("/scenarios", response_model=schemas.PromptScenarioResponse)
def create_scenario(
    data: schemas.PromptScenarioCreate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Create a new prompt scenario"""
    prompt_service = service.PromptService(db)
    return prompt_service.create_scenario(data, current_user.id)

@router.put("/scenarios/{scenario_id}", response_model=schemas.PromptScenarioResponse)
def update_scenario(
    scenario_id: int,
    data: schemas.PromptScenarioUpdate,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Update a prompt scenario"""
    prompt_service = service.PromptService(db)
    return prompt_service.update_scenario(scenario_id, data)

@router.get("/scenarios/{scenario_id}", response_model=schemas.PromptScenarioResponse)
def get_scenario(
    scenario_id: int,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get a prompt scenario by ID"""
    prompt_service = service.PromptService(db)
    return prompt_service.get_scenario(scenario_id)

@router.get("/templates/{template_id}/scenarios", response_model=List[schemas.PromptScenarioResponse])
def list_scenarios(
    template_id: int,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """List all scenarios for a template"""
    prompt_service = service.PromptService(db)
    return prompt_service.list_scenarios(template_id)

@router.post("/preview", response_model=schemas.PromptPreviewResponse)
def preview_prompt(
    data: schemas.PromptPreviewRequest,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Preview a rendered prompt with optional scenario and variables"""
    prompt_service = service.PromptService(db)
    rendered_prompt = prompt_service.preview_prompt(data)
    return schemas.PromptPreviewResponse(rendered_prompt=rendered_prompt)

@router.get("/applications/{application_id}/templates/{template_name}/versions", response_model=List[schemas.PromptTemplateResponse])
def get_template_versions(
    application_id: int,
    template_name: str,
    db: Session = Depends(get_db),
    current_user: User = Depends(get_current_user)
):
    """Get all versions of a template by name"""
    prompt_service = service.PromptService(db)
    return prompt_service.get_template_versions(application_id, template_name)
