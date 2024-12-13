import pymysql
from pymysql.cursors import DictCursor
from typing import Optional, Dict, List, Any
from dbutils.pooled_db import PooledDB
from dotenv import load_dotenv
import os

class MySQLManager:
    _pool = None  # 类变量，用于存储连接池实例
    
    @classmethod
    def initialize_pool(cls) -> None:
        """
        从环境变量初始化数据库连接池
        
        环境变量:
            DB_HOST: 数据库主机地址
            DB_USER: 数据库用户名
            DB_PASSWORD: 数据库密码
            DB_DATABASE: 数据库名
            DB_PORT: 数据库端口号（可选，默认3306）
            DB_MIN_CONNECTIONS: 最小连接数（可选，默认1）
            DB_MAX_CONNECTIONS: 最大连接数（可选，默认10）
        """
        load_dotenv()  # 加载 .env 文件中的环境变量
        
        cls._pool = PooledDB(
            creator=pymysql,
            maxconnections=int(os.getenv('DB_MAX_CONNECTIONS', 10)),
            mincached=int(os.getenv('DB_MIN_CONNECTIONS', 1)),
            maxcached=int(os.getenv('DB_MAX_CONNECTIONS', 10)),
            maxshared=int(os.getenv('DB_MAX_CONNECTIONS', 10)),
            blocking=True,
            maxusage=None,
            setsession=[],
            host=os.getenv('DB_HOST', 'localhost'),
            user=os.getenv('DB_USER', 'root'),
            password=os.getenv('DB_PASSWORD', ''),
            database=os.getenv('DB_DATABASE', 'zgi'),
            port=int(os.getenv('DB_PORT', 3306)),
            charset='utf8mb4',
            cursorclass=DictCursor
        )

    def __init__(self):
        """初始化，检查连接池是否已创建"""
        if not self._pool:
            raise Exception("连接池未初始化，请先调用initialize_pool方法")
        self.connection = None

    def connect(self) -> None:
        """从连接池获取数据库连接"""
        try:
            self.connection = self._pool.connection()
        except Exception as e:
            raise Exception(f"获取数据库连接失败: {str(e)}")

    def disconnect(self) -> None:
        """关闭数据库连接"""
        if self.connection:
            self.connection.close()
            self.connection = None

    def execute_query(self, query: str, params: Optional[tuple] = None) -> List[Dict[str, Any]]:
        """
        执行查询语句
        
        Args:
            query: SQL查询语句
            params: 查询参数元组
            
        Returns:
            查询结果列表
        """
        if not self.connection:
            self.connect()
            
        try:
            with self.connection.cursor() as cursor:
                cursor.execute(query, params or ())
                return cursor.fetchall()
        except Exception as e:
            raise Exception(f"查询执行失败: {str(e)}")

    def execute_update(self, query: str, params: Optional[tuple] = None) -> int:
        """
        执行更新语句（INSERT, UPDATE, DELETE等）
        
        Args:
            query: SQL更新语句
            params: 更新参数元组
            
        Returns:
            受影响的行数
        """
        if not self.connection:
            self.connect()
            
        try:
            with self.connection.cursor() as cursor:
                affected_rows = cursor.execute(query, params or ())
                self.connection.commit()
                return affected_rows
        except Exception as e:
            self.connection.rollback()
            raise Exception(f"更新执行失败: {str(e)}")
    
    def __enter__(self):
        """上下文管理器入口"""
        self.connect()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """上下文管理器出口"""
        self.disconnect()

MySQLManager.initialize_pool()
mysql_manager = MySQLManager()