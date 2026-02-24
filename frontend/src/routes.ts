import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  {
    path: '/paperless',
    name: 'Paperless',
    component: () => import('shell/vben/layouts').then((m) => m.BasicLayout),
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
      {
        path: 'signing/templates',
        name: 'PaperlessSigningTemplates',
        meta: {
          icon: 'lucide:file-signature',
          title: 'paperless.menu.signingTemplates',
          authority: ['platform:admin', 'tenant:manager'],
        },
        component: () => import('./views/signing/templates/index.vue'),
      },
      {
        path: 'signing/templates/:id/builder',
        name: 'PaperlessSigningBuilder',
        meta: {
          title: 'paperless.menu.signingTemplates',
          hideInMenu: true,
          authority: ['platform:admin', 'tenant:manager'],
        },
        component: () => import('./views/signing/builder/index.vue'),
      },
      {
        path: 'signing/requests',
        name: 'PaperlessSigningRequests',
        meta: {
          icon: 'lucide:pen-tool',
          title: 'paperless.menu.signingRequests',
          authority: ['platform:admin', 'tenant:manager'],
        },
        component: () => import('./views/signing/requests/index.vue'),
      },
      {
        path: 'signing/session/:token',
        name: 'PaperlessSigningSession',
        meta: {
          title: 'paperless.menu.signingRequests',
          hideInMenu: true,
        },
        component: () => import('./views/signing/session/index.vue'),
      },
    ],
  },
];

export default routes;
