import api from './api';

/**
 * Document service for handling document-related API calls
 */
const documentService = {
    /**
     * Upload a new document to the system
     * @param {File} file - The file to upload
     * @param {Object} options - Additional options like tags and metadata
     * @returns {Promise<Object>} The API response with file ID and status
     */
    uploadDocument(file, options = {}) {
        const formData = new FormData();
        formData.append('file', file);

        // Add optional parameters if provided
        if (options.tags) {
            formData.append('tags', options.tags);
        }

        if (options.metadata) {
            for (const [key, value] of Object.entries(options.metadata)) {
                formData.append(`metadata[${key}]`, value);
            }
        }

        return api.post('/documents', formData, {
            headers: {
                'Content-Type': 'multipart/form-data'
            }
        });
    },

    /**
     * Get a document's processing status
     * @param {string} fileId - The document ID
     * @returns {Promise<Object>} The API response with document status
     */
    getDocumentStatus(fileId) {
        return api.get(`/documents/${fileId}/status`);
    },

    /**
     * Get a list of all documents
     * @param {Object} params - Query parameters (page, pageSize, status, tags, etc.)
     * @returns {Promise<Object>} The API response with document list
     */
    listDocuments(params = {}) {
        return api.get('/documents', { params });
    },

    /**
     * Delete a document
     * @param {string} fileId - The document ID
     * @returns {Promise<Object>} The API response
     */
    deleteDocument(fileId) {
        return api.delete(`/documents/${fileId}`);
    },

    /**
     * Poll for document status until it's complete or fails
     * @param {string} fileId - The document ID
     * @param {number} interval - Polling interval in milliseconds
     * @param {number} timeout - Maximum polling time in milliseconds
     * @returns {Promise<Object>} The final document status
     */
    async pollDocumentStatus(fileId, interval = 2000, timeout = 300000) {
        const startTime = Date.now();

        // Continue polling until document is processed or timeout is reached
        while (Date.now() - startTime < timeout) {
            const status = await this.getDocumentStatus(fileId);

            // If document processing is complete or failed, return the status
            if (status.status === 'completed' || status.status === 'failed') {
                return status;
            }

            // Wait for the specified interval
            await new Promise(resolve => setTimeout(resolve, interval));
        }

        throw new Error('Document processing timed out');
    },

    /**
     * Get document URL for downloading (if your backend supports this)
     * @param {string} fileId - The document ID
     * @returns {string} The document download URL
     */
    getDocumentUrl(fileId) {
        // This assumes your API has a document download endpoint
        // Modify this according to your actual API design
        const baseUrl = process.env.VUE_APP_API_URL || '/api';
        return `${baseUrl}/documents/${fileId}/download`;
    }
};

export default documentService;