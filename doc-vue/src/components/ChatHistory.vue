<template>
  <div class="chat-history">
    <div class="chat-history__header">
      <h3 class="chat-history__title">Chat History</h3>
      <button
        class="chat-history__new-button"
        @click="createNewChat"
        :disabled="isLoading"
      >
        <i class="chat-history__new-icon">+</i>
        New Chat
      </button>
    </div>

    <div class="chat-history__search">
      <input
        v-model="searchTerm"
        class="chat-history__search-input"
        type="text"
        placeholder="Search chats..."
        @input="debouncedSearch"
      />
      <i v-if="searchTerm" class="chat-history__search-clear" @click="clearSearch">×</i>
    </div>

    <div class="chat-history__content">
      <div v-if="isLoading" class="chat-history__loading">
        Loading conversations...
      </div>

      <div v-else-if="filteredSessions.length === 0" class="chat-history__empty">
        <p v-if="searchTerm">No chats matching your search</p>
        <p v-else>No chat history yet</p>
        <p class="chat-history__empty-hint">
          Start a new conversation to begin
        </p>
      </div>

      <div v-else class="chat-history__sessions">
        <chat-session
          v-for="session in filteredSessions"
          :key="session.id"
          :chat="session"
          :is-active="activeSessionId === session.id"
          @select="selectSession"
          @renamed="handleSessionRenamed"
          @deleted="handleSessionDeleted"
        />
      </div>

      <div v-if="hasMoreSessions" class="chat-history__load-more">
        <button
          class="chat-history__load-button"
          @click="loadMoreSessions"
          :disabled="isLoadingMore"
        >
          {{ isLoadingMore ? 'Loading...' : 'Load more' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>
import ChatSession from './ChatSession.vue';
import chatService from '@/services/chatService';

export default {
  name: 'ChatHistory',
  components: {
    ChatSession
  },
  props: {
    // Currently active session ID
    activeSessionId: {
      type: String,
      default: null
    }
  },
  data() {
    return {
      sessions: [],
      isLoading: false,
      isLoadingMore: false,
      searchTerm: '',
      page: 1,
      pageSize: 10,
      totalSessions: 0,
      searchTimeout: null
    };
  },
  computed: {
    filteredSessions() {
      if (!this.searchTerm) {
        return this.sessions;
      }

      const searchLower = this.searchTerm.toLowerCase();
      return this.sessions.filter(session =>
        session.title.toLowerCase().includes(searchLower)
      );
    },
    hasMoreSessions() {
      return this.sessions.length < this.totalSessions;
    }
  },
  created() {
    this.fetchSessions();
  },
  methods: {
    async fetchSessions() {
      this.isLoading = true;

      try {
        const response = await chatService.listChats(this.page, this.pageSize);
        this.sessions = response.chats || [];
        this.totalSessions = response.total || 0;
      } catch (error) {
        console.error('Failed to fetch chat sessions:', error);
      } finally {
        this.isLoading = false;
      }
    },

    async loadMoreSessions() {
      if (this.isLoadingMore) return;

      this.isLoadingMore = true;
      this.page += 1;

      try {
        const response = await chatService.listChats(this.page, this.pageSize);
        this.sessions = [...this.sessions, ...(response.chats || [])];
      } catch (error) {
        console.error('Failed to load more sessions:', error);
        this.page -= 1; // Revert page increment on error
      } finally {
        this.isLoadingMore = false;
      }
    },

    selectSession(session) {
      this.$emit('select', session);
    },

    async createNewChat() {
      try {
        const newSession = await chatService.createChat('New Chat');
        // Insert at the beginning of the sessions array
        this.sessions.unshift({
          id: newSession.chat_id,
          title: newSession.title,
          createdAt: newSession.created_at,
          updatedAt: newSession.created_at,
          messageCount: 0
        });

        // Select the new session
        this.selectSession(this.sessions[0]);
      } catch (error) {
        console.error('Failed to create new chat:', error);
      }
    },

    handleSessionRenamed(sessionData) {
      // Update the renamed session in the list
      const index = this.sessions.findIndex(s => s.id === sessionData.id);
      if (index !== -1) {
        this.sessions[index] = {
          ...this.sessions[index],
          title: sessionData.title
        };
      }
    },

    handleSessionDeleted(sessionId) {
      // Remove the deleted session from the list
      this.sessions = this.sessions.filter(s => s.id !== sessionId);

      // Notify parent if the active session was deleted
      if (this.activeSessionId === sessionId) {
        this.$emit('active-deleted');
      }
    },

    debouncedSearch() {
      clearTimeout(this.searchTimeout);
      this.searchTimeout = setTimeout(() => {
        // If implementing server-side search, you would call an API here
        // For now, we're just filtering the client-side list
      }, 300);
    },

    clearSearch() {
      this.searchTerm = '';
    },

    resetFilter() {
      this.searchTerm = '';
      this.page = 1;
      this.fetchSessions();
    }
  }
}
</script>

<style scoped>
.chat-history {
  display: flex;
  flex-direction: column;
  height: 100%;
  background-color: #f8f9fa;
  border-right: 1px solid #eaeaea;
  width: 100%;
}

.chat-history__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px;
  border-bottom: 1px solid #eaeaea;
}

.chat-history__title {
  font-size: 18px;
  font-weight: 600;
  margin: 0;
  color: #333;
}

.chat-history__new-button {
  display: flex;
  align-items: center;
  gap: 6px;
  background-color: #3498db;
  color: white;
  border: none;
  border-radius: 4px;
  padding: 8px 12px;
  font-size: 14px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.chat-history__new-button:hover:not(:disabled) {
  background-color: #2980b9;
}

.chat-history__new-button:disabled {
  background-color: #a0cfee;
  cursor: not-allowed;
}

.chat-history__new-icon {
  font-style: normal;
  font-weight: bold;
  font-size: 16px;
}

.chat-history__search {
  padding: 12px 16px;
  position: relative;
}

.chat-history__search-input {
  width: 100%;
  padding: 8px 12px;
  padding-right: 30px;
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  font-size: 14px;
}

.chat-history__search-clear {
  position: absolute;
  right: 26px;
  top: 50%;
  transform: translateY(-50%);
  cursor: pointer;
  color: #999;
  font-style: normal;
  font-size: 18px;
}

.chat-history__search-clear:hover {
  color: #666;
}

.chat-history__content {
  flex: 1;
  overflow-y: auto;
  padding: 0 16px 16px;
}

.chat-history__loading {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  color: #666;
  font-style: italic;
}

.chat-history__empty {
  text-align: center;
  padding: 40px 20px;
  color: #666;
}

.chat-history__empty-hint {
  margin-top: 8px;
  font-size: 14px;
  color: #999;
}

.chat-history__sessions {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.chat-history__load-more {
  text-align: center;
  padding: 16px 0;
}

.chat-history__load-button {
  background: none;
  border: 1px solid #dcdfe6;
  border-radius: 4px;
  padding: 8px 16px;
  font-size: 14px;
  color: #606266;
  cursor: pointer;
  transition: all 0.2s;
}

.chat-history__load-button:hover:not(:disabled) {
  background-color: #f0f2f5;
  border-color: #c0c4cc;
}

.chat-history__load-button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>