<template>
  <div class="documents-view">
    <div class="documents-header">
      <h1 class="documents-title">Document Management</h1>
      <div class="documents-actions">
        <button class="refresh-button" @click="fetchDocuments" :disabled="isLoading">
          <span v-if="!isLoading">Refresh</span>
          <span v-else>Loading...</span>
        </button>
      </div>
    </div>

    <document-uploader @upload-complete="handleUploadComplete" />

    <div class="documents-list-section">
      <h2 class="documents-section-title">Your Documents</h2>

      <div v-if="isLoading" class="documents-loading">
        Loading documents...
      </div>

      <div v-else-if="documents.length === 0" class="documents-empty">
        <p>No documents uploaded yet.</p>
        <p class="documents-empty-hint">Use the upload area above to add your first document.</p>
      </div>

      <div v-else class="documents-list">
        <document-item
          v-for="doc in documents"
          :key="doc.file_id"
          :document="doc"
          @delete="handleDocumentDelete"
          @select="handleDocumentSelect"
        />
      </div>
    </div>
  </div>
</template>

<script>
import DocumentItem from '@/components/DocumentItem.vue'
import DocumentUploader from '@/components/DocumentUploader.vue'
import documentService from '@/services/documentService'

export default {
  name: 'DocumentsView',
  components: {
    DocumentItem,
    DocumentUploader
  },
  data() {
    return {
      documents: [],
      isLoading: false,
      error: null,
      pollingMap: new Map() // Map to track polling for processing documents
    }
  },
  created() {
    // Fetch documents when component is created
    this.fetchDocuments()
  },
  beforeUnmount() {
    // Clear all polling timers when component is unmounted
    for (const timer of this.pollingMap.values()) {
      clearInterval(timer)
    }
  },
  methods: {
    async fetchDocuments() {
      this.isLoading = true
      this.error = null

      try {
        const response = await documentService.listDocuments()
        this.documents = response.documents || []

        // Start polling for any processing documents
        this.documents.forEach(doc => {
          if (doc.status === 'processing') {
            this.startPollingStatus(doc.file_id)
          }
        })
      } catch (error) {
        console.error('Failed to fetch documents:', error)
        this.error = 'Failed to load documents: ' + (error.message || 'Unknown error')
      } finally {
        this.isLoading = false
      }
    },

    async handleUploadComplete(response) {
      // Add the new document to the list with processing status
      const newDoc = {
        file_id: response.file_id,
        filename: response.filename,
        status: 'processing',
        upload_time: new Date().toISOString()
      }

      this.documents.unshift(newDoc)

      // Start polling for status updates
      this.startPollingStatus(response.file_id)
    },

    async handleDocumentDelete(fileId) {
      try {
        // Stop polling if active
        this.stopPollingStatus(fileId)

        // Call API to delete document
        await documentService.deleteDocument(fileId)

        // Remove from local list
        this.documents = this.documents.filter(doc => doc.file_id !== fileId)
      } catch (error) {
        console.error('Failed to delete document:', error)
        alert('Failed to delete document: ' + (error.message || 'Unknown error'))
      }
    },

    handleDocumentSelect(document) {
      // Emit event that could be handled by parent component or router
      this.$emit('select-document', document)

      // Alternative: Navigate to chat view with the selected document
      // this.$router.push({
      //   name: 'chat',
      //   params: { documentId: document.file_id }
      // })
    },

    startPollingStatus(fileId) {
      // Stop existing polling if any
      this.stopPollingStatus(fileId)

      // Start polling every 3 seconds
      const timerId = setInterval(async () => {
        try {
          const status = await documentService.getDocumentStatus(fileId)

          // Update document in the list
          const index = this.documents.findIndex(d => d.file_id === fileId)
          if (index !== -1) {
            this.documents[index] = {
              ...this.documents[index],
              ...status
            }

            // If processing is complete or failed, stop polling
            if (status.status === 'completed' || status.status === 'failed') {
              this.stopPollingStatus(fileId)
            }
          } else {
            // Document not found in list, stop polling
            this.stopPollingStatus(fileId)
          }
        } catch (error) {
          console.error(`Error polling status for document ${fileId}:`, error)
          this.stopPollingStatus(fileId)
        }
      }, 3000) // Poll every 3 seconds

      this.pollingMap.set(fileId, timerId)
    },

    stopPollingStatus(fileId) {
      const timerId = this.pollingMap.get(fileId)
      if (timerId) {
        clearInterval(timerId)
        this.pollingMap.delete(fileId)
      }
    }
  }
}
</script>

<style scoped>
.documents-view {
  max-width: 800px;
  margin: 0 auto;
  padding: 20px;
}

.documents-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.documents-title {
  font-size: 24px;
  color: #333;
  margin: 0;
}

.documents-section-title {
  font-size: 20px;
  color: #333;
  margin: 32px 0 16px 0;
}

.refresh-button {
  padding: 8px 16px;
  background-color: #f0f0f0;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 14px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.refresh-button:hover:not(:disabled) {
  background-color: #e0e0e0;
}

.refresh-button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.documents-list-section {
  margin-top: 24px;
}

.documents-loading {
  text-align: center;
  padding: 40px;
  color: #666;
  font-style: italic;
}

.documents-empty {
  text-align: center;
  padding: 40px;
  background-color: #f9f9f9;
  border-radius: 8px;
  color: #666;
}

.documents-empty-hint {
  margin-top: 8px;
  font-size: 14px;
  color: #999;
}

.documents-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>