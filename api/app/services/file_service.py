# File service business logic

from fastapi import UploadFile
from app.models.document import Document
from app.utils.vector_store import VectorStore
# chunk_document,embed_chunks,store_vectors
import logging
from pathlib import Path
import chardet
from . import qiniu_service
from . import db_service

vectorClient = VectorStore()

def read_file(file: UploadFile) -> Document:
    content = file.file.read()
    
    # 自动检测编码
    detected = chardet.detect(content)
    encoding = detected['encoding']
    logging.info(f"Detected encoding: {encoding}")
    
    try:
        decoded_content = content.decode(encoding or 'utf-8')
    except UnicodeDecodeError:
        # 如果自动检测失败，尝试常用编码
        decoded_content = None  # 初始化变量
        for enc in ['utf-8', 'gbk', 'gb2312', 'gb18030', 'big5','latin1']:
            try:
                decoded_content = content.decode(enc)
                break
            except UnicodeDecodeError:
                continue
        
        if decoded_content is None:  # 如果所有编码都失败
            raise ValueError("Unable to decode file with any supported encoding")
    
    return Document(
        filename=file.filename,
        content=decoded_content,
        file_type=file.content_type
    )

async def upload_file(file: UploadFile, saveToData: bool = False):
    logging.info("start upload")
    file_path = await ensure_upload_directory(file)
    if saveToData:
        # 保存到数据库
        logging.info(f"File saved to database")
        await process_and_vectorize_file(file)
        
    return {
        "filename": file.filename,
        "file_path": file_path["local"],
        "cdn_path": file_path["cdn"]
    }

async def ensure_upload_directory(file: UploadFile, directory: str = "uploads"):
    """确保上传目录存在，如果不存在则创建"""
    upload_dir = Path(directory)
    upload_dir.mkdir(parents=True, exist_ok=True)
    
    # 添加时间戳到文件名
    
    new_filename = qiniu_service.getUploadName(file)
    
    if new_filename is not None:
        file_path = upload_dir / new_filename
    else:
        raise ValueError("Failed to generate a new filename for the file")
    
    # 保存文件到本地
    with open(file_path, "wb") as buffer:
        content = await file.read()
        buffer.write(content)
    logging.info(f"File saved to: {file_path}")
    
    # 重置文件指针位置
    await file.seek(0)
    cdnPath = await qiniu_service.uploadToQiniu(file_path, str(file.filename))
    # //上传完成后保存到数据库
    await db_service.insertUploadFile(local=str(file_path), cdn=str(cdnPath))
    return {
        "local": str(file_path),
        "cdn": cdnPath
    }

async def process_and_vectorize_file(file: UploadFile):
    logging.info(f"Starting to process file: {file.filename}")
    
    try:
        # 1. 文件读取和验证
        if not file.filename:
            raise ValueError("File name is required")
            
        # 检查文件大小
        file_size = 0
        chunk_size = 1024 * 1024  # 1MB
        while chunk := await file.read(chunk_size):
            file_size += len(chunk)
        if file_size > 10 * 1024 * 1024:  # 10MB
            raise ValueError("File size exceeds limit (10MB)")
        await file.seek(0)  # 重置文件指针
        
        # 2. 读取文件内容
        document = read_file(file)
        if not document.content.strip():
            raise ValueError("File content is empty")
        logging.info(f"File read successfully. Content length: {len(document.content)}")
        
        # 3. 文本分块
        logging.info("Starting chunking process")
        chunks = vectorClient.chunk_document(document)
        if not chunks:
            raise ValueError("No chunks were generated")
        logging.info(f"Generated {len(chunks)} chunks")
        
        # 4. 向量化处理
        logging.info("Starting embedding process")
        vectors = await vectorClient.embed_chunks(chunks)
        if len(vectors) != len(chunks):
            raise ValueError(f"Embedding mismatch: {len(vectors)} vectors for {len(chunks)} chunks")
        
        # 5. 存储向量
        try:
            vectorClient.store_vectors(document.filename, chunks,vectors)
        except Exception as e:
            logging.error(f"Vector storage failed: {str(e)}")
            raise ValueError(f"Failed to store vectors: {str(e)}")
            
        # 6. 返回处理结果
        result = {
            "filename": document.filename,
            "file_size": file_size,
            "chunk_count": len(chunks),
            "vector_count": len(vectors),
            "status": "success",
            "message": "File processed and vectorized successfully"
        }
        logging.info(f"Processing completed: {result}")
        return result
        
    except ValueError as ve:
        logging.error(f"Validation error: {str(ve)}")
        raise
    except Exception as e:
        logging.error(f"Unexpected error: {str(e)}", exc_info=True)
        raise
    finally:
        # 清理临时资源
        await file.close()

async def list_files():
    return []

