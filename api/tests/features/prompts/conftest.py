import pytest
from datetime import datetime
from sqlalchemy.orm import Session

from app.models.prompts import PromptTemplate, PromptScenario
from app.features.prompts import schemas

@pytest.fixture
def test_prompt_template(test_db: Session, test_application):
    """Create a test prompt template"""
    template = PromptTemplate(
        name="Test Template",
        description="Test template for unit tests",
        content="Hello, my name is {{name}} and I am {{age}} years old.",
        version="1.0",
        is_active=True,
        application_id=test_application.id,
        created_by=test_application.created_by
    )
    test_db.add(template)
    test_db.commit()
    test_db.refresh(template)
    return template

@pytest.fixture
def test_prompt_scenario(test_db: Session, test_prompt_template):
    """Create a test prompt scenario"""
    scenario = PromptScenario(
        name="Test Scenario",
        description="Test scenario for unit tests",
        content='{"name": "John", "age": "30"}',
        template_id=test_prompt_template.id,
        created_by=test_prompt_template.created_by
    )
    test_db.add(scenario)
    test_db.commit()
    test_db.refresh(scenario)
    return scenario

@pytest.fixture
def prompt_template_create_data(test_application):
    """Sample data for creating a prompt template"""
    return schemas.PromptTemplateCreate(
        name="New Template",
        description="A new test template",
        content="This is a {{variable}} template",
        version="1.0",
        application_id=test_application.id
    )

@pytest.fixture
def prompt_scenario_create_data(test_prompt_template):
    """Sample data for creating a prompt scenario"""
    return schemas.PromptScenarioCreate(
        name="New Scenario",
        description="A new test scenario",
        content='{"variable": "test"}',
        template_id=test_prompt_template.id
    )
