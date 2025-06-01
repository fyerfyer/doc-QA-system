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
    },

    /**
     * Get document metrics by status
     * @returns {Promise<Object>} Document counts by status
     */
    getDocumentMetrics() {
        return api.get('/documents/metrics');
    },

    /**
     * Get document statistics
     * @returns {Promise<Object>} Statistical information about documents
     */
    async getDocumentStatistics() {
        // Get all documents first - we'll calculate stats client-side
        const response = await this.listDocuments({ page_size: 100 });
        const documents = response.documents || [];

        if (documents.length === 0) {
            return {
                totalCount: 0,
                totalSize: 0,
                averageSize: 0,
                totalSegments: 0,
                averageSegments: 0,
                processingTimes: {
                    min: 0,
                    max: 0,
                    average: 0
                },
                statusCounts: {
                    uploaded: 0,
                    processing: 0,
                    completed: 0,
                    failed: 0
                }
            };
        }

        // Calculate statistics
        let totalSize = 0;
        let totalSegments = 0;
        let processingTimes = [];
        const statusCounts = {
            uploaded: 0,
            processing: 0,
            completed: 0,
            failed: 0
        };

        documents.forEach(doc => {
            // Count by status
            statusCounts[doc.status] = (statusCounts[doc.status] || 0) + 1;

            // Sum sizes
            if (doc.size) {
                totalSize += doc.size;
            }

            // Sum segments
            if (doc.segments) {
                totalSegments += doc.segments;
            }

            // Calculate processing time for completed documents
            if (doc.status === 'completed' && doc.upload_time) {
                const uploadTime = new Date(doc.upload_time);
                const processedTime = doc.updated_at ? new Date(doc.updated_at) : new Date();
                const processTime = (processedTime - uploadTime) / 1000; // in seconds
                processingTimes.push(processTime);
            }
        });

        // Calculate averages
        const totalCount = documents.length;
        const averageSize = totalCount > 0 ? totalSize / totalCount : 0;
        const averageSegments = totalCount > 0 ? totalSegments / totalCount : 0;

        // Calculate processing time stats
        let minProcessingTime = 0;
        let maxProcessingTime = 0;
        let averageProcessingTime = 0;

        if (processingTimes.length > 0) {
            minProcessingTime = Math.min(...processingTimes);
            maxProcessingTime = Math.max(...processingTimes);
            averageProcessingTime = processingTimes.reduce((a, b) => a + b, 0) / processingTimes.length;
        }

        return {
            totalCount,
            totalSize,
            averageSize,
            totalSegments,
            averageSegments,
            processingTimes: {
                min: minProcessingTime,
                max: maxProcessingTime,
                average: averageProcessingTime
            },
            statusCounts
        };
    },

    /**
     * Get document processing status distribution
     * @returns {Promise<Array>} Data formatted for charts
     */
    async getStatusDistribution() {
        const metrics = await this.getDocumentMetrics();

        return [
            { name: 'Uploaded', value: metrics.uploaded || 0 },
            { name: 'Processing', value: metrics.processing || 0 },
            { name: 'Completed', value: metrics.completed || 0 },
            { name: 'Failed', value: metrics.failed || 0 }
        ];
    },

    /**
     * Format file size for display
     * @param {number} bytes - File size in bytes
     * @returns {string} Formatted file size
     */
    formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';

        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));

        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }
};

export default documentService;