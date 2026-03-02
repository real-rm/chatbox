// Shared mock data for E2E tests

export const TEST_JWT_TOKEN = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoidGVzdC11c2VyLTEiLCJuYW1lIjoiVGVzdCBVc2VyIiwicm9sZXMiOlsidXNlciJdLCJleHAiOjk5OTk5OTk5OTl9.placeholder';

export const TEST_ADMIN_TOKEN = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiYWRtaW4tMSIsIm5hbWUiOiJUZXN0IEFkbWluIiwicm9sZXMiOlsiYWRtaW4iXSwiZXhwIjo5OTk5OTk5OTk5fQ.placeholder';

export const MOCK_SESSIONS = {
  user_id: 'test-user-1',
  sessions: [
    {
      id: 'session-001',
      name: 'Bug report discussion',
      start_time: '2026-03-01T10:00:00Z',
      last_activity: '2026-03-01T10:30:00Z',
      message_count: 12,
      admin_assisted: false,
    },
    {
      id: 'session-002',
      name: 'Feature request',
      start_time: '2026-03-02T14:00:00Z',
      last_activity: '2026-03-02T14:45:00Z',
      message_count: 8,
      admin_assisted: true,
    },
    {
      id: 'session-003',
      name: null, // Untitled
      start_time: '2026-02-28T09:00:00Z',
      last_activity: '2026-02-28T09:15:00Z',
      message_count: 3,
      admin_assisted: false,
    },
  ],
};

export const MOCK_EMPTY_SESSIONS = {
  user_id: 'test-user-1',
  sessions: [],
};

export const MOCK_ADMIN_SESSIONS = [
  {
    user_id: 'user-100',
    session_id: 'sess-aabbccdd-1234-5678-9012-abcdef123456',
    is_active: true,
    start_time: '2026-03-02T10:00:00Z',
    duration: 1800,
    last_activity: '2026-03-02T10:30:00Z',
    total_tokens: 4500,
    avg_response_time: 850,
    max_response_time: 2100,
    admin_assisted: false,
    assisting_admin_name: null,
  },
  {
    user_id: 'user-200',
    session_id: 'sess-11223344-5566-7788-9900-aabbccddeeff',
    is_active: false,
    start_time: '2026-03-01T08:00:00Z',
    duration: 3600,
    last_activity: '2026-03-01T09:00:00Z',
    total_tokens: 12000,
    avg_response_time: 1200,
    max_response_time: 3500,
    admin_assisted: true,
    assisting_admin_name: 'Admin Jane',
  },
];

export const MOCK_ADMIN_METRICS = {
  active_sessions: 5,
  total_sessions: 142,
  avg_concurrent: 3.2,
  total_tokens: 580000,
};

export const MOCK_CONNECTION_STATUS = {
  type: 'connection_status',
  session_id: 'session-new-001',
  user_id: 'test-user-1',
  models: [
    { id: 'gpt-4', name: 'GPT-4' },
    { id: 'claude-3', name: 'Claude 3' },
  ],
};

export const MOCK_AI_RESPONSE = {
  type: 'ai_response',
  content: 'Hello! How can I help you today?',
  sender: 'ai',
  timestamp: '2026-03-02T15:00:01Z',
};
