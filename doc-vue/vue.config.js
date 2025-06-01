const { defineConfig } = require('@vue/cli-service')

module.exports = defineConfig({
  transpileDependencies: true,
  
  // Use relative publicPath
  publicPath: '/',
  
  devServer: {
    // Don't force HTTPS for Codespaces
    https: process.env.CODESPACES ? false : true,
    allowedHosts: 'all',
    // Update WebSocket configuration for Codespaces
    client: {
      webSocketURL: process.env.CODESPACES 
        ? { hostname: '0.0.0.0', pathname: '/ws' }
        : { protocol: 'wss' }
    },
    // Proxy API requests to the backend
    proxy: {
      '/api': {
        target: 'http://localhost:8080', // Point to your backend port
        changeOrigin: true
      }
    }
  },
  
  productionSourceMap: false
})