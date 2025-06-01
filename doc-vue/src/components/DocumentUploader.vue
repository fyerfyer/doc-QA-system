<template>
  <div class="document-uploader">
    <div
      class="uploader-dropzone"
      :class="{
        'uploader-dropzone--active': isDragging,
        'uploader-dropzone--loading': isUploading
      }"
      @dragover.prevent="onDragOver"
      @dragleave.prevent="onDragLeave"
      @drop.prevent="onDrop"
    >
      <div v-if="isUploading" class="uploader-progress">
        <div class="uploader-progress__indicator"></div>
        <div class="uploader-progress__text">Uploading {{ currentFile.name }}...</div>
      </div>
      <div v-else class="uploader-content">
        <div class="uploader-icon">
          <i class="upload-icon">📄</i>
        </div>
        <div class="uploader-text">
          <p class="uploader-text__primary">Drag & drop your documents here</p>
          <p class="uploader-text__secondary">or</p>
          <label class="uploader-button">
            <span>Browse files</span>
            <input
              type="file"
              class="uploader-input"
              accept=".pdf,.md,.markdown,.txt"
              @change="onFileSelected"
            />
          </label>
          <p class="uploader-text__hint">Supported formats: PDF, Markdown, Text files</p>
        </div>
      </div>
    </div>

    <div v-if="uploadStatus" :class="`uploader-status uploader-status--${uploadStatus.type}`">
      {{ uploadStatus.message }}
    </div>
  </div>
</template>

<script>
import documentService from '../services/documentService';

export default {
  name: 'DocumentUploader',
  data() {
    return {
      isDragging: false,
      isUploading: false,
      currentFile: null,
      uploadStatus: null,
      acceptedTypes: ['.pdf', '.md', '.markdown', '.txt']
    };
  },
  methods: {
    onDragOver() {
      this.isDragging = true;
    },
    onDragLeave() {
      this.isDragging = false;
    },
    onDrop(e) {
      this.isDragging = false;

      const files = e.dataTransfer.files;
      if (files.length) {
        this.processFile(files[0]);
      }
    },
    onFileSelected(e) {
      const files = e.target.files;
      if (files.length) {
        this.processFile(files[0]);
        // Reset file input so the same file can be uploaded again
        e.target.value = '';
      }
    },
    processFile(file) {
      // Check file extension
      const extension = this.getFileExtension(file.name).toLowerCase();
      if (!this.acceptedTypes.includes(extension)) {
        this.showStatus('error', `Unsupported file type. Please upload PDF, MD or TXT files.`);
        return;
      }

      // Check file size (limit to 10MB)
      if (file.size > 10 * 1024 * 1024) {
        this.showStatus('error', 'File size exceeds the limit (10MB).');
        return;
      }

      this.uploadFile(file);
    },
    async uploadFile(file) {
      try {
        this.isUploading = true;
        this.currentFile = file;
        this.uploadStatus = null;

        // Upload the file
        const response = await documentService.uploadDocument(file);

        // Show success message
        this.showStatus('success', `File "${file.name}" uploaded successfully and is being processed.`);

        // Emit event to parent component
        this.$emit('upload-complete', response);

        // Reset state
        this.currentFile = null;
      } catch (error) {
        console.error('Upload failed:', error);
        this.showStatus('error', `Upload failed: ${error.message || 'Unknown error'}`);
      } finally {
        this.isUploading = false;
      }
    },
    showStatus(type, message) {
      this.uploadStatus = { type, message };

      // Auto-clear success messages after 5 seconds
      if (type === 'success') {
        setTimeout(() => {
          if (this.uploadStatus && this.uploadStatus.type === 'success') {
            this.uploadStatus = null;
          }
        }, 5000);
      }
    },
    getFileExtension(filename) {
      return `.${filename.split('.').pop()}`;
    }
  }
}
</script>

<style scoped>
.document-uploader {
  margin: 20px 0;
  width: 100%;
}

.uploader-dropzone {
  border: 2px dashed #ccc;
  border-radius: 8px;
  padding: 40px 20px;
  text-align: center;
  transition: all 0.3s ease;
  background-color: #f9f9f9;
  position: relative;
  min-height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.uploader-dropzone--active {
  border-color: #3498db;
  background-color: #edf7ff;
}

.uploader-dropzone--loading {
  border-color: #e0e0e0;
  background-color: #f0f0f0;
}

.uploader-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 100%;
}

.uploader-icon {
  font-size: 48px;
  color: #666;
  margin-bottom: 16px;
}

.uploader-text__primary {
  font-size: 18px;
  font-weight: 500;
  margin-bottom: 8px;
  color: #333;
}

.uploader-text__secondary {
  margin: 8px 0;
  color: #666;
}

.uploader-text__hint {
  margin-top: 16px;
  font-size: 14px;
  color: #666;
}

.uploader-button {
  display: inline-block;
  padding: 10px 20px;
  background-color: #3498db;
  color: white;
  border-radius: 4px;
  cursor: pointer;
  font-weight: 500;
  transition: background-color 0.2s;
}

.uploader-button:hover {
  background-color: #2980b9;
}

.uploader-input {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  border: 0;
}

.uploader-progress {
  width: 100%;
  padding: 0 20px;
  text-align: center;
}

.uploader-progress__indicator {
  height: 4px;
  background-color: #3498db;
  border-radius: 2px;
  width: 100%;
  position: relative;
  overflow: hidden;
  margin-bottom: 16px;
}

.uploader-progress__indicator::before {
  content: '';
  position: absolute;
  background-color: #fff;
  top: 0;
  left: 0;
  height: 100%;
  width: 50%;
  animation: progress-animation 1.5s infinite ease-in-out;
}

.uploader-progress__text {
  font-size: 16px;
  color: #666;
}

.uploader-status {
  margin-top: 16px;
  padding: 12px;
  border-radius: 4px;
  text-align: center;
  font-size: 14px;
}

.uploader-status--error {
  background-color: #ffebee;
  color: #c62828;
}

.uploader-status--success {
  background-color: #e8f5e9;
  color: #2e7d32;
}

@keyframes progress-animation {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(200%);
  }
}
</style>