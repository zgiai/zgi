from typing import List, Dict, Any
import numpy as np
from sklearn.metrics.pairwise import cosine_similarity

class VectorStore:
    def __init__(self):
        self.vectors = []
        self.metadata = []

    # ... existing code ...

    async def add_vectors(self, vectors, ids, metadata):
        # Implement the logic to add vectors to your storage system
        # This could involve calling an API, writing to a database, etc.
        # For example:
        for vector, id, meta in zip(vectors, ids, metadata):
            await self._store.add_item(id, vector, meta)  # This is just an example
        
        return True  # or some meaningful response

    async def search_vectors(self, query_vector: np.ndarray, top_k: int = 5) -> List[Dict[str, Any]]:
        """
        搜索与查询向量最相似的向量。

        :param query_vector: 查询向量
        :param top_k: 返回的最相似结果数量
        :return: 包含相似度得分和元数据的结果列表
        """
        if not self.vectors:
            return []

        # 计算余弦相似度
        similarities = cosine_similarity(query_vector, self.vectors)[0]

        # 获取top_k个最相似的索引
        top_indices = np.argsort(similarities)[-top_k:][::-1]

        # 构建结果
        results = []
        for idx in top_indices:
            results.append({
                "score": float(similarities[idx]),
                "content": self.vectors[idx].tolist(),  # 将numpy数组转换为列表
                "metadata": self.metadata[idx]
            })

        return results
