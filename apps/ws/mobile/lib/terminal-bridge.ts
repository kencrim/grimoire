import type { RefObject } from 'react';
import type WebView from 'react-native-webview';
import type { PaneInputMsg } from './types';
import { xtermTheme } from './theme';

// Messages sent from React Native to the WebView
interface ToWebView {
  type: 'write' | 'resize' | 'clear' | 'setTheme';
  data?: string;
  cols?: number;
  rows?: number;
  theme?: Record<string, string>;
}

// Messages sent from the WebView to React Native
export interface FromWebView {
  type: 'input' | 'ready' | 'fit' | 'special';
  data?: string;
  cols?: number;
  rows?: number;
}

// Send terminal output bytes to the xterm.js WebView
// If cols/rows are provided, xterm.js will reset before writing (full snapshot).
// If omitted, it appends (incremental pipe-pane stream).
export function writeToTerminal(
  webViewRef: RefObject<WebView | null>,
  content: string,
  cols?: number,
  rows?: number
): void {
  if (!webViewRef.current) return;
  const msg: ToWebView = { type: 'write', data: content, cols, rows };
  webViewRef.current.postMessage(JSON.stringify(msg));
}

// Clear the terminal
export function clearTerminal(webViewRef: RefObject<WebView | null>): void {
  if (!webViewRef.current) return;
  webViewRef.current.postMessage(JSON.stringify({ type: 'clear' }));
}

// Resize the terminal
export function resizeTerminal(
  webViewRef: RefObject<WebView | null>,
  cols: number,
  rows: number
): void {
  if (!webViewRef.current) return;
  const msg: ToWebView = { type: 'resize', cols, rows };
  webViewRef.current.postMessage(JSON.stringify(msg));
}

// Parse a message from the WebView
export function parseWebViewMessage(data: string): FromWebView | null {
  try {
    return JSON.parse(data);
  } catch {
    return null;
  }
}

// Convert a FromWebView message to a PaneInputMsg for the daemon
export function toPaneInput(msg: FromWebView): PaneInputMsg | null {
  switch (msg.type) {
    case 'input':
      return { type: 'input', data: msg.data ?? '' };
    case 'special':
      return { type: 'special', data: msg.data ?? '' };
    default:
      return null;
  }
}

// Generate the xterm.js HTML with the Catppuccin Mocha theme baked in
export function generateXtermHtml(): string {
  const themeJson = JSON.stringify(xtermTheme);

  return `<!DOCTYPE html>
<html>
<head>
<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  html, body { width: 100%; height: 100%; overflow: hidden; background: ${xtermTheme.background}; }
  #terminal { width: 100%; height: 100%; }
  .xterm { padding: 4px; }
</style>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/css/xterm.css">
</head>
<body>
<div id="terminal"></div>
<script src="https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/lib/xterm.js"></script>
<script src="https://cdn.jsdelivr.net/npm/@xterm/addon-fit@0.10.0/lib/addon-fit.js"></script>
<script>
(function() {
  const theme = ${themeJson};

  const term = new Terminal({
    fontSize: 12,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    scrollback: 2000,
    cursorBlink: true,
    cursorStyle: 'block',
    allowTransparency: true,
    theme: theme,
    convertEol: true,
  });

  const fitAddon = new FitAddon.FitAddon();
  term.loadAddon(fitAddon);
  term.open(document.getElementById('terminal'));

  // Initial fit
  setTimeout(function() {
    fitAddon.fit();
    sendToRN({ type: 'ready', cols: term.cols, rows: term.rows });
  }, 100);

  // Re-fit on window resize
  window.addEventListener('resize', function() {
    fitAddon.fit();
    sendToRN({ type: 'fit', cols: term.cols, rows: term.rows });
  });

  // Track whether the user has scrolled up — if so, don't auto-scroll to bottom
  var userScrolledUp = false;
  term.onScroll(function() {
    var viewport = term.buffer.active;
    var atBottom = viewport.baseY + term.rows >= viewport.length;
    userScrolledUp = !atBottom;
  });

  // Pinch-to-zoom for font size
  var currentFontSize = 12;
  var pinchStartDist = 0;
  var pinchStartFontSize = 12;

  document.addEventListener('touchstart', function(e) {
    if (e.touches.length === 2) {
      var dx = e.touches[0].clientX - e.touches[1].clientX;
      var dy = e.touches[0].clientY - e.touches[1].clientY;
      pinchStartDist = Math.sqrt(dx * dx + dy * dy);
      pinchStartFontSize = currentFontSize;
    }
  }, { passive: true });

  document.addEventListener('touchmove', function(e) {
    if (e.touches.length === 2) {
      var dx = e.touches[0].clientX - e.touches[1].clientX;
      var dy = e.touches[0].clientY - e.touches[1].clientY;
      var dist = Math.sqrt(dx * dx + dy * dy);
      var scale = dist / pinchStartDist;
      var newSize = Math.round(pinchStartFontSize * scale);
      newSize = Math.max(8, Math.min(24, newSize));
      if (newSize !== currentFontSize) {
        currentFontSize = newSize;
        term.options.fontSize = newSize;
        fitAddon.fit();
        sendToRN({ type: 'fit', cols: term.cols, rows: term.rows });
      }
    }
  }, { passive: true });

  // Handle keyboard input
  term.onData(function(data) {
    sendToRN({ type: 'input', data: data });
  });

  // Handle special keys
  term.onKey(function(ev) {
    // Map special keys to tmux key names
    var key = ev.domEvent.key;
    var ctrl = ev.domEvent.ctrlKey;
    var special = null;

    if (key === 'Enter') special = 'Enter';
    else if (key === 'Backspace') special = 'BSpace';
    else if (key === 'Tab') special = 'Tab';
    else if (key === 'Escape') special = 'Escape';
    else if (key === 'ArrowUp') special = 'Up';
    else if (key === 'ArrowDown') special = 'Down';
    else if (key === 'ArrowLeft') special = 'Left';
    else if (key === 'ArrowRight') special = 'Right';
    else if (ctrl && key === 'c') special = 'C-c';
    else if (ctrl && key === 'd') special = 'C-d';
    else if (ctrl && key === 'z') special = 'C-z';
    else if (ctrl && key === 'l') special = 'C-l';

    if (special) {
      sendToRN({ type: 'special', data: special });
    }
  });

  // Listen for messages from React Native
  window.addEventListener('message', function(event) {
    handleMessage(event.data);
  });
  // iOS
  document.addEventListener('message', function(event) {
    handleMessage(event.data);
  });

  function handleMessage(raw) {
    try {
      var msg = JSON.parse(raw);
      switch (msg.type) {
        case 'write':
          if (msg.data) {
            if (msg.cols && msg.rows) {
              term.write('\x1b[H\x1b[2J' + msg.data, function() {
                if (!userScrolledUp) {
                  term.scrollToBottom();
                }
              });
            } else {
              term.write(msg.data, function() {
                if (!userScrolledUp) {
                  term.scrollToBottom();
                }
              });
            }
          }
          break;
        case 'clear':
          term.reset();
          break;
        case 'resize':
          if (msg.cols && msg.rows) {
            term.resize(msg.cols, msg.rows);
          }
          break;
        case 'setTheme':
          if (msg.theme) {
            term.options.theme = msg.theme;
          }
          break;
      }
    } catch(e) {}
  }

  function sendToRN(msg) {
    if (window.ReactNativeWebView) {
      window.ReactNativeWebView.postMessage(JSON.stringify(msg));
    }
  }
})();
</script>
</body>
</html>`;
}
