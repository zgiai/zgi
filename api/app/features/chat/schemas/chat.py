from typing import Optional, List, Union, Dict, Any
from pydantic import BaseModel, Field
from datetime import datetime

class Message(BaseModel):
    role: str = Field(..., description="The role of the message sender (system, user, or assistant)")
    content: str = Field(..., description="The content of the message")
    name: Optional[str] = Field(None, description="The name of the message sender")

class ChatCompletionRequest(BaseModel):
    model: str = Field(..., description="ID of the model to use")
    messages: List[Message] = Field(..., description="A list of messages comprising the conversation so far")
    stream: bool = Field(False, description="Whether to stream back partial progress")
    temperature: Optional[float] = Field(0.7, description="What sampling temperature to use")
    top_p: Optional[float] = Field(1.0, description="An alternative to sampling with temperature")
    n: Optional[int] = Field(1, description="How many chat completion choices to generate for each input message")
    stop: Optional[Union[str, List[str]]] = Field(None, description="Up to 4 sequences where the API will stop generating")
    max_tokens: Optional[int] = Field(None, description="The maximum number of tokens to generate")
    presence_penalty: Optional[float] = Field(0, description="Penalty for new tokens based on presence in text")
    frequency_penalty: Optional[float] = Field(0, description="Penalty for new tokens based on frequency in text")
    user: Optional[str] = Field(None, description="A unique identifier representing your end-user")

class Usage(BaseModel):
    prompt_tokens: int
    completion_tokens: int
    total_tokens: int

class ChatCompletionResponseChoice(BaseModel):
    index: int
    message: Message
    finish_reason: Optional[str] = None

class ChatCompletionResponse(BaseModel):
    id: str
    object: str = "chat.completion"
    created: int
    model: str
    choices: List[ChatCompletionResponseChoice]
    usage: Usage

class DeltaMessage(BaseModel):
    role: Optional[str] = None
    content: Optional[str] = None

class ChatCompletionStreamResponseChoice(BaseModel):
    index: int
    delta: DeltaMessage
    finish_reason: Optional[str] = None

class ChatCompletionStreamResponse(BaseModel):
    id: str
    object: str = "chat.completion.chunk"
    created: int
    model: str
    choices: List[ChatCompletionStreamResponseChoice]

class ChatHistoryResponse(BaseModel):
    id: int
    conversation_id: str
    question: str
    answer: str
    model: str
    cost: float
    created_at: datetime
    status: int

class ChatDetailResponse(BaseModel):
    id: int
    conversation_id: str
    request_id: str
    question: str
    answer: str
    model: str
    prompt_tokens: int
    completion_tokens: int
    cost: float
    interaction_type: int
    source: Optional[int]
    ip_address: str
    is_violation: int
    status: int
    parameters: Dict[str, Any]
    openai_response_id: Optional[str]
    openai_created_at: Optional[int]
    raw_request: Dict[str, Any]
    raw_response_chunks: Optional[List[Dict[str, Any]]]
    finish_reason: Optional[str]
    created_at: datetime
    updated_at: datetime
