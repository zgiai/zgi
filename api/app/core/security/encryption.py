from cryptography.fernet import Fernet
from base64 import b64encode, b64decode
import os
from typing import Optional

from app.core.config import settings

class Encryptor:
    """Handle encryption and decryption of sensitive data"""
    
    def __init__(self):
        # Use environment variable for encryption key or generate a new one
        self.key = settings.ENCRYPTION_KEY.encode() if settings.ENCRYPTION_KEY else Fernet.generate_key()
        self.cipher_suite = Fernet(self.key)

    def encrypt(self, data: str) -> str:
        """Encrypt a string"""
        if not data:
            return data
        encrypted_data = self.cipher_suite.encrypt(data.encode())
        return b64encode(encrypted_data).decode()

    def decrypt(self, encrypted_data: str) -> Optional[str]:
        """Decrypt an encrypted string"""
        if not encrypted_data:
            return None
        try:
            decrypted_data = self.cipher_suite.decrypt(b64decode(encrypted_data))
            return decrypted_data.decode()
        except Exception:
            return None

class SecureTokenGenerator:
    """Generate secure tokens and API keys"""
    
    @staticmethod
    def generate_api_key() -> str:
        """Generate a secure API key"""
        # Generate 32 bytes of random data and encode as base64
        random_bytes = os.urandom(32)
        # Format as a readable string with prefix
        return f"zgi_{''.join(f'{b:02x}' for b in random_bytes)}"

    @staticmethod
    def generate_token(length: int = 32) -> str:
        """Generate a secure random token"""
        return b64encode(os.urandom(length)).decode('utf-8')
