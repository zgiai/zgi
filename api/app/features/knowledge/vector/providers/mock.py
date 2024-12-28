from typing import List, Dict, Any, Optional
from ..base import VectorDBProvider

class MockProvider(VectorDBProvider):
    """Mock vector database provider for testing"""
    
    def __init__(self, **kwargs):
        """Initialize mock provider"""
        super().__init__()
        self.collections = {}  # Simple in-memory storage for collections
        
    async def create_collection(self, name: str, dimension: int, **kwargs) -> bool:
        """Create a new collection"""
        if name not in self.collections:
            self.collections[name] = {
                "dimension": dimension,
                "vectors": {},
                "metadata": {}
            }
            return True
        return False
        
    async def delete_collection(self, name: str) -> bool:
        """Delete a collection"""
        if name in self.collections:
            del self.collections[name]
            return True
        return False
        
    async def insert_vectors(
        self,
        collection_name: str,
        vectors: List[List[float]],
        metadata: List[Dict[str, Any]]
    ) -> bool:
        """Insert vectors into collection"""
        if collection_name not in self.collections:
            return False
            
        collection = self.collections[collection_name]
        for i, (vector, meta) in enumerate(zip(vectors, metadata)):
            vector_id = str(len(collection["vectors"]))
            collection["vectors"][vector_id] = vector
            collection["metadata"][vector_id] = meta
        return True
        
    async def search(
        self,
        collection_name: str,
        query_vector: List[float],
        top_k: int = 5,
        score_threshold: Optional[float] = None,
        metadata_filter: Optional[Dict[str, Any]] = None,
        **kwargs
    ) -> List[Dict[str, Any]]:
        """Search similar vectors"""
        if collection_name not in self.collections:
            return []
            
        collection = self.collections[collection_name]
        # For mock, just return the first top_k items
        results = []
        for vector_id in list(collection["vectors"].keys())[:top_k]:
            score = 0.99  # Mock similarity score
            if score_threshold is not None and score < score_threshold:
                continue
                
            metadata = collection["metadata"][vector_id]
            if metadata_filter is not None:
                # Simple metadata filtering
                match = all(
                    metadata.get(k) == v 
                    for k, v in metadata_filter.items()
                )
                if not match:
                    continue
                    
            results.append({
                "id": vector_id,
                "score": score,
                "metadata": metadata
            })
        return results
        
    async def delete_vectors(
        self,
        collection_name: str,
        vector_ids: List[str]
    ) -> bool:
        """Delete vectors from collection"""
        if collection_name not in self.collections:
            return False
            
        collection = self.collections[collection_name]
        for vector_id in vector_ids:
            if vector_id in collection["vectors"]:
                del collection["vectors"][vector_id]
                del collection["metadata"][vector_id]
        return True
        
    async def get_collection_info(self, name: str) -> Optional[Dict[str, Any]]:
        """Get collection information"""
        if name not in self.collections:
            return None
            
        collection = self.collections[name]
        return {
            "name": name,
            "dimension": collection["dimension"],
            "vector_count": len(collection["vectors"])
        }
