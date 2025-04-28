<template>
  <div v-if="answer" class="answer-display">
    <div class="answer-display__question">
      <h3 class="answer-display__question-title">Question:</h3>
      <p class="answer-display__question-text">{{ answer.question }}</p>
    </div>

    <div class="answer-display__content">
      <h3 class="answer-display__answer-title">Answer:</h3>
      <div class="answer-display__answer-text" v-html="formattedAnswer"></div>
    </div>

    <div v-if="answer.sources && answer.sources.length > 0" class="answer-display__sources">
      <h3 class="answer-display__sources-title">Sources:</h3>
      <div class="answer-display__sources-list">
        <div
          v-for="(source, index) in answer.sources"
          :key="`source-${index}`"
          class="answer-display__source-item"
        >
          <div class="answer-display__source-header">
            <span class="answer-display__source-filename">{{ source.filename }}</span>
            <span class="answer-display__source-position">Position: {{ source.position }}</span>
          </div>
          <p class="answer-display__source-text">{{ source.text }}</p>
        </div>
      </div>
    </div>

    <div v-if="error" class="answer-display__error">
      {{ error }}
    </div>
  </div>
</template>

<script>
export default {
  name: 'AnswerDisplay',
  props: {
    answer: {
      type: Object,
      default: null
    },
    error: {
      type: String,
      default: null
    }
  },
  computed: {
    formattedAnswer() {
      if (!this.answer || !this.answer.answer) return '';

      // Simple formatting for newlines and bullet points
      return this.answer.answer
        .replace(/\n\n/g, '<br/><br/>')
        .replace(/\n/g, '<br/>')
        .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
        .replace(/\*(.*?)\*/g, '<em>$1</em>')
        .replace(/- (.*?)(?:<br\/>|$)/g, '• $1<br/>');
    }
  }
}
</script>

<style scoped>
.answer-display {
  margin: 20px 0;
  background-color: white;
  border-radius: 8px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.1);
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.answer-display__question,
.answer-display__content,
.answer-display__sources {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.answer-display__question-title,
.answer-display__answer-title,
.answer-display__sources-title {
  font-size: 18px;
  font-weight: 600;
  color: #333;
  margin: 0;
}

.answer-display__question-text {
  font-size: 16px;
  color: #666;
  margin: 0;
  line-height: 1.5;
}

.answer-display__answer-text {
  font-size: 16px;
  color: #333;
  line-height: 1.6;
}

.answer-display__answer-text :deep(em) {
  font-style: italic;
  color: #555;
}

.answer-display__answer-text :deep(strong) {
  font-weight: 600;
  color: #000;
}

.answer-display__sources-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 8px;
}

.answer-display__source-item {
  background-color: #f8f9fa;
  border-left: 4px solid #3498db;
  padding: 12px 16px;
  border-radius: 4px;
}

.answer-display__source-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
  font-size: 14px;
}

.answer-display__source-filename {
  font-weight: 600;
  color: #333;
}

.answer-display__source-position {
  color: #666;
}

.answer-display__source-text {
  font-size: 14px;
  color: #555;
  margin: 0;
  line-height: 1.5;
  white-space: pre-line;
}

.answer-display__error {
  margin-top: 16px;
  padding: 12px;
  background-color: #ffebee;
  color: #c62828;
  border-radius: 4px;
  font-size: 14px;
}
</style>