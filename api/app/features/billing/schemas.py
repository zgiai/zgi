from datetime import datetime
from typing import Optional, List, Dict
from pydantic import BaseModel, Field, EmailStr

class ModelPricingBase(BaseModel):
    model_name: str
    price_per_1k_tokens: float

class ModelPricingCreate(ModelPricingBase):
    pass

class ModelPricingResponse(ModelPricingBase):
    id: int
    is_active: bool
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

class BillingAlertBase(BaseModel):
    application_id: int
    alert_type: str = Field(..., regex="^(monthly_limit|daily_limit|trend_alert)$")
    threshold_amount: float = Field(..., gt=0)
    notification_email: EmailStr

class BillingAlertCreate(BillingAlertBase):
    pass

class BillingAlertUpdate(BaseModel):
    alert_type: Optional[str] = Field(None, regex="^(monthly_limit|daily_limit|trend_alert)$")
    threshold_amount: Optional[float] = Field(None, gt=0)
    notification_email: Optional[EmailStr]
    is_active: Optional[bool]

class BillingAlertResponse(BillingAlertBase):
    id: int
    is_active: bool
    last_triggered_at: Optional[datetime]
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

class BillingRecordBase(BaseModel):
    application_id: int
    date: datetime
    model_costs: Dict[str, float]
    total_cost: float

class BillingRecordCreate(BillingRecordBase):
    pass

class BillingRecordResponse(BillingRecordBase):
    id: int
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True

class BillingTrendResponse(BaseModel):
    dates: List[datetime]
    costs: List[float]
    model_breakdown: Dict[str, List[float]]

class BillingReportRequest(BaseModel):
    start_date: datetime
    end_date: datetime
    format: str = Field(..., regex="^(csv|pdf)$")
