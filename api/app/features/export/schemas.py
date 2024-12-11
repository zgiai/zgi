from datetime import datetime
from typing import List, Optional
from pydantic import BaseModel, Field

class ExportRequest(BaseModel):
    kb_ids: List[int] = Field(..., description="List of knowledge base IDs to export")
    format: str = Field(default="json", description="Export format (json/csv)")
    include_vectors: bool = Field(default=False, description="Include vector embeddings")
    include_metadata: bool = Field(default=True, description="Include document metadata")

class ImportRequest(BaseModel):
    kb_id: Optional[int] = Field(None, description="Target knowledge base ID, if None creates new")
    file_format: str = Field(..., description="Format of the import file")
    merge_strategy: str = Field(default="skip", description="How to handle duplicates (skip/overwrite/rename)")
    validate_only: bool = Field(default=False, description="Only validate without importing")

class BackupConfig(BaseModel):
    include_kb: bool = True
    include_users: bool = True
    include_settings: bool = True
    include_vectors: bool = True
    encryption_key: Optional[str] = None

class MigrationJob(BaseModel):
    job_id: str
    status: str
    progress: float
    started_at: datetime
    completed_at: Optional[datetime]
    error_message: Optional[str]
    stats: dict

class BulkOperation(BaseModel):
    operation_type: str  # upload/delete/update
    kb_id: int
    file_paths: List[str]
    options: dict
    status: str
    progress: float
    errors: List[dict]
