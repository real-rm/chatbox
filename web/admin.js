// Admin Dashboard JavaScript

// Configuration
const API_BASE_URL = window.location.origin;
const AUTO_REFRESH_INTERVAL = 10000; // 10 seconds

// State
let autoRefreshEnabled = true;
let autoRefreshTimer = null;
let currentFilters = {
    userID: '',
    dateStart: '',
    dateEnd: '',
    status: '',
    adminAssisted: ''
};
let currentSort = {
    field: 'start_time',
    order: 'desc'
};

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    initializeEventListeners();
    loadData();
    startAutoRefresh();
});

// Event Listeners
function initializeEventListeners() {
    document.getElementById('refresh-btn').addEventListener('click', () => loadData());
    document.getElementById('apply-filters-btn').addEventListener('click', applyFilters);
    document.getElementById('clear-filters-btn').addEventListener('click', clearFilters);
    document.getElementById('sort-field').addEventListener('change', () => loadData());
    document.getElementById('sort-order').addEventListener('change', () => loadData());
    
    // Auto-refresh toggle
    document.getElementById('auto-refresh-status').addEventListener('click', toggleAutoRefresh);
}

// Load Data
async function loadData() {
    showLoading();
    
    try {
        // Load metrics and sessions in parallel
        await Promise.all([
            loadMetrics(),
            loadSessions()
        ]);
        
        hideLoading();
    } catch (error) {
        console.error('Error loading data:', error);
        hideLoading();
        showError('Failed to load data');
    }
}

// Load Metrics
async function loadMetrics() {
    try {
        const params = new URLSearchParams();
        if (currentFilters.dateStart) params.append('start_time', currentFilters.dateStart);
        if (currentFilters.dateEnd) params.append('end_time', currentFilters.dateEnd);
        
        const response = await fetch(`${API_BASE_URL}/chat/admin/metrics?${params}`, {
            headers: {
                'Authorization': `Bearer ${getJWTToken()}`
            }
        });
        
        if (!response.ok) {
            throw new Error('Failed to fetch metrics');
        }
        
        const metrics = await response.json();
        updateMetricsDisplay(metrics);
    } catch (error) {
        console.error('Error loading metrics:', error);
    }
}

// Load Sessions
async function loadSessions() {
    try {
        const params = new URLSearchParams();
        
        // Apply filters
        if (currentFilters.userID) params.append('user_id', currentFilters.userID);
        if (currentFilters.dateStart) params.append('start_time', currentFilters.dateStart);
        if (currentFilters.dateEnd) params.append('end_time', currentFilters.dateEnd);
        if (currentFilters.status) params.append('status', currentFilters.status);
        if (currentFilters.adminAssisted) params.append('admin_assisted', currentFilters.adminAssisted);
        
        // Apply sorting
        params.append('sort_by', currentSort.field);
        params.append('sort_order', currentSort.order);
        
        const response = await fetch(`${API_BASE_URL}/chat/admin/sessions?${params}`, {
            headers: {
                'Authorization': `Bearer ${getJWTToken()}`
            }
        });
        
        if (!response.ok) {
            throw new Error('Failed to fetch sessions');
        }
        
        const sessions = await response.json();
        updateSessionsDisplay(sessions);
    } catch (error) {
        console.error('Error loading sessions:', error);
    }
}

// Update Metrics Display
function updateMetricsDisplay(metrics) {
    document.getElementById('active-sessions').textContent = metrics.active_sessions || 0;
    document.getElementById('total-sessions').textContent = metrics.total_sessions || 0;
    document.getElementById('avg-concurrent').textContent = 
        metrics.avg_concurrent ? metrics.avg_concurrent.toFixed(1) : '0.0';
    document.getElementById('total-tokens').textContent = 
        metrics.total_tokens ? formatNumber(metrics.total_tokens) : '0';
}

// Update Sessions Display
function updateSessionsDisplay(sessions) {
    const tbody = document.getElementById('sessions-tbody');
    const emptyState = document.getElementById('empty-state');
    const container = document.getElementById('sessions-container');
    
    if (!sessions || sessions.length === 0) {
        container.style.display = 'none';
        emptyState.style.display = 'block';
        return;
    }
    
    container.style.display = 'block';
    emptyState.style.display = 'none';
    
    tbody.innerHTML = sessions.map(session => `
        <tr>
            <td>
                <span class="user-link" onclick="showUserSessions('${session.user_id}')">
                    ${escapeHtml(session.user_id)}
                </span>
            </td>
            <td>${escapeHtml(session.session_id.substring(0, 8))}...</td>
            <td>
                <span class="status-badge ${session.is_active ? 'active' : 'ended'}">
                    ${session.is_active ? 'Active' : 'Ended'}
                </span>
            </td>
            <td>${formatDateTime(session.start_time)}</td>
            <td>${formatDuration(session.duration)}</td>
            <td>${formatDateTime(session.last_activity)}</td>
            <td>${formatNumber(session.total_tokens || 0)}</td>
            <td>${formatResponseTime(session.avg_response_time)}</td>
            <td>${formatResponseTime(session.max_response_time)}</td>
            <td>
                ${session.admin_assisted ? 
                    `<span class="admin-badge">${escapeHtml(session.assisting_admin_name || 'Admin')}</span>` : 
                    '-'}
            </td>
            <td>
                ${session.is_active ? 
                    `<button class="btn-small" onclick="takeoverSession('${session.session_id}')">Takeover</button>` : 
                    '-'}
            </td>
        </tr>
    `).join('');
}

// Show User Sessions Modal
async function showUserSessions(userID) {
    try {
        const response = await fetch(`${API_BASE_URL}/chat/admin/users/${userID}/sessions`, {
            headers: {
                'Authorization': `Bearer ${getJWTToken()}`
            }
        });
        
        if (!response.ok) {
            throw new Error('Failed to fetch user sessions');
        }
        
        const sessions = await response.json();
        displayUserSessionsModal(userID, sessions);
    } catch (error) {
        console.error('Error loading user sessions:', error);
        showError('Failed to load user sessions');
    }
}

// Display User Sessions Modal
function displayUserSessionsModal(userID, sessions) {
    const modal = document.getElementById('user-sessions-modal');
    const modalUserID = document.getElementById('modal-user-id');
    const tbody = document.getElementById('user-sessions-tbody');
    
    modalUserID.textContent = userID;
    
    tbody.innerHTML = sessions.map(session => `
        <tr>
            <td>${escapeHtml(session.session_id.substring(0, 8))}...</td>
            <td>${escapeHtml(session.name || 'Untitled')}</td>
            <td>
                <span class="status-badge ${session.is_active ? 'active' : 'ended'}">
                    ${session.is_active ? 'Active' : 'Ended'}
                </span>
            </td>
            <td>${formatDateTime(session.start_time)}</td>
            <td>${session.message_count || 0}</td>
            <td>
                ${session.is_active ? 
                    `<button class="btn-small" onclick="takeoverSession('${session.session_id}')">Takeover</button>` : 
                    '-'}
            </td>
        </tr>
    `).join('');
    
    modal.style.display = 'flex';
}

// Close User Sessions Modal
function closeUserSessionsModal() {
    document.getElementById('user-sessions-modal').style.display = 'none';
}

// Takeover Session
async function takeoverSession(sessionID) {
    if (!confirm('Are you sure you want to take over this session?')) {
        return;
    }
    
    try {
        const response = await fetch(`${API_BASE_URL}/chat/admin/takeover/${sessionID}`, {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${getJWTToken()}`,
                'Content-Type': 'application/json'
            }
        });
        
        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.message || 'Failed to takeover session');
        }
        
        const result = await response.json();
        
        // Open chat window with admin context
        const token = getJWTToken();
        const chatUrl = `chat.html?session_id=${sessionID}&admin=true&token=${encodeURIComponent(token)}`;
        window.open(chatUrl, '_blank', 'width=800,height=600');
        
        // Close modal if open
        closeUserSessionsModal();
        
        // Refresh data after a short delay
        setTimeout(() => {
            loadData();
        }, 1000);
    } catch (error) {
        console.error('Error taking over session:', error);
        showError(error.message || 'Failed to takeover session');
    }
}

// Apply Filters
function applyFilters() {
    currentFilters.userID = document.getElementById('filter-user-id').value.trim();
    currentFilters.dateStart = document.getElementById('filter-date-start').value;
    currentFilters.dateEnd = document.getElementById('filter-date-end').value;
    currentFilters.status = document.getElementById('filter-status').value;
    currentFilters.adminAssisted = document.getElementById('filter-admin').value;
    
    currentSort.field = document.getElementById('sort-field').value;
    currentSort.order = document.getElementById('sort-order').value;
    
    loadData();
}

// Clear Filters
function clearFilters() {
    document.getElementById('filter-user-id').value = '';
    document.getElementById('filter-date-start').value = '';
    document.getElementById('filter-date-end').value = '';
    document.getElementById('filter-status').value = '';
    document.getElementById('filter-admin').value = '';
    
    currentFilters = {
        userID: '',
        dateStart: '',
        dateEnd: '',
        status: '',
        adminAssisted: ''
    };
    
    loadData();
}

// Auto Refresh
function startAutoRefresh() {
    if (autoRefreshTimer) {
        clearInterval(autoRefreshTimer);
    }
    
    if (autoRefreshEnabled) {
        autoRefreshTimer = setInterval(() => {
            loadData();
        }, AUTO_REFRESH_INTERVAL);
    }
}

function toggleAutoRefresh() {
    autoRefreshEnabled = !autoRefreshEnabled;
    const statusElement = document.getElementById('auto-refresh-status');
    statusElement.innerHTML = `Auto-refresh: <strong>${autoRefreshEnabled ? 'ON' : 'OFF'}</strong>`;
    
    if (autoRefreshEnabled) {
        startAutoRefresh();
    } else {
        if (autoRefreshTimer) {
            clearInterval(autoRefreshTimer);
            autoRefreshTimer = null;
        }
    }
}

// Utility Functions
function getJWTToken() {
    // Get JWT token from URL parameter or localStorage
    const urlParams = new URLSearchParams(window.location.search);
    const token = urlParams.get('token') || localStorage.getItem('jwt_token');
    
    if (!token) {
        console.error('No JWT token found');
        showError('Authentication required');
    }
    
    return token;
}

function formatDateTime(dateString) {
    if (!dateString) return '-';
    const date = new Date(dateString);
    return date.toLocaleString();
}

function formatDuration(seconds) {
    if (!seconds) return '-';
    
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;
    
    if (hours > 0) {
        return `${hours}h ${minutes}m`;
    } else if (minutes > 0) {
        return `${minutes}m ${secs}s`;
    } else {
        return `${secs}s`;
    }
}

function formatResponseTime(milliseconds) {
    if (!milliseconds) return '-';
    
    if (milliseconds < 1000) {
        return `${milliseconds}ms`;
    } else {
        return `${(milliseconds / 1000).toFixed(2)}s`;
    }
}

function formatNumber(num) {
    if (!num) return '0';
    return num.toLocaleString();
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function showLoading() {
    document.getElementById('loading').style.display = 'block';
    document.getElementById('sessions-container').style.display = 'none';
    document.getElementById('empty-state').style.display = 'none';
}

function hideLoading() {
    document.getElementById('loading').style.display = 'none';
}

function showError(message) {
    alert(message); // Simple error display, can be enhanced with a toast notification
}

// Export functions for onclick handlers
window.showUserSessions = showUserSessions;
window.closeUserSessionsModal = closeUserSessionsModal;
window.takeoverSession = takeoverSession;
