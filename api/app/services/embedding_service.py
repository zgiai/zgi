import numpy as np

async def embed_chunks(chunks: list[str]):
    # 这里应该使用实际的嵌入模型，这里用简单的长度作为示例
    return [np.array([len(chunk)]) for chunk in chunks]
