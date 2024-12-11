from datetime import datetime
from typing import Optional, List, Dict
from sqlalchemy import func
from sqlalchemy.orm import Session

from app.models import ResourceUsage
from app.features.usage import schemas

class UsageService:
    def __init__(self, db: Session):
        self.db = db

    def record_usage(self, data: schemas.ResourceUsageCreate) -> ResourceUsage:
        """Record a new resource usage entry"""
        usage = ResourceUsage(**data.model_dump())
        self.db.add(usage)
        self.db.commit()
        self.db.refresh(usage)
        return usage

    def get_usage_stats(
        self,
        application_id: int,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None
    ) -> schemas.ResourceUsageStats:
        """Get aggregated usage statistics for an application within a time range"""
        query = self.db.query(ResourceUsage).filter(
            ResourceUsage.application_id == application_id
        )

        if start_time:
            query = query.filter(ResourceUsage.timestamp >= start_time)
        if end_time:
            query = query.filter(ResourceUsage.timestamp <= end_time)

        # Calculate total tokens
        token_usage = query.filter(
            ResourceUsage.resource_type == 'token'
        ).with_entities(
            func.sum(ResourceUsage.quantity)
        ).scalar() or 0.0

        # Calculate total API calls
        api_calls = query.filter(
            ResourceUsage.resource_type == 'api_call'
        ).count()

        # Get token usage by model
        token_usage_by_model = {}
        model_stats = query.filter(
            ResourceUsage.resource_type == 'token'
        ).with_entities(
            ResourceUsage.model,
            func.sum(ResourceUsage.quantity)
        ).group_by(ResourceUsage.model).all()
        
        for model, usage in model_stats:
            if model:
                token_usage_by_model[model] = float(usage)

        # Get API calls by endpoint
        api_calls_by_endpoint = {}
        endpoint_stats = query.filter(
            ResourceUsage.resource_type == 'api_call'
        ).with_entities(
            ResourceUsage.endpoint,
            func.count()
        ).group_by(ResourceUsage.endpoint).all()
        
        for endpoint, count in endpoint_stats:
            if endpoint:
                api_calls_by_endpoint[endpoint] = int(count)

        return schemas.ResourceUsageStats(
            total_tokens=token_usage,
            total_api_calls=api_calls,
            token_usage_by_model=token_usage_by_model,
            api_calls_by_endpoint=api_calls_by_endpoint
        )

    def list_usage(
        self,
        application_id: int,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        resource_type: Optional[str] = None
    ) -> List[ResourceUsage]:
        """List all usage records for an application with optional filters"""
        query = self.db.query(ResourceUsage).filter(
            ResourceUsage.application_id == application_id
        )

        if start_time:
            query = query.filter(ResourceUsage.timestamp >= start_time)
        if end_time:
            query = query.filter(ResourceUsage.timestamp <= end_time)
        if resource_type:
            query = query.filter(ResourceUsage.resource_type == resource_type)

        return query.order_by(ResourceUsage.timestamp.desc()).all()
