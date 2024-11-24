import logging
import traceback
from app.models.document import Document
from app.utils.vector_store import VectorStore
from openai import OpenAI, OpenAIError, APIConnectionError, AsyncOpenAI
from app.core.config import settings

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class RAGService:
    def __init__(self):
        self.vector_store = VectorStore()
        logger.info(f"Initializing OpenAI client with base_url: {settings.OPENAI_API_BASE}")
        logger.info(f"Initializing OpenAI client with OPENAI_API_KEY: {settings.OPENAI_API_KEY}")
        logger.info(f"OPENAI_API_BASE: {settings.OPENAI_API_BASE}")
        logger.info(f"OPENAI_API_KEY: {settings.OPENAI_API_KEY[:5]}...{settings.OPENAI_API_KEY[-5:]}")
        self.client = AsyncOpenAI(
            api_key=settings.OPENAI_API_KEY,
            base_url=settings.OPENAI_API_BASE
        )

    def process_document(self, document: Document):
        chunks = self.chunk_document(document)
        vectors = self.vectorize_chunks(chunks)
        self.vector_store.store_vectors(document.filename, chunks, vectors)

    def chunk_document(self, document: Document):
        # 简单的分块策略，每100个单词一个块
        words = document.content.split()
        return [' '.join(words[i:i+100]) for i in range(0, len(words), 100)]

    async def vectorize_chunks(self, chunks):
        return await self.vector_store.get_embedding(chunks[0])

    async def generate_response(self, prompt: str, stream: bool = False) -> str:
        try:
            logger.info(f"Attempting to connect to OpenAI API at {self.client.base_url}")
            response = await self.client.chat.completions.create(
                model=settings.OPENAI_MODEL,
                messages=[
                    {"role": "system", "content": "You are a helpful assistant."},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.7,
                max_tokens=1000,
                n=1,
                stop=None,
            )
            logger.info(f"Received response from OpenAI API: {response}")
            return response.choices[0].message.content.strip()
        except APIConnectionError as e:
            logger.error(f"APIConnectionError: {str(e)}")
            logger.error(f"Request details: {e.request.method} {e.request.url}")
            logger.error(f"Request headers: {e.request.headers}")
            raise
        except OpenAIError as e:
            logger.error(f"OpenAIError: {str(e)}")
            raise
        except Exception as e:
            logger.error(f"Unexpected error: {str(e)}")
            raise

    def search_vectors(self, query_vector):
        return self.vector_store.search_vectors(query_vector)

    async def close(self):
        # 关闭与 OpenAI 客户端的连接（如果需要）
        if hasattr(self.client, 'close'):
            await self.client.close()  # 确保这是一个异步调用
            self.vector_store.close()
        logger.info("Connections closed.")

rag_service = RAGService()
