import api from './api';

/**
 * Chat service for managing chat conversations with the document QA system
 */
const chatService = {
    /**
     * Create a new chat session
     * @param {string} title - Optional title for the chat session
     * @returns {Promise<Object>} The created chat session
     */
    createChat(title = '') {
        return api.chat.createChat(title);
    },

    /**
     * Get chat history for a specific session
     * @param {string} sessionId - The chat session ID
     * @param {number} page - Page number for pagination
     * @param {number} pageSize - Number of messages per page
     * @returns {Promise<Object>} The chat history with messages
     */
    getChatHistory(sessionId, page = 1, pageSize = 50) {
        return api.chat.getChatHistory(sessionId, page, pageSize);
    },

    /**
     * List all chat sessions
     * @param {number} page - Page number for pagination
     * @param {number} pageSize - Number of chats per page
     * @param {Object} filters - Optional filters like tags, start_time, end_time
     * @returns {Promise<Object>} List of chat sessions with pagination info
     */
    listChats(page = 1, pageSize = 10, filters = {}) {
        return api.chat.listChats(page, pageSize, filters);
    },

    /**
     * Add a message to an existing chat session
     * @param {string} sessionId - The chat session ID
     * @param {string} content - The message content
     * @param {Object} metadata - Optional metadata for the message
     * @returns {Promise<Object>} Response with user and assistant messages
     */
    async sendMessage(sessionId, content, metadata = {}) {
        const response = await api.chat.addMessage(sessionId, 'user', content, metadata);
        return response;
    },

    /**
     * Delete a chat session
     * @param {string} sessionId - The chat session ID to delete
     * @returns {Promise<Object>} Response indicating success
     */
    deleteChat(sessionId) {
        return api.chat.deleteChat(sessionId);
    },

    /**
     * Rename a chat session
     * @param {string} sessionId - The chat session ID
     * @param {string} newTitle - The new title for the chat
     * @returns {Promise<Object>} Updated chat session info
     */
    renameChat(sessionId, newTitle) {
        return api.chat.renameChat(sessionId, newTitle);
    },

    /**
     * Get recent questions for suggestions
     * @param {number} limit - Maximum number of questions to return
     * @returns {Promise<Array<string>>} List of recent questions
     */
    getRecentQuestions(limit = 10) {
        return api.chat.getRecentQuestions(limit);
    },

    /**
     * Create a new chat and immediately add the first message
     * @param {string} content - The first message content
     * @param {string} title - Optional title for the chat
     * @returns {Promise<Object>} The created chat with first message and response
     */
    startNewChat(content, title = '') {
        return api.chat.createChatWithMessage(content, title);
    },

    /**
     * Format chat messages for display
     * @param {Array} messages - Raw messages from API
     * @returns {Array} Formatted messages for UI
     */
    formatChatMessages(messages) {
        return messages.map(msg => ({
            id: msg.id,
            role: msg.role,
            content: msg.content,
            timestamp: new Date(msg.created_at),
            isUser: msg.role === 'user',
            sources: msg.sources || [],
            hasReferences: Array.isArray(msg.sources) && msg.sources.length > 0
        }));
    },

    /**
     * Generate a default chat title from the first message
     * @param {string} firstMessage - The first message in the chat
     * @returns {string} A generated title
     */
    generateChatTitle(firstMessage) {
        // Create a title from the first few words of the message
        const maxLength = 30;
        let title = firstMessage.trim();

        if (title.length <= maxLength) {
            return title;
        }

        // Try to find a good breakpoint for the title
        const breakAt = title.lastIndexOf(' ', maxLength);
        if (breakAt > 0) {
            title = title.substring(0, breakAt) + '...';
        } else {
            title = title.substring(0, maxLength) + '...';
        }

        return title;
    }
};

export default chatService;