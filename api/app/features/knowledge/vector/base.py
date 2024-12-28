from abc import ABC, abstractmethod
from typing import List, Dict, Any, Optional

class VectorDBProvider(ABC):
    """Vector database provider interface"""
    
    @abstractmethod
    async def create_collection(
        self,
        name: str,
        dimension: int,
        **kwargs
    ) -> bool:
        """Create a new collection
        
        Args:
            name: Collection name
            dimension: Vector dimension
            **kwargs: Additional provider-specific parameters
            
        Returns:
            bool: True if successful, False otherwise
        """
        pass
    
    @abstractmethod
    async def delete_collection(self, name: str) -> bool:
        """Delete a collection
        
        Args:
            name: Collection name
            
        Returns:
            bool: True if successful, False otherwise
        """
        pass
    
    @abstractmethod
    async def insert_vectors(
        self,
        collection_name: str,
        vectors: List[List[float]],
        metadata: List[Dict[str, Any]]
    ) -> bool:
        """Insert vectors into collection
        
        Args:
            collection_name: Collection name
            vectors: List of vectors to insert
            metadata: List of metadata for each vector
            
        Returns:
            bool: True if successful, False otherwise
        """
        pass
    
    @abstractmethod
    async def search(
        self,
        collection_name: str,
        query_vector: List[float],
        top_k: int = 5,
        score_threshold: Optional[float] = None,
        metadata_filter: Optional[Dict[str, Any]] = None,
        **kwargs
    ) -> List[Dict[str, Any]]:
        """Search similar vectors
        
        Args:
            collection_name: Collection name
            query_vector: Query vector
            top_k: Number of results to return
            score_threshold: Minimum similarity score threshold
            metadata_filter: Filter results by metadata
            **kwargs: Additional provider-specific parameters
            
        Returns:
            List[Dict[str, Any]]: List of search results with metadata
        """
        pass
    
    @abstractmethod
    async def get_collection_info(self, name: str) -> Dict[str, Any]:
        """Get collection information
        
        Args:
            name: Collection name
            
        Returns:
            Dict[str, Any]: Collection information
        """
        pass
