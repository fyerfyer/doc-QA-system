<template>
  <div
    class="chat-session"
    :class="{ 'chat-session--active': isActive }"
    @click="handleSelect">
    <div class="chat-session__content">
      <!-- Chat Title -->
      <div class="chat-session__title" :title="chat.title">
        {{ chat.title }}
      </div>

      <!-- Chat Metadata -->
      <div class="chat-session__meta">
        <span class="chat-session__date">{{ formattedDate }}</span>
        <span class="chat-session__count">{{ chat.messageCount }} {{ chat.messageCount === 1 ? 'message' : 'messages' }}</span>
      </div>
    </div>

    <!-- Action Buttons -->
    <div class="chat-session__actions">
      <button
        class="chat-session__action"
        title="Rename chat"
        @click.stop="startRename"
      >
        <i class="chat-session__icon">✏️</i>
      </button>
      <button
        class="chat-session__action"
        title="Delete chat"
        @click.stop="confirmDelete"
      >
        <i class="chat-session__icon">🗑️</i>
      </button>
    </div>

    <!-- Rename Dialog -->
    <div v-if="isRenaming" class="chat-session__rename-dialog" @click.stop>
      <input
        ref="renameInput"
        v-model="newTitle"
        class="chat-session__rename-input"
        type="text"
        placeholder="Enter new title"
        @keyup.enter="saveNewTitle"
        @keyup.esc="cancelRename"
      />
      <div class="chat-session__rename-actions">
        <button
          class="chat-session__rename-button chat-session__rename-button--cancel"
          @click="cancelRename"
        >
          Cancel
        </button>
        <button
          class="chat-session__rename-button chat-session__rename-button--save"
          @click="saveNewTitle"
        >
          Save
        </button>
      </div>
    </div>

    <!-- Delete Confirmation Dialog -->
    <div v-if="showDeleteConfirm" class="chat-session__delete-dialog" @click.stop>
      <p class="chat-session__delete-message">Delete this chat session?</p>
      <div class="chat-session__delete-actions">
        <button
          class="chat-session__delete-button chat-session__delete-button--cancel"
          @click="cancelDelete"
        >
          Cancel
        </button>
        <button
          class="chat-session__delete-button chat-session__delete-button--confirm"
          @click="handleDelete"
        >
          Delete
        </button>
      </div>
    </div>
  </div>
</template>

<script>
import chatService from '@/services/chatService';

export default {
  name: 'ChatSession',
  props: {
    chat: {
      type: Object,
      required: true
    },
    isActive: {
      type: Boolean,
      default: false
    }
  },
  data() {
    return {
      isRenaming: false,
      showDeleteConfirm: false,
      newTitle: '',
      isDeleting: false
    }
  },
  computed: {
    formattedDate() {
      // Return today, yesterday, or formatted date
      const date = this.chat.updatedAt ? new Date(this.chat.updatedAt) : new Date(this.chat.createdAt);

      const today = new Date();
      const yesterday = new Date();
      yesterday.setDate(yesterday.getDate() - 1);

      if (date.toDateString() === today.toDateString()) {
        return 'Today, ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      } else if (date.toDateString() === yesterday.toDateString()) {
        return 'Yesterday, ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      } else {
        return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
      }
    }
  },
  methods: {
    handleSelect() {
      if (!this.isRenaming && !this.showDeleteConfirm) {
        this.$emit('select', this.chat);
      }
    },

    startRename() {
      this.isRenaming = true;
      this.newTitle = this.chat.title;

      // Focus the input field after it's rendered
      this.$nextTick(() => {
        if (this.$refs.renameInput) {
          this.$refs.renameInput.focus();
          this.$refs.renameInput.select();
        }
      });
    },

    async saveNewTitle() {
      if (!this.newTitle.trim()) {
        // Don't save empty titles
        this.newTitle = this.chat.title;
        this.isRenaming = false;
        return;
      }

      try {
        await chatService.renameChat(this.chat.id, this.newTitle.trim());
        // Emit an event to inform parent components that the chat has been renamed
        this.$emit('renamed', {
          id: this.chat.id,
          title: this.newTitle.trim()
        });
      } catch (error) {
        console.error('Failed to rename chat:', error);
        // Revert to original title
        this.newTitle = this.chat.title;
      } finally {
        this.isRenaming = false;
      }
    },

    cancelRename() {
      this.isRenaming = false;
      this.newTitle = this.chat.title;
    },

    confirmDelete() {
      this.showDeleteConfirm = true;
    },

    cancelDelete() {
      this.showDeleteConfirm = false;
    },

    async handleDelete() {
      if (this.isDeleting) return;

      this.isDeleting = true;

      try {
        await chatService.deleteChat(this.chat.id);
        // Emit an event to inform parent components that the chat has been deleted
        this.$emit('deleted', this.chat.id);
      } catch (error) {
        console.error('Failed to delete chat:', error);
      } finally {
        this.isDeleting = false;
        this.showDeleteConfirm = false;
      }
    }
  }
}
</script>

<style scoped>
.chat-session {
  padding: 12px;
  border-radius: 8px;
  margin-bottom: 8px;
  background-color: #f8f9fa;
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  position: relative;
  transition: all 0.2s ease;
}

.chat-session:hover {
  background-color: #f0f2f5;
}

.chat-session--active {
  background-color: #e3f2fd;
  border-left: 3px solid #3498db;
}

.chat-session__content {
  flex: 1;
  min-width: 0; /* Allows text truncation to work */
}

.chat-session__title {
  font-size: 15px;
  font-weight: 500;
  margin-bottom: 4px;
  color: #333;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.chat-session__meta {
  display: flex;
  font-size: 12px;
  color: #666;
}

.chat-session__date {
  margin-right: 8px;
}

.chat-session__count {
  font-style: italic;
}

.chat-session__actions {
  display: flex;
  align-items: center;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.2s;
}

.chat-session:hover .chat-session__actions {
  opacity: 1;
}

.chat-session__action {
  background: none;
  border: none;
  font-size: 14px;
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  color: #666;
  transition: all 0.2s;
}

.chat-session__action:hover {
  background-color: rgba(0, 0, 0, 0.05);
  color: #333;
}

.chat-session__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  font-style: normal;
  font-size: 14px;
}

/* Rename Dialog Styles */
.chat-session__rename-dialog {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-color: #fff;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  z-index: 2;
  padding: 10px;
  display: flex;
  flex-direction: column;
}

.chat-session__rename-input {
  padding: 8px;
  border: 1px solid #ddd;
  border-radius: 4px;
  font-size: 14px;
  margin-bottom: 8px;
}

.chat-session__rename-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.chat-session__rename-button {
  padding: 6px 12px;
  border: none;
  border-radius: 4px;
  font-size: 13px;
  cursor: pointer;
}

.chat-session__rename-button--cancel {
  background-color: #f1f1f1;
  color: #333;
}

.chat-session__rename-button--save {
  background-color: #3498db;
  color: white;
}

/* Delete Confirmation Styles */
.chat-session__delete-dialog {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-color: #fff;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  z-index: 2;
  padding: 10px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.chat-session__delete-message {
  text-align: center;
  margin-bottom: 12px;
  color: #333;
}

.chat-session__delete-actions {
  display: flex;
  justify-content: center;
  gap: 8px;
}

.chat-session__delete-button {
  padding: 6px 12px;
  border: none;
  border-radius: 4px;
  font-size: 13px;
  cursor: pointer;
}

.chat-session__delete-button--cancel {
  background-color: #f1f1f1;
  color: #333;
}

.chat-session__delete-button--confirm {
  background-color: #e74c3c;
  color: white;
}
</style>