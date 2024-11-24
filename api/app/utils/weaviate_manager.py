from weaviate.classes.init import Auth
from weaviate.collections.classes.config import DataType, Property, Configure
import os
from dotenv import load_dotenv
import logging
from typing import List

# Load environment variables from .env file
load_dotenv()

# 更详细的日志配置
logging.basicConfig(
    level=logging.INFO,  # 可以改为 logging.INFO, logging.WARNING 等
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.StreamHandler(),  # 输出到控制台
        logging.FileHandler('weaviate.log')  # 同时写入文件
    ]
)
logger = logging.getLogger(__name__)

class WeaviateManager:
    def __init__(self):
        self.url = os.getenv("WEAVIATE_URL")
        self.local_url = os.getenv("WEAVIATE_LOCAL_URL")
        self.api_key = os.getenv("WEAVIATE_API_KEY")
        self.local_api_key = os.getenv("WEAVIATE_LOCAL_API_KEY")
        self.openai_api_key = os.getenv("OPENAI_API_KEY")
        self.openai_proxy = os.getenv("OPENAI_API_BASE", "https://api.openai.com/v1")
        self.openai_base_url = 'https://api.agicto.cn'
        self.client = None

    def connect(self):
        if not all([self.url, self.api_key, self.openai_api_key]):
            raise ValueError("Missing required environment variables for Weaviate connection")

        additional_headers = {
            "X-OpenAI-Api-Key": self.openai_api_key,
            "X-OpenAI-BaseURL": self.openai_proxy
        }

        logger.debug(f"Connecting to Weaviate at {self.url}")
        logger.debug(f"Headers: {additional_headers}")

        try:
            if os.getenv("ENVIRONMENT") == "development":
                from weaviate import connect_to_local
                self.client = connect_to_local(
                    host=self.local_url,
                    skip_init_checks=True,
                    headers=additional_headers
                )
                logger.info("Successfully connected to local Weaviate")
                logger.debug(f"Client configuration: {self.client.get_meta()}")
            else:
                from weaviate import connect_to_weaviate_cloud
                self.client = connect_to_weaviate_cloud(
                    cluster_url=self.url,
                    auth_credentials=Auth.api_key(self.api_key),
                    headers=additional_headers,
                    skip_init_checks=True,
                )
                print("Successfully connected to Weaviate")
        except Exception as e:
            logger.error(f"Failed to connect to Weaviate: {str(e)}")
            raise

    def ensure_schema(self, class_name, properties):
        if self.client is None:
            self.connect()

        if not self.client.collections.exists(class_name):
            # 修改属性定义
            weaviate_properties = [self._create_property(prop) for prop in properties]

            # 添加向量化器配置
            vectorizer_config = Configure.Vectorizer.text2vec_openai()

            self.client.collections.create(
                name=class_name,
                properties=weaviate_properties,
                vectorizer_config=vectorizer_config
            )
            print(f"Created collection: {class_name}")
        else:
            print(f"Collection {class_name} already exists")

    def _create_property(self, prop):
        data_type = self._get_data_type(prop["dataType"][0])
        property_config = {
            "name": prop["name"],
            "data_type": data_type
        }
        if "nestedProperties" in prop:
            property_config["nested_properties"] = [
                self._create_property(nested_prop) for nested_prop in prop["nestedProperties"]
            ]
        return Property(**property_config)

    def _get_data_type(self, data_type_str):
        data_type_map = {
            "text": DataType.TEXT,
            "string": DataType.TEXT,
            "int": DataType.INT,
            "number": DataType.NUMBER,
            "bool": DataType.BOOL,
            "date": DataType.DATE,
            "object": DataType.OBJECT,
        }
        return data_type_map.get(data_type_str.lower(), DataType.TEXT)

    def add_object(self, class_name: str, data_object: dict, vector: List[float] = None):
        if self.client is None:
            self.connect()
    
        try:
            collection = self.client.collections.get(class_name)
            if vector:
                # 使用指定的向量
                collection.data.insert(
                    properties=data_object,
                    vector=vector
                )
            else:
                # 让 Weaviate 自动生成向量
                collection.data.insert(properties=data_object)
                
            logger.debug(f"Successfully added object to collection {class_name}")
        except Exception as e:
            logger.error(f"Error adding object to Weaviate: {str(e)}")
            raise

    def close(self):
        if self.client:
            self.client.close()

    def query_objects(self, class_name: str, vector: List[float], limit: int = 5) -> List[dict]:
        """
        使用向量相似度搜索对象

        Args:
            class_name: 集合名称
            vector: 查询向量
            limit: 返回结果数量

        Returns:
            包含相似对象的列表，每个对象包含属性和相似度分数
        """
        if self.client is None:
            self.connect()

        try:
            collection = self.client.collections.get(class_name)
            
            # Convert vector to list of floats if needed
            vector = [float(v) for v in vector]
            
            results = collection.query.near_vector(
                near_vector=vector,
                limit=limit,
                return_metadata=["distance"],
                return_properties=["content", "metadata.*"]
            )
            
            # 处理结果，添加相似度分数
            processed_results = []
            for obj in results.objects:
                result = {
                    "content": obj.properties.get("content"),
                    "metadata": obj.properties.get("metadata"),
                    "score": obj.metadata.distance  # 或 similarity，取决于你的配置
                }
                processed_results.append(result)
                
            logger.debug(f"Successfully queried {len(processed_results)} objects from {class_name}")
            return processed_results
            
        except Exception as e:
            logger.error(f"Error querying objects from Weaviate: {str(e)}")
            raise
        
__all__ = ['WeaviateManager'] 