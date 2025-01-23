import os
import hashlib
from typing import Tuple

from ..base import StorageProvider


class LocalStorageProvider(StorageProvider):
    """Local storage provider"""

    def __init__(self, upload_dir: str):
        self.upload_dir = upload_dir

    async def store_file(
            self,
            file: bytes,
            kb_id: int,
            user_id: int,
            filename: str
    ) -> str:
        """Store a file locally"""
        # Create directory if not exists
        upload_dir = os.path.join(self.upload_dir, str(kb_id))
        os.makedirs(upload_dir, exist_ok=True)

        # Generate unique filename
        filepath = os.path.join(upload_dir, f"{user_id}_{filename}")

        # Write file
        with open(filepath, "wb") as f:
            f.write(file)

        return filepath

    async def delete_file(
            self,
            filepath: str
    ) -> None:
        """Delete a file locally"""
        if os.path.exists(filepath):
            os.remove(filepath)

    async def read_file(
            self,
            filepath: str
    ) -> bytes:
        """Read a file locally"""
        with open(filepath, "rb") as f:
            return f.read()
