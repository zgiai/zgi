"""Server-Sent Events (SSE) parsing utilities."""
from typing import Dict, Any, Tuple, Optional
import json
import logging

logger = logging.getLogger(__name__)

def parse_sse_line(line: str) -> Tuple[Optional[str], Optional[str]]:
    """Parse a line of SSE data.
    
    Args:
        line: Raw SSE line
        
    Returns:
        Tuple of (field_name, field_value)
    """
    line = line.strip()
    if not line:
        return None, None
        
    if ":" not in line:
        return None, None
        
    field_name, _, field_value = line.partition(":")
    if field_value.startswith(" "):
        field_value = field_value[1:]
        
    return field_name, field_value

def parse_sse_event(lines: list[str]) -> Tuple[Optional[str], Optional[Dict[str, Any]]]:
    """Parse a complete SSE event from multiple lines.
    
    Args:
        lines: List of raw SSE lines
        
    Returns:
        Tuple of (event_type, event_data)
    """
    event_type = None
    data_lines = []
    
    for line in lines:
        field, value = parse_sse_line(line)
        if not field:
            continue
            
        if field == "event":
            event_type = value
        elif field == "data":
            data_lines.append(value)
            
    if not data_lines:
        return None, None
        
    try:
        data = json.loads("".join(data_lines))
        return event_type, data
    except json.JSONDecodeError:
        logger.warning(f"Failed to parse SSE data: {''.join(data_lines)}")
        return event_type, None

class SSEBuffer:
    """Buffer for accumulating and parsing SSE events."""
    
    def __init__(self):
        """Initialize SSE buffer."""
        self.current_lines: list[str] = []
        
    def add_line(self, line: str) -> Optional[Tuple[Optional[str], Optional[Dict[str, Any]]]]:
        """Add a line to the buffer and try to parse a complete event.
        
        Args:
            line: Raw SSE line
            
        Returns:
            Parsed event if complete, None otherwise
        """
        line = line.strip()
        if not line:
            if self.current_lines:
                event = parse_sse_event(self.current_lines)
                self.current_lines = []
                return event
            return None
            
        self.current_lines.append(line)
        return None
        
    def flush(self) -> Optional[Tuple[Optional[str], Optional[Dict[str, Any]]]]:
        """Flush the buffer and parse any remaining event.
        
        Returns:
            Parsed event if any lines in buffer, None otherwise
        """
        if not self.current_lines:
            return None
            
        event = parse_sse_event(self.current_lines)
        self.current_lines = []
        return event
