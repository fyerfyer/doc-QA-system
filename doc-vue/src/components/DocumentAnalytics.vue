<template>
  <div class="document-analytics">
    <div v-if="isLoading" class="analytics-loading">
      <div class="loading-spinner"></div>
      <p>Loading analytics data...</p>
    </div>

    <div v-else-if="error" class="analytics-error">
      <p>{{ error }}</p>
      <el-button type="primary" size="small" @click="fetchData">Retry</el-button>
    </div>

    <div v-else class="analytics-content">
      <!-- Status Distribution Card -->
      <div class="analytics-card">
        <h3 class="analytics-card__title">Document Status</h3>
        <div class="status-distribution">
          <div
            v-for="(status, index) in statusDistribution"
            :key="index"
            class="status-item"
          >
            <div class="status-badge" :class="`status-badge--${status.name.toLowerCase()}`">
              {{ status.name }}
            </div>
            <div class="status-count">{{ status.value }}</div>
          </div>
        </div>
      </div>

      <!-- Statistics Card -->
      <div class="analytics-card">
        <h3 class="analytics-card__title">Document Statistics</h3>
        <div class="statistics-grid">
          <div class="stat-item">
            <span class="stat-label">Total Documents</span>
            <span class="stat-value">{{ stats.totalCount }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Total Size</span>
            <span class="stat-value">{{ formatSize(stats.totalSize) }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Average Size</span>
            <span class="stat-value">{{ formatSize(stats.averageSize) }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Total Segments</span>
            <span class="stat-value">{{ stats.totalSegments }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Average Segments</span>
            <span class="stat-value">{{ formatNumber(stats.averageSegments) }}</span>
          </div>
        </div>
      </div>

      <!-- Processing Times Card -->
      <div class="analytics-card">
        <h3 class="analytics-card__title">Processing Times</h3>
        <div class="statistics-grid">
          <div class="stat-item">
            <span class="stat-label">Average Time</span>
            <span class="stat-value">{{ formatTime(stats.processingTimes.average) }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Min Time</span>
            <span class="stat-value">{{ formatTime(stats.processingTimes.min) }}</span>
          </div>
          <div class="stat-item">
            <span class="stat-label">Max Time</span>
            <span class="stat-value">{{ formatTime(stats.processingTimes.max) }}</span>
          </div>
        </div>
      </div>

      <!-- Bar Chart -->
      <div v-if="hasDocuments" class="analytics-card">
        <h3 class="analytics-card__title">Status Distribution</h3>
        <div class="chart-container" ref="chartContainer"></div>
      </div>

      <!-- No Data Message -->
      <div v-if="!hasDocuments" class="no-data">
        <p>No document data available yet. Upload documents to see analytics.</p>
      </div>

      <!-- Last Update Info -->
      <div v-if="hasDocuments" class="last-updated">
        Last updated: {{ formatLastUpdated() }}
        <el-button type="text" @click="fetchData" :loading="isLoading">
          <i class="el-icon-refresh"></i> Refresh
        </el-button>
      </div>
    </div>
  </div>
</template>

<script>
import documentService from '@/services/documentService';
import * as echarts from 'echarts/core';
import { PieChart } from 'echarts/charts';
import {
  GridComponent,
  TooltipComponent,
  TitleComponent,
  LegendComponent
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

// Register the required components
echarts.use([
  PieChart,  // 改为PieChart
  GridComponent,
  TooltipComponent,
  TitleComponent,
  LegendComponent,
  CanvasRenderer
]);

export default {
  name: 'DocumentAnalytics',
  data() {
    return {
      isLoading: false,
      error: null,
      lastUpdated: new Date(),
      statusDistribution: [],
      stats: {
        totalCount: 0,
        totalSize: 0,
        averageSize: 0,
        totalSegments: 0,
        averageSegments: 0,
        processingTimes: {
          min: 0,
          max: 0,
          average: 0
        },
        statusCounts: {
          uploaded: 0,
          processing: 0,
          completed: 0,
          failed: 0
        }
      },
      chart: null
    };
  },
  computed: {
    hasDocuments() {
      return this.stats.totalCount > 0;
    }
  },
  mounted() {
    this.fetchData();
    // Resize chart when window is resized
    window.addEventListener('resize', this.resizeChart);
  },
  beforeUnmount() {
    window.removeEventListener('resize', this.resizeChart);
    if (this.chart) {
      this.chart.dispose();
    }
  },
  methods: {
    async fetchData() {
      this.isLoading = true;
      this.error = null;

      try {
        // Get status distribution
        this.statusDistribution = await documentService.getStatusDistribution();

        // Get detailed document statistics
        this.stats = await documentService.getDocumentStatistics();

        this.lastUpdated = new Date();

        // Initialize or update chart
        this.$nextTick(() => {
          this.initChart();
        });
      } catch (error) {
        console.error('Failed to fetch analytics data:', error);
        this.error = 'Failed to load analytics data: ' + (error.message || 'Unknown error');
      } finally {
        this.isLoading = false;
      }
    },

    initChart() {
      if (!this.hasDocuments) return;
      if (!this.$refs.chartContainer) return;

      if (this.chart) {
        this.chart.dispose();
      }

      this.chart = echarts.init(this.$refs.chartContainer);

      const colorMap = {
        'Uploaded': '#909399',
        'Processing': '#409EFF',
        'Completed': '#67C23A',
        'Failed': '#F56C6C'
      };

      const option = {
        tooltip: {
          trigger: 'item',
          formatter: '{b}: {c} ({d}%)'
        },
        legend: {
          orient: 'horizontal',
          top: 'bottom',
          left: 'center',
          padding: [10, 0, 0, 0],
          itemGap: 20,
          textStyle: {
            fontSize: 14
          }
        },
        color: Object.values(colorMap),
        series: [{
          type: 'pie',
          radius: ['40%', '70%'],
          center: ['50%', '45%'],
          avoidLabelOverlap: true,
          itemStyle: {
            borderRadius: 6,
            borderColor: '#fff',
            borderWidth: 2
          },
          label: {
            show: false
          },
          labelLine: {
            show: false
          },
          emphasis: {
            label: {
              show: true,
              fontSize: '16',
              fontWeight: 'bold'
            },
            itemStyle: {
              shadowBlur: 10,
              shadowOffsetX: 0,
              shadowColor: 'rgba(0, 0, 0, 0.5)'
            }
          },
          data: this.statusDistribution.map(item => ({
            name: item.name,
            value: item.value,
            itemStyle: {
              color: colorMap[item.name]
            }
          }))
        }]
      };

      this.chart.setOption(option);
    },

    resizeChart() {
      if (this.chart) {
        this.chart.resize();
      }
    },

    formatSize(bytes) {
      return documentService.formatFileSize(bytes);
    },

    formatNumber(value) {
      if (value === null || value === undefined) return '0';

      // Format with 2 decimals if it's not a whole number
      if (Number.isInteger(value)) {
        return value.toString();
      } else {
        return value.toFixed(2);
      }
    },

    formatTime(seconds) {
      if (!seconds) return '0s';

      if (seconds < 60) {
        return `${seconds.toFixed(1)}s`;
      } else if (seconds < 3600) {
        const minutes = Math.floor(seconds / 60);
        const remainingSeconds = (seconds % 60).toFixed(0);
        return `${minutes}m ${remainingSeconds}s`;
      } else {
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        return `${hours}h ${minutes}m`;
      }
    },

    formatLastUpdated() {
      return this.lastUpdated.toLocaleTimeString();
    }
  }
}
</script>

<style scoped>
.document-analytics {
  padding: 20px;
  width: 100%;
}

.analytics-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 200px;
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

.analytics-error {
  padding: 20px;
  background-color: #fff2f0;
  border: 1px solid #ffccc7;
  border-radius: 4px;
  color: #cf1322;
  text-align: center;
}

.analytics-content {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 20px;
  margin-top: 10px;
}

.analytics-card {
  background-color: white;
  border-radius: 8px;
  box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.1);
  padding: 20px;
  display: flex;
  flex-direction: column;
}

.analytics-card__title {
  font-size: 18px;
  margin: 0 0 16px 0;
  color: #333;
  font-weight: 500;
  border-bottom: 1px solid #eee;
  padding-bottom: 10px;
}

.status-distribution {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}

.status-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1;
  padding: 10px;
  border-radius: 4px;
  background-color: #f8f8f8;
  min-width: 100px;
}

.status-badge {
  padding: 4px 8px;
  border-radius: 12px;
  margin-bottom: 8px;
  font-size: 12px;
  font-weight: 500;
  color: white;
}

.status-badge--uploaded {
  background-color: #909399;
}

.status-badge--processing {
  background-color: #409EFF;
}

.status-badge--completed {
  background-color: #67C23A;
}

.status-badge--failed {
  background-color: #F56C6C;
}

.status-count {
  font-size: 20px;
  font-weight: 500;
}

.statistics-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.stat-item {
  display: flex;
  flex-direction: column;
  padding: 10px;
  background-color: #f8f8f8;
  border-radius: 4px;
}

.stat-label {
  font-size: 12px;
  color: #666;
  margin-bottom: 4px;
}

.stat-value {
  font-size: 16px;
  font-weight: 600;
  color: #333;
}

.chart-container {
  height: 300px;
  margin-top: 10px;
}

.no-data {
  grid-column: 1 / -1;
  text-align: center;
  padding: 40px;
  background-color: #f9f9f9;
  border-radius: 8px;
  color: #666;
}

.last-updated {
  grid-column: 1 / -1;
  text-align: right;
  margin-top: 10px;
  font-size: 12px;
  color: #999;
}
</style>