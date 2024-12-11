from qiniu import Auth,etag,put_file
import os  # 导入 os 模块
from fastapi import UploadFile
import time
from pathlib import Path

# 从环境变量获取 QINIU_SK 和 QINIU_AK
access_key = os.getenv('QINIU_AK')  # 获取 QINIU_AK
secret_key = os.getenv('QINIU_SK')  # 获取 QINIU_SK
bucket = os.getenv('QINIU_BUCKET')
cdn_url = os.getenv('QINIU_BASEURL')

q = Auth(access_key, secret_key)  # 使用环境变量中的密钥

async def uploadToQiniu(file: Path,key:str):
  #要上传的空间
  bucket_name = bucket
  #生成上传 Token，可以指定过期时间等
  token = q.upload_token(bucket_name, key, 3600)
  #要上传文件的本地路径
  localfile = file
  ret, info = put_file(token, key, localfile, version='v2')
  assert ret['key'] == key
  assert ret['hash'] == etag(localfile)
  return f"{cdn_url}{ret['key']}"

def getUploadName(file: UploadFile):
  if file.filename is None:
    return None
  filename, extension = os.path.splitext(file.filename)
  timestamp = time.strftime("%Y%m%d%H%M%S")
  new_filename = f"{filename}{timestamp}{extension}"
  return new_filename