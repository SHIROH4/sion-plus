const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
  dragStart: () => ipcRenderer.send('drag:start'),
  dragStop: () => ipcRenderer.send('drag:stop'),
  resizeWindow: (w, h) => ipcRenderer.send('window:resize', w, h),
})
