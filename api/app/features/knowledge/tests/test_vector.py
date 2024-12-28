import pytest
from typing import List, Dict, Any
from app.features.knowledge.vector.base import VectorDBProvider
from app.features.knowledge.vector.factory import VectorDBFactory

class MockVectorDBProvider(VectorDBProvider):
    """Mock vector database provider for testing"""
    
    def __init__(self, **kwargs):
        self.collections = {}
        self.vectors = {}
    
    async def create_collection(
        self,
        name: str,
        dimension: int,
        **kwargs
    ) -> bool:
        if name in self.collections:
            return False
        self.collections[name] = {"dimension": dimension}
        self.vectors[name] = []
        return True
    
    async def delete_collection(self, name: str) -> bool:
        if name not in self.collections:
            return False
        del self.collections[name]
        del self.vectors[name]
        return True
    
    async def insert_vectors(
        self,
        collection_name: str,
        vectors: List[List[float]],
        metadata: List[Dict[str, Any]]
    ) -> bool:
        if collection_name not in self.collections:
            return False
        for vec, meta in zip(vectors, metadata):
            self.vectors[collection_name].append({
                "vector": vec,
                "metadata": meta
            })
        return True
    
    async def search(
        self,
        collection_name: str,
        query_vector: List[float],
        top_k: int = 5,
        score_threshold: float = None,
        metadata_filter: Dict[str, Any] = None,
        **kwargs
    ) -> List[Dict[str, Any]]:
        if collection_name not in self.collections:
            return []
        
        # Simple mock search - return all vectors with mock scores
        results = []
        for i, item in enumerate(self.vectors[collection_name]):
            if metadata_filter:
                match = all(
                    item["metadata"].get(k) == v
                    for k, v in metadata_filter.items()
                )
                if not match:
                    continue
            
            score = 1.0 - (i * 0.1)  # Mock scores
            if score_threshold and score < score_threshold:
                continue
            
            results.append({
                "vector": item["vector"],
                "metadata": item["metadata"],
                "score": score
            })
            
            if len(results) >= top_k:
                break
        
        return results
    
    async def get_collection_info(self, name: str) -> Dict[str, Any]:
        if name not in self.collections:
            return {}
        return {
            "name": name,
            "dimension": self.collections[name]["dimension"],
            "vector_count": len(self.vectors[name])
        }

def test_vector_db_factory_registration():
    # Register mock provider
    VectorDBFactory.register("mock", MockVectorDBProvider)
    
    # Create instance
    provider = VectorDBFactory.create("mock")
    
    assert isinstance(provider, MockVectorDBProvider)

@pytest.mark.asyncio
async def test_mock_vector_db_operations():
    provider = MockVectorDBProvider()
    
    # Test collection creation
    success = await provider.create_collection("test", 3)
    assert success is True
    
    # Test duplicate collection
    success = await provider.create_collection("test", 3)
    assert success is False
    
    # Test vector insertion
    vectors = [[1.0, 2.0, 3.0], [4.0, 5.0, 6.0]]
    metadata = [{"id": 1}, {"id": 2}]
    success = await provider.insert_vectors("test", vectors, metadata)
    assert success is True
    
    # Test search
    results = await provider.search(
        "test",
        [1.0, 2.0, 3.0],
        top_k=2,
        score_threshold=0.5
    )
    assert len(results) == 2
    assert all("score" in r for r in results)
    assert all("metadata" in r for r in results)
    
    # Test metadata filter
    results = await provider.search(
        "test",
        [1.0, 2.0, 3.0],
        metadata_filter={"id": 1}
    )
    assert len(results) == 1
    assert results[0]["metadata"]["id"] == 1
    
    # Test collection info
    info = await provider.get_collection_info("test")
    assert info["name"] == "test"
    assert info["dimension"] == 3
    assert info["vector_count"] == 2
    
    # Test collection deletion
    success = await provider.delete_collection("test")
    assert success is True
    
    # Test operations on non-existent collection
    success = await provider.delete_collection("test")
    assert success is False
    
    results = await provider.search("test", [1.0, 2.0, 3.0])
    assert len(results) == 0
    
    info = await provider.get_collection_info("test")
    assert info == {}
