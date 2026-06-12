const { app, BrowserWindow, screen, ipcMain } = require('electron')
const { spawn, execSync } = require('child_process')
const { join } = require('path')
const { existsSync } = require('fs')

const isDev = !app.isPackaged
const VITE_DEV_URL = 'http://localhost:5173'
const PRELOAD = join(__dirname, 'preload.cjs')

let goProcess = null
let petWindow = null

// Kill processes by port
function killPort(port) {
  try {
    const pids = execSync(`lsof -ti :${port}`, { encoding: 'utf8' }).trim()
    if (pids) {
      for (const pid of pids.split('\n')) {
        try { process.kill(parseInt(pid), 'SIGTERM') } catch { /* */ }
      }
      console.log(`[Sion] killed port :${port} (pids: ${pids})`)
    }
  } catch { /* no process on port */ }
}

function cleanup() {
  killPort(8080)  // Go backend
  killPort(5173)  // Vite dev server
  if (goProcess) { goProcess.kill('SIGTERM'); goProcess = null }
}

function startGoBackend() {
  // In dev, assume user starts Go server manually. In production, spawn sidecar.
  if (isDev) return
  const binPath = join(process.resourcesPath, 'sion-server')
  if (!existsSync(binPath)) return
  console.log('[Sion] starting Go backend:', binPath)
  goProcess = spawn(binPath, ['server'], { stdio: 'inherit', env: { ...process.env } })
  goProcess.on('exit', (code) => console.log('[Sion] Go backend exited:', code))
}

function loadURL(win, path) {
  if (isDev) win.loadURL(`${VITE_DEV_URL}${path}`)
  else {
    const f = path === '/' ? 'index.html' : 'pet.html'
    win.loadFile(join(__dirname, '..', 'dist', f))
  }
}

function createDashboardWindow() {
  const win = new BrowserWindow({
    title: 'Sion Dashboard',
    width: 960, height: 680,
    minWidth: 800, minHeight: 500,
    transparent: true,
    vibrancy: 'under-window',
    visualEffectState: 'active',
    titleBarStyle: 'hiddenInset',
    trafficLightPosition: { x: 12, y: 10 },
    webPreferences: { contextIsolation: true, nodeIntegration: false },
  })
  loadURL(win, '/')
  if (isDev) win.webContents.openDevTools({ mode: 'detach' })
  return win
}

function createPetWindow() {
  const { width: screenW } = screen.getPrimaryDisplay().workAreaSize
  petWindow = new BrowserWindow({
    title: 'Sion',
    width: 320, height: 440,
    minWidth: 260, minHeight: 320,
    resizable: false,
    frame: false,
    transparent: true,
    hasShadow: false,
    alwaysOnTop: true,
    x: screenW - 340, y: 60,
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      preload: PRELOAD,
    },
  })
  petWindow.setVisibleOnAllWorkspaces(true)
  petWindow.setAlwaysOnTop(true, 'floating')
  loadURL(petWindow, '/pet.html')
  if (isDev) petWindow.webContents.openDevTools({ mode: 'detach' })
  return petWindow
}

// ── Window resize from renderer ──
ipcMain.on('window:resize', (event, w, h) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (win) {
    const [x, y] = win.getPosition()
    win.setBounds({ x, y, width: Math.round(w), height: Math.round(h) })
  }
})

// ── JS-based window drag (no feedback loop) ──
// Uses screen-absolute coordinates to avoid the renderer
// pointermove feedback loop that causes flicker.

let dragState = null

ipcMain.on('drag:start', (event) => {
  const win = BrowserWindow.fromWebContents(event.sender)
  if (!win || dragState) return
  const cursor = screen.getCursorScreenPoint()
  const [wx, wy] = win.getPosition()
  dragState = { win, sx: cursor.x, sy: cursor.y, wx, wy }
})

ipcMain.on('drag:stop', () => {
  dragState = null
})

// Poll cursor position during drag (smooth, no renderer feedback)
function dragLoop() {
  if (!dragState) return
  const cursor = screen.getCursorScreenPoint()
  const nx = dragState.wx + (cursor.x - dragState.sx)
  const ny = dragState.wy + (cursor.y - dragState.sy)
  dragState.win.setPosition(Math.round(nx), Math.round(ny))
}

// Run drag loop at ~60fps when dragging
setInterval(dragLoop, 16)

app.whenReady().then(() => {
  startGoBackend()
  createDashboardWindow()
  createPetWindow()
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createDashboardWindow()
      createPetWindow()
    }
  })
})

app.on('window-all-closed', () => { cleanup(); if (process.platform !== 'darwin') app.quit() })
app.on('before-quit', cleanup)
app.on('will-quit', cleanup)

// Also kill on Ctrl+C / SIGTERM
process.on('SIGTERM', () => { cleanup(); app.quit() })
process.on('SIGINT', () => { cleanup(); app.quit() })
