<template>
  <div class="analytics-view">
    <div class="analytics-header">
      <h1 class="analytics-title">System Analytics</h1>
      <div class="analytics-actions">
        <button class="refresh-button" @click="refreshData" :disabled="isLoading">
          <i class="el-icon-refresh"></i>
          <span v-if="!isLoading">Refresh</span>
          <span v-else>Loading...</span>
        </button>
      </div>
    </div>

    <div class="analytics-sections">
      <!-- Document Analytics Component -->
      <document-analytics ref="documentAnalytics" />

      <!-- System Overview Card -->
      <div class="analytics-card system-overview">
        <h3 class="analytics-card__title">System Overview</h3>
        <div v-if="isLoading" class="analytics-loading">
          Loading system information...
        </div>
        <div v-else class="overview-stats">
          <div class="overview-stat">
            <div class="overview-stat__label">Total Documents</div>
            <div class="overview-stat__value">{{ stats.documentCount }}</div>
          </div>
          <div class="overview-stat">
            <div class="overview-stat__label">Total Chats</div>
            <div class="overview-stat__value">{{ stats.chatCount }}</div>
          </div>
          <div class="overview-stat">
            <div class="overview-stat__label">Total Messages</div>
            <div class="overview-stat__value">{{ stats.messageCount }}</div>
          </div>
          <div class="overview-stat">
            <div class="overview-stat__label">Last Activity</div>
            <div class="overview-stat__value">{{ formatDate(stats.lastActivity) }}</div>
          </div>
        </div>
      </div>

      <!-- Recent Questions Card -->
      <div class="analytics-card recent-questions">
        <h3 class="analytics-card__title">Recent Questions</h3>
        <div v-if="isLoadingQuestions" class="analytics-loading">
          Loading recent questions...
        </div>
        <div v-else-if="recentQuestions.length === 0" class="no-data">
          No questions have been asked yet.
        </div>
        <div v-else class="question-list">
          <div v-for="(question, index) in recentQuestions" :key="index" class="question-item">
            <span class="question-number">{{ index + 1 }}.</span>
            <span class="question-text">{{ question }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import DocumentAnalytics from '@/components/DocumentAnalytics.vue';
import documentService from '@/services/documentService';
import chatService from '@/services/chatService';

export default {
  name: 'AnalyticsView',
  components: {
    DocumentAnalytics
  },
  data() {
    return {
      isLoading: false,
      isLoadingQuestions: false,
      stats: {
        documentCount: 0,
        chatCount: 0,
        messageCount: 0,
        lastActivity: new Date()
      },
      recentQuestions: []
    };
  },
  created() {
    this.fetchData();
  },
  methods: {
    async fetchData() {
      this.isLoading = true;

      try {
        // Fetch document metrics
        const metrics = await documentService.getDocumentMetrics();
        this.stats.documentCount = metrics.total || 0;

        // Get recent questions
        this.fetchRecentQuestions();

        // Fetch chat statistics
        this.fetchChatStats();

      } catch (error) {
        console.error('Failed to fetch analytics data:', error);
      } finally {
        this.isLoading = false;
      }

      // Refresh the document analytics component
      if (this.$refs.documentAnalytics) {
        this.$refs.documentAnalytics.fetchData();
      }
    },

    async fetchRecentQuestions() {
      this.isLoadingQuestions = true;

      try {
        const response = await chatService.getRecentQuestions(10);
        this.recentQuestions = response.questions || [];
      } catch (error) {
        console.error('Failed to fetch recent questions:', error);
      } finally {
        this.isLoadingQuestions = false;
      }
    },

    async fetchChatStats() {
      try {
        // Fetch chat sessions
        const chatResponse = await chatService.listChats(1, 1);
        this.stats.chatCount = chatResponse.total || 0;

        // The backend doesn't have a direct endpoint for message count,
        // so we're estimating based on available data
        const chatSessionsWithMessages = await chatService.listChats(1, 10);
        let messageCount = 0;

        if (chatSessionsWithMessages.chats) {
          chatSessionsWithMessages.chats.forEach(chat => {
            messageCount += chat.message_count || 0;
          });

          // If we have at least one chat, use its updated_at as last activity
          if (chatSessionsWithMessages.chats.length > 0) {
            const mostRecentChat = chatSessionsWithMessages.chats.sort((a, b) => {
              return new Date(b.updated_at) - new Date(a.updated_at);
            })[0];

            this.stats.lastActivity = new Date(mostRecentChat.updated_at);
          }
        }

        // If we got data from the first 10 chats, extrapolate for the total
        if (chatSessionsWithMessages.chats && chatSessionsWithMessages.chats.length > 0) {
          const avgMessagesPerChat = messageCount / chatSessionsWithMessages.chats.length;
          this.stats.messageCount = Math.round(avgMessagesPerChat * this.stats.chatCount);
        } else {
          this.stats.messageCount = 0;
        }
      } catch (error) {
        console.error('Failed to fetch chat statistics:', error);
      }
    },

    refreshData() {
      this.fetchData();
    },

    formatDate(date) {
      if (!date) return 'N/A';

      const now = new Date();

      // If it's today, show "Today, HH:MM"
      if (date.toDateString() === now.toDateString()) {
        return `Today, ${date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
      }

      // If it's yesterday, show "Yesterday, HH:MM"
      const yesterday = new Date();
      yesterday.setDate(yesterday.getDate() - 1);
      if (date.toDateString() === yesterday.toDateString()) {
        return `Yesterday, ${date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
      }

      // Otherwise, show the full date
      return date.toLocaleDateString([], { day: 'numeric', month: 'short', year: 'numeric' });
    }
  }
};
</script>

<style scoped>
.analytics-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px;
}

.analytics-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.analytics-title {
  font-size: 24px;
  color: #333;
  margin: 0;
}

.refresh-button {
  padding: 8px 16px;
  background-color: #f0f0f0;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 14px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  transition: background-color 0.2s;
}

.refresh-button:hover:not(:disabled) {
  background-color: #e0e0e0;
}

.refresh-button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.analytics-sections {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 20px;
}

.analytics-card {
  background-color: white;
  border-radius: 8px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
  padding: 20px;
  margin-bottom: 20px;
}

.analytics-card__title {
  font-size: 18px;
  margin: 0 0 16px 0;
  color: #333;
  font-weight: 500;
  border-bottom: 1px solid #eee;
  padding-bottom: 10px;
}

.system-overview {
  grid-column: span 1;
}

.recent-questions {
  grid-column: span 1;
}

.overview-stats {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.overview-stat {
  background-color: #f8f8f8;
  padding: 15px;
  border-radius: 6px;
  text-align: center;
}

.overview-stat__label {
  font-size: 14px;
  color: #666;
  margin-bottom: 6px;
}

.overview-stat__value {
  font-size: 20px;
  font-weight: 600;
  color: #333;
}

.analytics-loading {
  padding: 20px;
  text-align: center;
  color: #666;
  font-style: italic;
}

.no-data {
  padding: 20px;
  text-align: center;
  color: #666;
}

.question-list {
  max-height: 400px;
  overflow-y: auto;
}

.question-item {
  padding: 10px;
  border-bottom: 1px solid #eee;
  display: flex;
  align-items: flex-start;
}

.question-item:last-child {
  border-bottom: none;
}

.question-number {
  font-weight: 600;
  color: #3498db;
  margin-right: 10px;
  min-width: 20px;
}

.question-text {
  flex: 1;
  line-height: 1.4;
}

/* Make DocumentAnalytics component take full width */
.analytics-sections > :first-child {
  grid-column: 1 / -1;
}
</style>