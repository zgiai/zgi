import hashlib
from typing import Tuple

import requests
from qiniu import Auth, put_data, BucketManager
from ..base import StorageProvider


class QiniuStorageProvider(StorageProvider):
    """Qiniu storage provider"""

    def __init__(self, access_key: str, secret_key: str, bucket_name: str, domain: str, prefix: str):
        self.auth = Auth(access_key, secret_key)
        self.bucket_name = bucket_name
        self.domain = domain
        self.bucket = BucketManager(self.auth)
        if prefix:
            prefix = f"{prefix}/"
        self.prefix = prefix

    async def store_file(
            self,
            file: bytes,
            kb_id: int,
            user_id: int,
            filename: str
    ) -> str:
        """Store a file on Qiniu"""
        # Generate unique filename
        remote_filename = f"{self.prefix}{kb_id}/{user_id}_{filename}"

        # Upload file
        token = self.auth.upload_token(self.bucket_name, remote_filename)
        ret, info = put_data(token, remote_filename, file)
        if info.status_code != 200:
            raise Exception(f"Failed to upload file to Qiniu: {info.text_body}")

        # filepath = f"https://{self.domain}/{remote_filename}"
        return remote_filename

    async def delete_file(
            self,
            filepath: str
    ) -> None:
        """Delete a file on Qiniu"""
        # remote_filename = filepath.split('/')[-1]
        remote_filename = filepath
        ret, info = self.bucket.delete(self.bucket_name, remote_filename)
        if info.status_code != 200:
            print(f"Failed to delete file from Qiniu: {info.text_body}")
            # raise Exception(f"Failed to delete file from Qiniu: {info.text_body}")

    async def read_file(
            self,
            filepath: str
    ) -> bytes:
        """Read a file from Qiniu"""
        # remote_filename = filepath.split('/')[-1]
        remote_filename = filepath
        ret, info = self.bucket.stat(self.bucket_name, remote_filename)
        if info.status_code != 200:
            raise Exception(f"Failed to get file info from Qiniu: {info.text_body}")

        url = self.auth.private_download_url(f"https://{self.domain}/{remote_filename}")
        response = requests.get(url)
        if response.status_code != 200:
            raise Exception(f"Failed to download file from Qiniu: {response.text}")

        return response.content
