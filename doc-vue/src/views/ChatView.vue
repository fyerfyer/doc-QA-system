<template>
  <div class="chat-view">
    <div class="chat-header">
      <h1 class="chat-title">Document Q&A</h1>
      <div class="document-selector" v-if="documents.length > 0">
        <el-select
          v-model="selectedDocumentId"
          placeholder="Ask about a specific document..."
          clearable
          @change="handleDocumentSelect"
        >
          <el-option
            v-for="doc in documents"
            :key="doc.file_id"
            :label="doc.filename"
            :value="doc.file_id"
            :disabled="doc.status !== 'completed'"
          >
            <span>{{ doc.filename }}</span>
            <span v-if="doc.status !== 'completed'" class="document-status">
              ({{ doc.status }})
            </span>
          </el-option>
        </el-select>
      </div>
    </div>

    <div class="chat-container" ref="chatContainer">
      <div v-if="chatHistory.length === 0" class="empty-chat">
        <div class="empty-chat-content">
          <div class="empty-chat-icon">💬</div>
          <p class="empty-chat-title">Ask questions about your documents</p>
          <p class="empty-chat-subtitle">
            Upload documents in the Documents section and ask questions here.
            The AI will search through your documents to provide relevant answers.
          </p>
        </div>
      </div>

      <div v-else class="chat-messages">
        <answer-display
          v-for="(item, index) in chatHistory"
          :key="index"
          :answer="item"
        />
      </div>
    </div>

    <div class="chat-input">
      <question-input
        :selected-document="selectedDocument"
        @answer-received="handleAnswerReceived"
        @error="handleError"
        @clear-document="clearSelectedDocument"
      />
    </div>
  </div>
</template>

<script>
import QuestionInput from '@/components/QuestionInput.vue'
import AnswerDisplay from '@/components/AnswerDisplay.vue'
import documentService from '@/services/documentService'

export default {
  name: 'ChatView',
  components: {
    QuestionInput,
    AnswerDisplay
  },
  data() {
    return {
      chatHistory: [],
      documents: [],
      selectedDocumentId: null,
      selectedDocument: null,
      isLoading: false,
      error: null
    }
  },
  created() {
    this.fetchDocuments()
  },
  methods: {
    async fetchDocuments() {
      this.isLoading = true

      try {
        const response = await documentService.listDocuments()
        this.documents = response.documents || []
      } catch (error) {
        console.error('Failed to fetch documents:', error)
        this.error = 'Failed to load documents: ' + (error.message || 'Unknown error')
      } finally {
        this.isLoading = false
      }
    },

    handleDocumentSelect(documentId) {
      if (!documentId) {
        this.selectedDocument = null
        return
      }

      const document = this.documents.find(doc => doc.file_id === documentId)
      if (document) {
        this.selectedDocument = document
      }
    },

    clearSelectedDocument() {
      this.selectedDocumentId = null
      this.selectedDocument = null
    },

    handleAnswerReceived(answerData) {
      // Add the new Q&A to chat history
      this.chatHistory.push(answerData)

      // Scroll to bottom after the DOM update
      this.$nextTick(() => {
        this.scrollToBottom()
      })
    },

    handleError(errorMessage) {
      this.error = errorMessage

      // Add error to chat history
      this.chatHistory.push({
        question: 'Last question failed',
        error: errorMessage
      })
    },

    scrollToBottom() {
      const container = this.$refs.chatContainer
      if (container) {
        container.scrollTop = container.scrollHeight
      }
    }
  }
}
</script>

<style scoped>
.chat-view {
  max-width: 800px;
  margin: 0 auto;
  padding: 20px;
  display: flex;
  flex-direction: column;
  height: calc(100vh - 80px);
}

.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.chat-title {
  font-size: 24px;
  color: #333;
  margin: 0;
}

.document-selector {
  width: 300px;
}

.document-status {
  color: #999;
  font-style: italic;
  margin-left: 8px;
}

.chat-container {
  flex: 1;
  overflow-y: auto;
  padding: 10px 0;
  margin-bottom: 20px;
}

.empty-chat {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-chat-content {
  text-align: center;
  padding: 40px;
  max-width: 500px;
}

.empty-chat-icon {
  font-size: 48px;
  margin-bottom: 20px;
}

.empty-chat-title {
  font-size: 22px;
  font-weight: 500;
  color: #333;
  margin-bottom: 12px;
}

.empty-chat-subtitle {
  font-size: 16px;
  color: #666;
  line-height: 1.5;
}

.chat-messages {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.chat-input {
  margin-top: auto;
}
</style>