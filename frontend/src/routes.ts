import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  {
    path: '/paperless',
    name: 'Paperless',
    component: () => import('shell/app-layout'),
    redirect: '/paperless/documents',
    meta: {
      order: 2020,
      icon: 'lucide:file-text',
      title: 'paperless.menu.paperless',
      keepAlive: true,
      authority: ['platform:admin', 'tenant:manager'],
    },
    children: [
      {
        path: 'documents',
        name: 'PaperlessDocuments',
        meta: {
          icon: 'lucide:files',
          title: 'paperless.menu.documents',
          authority: ['platform:admin', 'tenant:manager'],
        },
        component: () => import('./views/document/index.vue'),
      },
      {
        path: 'categories',
        name: 'PaperlessCategories',
        meta: {
          icon: 'lucide:folder-tree',
          title: 'paperless.menu.categories',
          authority: ['platform:admin', 'tenant:manager'],
        },
        component: () => import('./views/category/index.vue'),
      },
    ],
  },
];

export default routes;
