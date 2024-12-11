import pytest
from fastapi.testclient import TestClient
from sqlalchemy.orm import Session

def test_create_template(test_client: TestClient, test_application, test_user_headers):
    """Test creating a prompt template via API"""
    response = test_client.post(
        "/api/v1/prompts/templates",
        headers=test_user_headers,
        json={
            "name": "API Test Template",
            "description": "Template created via API test",
            "content": "This is a {{test}} template",
            "version": "1.0",
            "application_id": test_application.id
        }
    )
    
    assert response.status_code == 201
    data = response.json()
    assert data["name"] == "API Test Template"
    assert data["content"] == "This is a {{test}} template"
    assert data["is_active"] == True

def test_update_template(test_client: TestClient, test_prompt_template, test_user_headers):
    """Test updating a prompt template via API"""
    response = test_client.patch(
        f"/api/v1/prompts/templates/{test_prompt_template.id}",
        headers=test_user_headers,
        json={
            "name": "Updated API Template",
            "description": "Updated via API test"
        }
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == "Updated API Template"
    assert data["description"] == "Updated via API test"

def test_get_template(test_client: TestClient, test_prompt_template, test_user_headers):
    """Test retrieving a prompt template via API"""
    response = test_client.get(
        f"/api/v1/prompts/templates/{test_prompt_template.id}",
        headers=test_user_headers
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["id"] == test_prompt_template.id
    assert data["name"] == test_prompt_template.name

def test_list_templates(test_client: TestClient, test_prompt_template, test_application, test_user_headers):
    """Test listing prompt templates via API"""
    response = test_client.get(
        f"/api/v1/prompts/templates?application_id={test_application.id}",
        headers=test_user_headers
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 1
    assert data[0]["id"] == test_prompt_template.id

def test_create_scenario(test_client: TestClient, test_prompt_template, test_user_headers):
    """Test creating a prompt scenario via API"""
    response = test_client.post(
        "/api/v1/prompts/scenarios",
        headers=test_user_headers,
        json={
            "name": "API Test Scenario",
            "description": "Scenario created via API test",
            "content": '{"test": "value"}',
            "template_id": test_prompt_template.id
        }
    )
    
    assert response.status_code == 201
    data = response.json()
    assert data["name"] == "API Test Scenario"
    assert data["content"] == '{"test": "value"}'

def test_update_scenario(test_client: TestClient, test_prompt_scenario, test_user_headers):
    """Test updating a prompt scenario via API"""
    response = test_client.patch(
        f"/api/v1/prompts/scenarios/{test_prompt_scenario.id}",
        headers=test_user_headers,
        json={
            "name": "Updated API Scenario",
            "description": "Updated via API test"
        }
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["name"] == "Updated API Scenario"
    assert data["description"] == "Updated via API test"

def test_get_scenario(test_client: TestClient, test_prompt_scenario, test_user_headers):
    """Test retrieving a prompt scenario via API"""
    response = test_client.get(
        f"/api/v1/prompts/scenarios/{test_prompt_scenario.id}",
        headers=test_user_headers
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["id"] == test_prompt_scenario.id
    assert data["name"] == test_prompt_scenario.name

def test_list_scenarios(test_client: TestClient, test_prompt_template, test_prompt_scenario, test_user_headers):
    """Test listing prompt scenarios via API"""
    response = test_client.get(
        f"/api/v1/prompts/scenarios?template_id={test_prompt_template.id}",
        headers=test_user_headers
    )
    
    assert response.status_code == 200
    data = response.json()
    assert len(data) == 1
    assert data[0]["id"] == test_prompt_scenario.id

def test_preview_prompt(test_client: TestClient, test_prompt_template, test_prompt_scenario, test_user_headers):
    """Test prompt preview via API"""
    response = test_client.post(
        "/api/v1/prompts/preview",
        headers=test_user_headers,
        json={
            "template_id": test_prompt_template.id,
            "scenario_id": test_prompt_scenario.id
        }
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["rendered_prompt"] == "Hello, my name is John and I am 30 years old."

def test_preview_prompt_with_variables(test_client: TestClient, test_prompt_template, test_user_headers):
    """Test prompt preview with custom variables via API"""
    response = test_client.post(
        "/api/v1/prompts/preview",
        headers=test_user_headers,
        json={
            "template_id": test_prompt_template.id,
            "variables": {
                "name": "Alice",
                "age": "20"
            }
        }
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["rendered_prompt"] == "Hello, my name is Alice and I am 20 years old."

def test_preview_prompt_invalid_template(test_client: TestClient, test_user_headers):
    """Test preview with non-existent template via API"""
    response = test_client.post(
        "/api/v1/prompts/preview",
        headers=test_user_headers,
        json={
            "template_id": 999,
            "variables": {"test": "value"}
        }
    )
    
    assert response.status_code == 404

def test_preview_prompt_invalid_variables(test_client: TestClient, test_prompt_template, test_user_headers):
    """Test preview with missing required variables via API"""
    response = test_client.post(
        "/api/v1/prompts/preview",
        headers=test_user_headers,
        json={
            "template_id": test_prompt_template.id,
            "variables": {
                "name": "Test"  # Missing 'age' variable
            }
        }
    )
    
    assert response.status_code == 400

def test_unauthorized_access(test_client: TestClient, test_prompt_template):
    """Test unauthorized access to prompt endpoints"""
    # Try to access template without auth headers
    response = test_client.get(f"/api/v1/prompts/templates/{test_prompt_template.id}")
    assert response.status_code == 401

def test_template_not_found(test_client: TestClient, test_user_headers):
    """Test accessing non-existent template"""
    response = test_client.get(
        "/api/v1/prompts/templates/999",
        headers=test_user_headers
    )
    assert response.status_code == 404
