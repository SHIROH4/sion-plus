/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

interface Window {
  electronAPI?: {
    dragStart: () => void
    dragStop: () => void
    resizeWindow: (w: number, h: number) => void
  }
}
