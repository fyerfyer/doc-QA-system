import api from './api';

/**
 * QA service for handling question answering API calls
 */
const qaService = {
    /**
     * Ask a question and get an answer
     * @param {string} question - The question to ask
     * @param {Object} options - Additional options like maxTokens
     * @returns {Promise<Object>} The API response with answer and sources
     */
    askQuestion(question, options = {}) {
        const payload = {
            question: question,
            max_tokens: options.maxTokens || undefined
        };

        return api.post('/qa', payload);
    },

    /**
     * Ask a question about a specific document
     * @param {string} question - The question to ask
     * @param {string} fileId - ID of the document to query
     * @param {Object} options - Additional options like maxTokens
     * @returns {Promise<Object>} The API response with answer and sources
     */
    askQuestionAboutDocument(question, fileId, options = {}) {
        const payload = {
            question: question,
            file_id: fileId,
            max_tokens: options.maxTokens || undefined
        };

        return api.post('/qa', payload);
    },

    /**
     * Ask a question with metadata filters
     * @param {string} question - The question to ask
     * @param {Object} metadata - Metadata filters to apply
     * @param {Object} options - Additional options like maxTokens
     * @returns {Promise<Object>} The API response with answer and sources
     */
    askQuestionWithMetadata(question, metadata, options = {}) {
        const payload = {
            question: question,
            metadata: metadata,
            max_tokens: options.maxTokens || undefined
        };

        return api.post('/qa', payload);
    },

    /**
     * Check if a backend connection is available
     * @returns {Promise<boolean>} True if connection is successful
     */
    async checkConnection() {
        try {
            await api.get('/health');
            return true;
        } catch (error) {
            console.error('API connection check failed', error);
            return false;
        }
    }
};

export default qaService;