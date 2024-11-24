from pydantic import BaseModel

class Document(BaseModel):
    filename: str
    content: str
    file_type: str
