declare module 'shell/vben/common-ui' {
  import type { Component, DefineComponent } from 'vue';
  export const Page: DefineComponent<any, any, any>;
  export function useVbenDrawer(options?: any): [Component, any];
  export function useVbenModal(options?: any): [Component, any];
  export type VbenFormProps = any;
}

declare module 'shell/vben/icons' {
  import type { Component } from 'vue';
  export const LucideArrowLeft: Component;
  export const LucideCalendar: Component;
  export const LucideCopy: Component;
  export const LucideDownload: Component;
  export const LucideEye: Component;
  export const LucideFile: Component;
  export const LucideFileSignature: Component;
  export const LucideFolder: Component;
  export const LucideFolderOpen: Component;
  export const LucideFolderPlus: Component;
  export const LucideGripVertical: Component;
  export const LucideKey: Component;
  export const LucideLayoutGrid: Component;
  export const LucideMail: Component;
  export const LucidePencil: Component;
  export const LucidePenLine: Component;
  export const LucidePenTool: Component;
  export const LucidePlus: Component;
  export const LucideSave: Component;
  export const LucideShare2: Component;
  export const LucideSquareCheck: Component;
  export const LucideTrash: Component;
  export const LucideType: Component;
  export const LucideUpload: Component;
  export const LucideX: Component;
  export const LucideXCircle: Component;
}

declare module 'shell/vben/stores' {
  import type { StoreDefinition } from 'pinia';
  export const useAccessStore: StoreDefinition;
}

declare module 'shell/vben/layouts' {
  import type { Component } from 'vue';
  export const BasicLayout: Component;
}

declare module 'shell/adapter/vxe-table' {
  import type { DefineComponent } from 'vue';
  export function useVbenVxeGrid(options: any): [DefineComponent<any, any, any>, any];
  export type VxeGridProps<T = any> = any;
}

declare module 'shell/locales' {
  export function $t(key: string, ...args: any[]): string;
}
