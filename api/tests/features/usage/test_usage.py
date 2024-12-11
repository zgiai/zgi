import pytest
from datetime import datetime, timedelta
from sqlalchemy.orm import Session
from fastapi.testclient import TestClient

from app.features.usage import service, schemas
from app.models.usage import ResourceUsage

def test_record_usage(test_db: Session, test_application):
    """测试记录资源使用情况"""
    usage_service = service.UsageService(test_db)
    
    # 创建测试数据
    usage_data = schemas.ResourceUsageCreate(
        application_id=test_application.id,
        resource_type="token",
        quantity=100.0,
        model="gpt-4",
        timestamp=datetime.utcnow()
    )
    
    # 记录使用情况
    usage = usage_service.record_usage(usage_data)
    
    # 验证记录
    assert usage.application_id == test_application.id
    assert usage.resource_type == "token"
    assert usage.quantity == 100.0
    assert usage.model == "gpt-4"

def test_get_usage_stats(test_db: Session, test_application):
    """测试获取使用统计信息"""
    usage_service = service.UsageService(test_db)
    
    # 创建测试数据
    now = datetime.utcnow()
    test_data = [
        # Token使用记录
        ResourceUsage(
            application_id=test_application.id,
            resource_type="token",
            quantity=100.0,
            model="gpt-4",
            timestamp=now - timedelta(days=1)
        ),
        ResourceUsage(
            application_id=test_application.id,
            resource_type="token",
            quantity=200.0,
            model="gpt-3.5",
            timestamp=now
        ),
        # API调用记录
        ResourceUsage(
            application_id=test_application.id,
            resource_type="api_call",
            quantity=1.0,
            endpoint="/v1/chat/completions",
            timestamp=now
        ),
        ResourceUsage(
            application_id=test_application.id,
            resource_type="api_call",
            quantity=1.0,
            endpoint="/v1/embeddings",
            timestamp=now
        )
    ]
    
    for usage in test_data:
        test_db.add(usage)
    test_db.commit()
    
    # 获取统计信息
    stats = usage_service.get_usage_stats(
        test_application.id,
        start_time=now - timedelta(days=2),
        end_time=now + timedelta(days=1)
    )
    
    # 验证统计结果
    assert stats.total_tokens == 300.0
    assert stats.total_api_calls == 2
    assert stats.token_usage_by_model == {
        "gpt-4": 100.0,
        "gpt-3.5": 200.0
    }
    assert stats.api_calls_by_endpoint == {
        "/v1/chat/completions": 1,
        "/v1/embeddings": 1
    }

def test_list_usage_with_filters(test_db: Session, test_application):
    """测试使用过滤器列出使用记录"""
    usage_service = service.UsageService(test_db)
    
    # 创建测试数据
    now = datetime.utcnow()
    test_data = [
        ResourceUsage(
            application_id=test_application.id,
            resource_type="token",
            quantity=100.0,
            model="gpt-4",
            timestamp=now - timedelta(days=1)
        ),
        ResourceUsage(
            application_id=test_application.id,
            resource_type="api_call",
            quantity=1.0,
            endpoint="/v1/chat/completions",
            timestamp=now
        )
    ]
    
    for usage in test_data:
        test_db.add(usage)
    test_db.commit()
    
    # 测试时间范围过滤
    records = usage_service.list_usage(
        test_application.id,
        start_time=now - timedelta(hours=1),
        end_time=now + timedelta(hours=1)
    )
    assert len(records) == 1
    assert records[0].resource_type == "api_call"
    
    # 测试资源类型过滤
    records = usage_service.list_usage(
        test_application.id,
        resource_type="token"
    )
    assert len(records) == 1
    assert records[0].resource_type == "token"

def test_api_endpoints(test_client: TestClient, test_application, test_user_token):
    """测试API端点"""
    headers = {"Authorization": f"Bearer {test_user_token}"}
    
    # 测试记录使用情况
    usage_data = {
        "application_id": test_application.id,
        "resource_type": "token",
        "quantity": 100.0,
        "model": "gpt-4",
        "timestamp": datetime.utcnow().isoformat()
    }
    response = test_client.post("/v1/console/usage/record", json=usage_data, headers=headers)
    assert response.status_code == 200
    
    # 测试获取统计信息
    response = test_client.get(
        f"/v1/console/usage/applications/{test_application.id}/stats",
        headers=headers
    )
    assert response.status_code == 200
    stats = response.json()
    assert stats["total_tokens"] == 100.0
    
    # 测试列出使用记录
    response = test_client.get(
        f"/v1/console/usage/applications/{test_application.id}/records",
        headers=headers
    )
    assert response.status_code == 200
    records = response.json()
    assert len(records) == 1
