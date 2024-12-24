from pydantic import BaseModel, Field, root_validator, ConfigDict
from typing import List, Dict, Any, Optional, Union
from datetime import datetime

class Message(BaseModel):
    """A single message in a chat completion request"""
    role: str = Field(..., description="The role of the message author (system, user, or assistant)")
    content: str = Field(..., description="The content of the message")
    name: Optional[str] = Field(None, description="The name of the author of this message")
    function_call: Optional[Dict[str, Any]] = Field(None, description="The name and arguments of the function to call")

class FunctionCall(BaseModel):
    """Function call specification"""
    name: str = Field(..., description="The name of the function to call")
    arguments: str = Field(..., description="The arguments to pass to the function")

class FunctionParameters(BaseModel):
    """Function parameters specification"""
    type: str = Field("object", description="The type of the parameters")
    properties: Dict[str, Any] = Field(..., description="The properties of the parameters")
    required: Optional[List[str]] = Field(None, description="Required properties")

class Function(BaseModel):
    """Function specification"""
    name: str = Field(..., description="The name of the function")
    description: Optional[str] = Field(None, description="A description of what the function does")
    parameters: FunctionParameters = Field(..., description="The parameters the function accepts")

class ChatCompletionRequest(BaseModel):
    """Request schema for chat completion endpoint"""
    model: str = Field(..., description="ID of the model to use")
    messages: List[Message] = Field(..., description="A list of messages comprising the conversation so far")
    functions: Optional[List[Function]] = Field(None, description="A list of functions the model may generate JSON inputs for")
    function_call: Optional[Union[str, Dict[str, Any]]] = Field(None, description="Controls how the model responds to function calls")
    temperature: Optional[float] = Field(0.7, ge=0, le=2, description="What sampling temperature to use, between 0 and 2")
    top_p: Optional[float] = Field(1, ge=0, le=1, description="An alternative to sampling with temperature")
    n: Optional[int] = Field(1, ge=1, le=128, description="How many chat completion choices to generate for each input message")
    stream: Optional[bool] = Field(False, description="Whether to stream back partial progress")
    stop: Optional[Union[str, List[str]]] = Field(None, description="Up to 4 sequences where the API will stop generating")
    max_tokens: Optional[int] = Field(None, description="The maximum number of tokens to generate")
    presence_penalty: Optional[float] = Field(0, ge=-2, le=2, description="Penalty for new tokens based on their presence in the prompt")
    frequency_penalty: Optional[float] = Field(0, ge=-2, le=2, description="Penalty for new tokens based on their frequency in the prompt")
    logit_bias: Optional[Dict[str, float]] = Field(None, description="Modify the likelihood of specified tokens appearing in the completion")
    user: Optional[str] = Field(None, description="A unique identifier representing your end-user")
    base_url: Optional[str] = Field(None, description="Base URL for the provider API")

    @root_validator(pre=True)
    def validate_model(cls, values):
        """Validate model name."""
        model = values.get("model", "")
        if not isinstance(model, str):
            raise ValueError("Model must be a string")
        return values

    @root_validator(pre=True)
    def validate_function_call(cls, values):
        """Validate function_call field"""
        function_call = values.get('function_call')
        functions = values.get('functions')
        
        if function_call and not functions:
            raise ValueError("function_call can only be present when functions is present")
            
        if isinstance(function_call, str) and function_call not in ['none', 'auto']:
            raise ValueError("function_call as string must be 'none' or 'auto'")
            
        return values

class ChatCompletionResponseChoice(BaseModel):
    """A single chat completion choice"""
    index: int = Field(..., description="The index of this choice among the choices")
    message: Message = Field(..., description="The message output by the model")
    finish_reason: Optional[str] = Field(None, description="The reason why the model stopped generating")
    logprobs: Optional[Any] = Field(None, description="Log probabilities of tokens")

class Usage(BaseModel):
    """Token usage information"""
    prompt_tokens: int = Field(..., description="Number of tokens in the prompt")
    completion_tokens: int = Field(..., description="Number of tokens in the completion")
    total_tokens: int = Field(..., description="Total number of tokens used")
    prompt_cache_hit_tokens: Optional[int] = Field(0, description="Number of tokens hit in prompt cache")
    prompt_cache_miss_tokens: Optional[int] = Field(0, description="Number of tokens missed in prompt cache")

class ChatCompletionResponse(BaseModel):
    """Response schema for chat completion endpoint"""
    id: str = Field(..., description="Unique identifier for the completion")
    object: str = Field("chat.completion", description="Object type, always 'chat.completion'")
    created: int = Field(..., description="Unix timestamp for when the completion was created")
    model: str = Field(..., description="The model used for completion")
    choices: List[ChatCompletionResponseChoice] = Field(..., description="The list of completion choices")
    usage: Usage = Field(..., description="Token usage information for the request")
    system_fingerprint: Optional[str] = Field(None, description="System fingerprint for the response")


class AddChatMessagesRequest(BaseModel):
    session_id: Optional[str] = Field(None, description="Unique identifier for the chat session")
    messages: List[Message] = Field(..., description="The list of chat messages")


class ConversationResponse(BaseModel):
    session_id: str = Field(..., description="Unique identifier for the conversation")

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )


class MessageResponse(BaseModel):
    role: str = Field(..., description="The role of the message author (system, user, or assistant)")
    content: str = Field(..., description="The content of the message")

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )

class ChatMessages(BaseModel):
    session_id: str = Field(..., description="Unique identifier for the chat session")
    messages: List[MessageResponse] = Field([], description="The list of chat messages")

    model_config = ConfigDict(
        from_attributes=True,
        json_encoders={
            datetime: lambda v: v.isoformat() if v else None,
            bytes: lambda v: v.decode() if v else None
        }
    )


class ChatMessagesResponse(ChatMessages):
    total: int


class ConversationListResponse(BaseModel):
    messages: List[ChatMessages]
    total: int

    class Config:
        from_attributes = True