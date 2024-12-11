import pytest
from datetime import datetime
from sqlalchemy.orm import Session

from app.features.prompts import service, schemas
from app.core.exceptions import NotFoundException, ValidationError
from app.models.prompts import PromptTemplate, PromptScenario

def test_create_template(test_db: Session, prompt_template_create_data, test_user):
    """Test creating a prompt template"""
    prompt_service = service.PromptService(test_db)
    template = prompt_service.create_template(prompt_template_create_data, test_user.id)
    
    assert template.name == prompt_template_create_data.name
    assert template.description == prompt_template_create_data.description
    assert template.content == prompt_template_create_data.content
    assert template.version == prompt_template_create_data.version
    assert template.is_active == True
    assert template.created_by == test_user.id

def test_update_template(test_db: Session, test_prompt_template):
    """Test updating a prompt template"""
    prompt_service = service.PromptService(test_db)
    
    update_data = schemas.PromptTemplateUpdate(
        name="Updated Template",
        description="Updated description",
        is_active=False
    )
    
    template = prompt_service.update_template(test_prompt_template.id, update_data)
    
    assert template.name == "Updated Template"
    assert template.description == "Updated description"
    assert template.is_active == False
    # Content and version should remain unchanged
    assert template.content == test_prompt_template.content
    assert template.version == test_prompt_template.version

def test_update_nonexistent_template(test_db: Session):
    """Test updating a non-existent template"""
    prompt_service = service.PromptService(test_db)
    
    update_data = schemas.PromptTemplateUpdate(name="Updated Template")
    
    with pytest.raises(NotFoundException):
        prompt_service.update_template(999, update_data)

def test_get_template(test_db: Session, test_prompt_template):
    """Test retrieving a prompt template"""
    prompt_service = service.PromptService(test_db)
    template = prompt_service.get_template(test_prompt_template.id)
    
    assert template.id == test_prompt_template.id
    assert template.name == test_prompt_template.name
    assert template.content == test_prompt_template.content

def test_list_templates(test_db: Session, test_prompt_template, test_application):
    """Test listing prompt templates"""
    prompt_service = service.PromptService(test_db)
    templates = prompt_service.list_templates(test_application.id)
    
    assert len(templates) == 1
    assert templates[0].id == test_prompt_template.id

def test_create_scenario(test_db: Session, prompt_scenario_create_data, test_user):
    """Test creating a prompt scenario"""
    prompt_service = service.PromptService(test_db)
    scenario = prompt_service.create_scenario(prompt_scenario_create_data, test_user.id)
    
    assert scenario.name == prompt_scenario_create_data.name
    assert scenario.description == prompt_scenario_create_data.description
    assert scenario.content == prompt_scenario_create_data.content
    assert scenario.created_by == test_user.id

def test_update_scenario(test_db: Session, test_prompt_scenario):
    """Test updating a prompt scenario"""
    prompt_service = service.PromptService(test_db)
    
    update_data = schemas.PromptScenarioUpdate(
        name="Updated Scenario",
        description="Updated description",
        content='{"name": "Jane", "age": "25"}'
    )
    
    scenario = prompt_service.update_scenario(test_prompt_scenario.id, update_data)
    
    assert scenario.name == "Updated Scenario"
    assert scenario.description == "Updated description"
    assert scenario.content == '{"name": "Jane", "age": "25"}'

def test_get_scenario(test_db: Session, test_prompt_scenario):
    """Test retrieving a prompt scenario"""
    prompt_service = service.PromptService(test_db)
    scenario = prompt_service.get_scenario(test_prompt_scenario.id)
    
    assert scenario.id == test_prompt_scenario.id
    assert scenario.name == test_prompt_scenario.name
    assert scenario.content == test_prompt_scenario.content

def test_list_scenarios(test_db: Session, test_prompt_template, test_prompt_scenario):
    """Test listing prompt scenarios"""
    prompt_service = service.PromptService(test_db)
    scenarios = prompt_service.list_scenarios(test_prompt_template.id)
    
    assert len(scenarios) == 1
    assert scenarios[0].id == test_prompt_scenario.id

def test_preview_prompt(test_db: Session, test_prompt_template, test_prompt_scenario):
    """Test prompt preview with scenario"""
    prompt_service = service.PromptService(test_db)
    
    preview_data = schemas.PromptPreviewRequest(
        template_id=test_prompt_template.id,
        scenario_id=test_prompt_scenario.id
    )
    
    rendered_prompt = prompt_service.preview_prompt(preview_data)
    assert rendered_prompt == "Hello, my name is John and I am 30 years old."

def test_preview_prompt_with_variables(test_db: Session, test_prompt_template):
    """Test prompt preview with custom variables"""
    prompt_service = service.PromptService(test_db)
    
    preview_data = schemas.PromptPreviewRequest(
        template_id=test_prompt_template.id,
        variables={"name": "Alice", "age": "20"}
    )
    
    rendered_prompt = prompt_service.preview_prompt(preview_data)
    assert rendered_prompt == "Hello, my name is Alice and I am 20 years old."

def test_preview_prompt_invalid_template(test_db: Session):
    """Test preview with non-existent template"""
    prompt_service = service.PromptService(test_db)
    
    preview_data = schemas.PromptPreviewRequest(
        template_id=999,
        variables={"name": "Test"}
    )
    
    with pytest.raises(NotFoundException):
        prompt_service.preview_prompt(preview_data)

def test_preview_prompt_invalid_variables(test_db: Session, test_prompt_template):
    """Test preview with missing required variables"""
    prompt_service = service.PromptService(test_db)
    
    preview_data = schemas.PromptPreviewRequest(
        template_id=test_prompt_template.id,
        variables={"name": "Test"}  # Missing 'age' variable
    )
    
    with pytest.raises(ValidationError):
        prompt_service.preview_prompt(preview_data)
