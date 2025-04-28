<template>
  <div class="document-item" :class="{'document-item--processing': isProcessing}">
    <div class="document-item__header">
      <div class="document-item__name">{{ document.filename }}</div>
      <div class="document-item__actions">
        <button
          class="document-item__action document-item__action--delete"
          @click.stop="confirmDelete"
          title="Delete document"
        >
          <i class="delete-icon">✕</i>
        </button>
      </div>
    </div>

    <div class="document-item__details">
      <div class="document-item__status-badge" :class="statusClass">
        {{ document.status }}
      </div>
      <div class="document-item__meta">
        <div class="document-item__meta-item">
          <span class="document-item__meta-label">Uploaded:</span>
          <span class="document-item__meta-value">{{ formatDate(document.upload_time) }}</span>
        </div>
        <div v-if="document.segments" class="document-item__meta-item">
          <span class="document-item__meta-label">Segments:</span>
          <span class="document-item__meta-value">{{ document.segments }}</span>
        </div>
      </div>
    </div>

    <div v-if="showDeleteConfirmation" class="document-item__delete-confirm">
      <p>Are you sure you want to delete this document?</p>
      <div class="document-item__delete-actions">
        <button class="document-item__delete-btn document-item__delete-btn--cancel" @click="cancelDelete">Cancel</button>
        <button class="document-item__delete-btn document-item__delete-btn--confirm" @click="handleDelete">Delete</button>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'DocumentItem',
  props: {
    document: {
      type: Object,
      required: true
    }
  },
  data() {
    return {
      showDeleteConfirmation: false
    };
  },
  computed: {
    isProcessing() {
      return this.document.status === 'processing';
    },
    statusClass() {
      const status = this.document.status;
      return {
        'document-item__status-badge--processing': status === 'processing',
        'document-item__status-badge--completed': status === 'completed',
        'document-item__status-badge--failed': status === 'failed'
      };
    }
  },
  methods: {
    formatDate(dateString) {
      if (!dateString) return 'Unknown';

      const date = new Date(dateString);
      return date.toLocaleString();
    },
    confirmDelete() {
      this.showDeleteConfirmation = true;
    },
    cancelDelete() {
      this.showDeleteConfirmation = false;
    },
    handleDelete() {
      this.$emit('delete', this.document.file_id);
      this.showDeleteConfirmation = false;
    },
    handleSelect() {
      this.$emit('select', this.document);
    }
  }
}
</script>

<style scoped>
.document-item {
  background-color: white;
  border-radius: 8px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.1);
  padding: 16px;
  margin-bottom: 16px;
  position: relative;
  transition: all 0.3s ease;
}

.document-item--processing {
  border-left: 4px solid #3498db;
}

.document-item__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.document-item__name {
  font-weight: bold;
  font-size: 16px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 80%;
}

.document-item__actions {
  display: flex;
}

.document-item__action {
  background: none;
  border: none;
  cursor: pointer;
  padding: 4px;
  font-size: 16px;
  color: #555;
  margin-left: 8px;
  transition: color 0.2s;
}

.document-item__action--delete:hover {
  color: #e74c3c;
}

.document-item__details {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.document-item__status-badge {
  display: inline-block;
  padding: 4px 8px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
  text-transform: capitalize;
  background-color: #f0f0f0;
  color: #666;
  width: fit-content;
}

.document-item__status-badge--processing {
  background-color: #e3f2fd;
  color: #1976d2;
}

.document-item__status-badge--completed {
  background-color: #e8f5e9;
  color: #2e7d32;
}

.document-item__status-badge--failed {
  background-color: #ffebee;
  color: #c62828;
}

.document-item__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 14px;
  color: #666;
}

.document-item__meta-item {
  display: flex;
  gap: 4px;
}

.document-item__meta-label {
  font-weight: 500;
}

.document-item__delete-confirm {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-color: rgba(255, 255, 255, 0.95);
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  border-radius: 8px;
  z-index: 1;
}

.document-item__delete-actions {
  display: flex;
  gap: 12px;
  margin-top: 16px;
}

.document-item__delete-btn {
  padding: 8px 16px;
  border: none;
  border-radius: 4px;
  font-weight: bold;
  cursor: pointer;
}

.document-item__delete-btn--cancel {
  background-color: #f0f0f0;
  color: #333;
}

.document-item__delete-btn--confirm {
  background-color: #e74c3c;
  color: white;
}
</style>