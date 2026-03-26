import './styles/tailwind.css';
import type { TangraModule } from './sdk';
import routes from './routes';
import { usePaperlessCategoryStore } from './stores/paperless-category.state';
import { usePaperlessDocumentStore } from './stores/paperless-document.state';
import { usePaperlessPermissionStore } from './stores/paperless-permission.state';
import enUS from './locales/en-US.json';

const paperlessModule: TangraModule = {
  id: 'paperless',
  version: '1.0.0',
  routes,
  stores: {
    'paperless-category': usePaperlessCategoryStore,
    'paperless-document': usePaperlessDocumentStore,
    'paperless-permission': usePaperlessPermissionStore,
  },
  locales: {
    'en-US': enUS,
  },
};

export default paperlessModule;
