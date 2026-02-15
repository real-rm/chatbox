# Chat Client Frontend

This directory contains the HTML/JavaScript frontend for the chat application.

## Files

- `chat.html` - Main HTML structure
- `chat.css` - Styling and responsive design
- `chat.js` - WebSocket client and UI logic

## Features

### 1. WebSocket Connection Management
- Automatic connection with JWT token authentication
- Exponential backoff reconnection (1s, 2s, 4s, 8s, 16s, 30s max)
- Heartbeat ping/pong every 30 seconds
- Connection status indicator (connected, connecting, disconnected)

### 2. Message Display
- Chronological message ordering
- Visual distinction between user, AI, and admin messages
- Timestamp display for each message
- File attachments with download links
- Voice message playback controls

### 3. File Upload
- File selection from device
- Camera access for taking photos (mobile)
- Photo library access (mobile)
- Upload progress indicator
- Support for images, videos, audio, PDFs, and documents

### 4. Voice Messages
- Audio recording with MediaRecorder API
- Recording time indicator
- Visual recording state (pulsing microphone button)
- Automatic upload after recording stops
- Audio playback controls for received voice messages

### 5. Model Selection
- Dynamic model selector (shown when multiple models configured)
- Switches between different LLM models
- Hidden when admin joins session

### 6. Admin Assistance
- Help request button (🆘)
- Admin name display when admin joins
- Visual indication of admin-assisted session
- System messages for admin join/leave events

### 7. Loading Animation
- Animated dots indicator
- Shows when AI is processing message
- Hides when response received

## Usage

### Embedding in React Native WebView

```javascript
import { WebView } from 'react-native-webview';

const ChatScreen = ({ jwtToken }) => {
  return (
    <WebView
      source={{ uri: `https://your-domain.com/chat.html?token=${jwtToken}` }}
      onMessage={(event) => {
        const data = JSON.parse(event.nativeEvent.data);
        if (data.type === 'request_token' || data.type === 'refresh_token') {
          // Send token to webview
          webViewRef.current.postMessage(JSON.stringify({
            type: 'token',
            token: jwtToken
          }));
        }
      }}
    />
  );
};
```

### Embedding in Web iframe

```html
<iframe
  id="chat-iframe"
  src="https://your-domain.com/chat.html?token=YOUR_JWT_TOKEN"
  width="100%"
  height="600px"
  frameborder="0"
></iframe>

<script>
  // Listen for token requests from iframe
  window.addEventListener('message', (event) => {
    if (event.data.type === 'request_token' || event.data.type === 'refresh_token') {
      // Send token to iframe
      const iframe = document.getElementById('chat-iframe');
      iframe.contentWindow.postMessage({
        type: 'token',
        token: 'YOUR_JWT_TOKEN'
      }, '*');
    }
  });
</script>
```

### JWT Token Authentication

The client supports three methods for JWT token authentication:

1. **URL Parameter**: `?token=YOUR_JWT_TOKEN`
2. **Session Storage**: Automatically stored after first load
3. **Parent Window Message**: Requests token from parent via postMessage

The client will automatically request token refresh from the parent application when needed.

## WebSocket Protocol

### Connection URL

```
ws://localhost:8080/chat/ws?token=JWT_TOKEN&session_id=SESSION_ID
```

- `token`: Required JWT token for authentication
- `session_id`: Optional session ID for reconnection

### Message Format

All messages use JSON format:

```json
{
  "type": "message_type",
  "content": "message content",
  "timestamp": "2024-01-01T12:00:00Z",
  "sender": "user|ai|admin",
  "metadata": {}
}
```

### Message Types

- `user_message` - User sends text message
- `ai_response` - AI response
- `file_upload` - File upload notification
- `voice_message` - Voice message
- `error` - Error notification
- `connection_status` - Connection state
- `help_request` - User requests help
- `admin_join` - Admin joins session
- `admin_leave` - Admin leaves session
- `model_select` - User selects model
- `loading` - Loading indicator state
- `ping` - Heartbeat ping

## Browser Compatibility

- Modern browsers with WebSocket support
- MediaRecorder API for voice recording (Chrome, Firefox, Edge)
- getUserMedia API for microphone access
- File API for file uploads

## Mobile Considerations

- Responsive design for mobile screens
- Touch-friendly button sizes
- Camera capture attribute for photo taking
- Audio recording with mobile-optimized UI
- Optimized for React Native WebView

## Configuration

The client automatically adapts to:
- HTTP/HTTPS protocol (uses WS/WSS accordingly)
- Current host (or defaults to localhost:8080)
- Multiple model configurations (shows/hides selector)
- Admin assistance mode (shows admin name, hides model selector)

## Error Handling

- Connection errors trigger automatic reconnection
- Upload errors display user-friendly messages
- Microphone access errors show permission messages
- All errors logged to console for debugging

## Security

- JWT token stored in sessionStorage (not localStorage)
- Secure WebSocket (WSS) for HTTPS sites
- Token refresh mechanism for long sessions
- No sensitive data in URL after initial load


## Admin Dashboard

### Files

- `admin.html` - Admin dashboard HTML structure
- `admin.css` - Admin dashboard styling
- `admin.js` - Admin dashboard functionality

### Features

#### 1. Session Monitoring
- Real-time view of all active and ended sessions
- Session metrics dashboard:
  - Active sessions count
  - Total sessions in time period
  - Average concurrent sessions
  - Total tokens used across all sessions

#### 2. Filtering and Sorting
- Filter sessions by:
  - User ID (text search)
  - Date range (start and end dates)
  - Status (active/ended)
  - Admin-assisted flag (yes/no/all)
- Sort sessions by:
  - Connection time
  - Duration
  - User ID
  - Last activity timestamp
- Ascending or descending order

#### 3. Session Details
Each session displays:
- User ID (clickable to view all user sessions)
- Session ID (truncated)
- Status badge (active/ended)
- Start time
- Duration
- Last activity time
- Total tokens used
- Average response time
- Maximum response time
- Admin assistance status
- Takeover button (for active sessions)

#### 4. User Session View
- Click on any user ID to view modal with all their sessions
- Shows session name, status, start time, and message count
- Takeover button for active sessions
- Easy navigation between user sessions

#### 5. Session Takeover
- One-click takeover of active user sessions
- Opens new window with chat interface in admin mode
- Admin name displays in user's chat
- Bidirectional message routing between admin and user
- Automatic session locking (one admin per session)
- Admin can close window to leave session

#### 6. Auto-Refresh
- Automatic data refresh every 10 seconds
- Toggle on/off by clicking auto-refresh status
- Manual refresh button available
- Keeps dashboard data current without page reload

### Usage

#### Accessing Admin Dashboard

```
https://your-domain.com/admin.html?token=ADMIN_JWT_TOKEN
```

The JWT token must have admin role to access the dashboard.

#### Monitoring Sessions

1. Dashboard loads with current metrics and session list
2. Use filters to narrow down sessions of interest
3. Click on user IDs to view all sessions for that user
4. Sort by different criteria to find specific sessions

#### Taking Over Sessions

1. Find the active session you want to assist
2. Click "Takeover" button
3. New window opens with chat interface
4. Your admin name appears in user's chat
5. Send messages to communicate with user
6. Close window when done to leave session

#### Viewing Metrics

The metrics panel shows:
- **Active Sessions**: Currently connected users
- **Total Sessions**: All sessions in filtered time period
- **Avg Concurrent**: Average number of simultaneous sessions
- **Total Tokens**: Sum of all tokens used across sessions

### Admin API Endpoints

#### GET /chat/admin/sessions
List all sessions with filtering and sorting

Query Parameters:
- `user_id` - Filter by user ID
- `start_time` - Filter by start date (ISO 8601)
- `end_time` - Filter by end date (ISO 8601)
- `status` - Filter by status (active/ended)
- `admin_assisted` - Filter by admin assistance (true/false)
- `sort_by` - Sort field (start_time/duration/user_id/last_activity)
- `sort_order` - Sort order (asc/desc)

Response:
```json
[
  {
    "session_id": "uuid",
    "user_id": "user123",
    "is_active": true,
    "start_time": "2024-01-01T12:00:00Z",
    "last_activity": "2024-01-01T12:30:00Z",
    "duration": 1800,
    "total_tokens": 1500,
    "avg_response_time": 2500,
    "max_response_time": 5000,
    "admin_assisted": false,
    "assisting_admin_name": null
  }
]
```

#### GET /chat/admin/metrics
Get session metrics for time period

Query Parameters:
- `start_time` - Start date (ISO 8601)
- `end_time` - End date (ISO 8601)

Response:
```json
{
  "active_sessions": 15,
  "total_sessions": 250,
  "avg_concurrent": 12.5,
  "total_tokens": 125000
}
```

#### GET /chat/admin/users/:userID/sessions
Get all sessions for specific user

Response:
```json
[
  {
    "session_id": "uuid",
    "name": "Session Name",
    "is_active": true,
    "start_time": "2024-01-01T12:00:00Z",
    "message_count": 25
  }
]
```

#### POST /chat/admin/takeover/:sessionID
Take over an active session

Response:
```json
{
  "success": true,
  "session_id": "uuid",
  "message": "Takeover successful"
}
```

### Security

- Admin dashboard requires JWT token with admin role
- All API endpoints verify admin authorization
- Session takeover logged with admin ID and timestamps
- One admin per session (session locking)
- Admin actions tracked in session metadata

### Responsive Design

- Desktop-optimized layout with data tables
- Mobile-friendly with responsive grid
- Scrollable tables for large datasets
- Touch-friendly controls
- Modal dialogs for user session details

### Browser Compatibility

- Modern browsers (Chrome, Firefox, Safari, Edge)
- JavaScript ES6+ features
- Fetch API for HTTP requests
- No external dependencies (vanilla JavaScript)
