import csv
import io
from datetime import datetime, timedelta
from typing import Optional, List, Dict, Tuple
from sqlalchemy import func
from sqlalchemy.orm import Session
from fastapi import HTTPException

from app.models.billing import ModelPricing, BillingAlert, BillingRecord
from app.models.usage import ResourceUsage
from app.features.billing import schemas
from app.core.exceptions import NotFoundException
from app.core.notifications import BillingNotifier
from app.core.reports import BillingReportGenerator
from app.models.application import Application

class BillingService:
    def __init__(self, db: Session):
        self.db = db
        self.notifier = BillingNotifier()
        self.report_generator = BillingReportGenerator()

    def get_or_create_pricing(self, model_name: str) -> ModelPricing:
        """Get or create model pricing"""
        pricing = self.db.query(ModelPricing).filter(
            ModelPricing.model_name == model_name,
            ModelPricing.is_active == True
        ).first()
        
        if not pricing:
            # Default pricing if not set
            pricing = ModelPricing(
                model_name=model_name,
                price_per_1k_tokens=0.01  # Default price
            )
            self.db.add(pricing)
            self.db.commit()
            self.db.refresh(pricing)
        
        return pricing

    def calculate_daily_costs(self, application_id: int, date: datetime) -> Tuple[Dict[str, float], float]:
        """Calculate costs for a specific day"""
        # Get token usage for the day
        usage_records = self.db.query(ResourceUsage).filter(
            ResourceUsage.application_id == application_id,
            ResourceUsage.resource_type == "token",
            func.date(ResourceUsage.timestamp) == date.date()
        ).all()

        model_costs = {}
        total_cost = 0.0

        for usage in usage_records:
            if not usage.model:
                continue

            pricing = self.get_or_create_pricing(usage.model)
            cost = (usage.quantity / 1000) * pricing.price_per_1k_tokens
            
            if usage.model in model_costs:
                model_costs[usage.model] += cost
            else:
                model_costs[usage.model] = cost
            
            total_cost += cost

        return model_costs, total_cost

    def record_daily_billing(self, application_id: int, date: datetime) -> BillingRecord:
        """Record daily billing information"""
        model_costs, total_cost = self.calculate_daily_costs(application_id, date)

        record = BillingRecord(
            application_id=application_id,
            date=date,
            model_costs=model_costs,
            total_cost=total_cost
        )
        self.db.add(record)
        self.db.commit()
        self.db.refresh(record)

        # Check billing alerts
        self._check_billing_alerts(application_id, total_cost)

        return record

    def get_billing_trend(
        self,
        application_id: int,
        start_date: datetime,
        end_date: datetime
    ) -> schemas.BillingTrendResponse:
        """Get billing trend data"""
        records = self.db.query(BillingRecord).filter(
            BillingRecord.application_id == application_id,
            BillingRecord.date >= start_date,
            BillingRecord.date <= end_date
        ).order_by(BillingRecord.date).all()

        dates = []
        costs = []
        model_costs = {}

        for record in records:
            dates.append(record.date)
            costs.append(record.total_cost)
            
            for model, cost in record.model_costs.items():
                if model not in model_costs:
                    model_costs[model] = [0] * len(dates)
                model_costs[model][-1] = cost

        return schemas.BillingTrendResponse(
            dates=dates,
            costs=costs,
            model_breakdown=model_costs
        )

    def create_billing_alert(self, data: schemas.BillingAlertCreate) -> BillingAlert:
        """Create a new billing alert"""
        alert = BillingAlert(**data.model_dump())
        self.db.add(alert)
        self.db.commit()
        self.db.refresh(alert)
        return alert

    def update_billing_alert(
        self,
        alert_id: int,
        data: schemas.BillingAlertUpdate
    ) -> BillingAlert:
        """Update billing alert settings"""
        alert = self.db.query(BillingAlert).filter(BillingAlert.id == alert_id).first()
        if not alert:
            raise NotFoundException(f"Alert {alert_id} not found")

        for key, value in data.model_dump(exclude_unset=True).items():
            setattr(alert, key, value)

        self.db.commit()
        self.db.refresh(alert)
        return alert

    def _check_billing_alerts(self, application_id: int, daily_cost: float):
        """Check and trigger billing alerts"""
        alerts = self.db.query(BillingAlert).filter(
            BillingAlert.application_id == application_id,
            BillingAlert.is_active == True
        ).all()

        # Get application name
        application = self.db.query(Application).filter(Application.id == application_id).first()
        if not application:
            return

        for alert in alerts:
            if alert.alert_type == "daily_limit" and daily_cost >= alert.threshold_amount:
                # Get model costs for the day
                model_costs, _ = self.calculate_daily_costs(application_id, datetime.utcnow())
                
                # Send notification
                self.notifier.send_daily_limit_alert(
                    to_email=alert.notification_email,
                    application_name=application.name,
                    current_cost=daily_cost,
                    threshold=alert.threshold_amount,
                    date=datetime.utcnow().date().isoformat(),
                    model_costs=model_costs
                )
                
                alert.last_triggered_at = datetime.utcnow()
                self.db.commit()
            
            elif alert.alert_type == "monthly_limit":
                # Calculate monthly cost
                start_of_month = datetime.utcnow().replace(day=1, hour=0, minute=0, second=0, microsecond=0)
                monthly_records = self.db.query(BillingRecord).filter(
                    BillingRecord.application_id == application_id,
                    BillingRecord.date >= start_of_month
                ).all()
                
                monthly_cost = sum(record.total_cost for record in monthly_records)
                
                if monthly_cost >= alert.threshold_amount:
                    # Get top 5 most expensive models
                    model_costs = {}
                    for record in monthly_records:
                        for model, cost in record.model_costs.items():
                            model_costs[model] = model_costs.get(model, 0) + cost
                    
                    top_models = sorted(
                        model_costs.items(),
                        key=lambda x: x[1],
                        reverse=True
                    )[:5]
                    
                    # Send notification
                    self.notifier.send_monthly_limit_alert(
                        to_email=alert.notification_email,
                        application_name=application.name,
                        current_cost=monthly_cost,
                        threshold=alert.threshold_amount,
                        month=start_of_month.strftime('%B %Y'),
                        top_models=top_models
                    )
                    
                    alert.last_triggered_at = datetime.utcnow()
                    self.db.commit()

    def generate_billing_report(
        self,
        application_id: int,
        start_date: datetime,
        end_date: datetime,
        format: str
    ) -> bytes:
        """Generate billing report in specified format"""
        # Get application
        application = self.db.query(Application).filter(Application.id == application_id).first()
        if not application:
            raise NotFoundException(f"Application {application_id} not found")

        # Get billing records
        records = self.db.query(BillingRecord).filter(
            BillingRecord.application_id == application_id,
            BillingRecord.date >= start_date,
            BillingRecord.date <= end_date
        ).order_by(BillingRecord.date).all()

        if format == "csv":
            output = io.StringIO()
            writer = csv.writer(output)
            writer.writerow(["Date", "Model", "Cost (USD)"])
            
            for record in records:
                for model, cost in record.model_costs.items():
                    writer.writerow([record.date.date(), model, cost])
                writer.writerow([record.date.date(), "Total", record.total_cost])
                writer.writerow([])  # Empty row between days
            
            return output.getvalue().encode('utf-8')
        else:
            # Generate PDF report
            daily_costs = [{
                'date': record.date,
                'total_cost': record.total_cost,
                'model_costs': record.model_costs
            } for record in records]

            trend_data = self.get_billing_trend(
                application_id,
                start_date,
                end_date
            ).model_dump()

            return self.report_generator.generate_pdf(
                application_name=application.name,
                start_date=start_date,
                end_date=end_date,
                daily_costs=daily_costs,
                trend_data=trend_data
            )
