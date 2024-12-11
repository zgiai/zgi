import smtplib
import logging
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from typing import List, Optional
from pydantic import EmailStr
from jinja2 import Template

from app.core.config import settings

logger = logging.getLogger(__name__)

class EmailNotifier:
    def __init__(self):
        self.smtp_server = settings.SMTP_SERVER
        self.smtp_port = settings.SMTP_PORT
        self.smtp_username = settings.SMTP_USERNAME
        self.smtp_password = settings.SMTP_PASSWORD
        self.sender_email = settings.SENDER_EMAIL

    def send_email(
        self,
        to_email: EmailStr,
        subject: str,
        html_content: str,
        cc_emails: Optional[List[EmailStr]] = None
    ):
        """Send email using SMTP"""
        try:
            msg = MIMEMultipart('alternative')
            msg['Subject'] = subject
            msg['From'] = self.sender_email
            msg['To'] = to_email
            if cc_emails:
                msg['Cc'] = ', '.join(cc_emails)

            html_part = MIMEText(html_content, 'html')
            msg.attach(html_part)

            with smtplib.SMTP(self.smtp_server, self.smtp_port) as server:
                server.starttls()
                server.login(self.smtp_username, self.smtp_password)
                recipients = [to_email]
                if cc_emails:
                    recipients.extend(cc_emails)
                server.sendmail(self.sender_email, recipients, msg.as_string())
                logger.info(f"Email sent successfully to {to_email}")
        except Exception as e:
            logger.error(f"Failed to send email: {str(e)}")
            raise

class BillingNotifier:
    def __init__(self):
        self.email_notifier = EmailNotifier()
        self._load_templates()

    def _load_templates(self):
        """Load email templates"""
        self.daily_limit_template = Template("""
            <h2>Daily Billing Alert</h2>
            <p>The daily cost for application {{ application_name }} has exceeded the threshold.</p>
            <ul>
                <li>Current daily cost: ${{ current_cost }}</li>
                <li>Threshold: ${{ threshold }}</li>
                <li>Date: {{ date }}</li>
            </ul>
            <h3>Cost Breakdown by Model:</h3>
            <ul>
            {% for model, cost in model_costs.items() %}
                <li>{{ model }}: ${{ "%.2f"|format(cost) }}</li>
            {% endfor %}
            </ul>
        """)

        self.monthly_limit_template = Template("""
            <h2>Monthly Billing Alert</h2>
            <p>The monthly cost for application {{ application_name }} has exceeded the threshold.</p>
            <ul>
                <li>Current monthly cost: ${{ current_cost }}</li>
                <li>Threshold: ${{ threshold }}</li>
                <li>Month: {{ month }}</li>
            </ul>
            <h3>Top 5 Most Expensive Models:</h3>
            <ul>
            {% for model, cost in top_models %}
                <li>{{ model }}: ${{ "%.2f"|format(cost) }}</li>
            {% endfor %}
            </ul>
        """)

    def send_daily_limit_alert(
        self,
        to_email: EmailStr,
        application_name: str,
        current_cost: float,
        threshold: float,
        date: str,
        model_costs: dict
    ):
        """Send daily limit alert email"""
        html_content = self.daily_limit_template.render(
            application_name=application_name,
            current_cost=current_cost,
            threshold=threshold,
            date=date,
            model_costs=model_costs
        )
        
        subject = f"[Billing Alert] Daily cost exceeded for {application_name}"
        self.email_notifier.send_email(to_email, subject, html_content)

    def send_monthly_limit_alert(
        self,
        to_email: EmailStr,
        application_name: str,
        current_cost: float,
        threshold: float,
        month: str,
        top_models: List[tuple]
    ):
        """Send monthly limit alert email"""
        html_content = self.monthly_limit_template.render(
            application_name=application_name,
            current_cost=current_cost,
            threshold=threshold,
            month=month,
            top_models=top_models
        )
        
        subject = f"[Billing Alert] Monthly cost exceeded for {application_name}"
        self.email_notifier.send_email(to_email, subject, html_content)
