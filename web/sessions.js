// Session List Application
class SessionList {
    constructor() {
        this.sessions = [];
        this.userID = null;
        
        this.initializeElements();
        this.attachEventListeners();
        this.loadSessions();
    }

    initializeElements() {
        this.statusBar = document.getElementById('connection-status');
        this.statusText = document.getElementById('status-text');
        this.sessionsList = document.getElementById('sessions-list');
        this.emptyState = document.getElementById('empty-state');
        this.loading = document.getElementById('loading');
        this.newSessionBtn = document.getElementById('new-session-btn');
    }

    attachEventListeners() {
        this.newSessionBtn.addEventListener('click', () => this.createNewSession());
    }

    // Load Sessions from Server
    async loadSessions() {
        this.showLoading();
        
        try {
            const token = this.getJWTToken();
            if (!token) {
                this.showError('No authentication token');
                return;
            }

            // Fetch sessions from server
            const response = await fetch('/chat/sessions', {
                method: 'GET',
                headers: {
                    'Authorization': `Bearer ${token}`,
                    'Content-Type': 'application/json'
                }
            });

            if (!response.ok) {
                throw new Error(`Failed to load sessions: ${response.status}`);
            }

            const data = await response.json();
            this.sessions = data.sessions || [];
            this.userID = data.user_id;
            
            this.hideLoading();
            this.renderSessions();
        } catch (error) {
            console.error('Error loading sessions:', error);
            this.hideLoading();
            this.showError('Failed to load sessions');
        }
    }

    // Render Sessions List
    renderSessions() {
        // Clear existing sessions
        this.sessionsList.innerHTML = '';

        if (this.sessions.length === 0) {
            this.showEmptyState();
            return;
        }

        this.hideEmptyState();

        // Sort sessions by last activity (most recent first)
        const sortedSessions = [...this.sessions].sort((a, b) => {
            const timeA = new Date(a.last_activity || a.start_time);
            const timeB = new Date(b.last_activity || b.start_time);
            return timeB - timeA;
        });

        // Render each session
        sortedSessions.forEach(session => {
            const sessionItem = this.createSessionItem(session);
            this.sessionsList.appendChild(sessionItem);
        });
    }

    // Create Session Item Element
    createSessionItem(session) {
        const item = document.createElement('div');
        item.className = 'session-item';
        item.onclick = () => this.openSession(session.id);

        // Session Header (Name + Admin Badge)
        const header = document.createElement('div');
        header.className = 'session-header';

        const name = document.createElement('div');
        name.className = 'session-name';
        name.textContent = session.name || 'Untitled Chat';
        header.appendChild(name);

        // Admin-assisted badge
        if (session.admin_assisted) {
            const badge = document.createElement('span');
            badge.className = 'admin-badge';
            badge.textContent = 'Admin Assisted';
            header.appendChild(badge);
        }

        item.appendChild(header);

        // Session Metadata
        const meta = document.createElement('div');
        meta.className = 'session-meta';

        // Timestamp
        const timestamp = document.createElement('div');
        timestamp.className = 'session-meta-item session-timestamp';
        timestamp.textContent = this.formatTimestamp(session.last_activity || session.start_time);
        meta.appendChild(timestamp);

        // Message count
        const messageCount = document.createElement('div');
        messageCount.className = 'session-meta-item session-messages';
        messageCount.textContent = `${session.message_count || 0} messages`;
        meta.appendChild(messageCount);

        item.appendChild(meta);

        return item;
    }

    // Open Session (Navigate to Chat)
    openSession(sessionID) {
        if (!sessionID) return;

        // Navigate to chat page with session ID
        const token = this.getJWTToken();
        const chatUrl = `chat.html?token=${encodeURIComponent(token)}&session_id=${encodeURIComponent(sessionID)}`;
        window.location.href = chatUrl;
    }

    // Create New Session
    createNewSession() {
        // Navigate to chat page without session ID (will create new session)
        const token = this.getJWTToken();
        const chatUrl = `chat.html?token=${encodeURIComponent(token)}`;
        window.location.href = chatUrl;
    }

    // UI State Management
    showLoading() {
        this.loading.style.display = 'flex';
        this.sessionsList.style.display = 'none';
        this.emptyState.style.display = 'none';
    }

    hideLoading() {
        this.loading.style.display = 'none';
        this.sessionsList.style.display = 'block';
    }

    showEmptyState() {
        this.emptyState.style.display = 'flex';
        this.sessionsList.style.display = 'none';
    }

    hideEmptyState() {
        this.emptyState.style.display = 'none';
        this.sessionsList.style.display = 'block';
    }

    showError(message) {
        this.statusText.textContent = message;
        this.statusBar.style.display = 'block';
        this.statusBar.style.background = '#f8d7da';
        this.statusBar.style.color = '#721c24';
    }

    hideError() {
        this.statusBar.style.display = 'none';
    }

    // Utility Functions
    formatTimestamp(timestamp) {
        if (!timestamp) return 'Unknown';

        const date = new Date(timestamp);
        const now = new Date();
        const diffMs = now - date;
        const diffMins = Math.floor(diffMs / 60000);
        const diffHours = Math.floor(diffMs / 3600000);
        const diffDays = Math.floor(diffMs / 86400000);

        // Less than 1 minute
        if (diffMins < 1) {
            return 'Just now';
        }
        
        // Less than 1 hour
        if (diffMins < 60) {
            return `${diffMins} min${diffMins > 1 ? 's' : ''} ago`;
        }
        
        // Less than 24 hours
        if (diffHours < 24) {
            return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`;
        }
        
        // Less than 7 days
        if (diffDays < 7) {
            return `${diffDays} day${diffDays > 1 ? 's' : ''} ago`;
        }
        
        // Format as date
        const options = { month: 'short', day: 'numeric' };
        if (date.getFullYear() !== now.getFullYear()) {
            options.year = 'numeric';
        }
        return date.toLocaleDateString(undefined, options);
    }

    getJWTToken() {
        // Try to get token from URL parameter
        const urlParams = new URLSearchParams(window.location.search);
        let token = urlParams.get('token');
        
        if (token) {
            // Store token for future use
            sessionStorage.setItem('jwt_token', token);
            return token;
        }
        
        // Try to get token from session storage
        token = sessionStorage.getItem('jwt_token');
        if (token) {
            return token;
        }
        
        // Try to get token from parent window (iframe/webview)
        if (window.parent !== window) {
            try {
                window.parent.postMessage({ type: 'request_token' }, '*');
            } catch (error) {
                console.error('Failed to request token from parent:', error);
            }
        }
        
        return null;
    }
}

// Global function for empty state button
function createNewSession() {
    if (sessionList) {
        sessionList.createNewSession();
    }
}

// Listen for token from parent window
window.addEventListener('message', (event) => {
    if (event.data.type === 'token') {
        sessionStorage.setItem('jwt_token', event.data.token);
        // Reload sessions if needed
        if (sessionList) {
            sessionList.loadSessions();
        }
    }
});

// Initialize session list when DOM is ready
let sessionList;
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        sessionList = new SessionList();
    });
} else {
    sessionList = new SessionList();
}
