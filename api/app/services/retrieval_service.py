import weaviate
import weaviate.classes as wvc
# from app.core.config import settings
from app.utils.weaviate_manager import WeaviateManager

class RetrievalService:
    def __init__(self):
        self.collection_name = "Documents"
        self.weaviate_manager = WeaviateManager()
        self.weaviate_manager.connect()
        self.client = self.weaviate_manager.client

        # 属性定义保持不变
        properties = [
            {
                "name": "content",
                "dataType": ["text"],
            },
            {
                "name": "metadata",
                "dataType": ["object"],
                "nestedProperties": [
                    {
                        "name": "source",
                        "dataType": ["text"],
                    },
                    {
                        "name": "timestamp",
                        "dataType": ["date"],
                    },
                    {
                        "name": "author",
                        "dataType": ["text"],
                    }
                ]
            }
        ]

        # 如果集合已存在，删除它
        if self.client.collections.exists(self.collection_name):
            self.client.collections.delete(self.collection_name)
            print(f"Deleted existing collection: {self.collection_name}")

        # 确保集合存在（这将重新创建集）
        self.weaviate_manager.ensure_schema(self.collection_name, properties)
    async def search_vectors(self, query, limit=5):
        collection = self.client.collections.get(self.collection_name)
        # Log the query
        print(f"\033[94mExecuting search query: {query}\033[0m")
        response = collection.query.near_text(
            query=query,
            limit=limit,
            return_metadata=wvc.query.MetadataQuery(distance=True)
        )
        print(response, "response") 
        results = []
        for obj in response.objects:
            result = {
                "content": obj.properties["content"],
                "metadata": obj.properties["metadata"],
                "distance": obj.metadata.distance
            }
            results.append(result)
        
        # Log the search results
        print("\033[92mSearch results:\033[0m")
        for i, result in enumerate(results, 1):
            print(f"\033[93mResult {i}:\033[0m")
            print(f"  Content: {result['content'][:100]}...")  # Print first 100 characters
            print(f"  Distance: {result['distance']}")
            print(f"  Metadata: {result['metadata']}")
            print()
        
        return results

    # 如果需要添加向量的方法，可以添加这个方法
    async def add_vectors(self, texts, metadata):
        # 实现添加向量的逻辑
        pass

