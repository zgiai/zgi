from app.utils.mysql_manager import MySQLManager
from datetime import datetime

# 创建 MySQLManager 实例
mysql_manager = MySQLManager()

async def insertUploadFile(local: str, cdn: str) -> int:
    query = "INSERT INTO upload_file (local, cdn_path,create_at) VALUES (%s, %s,%s)"
    formatted_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S") 
    params = (local, cdn,formatted_time)  # 参数元组
    print(params,'>>>>>parmas')
    return mysql_manager.execute_update(query, params) 