import secrets

def generate_api_key() -> str:
    """Generate a secure API key"""
    return f"zgi_{secrets.token_urlsafe(32)}"
