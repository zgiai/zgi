from datetime import datetime
from typing import List, Optional
from sqlalchemy.orm import Session
from sqlalchemy import desc
from jinja2 import Template

from app.models.prompts import PromptTemplate, PromptScenario
from app.features.prompts import schemas
from app.core.exceptions import NotFoundException, ValidationError

class PromptService:
    def __init__(self, db: Session):
        self.db = db

    def create_template(self, data: schemas.PromptTemplateCreate, user_id: int) -> PromptTemplate:
        template = PromptTemplate(
            **data.model_dump(),
            created_by=user_id
        )
        self.db.add(template)
        self.db.commit()
        self.db.refresh(template)
        return template

    def update_template(self, template_id: int, data: schemas.PromptTemplateUpdate) -> PromptTemplate:
        template = self.db.query(PromptTemplate).filter(PromptTemplate.id == template_id).first()
        if not template:
            raise NotFoundException(f"Template {template_id} not found")

        for key, value in data.model_dump(exclude_unset=True).items():
            setattr(template, key, value)

        self.db.commit()
        self.db.refresh(template)
        return template

    def get_template(self, template_id: int) -> PromptTemplate:
        template = self.db.query(PromptTemplate).filter(PromptTemplate.id == template_id).first()
        if not template:
            raise NotFoundException(f"Template {template_id} not found")
        return template

    def list_templates(self, application_id: int) -> List[PromptTemplate]:
        return self.db.query(PromptTemplate)\
            .filter(PromptTemplate.application_id == application_id)\
            .order_by(desc(PromptTemplate.created_at))\
            .all()

    def create_scenario(self, data: schemas.PromptScenarioCreate, user_id: int) -> PromptScenario:
        # Verify template exists
        template = self.get_template(data.template_id)
        
        scenario = PromptScenario(
            **data.model_dump(),
            created_by=user_id
        )
        self.db.add(scenario)
        self.db.commit()
        self.db.refresh(scenario)
        return scenario

    def update_scenario(self, scenario_id: int, data: schemas.PromptScenarioUpdate) -> PromptScenario:
        scenario = self.db.query(PromptScenario).filter(PromptScenario.id == scenario_id).first()
        if not scenario:
            raise NotFoundException(f"Scenario {scenario_id} not found")

        for key, value in data.model_dump(exclude_unset=True).items():
            setattr(scenario, key, value)

        self.db.commit()
        self.db.refresh(scenario)
        return scenario

    def get_scenario(self, scenario_id: int) -> PromptScenario:
        scenario = self.db.query(PromptScenario).filter(PromptScenario.id == scenario_id).first()
        if not scenario:
            raise NotFoundException(f"Scenario {scenario_id} not found")
        return scenario

    def list_scenarios(self, template_id: int) -> List[PromptScenario]:
        return self.db.query(PromptScenario)\
            .filter(PromptScenario.template_id == template_id)\
            .order_by(desc(PromptScenario.created_at))\
            .all()

    def preview_prompt(self, data: schemas.PromptPreviewRequest) -> str:
        template = self.get_template(data.template_id)
        
        # Get variables from scenario if provided
        variables = data.variables or {}
        if data.scenario_id:
            scenario = self.get_scenario(data.scenario_id)
            try:
                scenario_vars = eval(scenario.content)  # Safely evaluate scenario content as Python dict
                variables.update(scenario_vars)
            except Exception as e:
                raise ValidationError(f"Invalid scenario content format: {str(e)}")

        # Render template with variables
        try:
            jinja_template = Template(template.content)
            rendered_prompt = jinja_template.render(**variables)
            return rendered_prompt
        except Exception as e:
            raise ValidationError(f"Error rendering template: {str(e)}")

    def get_template_versions(self, application_id: int, template_name: str) -> List[PromptTemplate]:
        return self.db.query(PromptTemplate)\
            .filter(
                PromptTemplate.application_id == application_id,
                PromptTemplate.name == template_name
            )\
            .order_by(desc(PromptTemplate.created_at))\
            .all()
