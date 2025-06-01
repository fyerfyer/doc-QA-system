import axios from 'axios';

// Create axios instance with default config
const api = axios.create({
    baseURL: process.env.VUE_APP_API_URL || '/api',
    timeout: 30000, // 30 second timeout
    headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
    }
});

// Request interceptor
api.interceptors.request.use(
    config => {
        // You can add auth token here if needed
        // const token = localStorage.getItem('token');
        // if (token) {
        //   config.headers.Authorization = `Bearer ${token}`;
        // }
        return config;
    },
    error => {
        return Promise.reject(error);
    }
);

// Response interceptor
api.interceptors.response.use(
    response => {
        // Extract data from standard API response wrapper
        const data = response.data;

        // Check if API returned success response
        if (data && data.code === 0) {
            return data.data;
        }

        // If API returned error code, convert to error
        return Promise.reject(new Error(data.message || 'Unknown error'));
    },
    error => {
        // Handle HTTP errors
        let message = 'Network error';
        if (error.response) {
            // Server responded with error
            const { data } = error.response;
            message = (data && data.message) || `Error: ${error.response.status}`;
        } else if (error.request) {
            // Request made but no response
            message = 'Server did not respond';
        }
        return Promise.reject(new Error(message));
    }
);

// Chat API endpoints
api.chat = {
    // Create a new chat session
    createChat: (title) => {
        return api.post('/chats', { title });
    },

    // Get chat history for a specific session
    getChatHistory: (sessionId, page = 1, pageSize = 50) => {
        return api.get(`/chats/${sessionId}`, { params: { page, page_size: pageSize } });
    },

    // List all chat sessions
    listChats: (page = 1, pageSize = 10, filters = {}) => {
        return api.get('/chats', {
            params: {
                page,
                page_size: pageSize,
                ...filters
            }
        });
    },

    // Add a message to a chat session
    addMessage: (sessionId, role, content, metadata = {}) => {
        return api.post('/chats/messages', {
            session_id: sessionId,
            role,
            content,
            metadata
        });
    },

    // Delete a chat session
    deleteChat: (sessionId) => {
        return api.delete(`/chats/${sessionId}`);
    },

    // Rename a chat session
    renameChat: (sessionId, title) => {
        return api.patch(`/chats/${sessionId}`, { title });
    },

    // Get recent questions
    getRecentQuestions: (limit = 10) => {
        return api.get('/recent-questions', { params: { limit } });
    },

    // Create a chat session with an initial message
    createChatWithMessage: (content, title = '') => {
        return api.post('/chats/with-message', {
            content,
            title
        });
    }
};

export default api;