<template>
  <div class="chat-view">
    <!-- Split View Layout -->
    <div class="chat-layout">
      <!-- Left Sidebar - Chat History -->
      <div class="chat-sidebar">
        <chat-history
          :active-session-id="currentSessionId"
          @select="selectChatSession"
          @active-deleted="handleActiveSessionDeleted"
        />
      </div>

      <!-- Right Content - Chat Messages -->
      <div class="chat-content">
        <div class="chat-header">
          <h1 class="chat-title">{{ chatTitle }}</h1>
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
          <div v-if="isLoading" class="chat-loading">
            <div class="loading-spinner"></div>
            <p>Loading conversation...</p>
          </div>

          <div v-else-if="messages.length === 0" class="empty-chat">
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
              v-for="(message, index) in messages"
              :key="index"
              :answer="formatMessageForDisplay(message)"
            />
          </div>
        </div>

        <div class="chat-input">
          <question-input
            :selected-document="selectedDocument"
            @answer-received="handleAnswerReceived"
            @error="handleError"
            @clear-document="clearSelectedDocument"
            :disabled="!currentSessionId && messages.length > 0"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import QuestionInput from '@/components/QuestionInput.vue'
import AnswerDisplay from '@/components/AnswerDisplay.vue'
import ChatHistory from '@/components/ChatHistory.vue'
import documentService from '@/services/documentService'
import chatService from '@/services/chatService'

export default {
  name: 'ChatView',
  components: {
    QuestionInput,
    AnswerDisplay,
    ChatHistory
  },
  data() {
    return {
      currentSessionId: null,
      chatTitle: 'Q&A Chat',
      messages: [],
      documents: [],
      selectedDocumentId: null,
      selectedDocument: null,
      isLoading: false,
      error: null,
    }
  },
  created() {
    this.fetchDocuments();
    // Check if there's a session ID in the URL
    if (this.$route.params.sessionId) {
      this.loadChatSession(this.$route.params.sessionId);
    }
  },
  methods: {
    async fetchDocuments() {
      this.isLoading = true;

      try {
        const response = await documentService.listDocuments();
        this.documents = response.documents || [];
      } catch (error) {
        console.error('Failed to fetch documents:', error);
        this.error = 'Failed to load documents: ' + (error.message || 'Unknown error');
      } finally {
        this.isLoading = false;
      }
    },

    handleDocumentSelect(documentId) {
      if (!documentId) {
        this.selectedDocument = null;
        return;
      }

      const document = this.documents.find(doc => doc.file_id === documentId);
      if (document) {
        this.selectedDocument = document;
      }
    },

    clearSelectedDocument() {
      this.selectedDocumentId = null;
      this.selectedDocument = null;
    },

    async selectChatSession(session) {
      if (session.id === this.currentSessionId) return;

      this.isLoading = true;
      try {
        await this.loadChatSession(session.id);
        // Update the URL to include the session ID
        this.$router.push({ name: 'chat', params: { sessionId: session.id } });
      } catch (error) {
        console.error('Failed to load chat session:', error);
      } finally {
        this.isLoading = false;
      }
    },

    async loadChatSession(sessionId) {
      this.isLoading = true;
      try {
        const response = await chatService.getChatHistory(sessionId);
        this.currentSessionId = sessionId;
        this.chatTitle = response.title || 'Chat';
        this.messages = chatService.formatChatMessages(response.messages || []);

        this.scrollToBottom();
      } catch (error) {
        console.error('Failed to load chat history:', error);
        this.currentSessionId = null;
        this.chatTitle = 'Q&A Chat';
        this.messages = [];
      } finally {
        this.isLoading = false;
      }
    },

    async createNewChat(question) {
      try {
        const response = await chatService.startNewChat(question);

        this.currentSessionId = response.session_id;
        this.chatTitle = response.title;

        // Initialize with the first message
        const initialMessage = {
          role: 'user',
          content: question,
          timestamp: new Date(),
          isUser: true
        };

        const assistantMessage = {
          role: 'assistant',
          content: response.message.content,
          timestamp: new Date(response.message.created_at),
          isUser: false,
          sources: response.message.sources || []
        };

        this.messages = [initialMessage, assistantMessage];

        // Update URL with new session ID
        this.$router.push({ name: 'chat', params: { sessionId: response.session_id } });

        // Scroll to show the new message
        this.scrollToBottom();

        return assistantMessage;
      } catch (error) {
        console.error('Failed to create new chat:', error);
        throw error;
      }
    },

    async sendMessageToCurrentSession(question) {
      try {
        // Add user message immediately for better UX
        this.messages.push({
          role: 'user',
          content: question,
          timestamp: new Date(),
          isUser: true
        });

        // Send the message to the server
        const response = await chatService.sendMessage(this.currentSessionId, question);

        // Add the assistant's response
        const assistantMessage = {
          role: 'assistant',
          content: response.assistant_message.content,
          timestamp: new Date(response.assistant_message.created_at),
          isUser: false,
          sources: response.assistant_message.sources || []
        };

        this.messages.push(assistantMessage);
        this.scrollToBottom();

        return assistantMessage;
      } catch (error) {
        console.error('Failed to send message:', error);
        throw error;
      }
    },

    async handleAnswerReceived(answerData) {
      try {
        let response;

        if (!this.currentSessionId) {
          // Create a new chat session
          response = await this.createNewChat(answerData.question);
        } else {
          // Send to existing chat session
          response = await this.sendMessageToCurrentSession(answerData.question);
        }

        // Scroll to bottom after the DOM update
        this.$nextTick(() => {
          this.scrollToBottom();
        });
      } catch (error) {
        this.handleError(error.message || 'Failed to process question');
      }
    },

    handleError(errorMessage) {
      this.error = errorMessage;

      // Add error to messages
      this.messages.push({
        role: 'system',
        content: 'Error: ' + errorMessage,
        timestamp: new Date(),
        isUser: false,
        isError: true
      });

      this.scrollToBottom();
    },

    scrollToBottom() {
      this.$nextTick(() => {
        const container = this.$refs.chatContainer;
        if (container) {
          container.scrollTop = container.scrollHeight;
        }
      });
    },

    formatMessageForDisplay(message) {
      return {
        question: message.isUser ? message.content : '',
        answer: !message.isUser ? message.content : '',
        sources: message.sources || [],
        error: message.isError ? message.content : null
      };
    },

    handleActiveSessionDeleted() {
      // Reset the chat view when the active session is deleted
      this.currentSessionId = null;
      this.chatTitle = 'Q&A Chat';
      this.messages = [];
      this.$router.push({ name: 'chat' });
    }
  }
}
</script>

<style scoped>
.chat-view {
  height: calc(100vh - 80px);
}

.chat-layout {
  display: flex;
  height: 100%;
}

.chat-sidebar {
  width: 300px;
  border-right: 1px solid #eaeaea;
  height: 100%;
  overflow: hidden;
}

.chat-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 0 20px;
}

.chat-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 0;
  margin-bottom: 10px;
}

.chat-title {
  font-size: 24px;
  color: #333;
  margin: 0;
  max-width: 60%;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
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

.chat-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: #666;
}

.loading-spinner {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  border: 3px solid #f3f3f3;
  border-top-color: #3498db;
  animation: spin 1s linear infinite;
  margin-bottom: 16px;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
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
  padding-bottom: 20px;
}
</style>