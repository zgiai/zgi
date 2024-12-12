from typing import Optional
from fastapi_mail import FastMail, MessageSchema, ConnectionConfig
from pydantic import EmailStr
from app.core.config import settings

class EmailService:
    def __init__(self):
        self.config = ConnectionConfig(
            MAIL_USERNAME=settings.MAIL_USERNAME,
            MAIL_PASSWORD=settings.MAIL_PASSWORD,
            MAIL_FROM=settings.MAIL_FROM,
            MAIL_PORT=settings.MAIL_PORT,
            MAIL_SERVER=settings.MAIL_SERVER,
            MAIL_TLS=settings.MAIL_TLS,
            MAIL_SSL=settings.MAIL_SSL,
            USE_CREDENTIALS=settings.USE_CREDENTIALS,
            VALIDATE_CERTS=settings.VALIDATE_CERTS,
            TEMPLATE_FOLDER=settings.EMAIL_TEMPLATES_DIR
        )
        self.fm = FastMail(self.config)

    async def send_email(
        self,
        email_to: EmailStr,
        subject: str,
        body: str,
        template_name: Optional[str] = None
    ) -> None:
        """
        发送邮件
        """
        message = MessageSchema(
            subject=subject,
            recipients=[email_to],
            body=body,
            subtype="html"
        )
        
        await self.fm.send_message(message, template_name=template_name)

_email_service = None

def get_email_service() -> EmailService:
    """
    获取邮件服务单例
    """
    global _email_service
    if _email_service is None:
        _email_service = EmailService()
    return _email_service
