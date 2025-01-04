from fastapi import HTTPException, status

class FlowOnlineEditError(HTTPException):
    """Flow online edit error"""
    def __init__(self, detail: str = "Cannot edit online flow"):
        super().__init__(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=detail
        )
