<template>
  <div class="app-container">
    <!-- Navigation Header -->
    <header class="app-header">
      <div class="app-title">Document QA System</div>
      <nav class="app-nav">
        <router-link to="/documents" class="nav-item">Documents</router-link>
        <router-link to="/chat" class="nav-item">Q&A Chat</router-link>
      </nav>
    </header>

    <!-- Main Content Area -->
    <main class="app-content">
      <router-view />
    </main>

    <!-- Footer -->
    <footer class="app-footer">
      <div class="footer-content">
        Document QA System - Powered by RAG
      </div>
    </footer>
  </div>
</template>

<script>
export default {
  name: 'App',
  mounted() {
    // Check API connection on app start
    this.checkApiConnection();
  },
  methods: {
    async checkApiConnection() {
      try {
        // Importing this way to avoid circular dependency
        const qaService = await import('./services/qaService').then(module => module.default);
        const isConnected = await qaService.checkConnection();

        if (!isConnected) {
          console.warn('Warning: API connection failed. Check your backend service.');
        }
      } catch (error) {
        console.error('API connection check error:', error);
      }
    }
  }
}
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  background-color: #f8f9fa;
  color: #333;
  min-height: 100vh;
}

.app-container {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.app-header {
  background-color: #ffffff;
  padding: 0 20px;
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 100;
}

.app-title {
  font-size: 20px;
  font-weight: 600;
  color: #3498db;
}

.app-nav {
  display: flex;
  gap: 20px;
}

.nav-item {
  color: #666;
  text-decoration: none;
  font-weight: 500;
  padding: 8px 12px;
  border-radius: 4px;
  transition: all 0.2s;
}

.nav-item:hover {
  color: #3498db;
  background-color: #f0f7ff;
}

.router-link-active {
  color: #3498db;
  background-color: #f0f7ff;
}

.app-content {
  margin-top: 60px;
  padding: 20px;
  flex: 1;
}

.app-footer {
  background-color: #ffffff;
  padding: 15px 20px;
  text-align: center;
  font-size: 14px;
  color: #666;
  border-top: 1px solid #eee;
}
</style>