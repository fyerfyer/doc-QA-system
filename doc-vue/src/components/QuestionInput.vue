<template>
  <div class="question-input">
    <div class="question-input__container">
      <textarea
        v-model="question"
        class="question-input__field"
        :class="{ 'question-input__field--disabled': isLoading }"
        placeholder="Ask a question about your documents..."
        :disabled="isLoading"
        @keydown.ctrl.enter="handleSubmit"
        @keydown.meta.enter="handleSubmit"
      ></textarea>

      <button
        class="question-input__submit"
        :class="{ 'question-input__submit--loading': isLoading }"
        :disabled="!canSubmit || isLoading"
        @click="handleSubmit"
      >
        <span v-if="!isLoading">Ask</span>
        <span v-else class="question-input__loading-indicator">
          <span class="dot"></span>
          <span class="dot"></span>
          <span class="dot"></span>
        </span>
      </button>
    </div>

    <div class="question-input__footer">
      <span class="question-input__tip">Press Ctrl+Enter to send</span>
      <span v-if="selectedDocument" class="question-input__context">
        Asking about: <strong>{{ selectedDocument.filename }}</strong>
        <button
          class="question-input__clear-context"
          @click="clearSelectedDocument"
          title="Clear selected document"
        >×</button>
      </span>
    </div>
  </div>
</template>

<script>
import qaService from '../services/qaService';

export default {
  name: 'QuestionInput',
  props: {
    selectedDocument: {
      type: Object,
      default: null
    }
  },
  data() {
    return {
      question: '',
      isLoading: false,
      error: null
    };
  },
  computed: {
    canSubmit() {
      return this.question.trim().length > 0;
    }
  },
  methods: {
    async handleSubmit() {
      if (!this.canSubmit || this.isLoading) {
        return;
      }

      this.isLoading = true;
      this.error = null;

      try {
        let response;
        const questionText = this.question.trim();

        if (this.selectedDocument) {
          // Ask question about specific document
          response = await qaService.askQuestionAboutDocument(
            questionText,
            this.selectedDocument.file_id
          );
        } else {
          // Ask general question
          response = await qaService.askQuestion(questionText);
        }

        // Emit the response to parent component
        this.$emit('answer-received', {
          question: questionText,
          answer: response.answer,
          sources: response.sources
        });

        // Clear the input field after successful submission
        this.question = '';
      } catch (err) {
        this.error = err.message || 'Failed to get an answer';
        this.$emit('error', this.error);
      } finally {
        this.isLoading = false;
      }
    },
    clearSelectedDocument() {
      this.$emit('clear-document');
    }
  }
}
</script>

<style scoped>
.question-input {
  width: 100%;
  margin-bottom: 20px;
}

.question-input__container {
  display: flex;
  border: 1px solid #dcdfe6;
  border-radius: 8px;
  overflow: hidden;
  background-color: #fff;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.05);
}

.question-input__field {
  flex: 1;
  padding: 12px 15px;
  min-height: 60px;
  max-height: 120px;
  border: none;
  resize: vertical;
  font-family: inherit;
  font-size: 16px;
  line-height: 1.5;
  outline: none;
}

.question-input__field--disabled {
  background-color: #f5f7fa;
  color: #909399;
}

.question-input__submit {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 80px;
  background-color: #3498db;
  color: white;
  border: none;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 0.2s;
}

.question-input__submit:hover:not(:disabled) {
  background-color: #2980b9;
}

.question-input__submit:disabled {
  background-color: #a0cfee;
  cursor: not-allowed;
}

.question-input__submit--loading {
  background-color: #a0cfee;
}

.question-input__loading-indicator {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
}

.dot {
  width: 6px;
  height: 6px;
  background-color: white;
  border-radius: 50%;
  animation: pulse 1.4s infinite ease-in-out;
}

.dot:nth-child(2) {
  animation-delay: 0.2s;
}

.dot:nth-child(3) {
  animation-delay: 0.4s;
}

.question-input__footer {
  display: flex;
  justify-content: space-between;
  padding: 6px 10px;
  font-size: 12px;
  color: #909399;
}

.question-input__context {
  display: flex;
  align-items: center;
  gap: 6px;
}

.question-input__clear-context {
  background: none;
  border: none;
  font-size: 16px;
  color: #909399;
  cursor: pointer;
  padding: 0;
  line-height: 1;
}

.question-input__clear-context:hover {
  color: #f56c6c;
}

@keyframes pulse {
  0%, 80%, 100% {
    transform: scale(0.6);
    opacity: 0.6;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}
</style>