import { useRef, useEffect, useCallback } from 'react';
import { View, StyleSheet } from 'react-native';
import { WebView } from 'react-native-webview';
import type { WebViewMessageEvent } from 'react-native-webview';
import {
  generateXtermHtml,
  writeToTerminal,
  parseWebViewMessage,
  toPaneInput,
} from '../lib/terminal-bridge';
import type { PaneFrame, PaneInputMsg } from '../lib/types';
import { hex } from '../lib/theme';

interface TerminalViewProps {
  onInput: (input: PaneInputMsg) => void;
  onReady?: (cols: number, rows: number) => void;
}

export function TerminalView({ onInput, onReady }: TerminalViewProps) {
  const webViewRef = useRef<WebView>(null);
  const htmlRef = useRef(generateXtermHtml());

  // Write pane frame content to the terminal
  const writeFrame = useCallback((frame: PaneFrame) => {
    writeToTerminal(webViewRef, frame.content);
  }, []);

  const handleMessage = useCallback(
    (event: WebViewMessageEvent) => {
      const msg = parseWebViewMessage(event.nativeEvent.data);
      if (!msg) return;

      if (msg.type === 'ready' && onReady && msg.cols && msg.rows) {
        onReady(msg.cols, msg.rows);
        return;
      }

      const paneInput = toPaneInput(msg);
      if (paneInput) {
        onInput(paneInput);
      }
    },
    [onInput, onReady]
  );

  return (
    <View style={styles.container}>
      <WebView
        ref={webViewRef}
        source={{ html: htmlRef.current }}
        style={styles.webview}
        originWhitelist={['*']}
        javaScriptEnabled
        onMessage={handleMessage}
        scrollEnabled={false}
        bounces={false}
        overScrollMode="never"
        keyboardDisplayRequiresUserAction={false}
        hideKeyboardAccessoryView
        contentMode="mobile"
        allowsInlineMediaPlayback
        mediaPlaybackRequiresUserAction={false}
        setBuiltInZoomControls={false}
        showsHorizontalScrollIndicator={false}
        showsVerticalScrollIndicator={false}
      />
    </View>
  );
}

// Expose writeFrame for parent to call imperatively
TerminalView.writeFrame = (
  ref: React.RefObject<{ writeFrame: (frame: PaneFrame) => void } | null>,
  frame: PaneFrame
) => {
  ref.current?.writeFrame(frame);
};

export { type TerminalViewProps };

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: hex.base,
  },
  webview: {
    flex: 1,
    backgroundColor: 'transparent',
  },
});
