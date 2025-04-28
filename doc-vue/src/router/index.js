import { createRouter, createWebHistory } from 'vue-router';
import DocumentsView from '../views/DocumentsView.vue';
import ChatView from '../views/ChatView.vue';

// Define routes
const routes = [
    {
        path: '/',
        redirect: '/documents'
    },
    {
        path: '/documents',
        name: 'documents',
        component: DocumentsView,
        meta: {
            title: 'Document Management'
        }
    },
    {
        path: '/chat',
        name: 'chat',
        component: ChatView,
        meta: {
            title: 'Q&A Chat'
        },
        props: (route) => ({
            // Allow passing document ID through the route
            documentId: route.query.documentId
        })
    },
    {
        // Catch-all route for 404
        path: '/:pathMatch(.*)*',
        redirect: '/documents'
    }
];

// Create router instance
const router = createRouter({
    history: createWebHistory(process.env.BASE_URL || '/'),
    routes
});

// Navigation guard to set document title
router.beforeEach((to, from, next) => {
    // Set page title based on route meta
    if (to.meta.title) {
        document.title = `${to.meta.title} - Document QA System`;
    } else {
        document.title = 'Document QA System';
    }
    next();
});

export default router;