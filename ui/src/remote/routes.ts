import type { RouteRecordRaw } from 'vue-router'
import '@/main.css'

// Routes mounted by the platform shell under their own error boundary.
export const routes: RouteRecordRaw[] = [
  { path: '/paperless', name: 'paperless-documents', component: () => import('@/views/documents/index.vue'), meta: { module: 'paperless' } },
  { path: '/paperless/categories', name: 'paperless-categories', component: () => import('@/views/categories/index.vue'), meta: { module: 'paperless' } },
  { path: '/paperless/search', name: 'paperless-search', component: () => import('@/views/search/index.vue'), meta: { module: 'paperless' } },
  { path: '/paperless/dashboard', name: 'paperless-dashboard', component: () => import('@/views/dashboard/index.vue'), meta: { module: 'paperless' } },
]
export default routes
