# Chat Component Architecture

## Overview

The chat component has been refactored to support multiple conversation management modes through a controller-based architecture. This design decouples UI rendering from business logic and API calls, making the component adaptable to different backends and use cases.

## Architecture Layers

### 1. ChatWithController (Presentation)
Pure rendering component that consumes a controller and displays conversation UI.

- **Responsibilities:**
  - Render conversation list (if applicable)
  - Render messages and input area
  - Show loading skeletons
  - Trigger controller actions on user interactions
- **No business logic or API calls**

### 2. ChatController (Orchestration)
Mode-aware controller that manages conversation state and coordinates with transport.

- **Implementations:**
  - `SingleChatController` - For singleChat mode (list management, history, new conversations)
  - Future: `SingleTestController`, `GroupChatController`, etc.
- **Responsibilities:**
  - Maintain conversation list and active selection
  - Delegate data operations to transport
  - Manage loading states
  - Adopt server conversation IDs after first send
  - Update chat store (messages, streaming)

### 3. ConversationTransport (Adapter)
Backend-specific adapter that implements actual API calls.

- **Implementations:**
  - `WebappConversationTransport` - For webapp endpoints
  - Future: Console transport, third-party adapters, etc.
- **Responsibilities:**
  - List conversations (paginated)
  - Get conversation detail (messages)
  - Send message via SSE streaming
  - Delete/rename conversations (if supported)
  - Handle toasts and errors (inside hooks)

## Chat Modes

### singleChat (Implemented)
- One active conversation at a time
- Full conversation list with history
- Create new conversations
- Server-side persistence
- **Use case:** Webapp, console chat

### singleTest (Legacy)
- One conversation, no list management
- No create/delete, but track current ID
- **Use case:** Draft testing in workflow editor

### groupTest (Not Implemented)
- Multiple conversations side-by-side
- No history management
- **Use case:** A/B testing different models

### groupChat (Not Implemented)
- Multiple conversations with full history
- Conversation groups
- **Use case:** Multi-agent scenarios

## Data Flow: singleChat

1. **Init**
   - Controller loads first page of conversation list
   - Auto-selects latest or creates a draft if empty

2. **Select conversation**
   - Controller fetches detail (messages) from transport
   - Initializes chat store with loaded messages
   - Shows skeleton during load

3. **Send message**
   - User enters query → controller.send()
   - Controller creates user + assistant message optimistically
   - Transport starts SSE stream:
     - `onStarted`: Adopts server conversation_id if draft
     - `onToken`: Streams into assistant message
     - `onNodeStarted/Finished`: Updates workflow run info
     - `onFinished`: Finalizes message, refreshes list
     - `onError`: Rolls back, shows toast

4. **Create new conversation**
   - Creates local draft (conversationId: '')
   - First send assigns real server ID automatically

## Usage Example: Webapp

```typescript
import { ChatWithController, SingleChatController, WebappConversationTransport } from '@/components/chat';
import { useEffect, useMemo } from 'react';

// In your page/component
const transport = useMemo(() => new WebappConversationTransport(versionUuid), [versionUuid]);
const controller = useMemo(() => new SingleChatController(transport), [transport]);

// Initialize controller transport in effect to avoid setState during render
useEffect(() => {
  controller.initTransport();
}, [controller]);

return (
  <ChatWithController
    controller={controller}
    features={features}
    placeholder="Start chatting..."
    workflowRunShowDetail={false}
  />
);
```

## File Structure

```
src/components/chat/
├── index.tsx                              # Legacy Chat (backward compat)
├── chat-with-controller.tsx               # Pure presentation component
├── controllers/
│   ├── types.ts                           # Controller & transport interfaces
│   └── single-chat-controller.ts          # singleChat implementation
├── transports/
│   └── webapp-transport.ts                # Webapp API adapter
├── store/
│   └── index.ts                           # Zustand store (messages, streaming)
├── types.ts                               # Message, Conversation, NodeInfo, etc.
├── ui/                                    # UI primitives (ConversationBox, UserInput, etc.)
└── README.md                              # This file
```

## Key Design Principles

1. **Separation of concerns**
   - UI only renders state and triggers actions
   - Controller manages state and orchestrates
   - Transport handles API calls and side-effects

2. **Strict TypeScript**
   - No `any` types
   - Explicit interfaces for all layers

3. **External data operations**
   - All API calls via transport
   - Toasts and optimistic updates in hooks/transport
   - Component remains pure

4. **New conversation pattern**
   - Create draft with empty server ID
   - First send assigns real ID automatically
   - No explicit "create" API call needed for webapp

5. **Performance**
   - Memoize transport and controller instances
   - Show skeletons for initial loads
   - Placeholder data for pagination (avoid flashes)
   - Minimal re-renders via zustand selectors

## Backward Compatibility

The original `Chat` component remains available and unchanged for existing consumers (workflow editor, etc.). New features should use `ChatWithController` with a controller instance.

## Future Extensions

- **SingleTestController**: For workflow draft testing (no list, fixed ID)
- **GroupChatController**: For multi-conversation scenarios
- **Console transport**: For console chat endpoints
- **Conversation sidebar**: Can be added to ChatWithController for singleChat mode
- **Rename/delete UI**: Wire to controller actions when transport supports them

## Testing Strategy

1. Unit test controllers with mock transport
2. Unit test transport mappers
3. Integration test with real webapp endpoints (dev server)
4. E2E test conversation flow: create → send → load history → delete
