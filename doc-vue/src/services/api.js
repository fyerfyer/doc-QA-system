import axios from 'axios';

// Create axios instance with default config
const api = axios.create({
    baseURL: process.env.VUE_APP_API_URL || '/api',
    timeout: 30000, // 30 second timeout
    headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
    }
});

// Request interceptor
api.interceptors.request.use(
    config => {
        // You can add auth token here if needed
        // const token = localStorage.getItem('token');
        // if (token) {
        //   config.headers.Authorization = `Bearer ${token}`;
        // }
        return config;
    },
    error => {
        return Promise.reject(error);
    }
);

// Response interceptor
api.interceptors.response.use(
    response => {
        // Extract data from standard API response wrapper
        const data = response.data;

        // Check if API returned success response
        if (data && data.code === 0) {
            return data.data;
        }

        // If API returned error code, convert to error
        return Promise.reject(new Error(data.message || 'Unknown error'));
    },
    error => {
        // Handle HTTP errors
        let message = 'Network error';
        if (error.response) {
            // Server responded with error
            const { data } = error.response;
            message = (data && data.message) || `Error: ${error.response.status}`;
        } else if (error.request) {
            // Request made but no response
            message = 'Server did not respond';
        }
        return Promise.reject(new Error(message));
    }
);

export default api;