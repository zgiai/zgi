from abc import ABC, abstractmethod
from typing import Tuple


class StorageProvider(ABC):
    """Base class for storage providers"""

    @abstractmethod
    async def store_file(
            self,
            file: bytes,
            kb_id: int,
            user_id: int,
            filename: str
    ) -> str:
        """Store a file

        Args:
            file: File content in bytes
            kb_id: Knowledge base ID
            user_id: User ID
            filename: Original filename

        Returns:
            str: file_path
        """
        pass

    @abstractmethod
    async def delete_file(
            self,
            filepath: str
    ) -> None:
        """Delete a file

        Args:
            filepath: Path of the file to be deleted
        """
        pass

    @abstractmethod
    async def read_file(
            self,
            filepath: str
    ) -> bytes:
        """Read a file

        Args:
            filepath: Path of the file to be read

        Returns:
            bytes: File content
        """
        pass
