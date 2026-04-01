import { useEffect, useRef, useState, useCallback } from 'react';
import {
  View,
  Text,
  TextInput,
  Pressable,
  StyleSheet,
  Keyboard,
  Platform,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, Stack } from 'expo-router';
import { WebView } from 'react-native-webview';
import type { WebViewMessageEvent } from 'react-native-webview';
import * as Haptics from 'expo-haptics';
import { useRelay } from '../_layout';
import {
  generateXtermHtml,
  writeToTerminal,
  clearTerminal,
  parseWebViewMessage,
  toPaneInput,
} from '../../lib/terminal-bridge';
import type { PaneFrame, PaneInputMsg } from '../../lib/types';
import { catppuccin } from '../../lib/theme';

type PaneTab = 'agent' | 'terminal';

export default function StreamScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { client, agents } = useRelay();
  const webViewRef = useRef<WebView>(null);
  const htmlRef = useRef(generateXtermHtml());
  const [activeTab, setActiveTab] = useState<PaneTab>('agent');
  const [inputText, setInputText] = useState('');
  const [termReady, setTermReady] = useState(false);

  // Find the agent to get pane info
  const agent = agents.find((a) => a.id === id);
  const displayName = id?.includes('/') ? id.split('/').pop() : id;

  // The agent ID is what we pass to the daemon — it resolves the pane internally
  const agentRef = id ?? '';

  // Connect pane WebSocket when ready, reconnect when tab switches
  useEffect(() => {
    if (!client || !agentRef || !termReady) return;

    clearTerminal(webViewRef);
    client.connectPane(agentRef, activeTab);

    const unsub = client.onPane((frame: PaneFrame) => {
      // Every frame is a full capture-pane snapshot with cols/rows matching the
      // desktop terminal. xterm.js resizes to match and clears before writing.
      writeToTerminal(webViewRef, frame.content, frame.cols, frame.rows);
    });

    return () => {
      unsub();
      client.disconnectPane();
    };
  }, [client, agentRef, termReady, activeTab]);

  // Handle messages from xterm.js WebView
  const handleWebViewMessage = useCallback(
    (event: WebViewMessageEvent) => {
      const msg = parseWebViewMessage(event.nativeEvent.data);
      if (!msg) return;

      if (msg.type === 'ready') {
        setTermReady(true);
        return;
      }

      if (msg.type === 'error') {
        console.error('[xterm.js]', msg.data);
        return;
      }

      // Forward input to the daemon
      const paneInput = toPaneInput(msg);
      if (paneInput && client) {
        client.sendPaneInput(paneInput);
      }
    },
    [client]
  );

  // Handle send button for the text input bar
  const handleSend = () => {
    if (!inputText.trim() || !client) return;

    // Send as literal text + Enter
    client.sendPaneInput({ type: 'input', data: inputText });
    client.sendPaneInput({ type: 'special', data: 'Enter' });

    setInputText('');
    Keyboard.dismiss();
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  };

  const switchTab = (tab: PaneTab) => {
    if (tab === activeTab) return;
    setActiveTab(tab);
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  };

  const insets = useSafeAreaInsets();
  const [keyboardHeight, setKeyboardHeight] = useState(0);

  useEffect(() => {
    const showSub = Keyboard.addListener(
      Platform.OS === 'ios' ? 'keyboardWillShow' : 'keyboardDidShow',
      (e) => setKeyboardHeight(e.endCoordinates.height)
    );
    const hideSub = Keyboard.addListener(
      Platform.OS === 'ios' ? 'keyboardWillHide' : 'keyboardDidHide',
      () => setKeyboardHeight(0)
    );
    return () => {
      showSub.remove();
      hideSub.remove();
    };
  }, []);

  const bottomPadding = keyboardHeight > 0 ? keyboardHeight : insets.bottom;

  const statusColor =
    agent?.status === 'alive'
      ? catppuccin.green
      : agent?.status === 'idle'
        ? catppuccin.yellow
        : catppuccin.overlay0;

  return (
    <View style={styles.container}>
      <Stack.Screen
        options={{
          title: displayName ?? 'Terminal',
          headerRight: () => (
            <View style={styles.headerRight}>
              <View style={[styles.statusDot, { backgroundColor: statusColor }]} />
              <Text style={styles.headerAgent}>{agent?.agent ?? ''}</Text>
            </View>
          ),
        }}
      />

      {/* Terminal WebView */}
      <View style={styles.terminal}>
        <WebView
          ref={webViewRef}
          source={{ html: htmlRef.current }}
          style={styles.webview}
          originWhitelist={['*']}
          javaScriptEnabled
          onMessage={handleWebViewMessage}
          scrollEnabled
          bounces
          overScrollMode="always"
          keyboardDisplayRequiresUserAction={false}
          hideKeyboardAccessoryView
          showsHorizontalScrollIndicator={false}
          showsVerticalScrollIndicator
        />
      </View>

      {/* Pane tab switcher */}
      <View style={styles.tabBar}>
        <Pressable
          style={[styles.tab, activeTab === 'agent' && styles.tabActive]}
          onPress={() => switchTab('agent')}
        >
          <Text style={[styles.tabText, activeTab === 'agent' && styles.tabTextActive]}>
            Agent
          </Text>
        </Pressable>
        <Pressable
          style={[styles.tab, activeTab === 'terminal' && styles.tabActive]}
          onPress={() => switchTab('terminal')}
        >
          <Text style={[styles.tabText, activeTab === 'terminal' && styles.tabTextActive]}>
            Terminal
          </Text>
        </Pressable>
      </View>

      {/* Input bar */}
      <View style={[styles.inputBar, { paddingBottom: bottomPadding }]}>
        <TextInput
          style={styles.input}
          value={inputText}
          onChangeText={setInputText}
          placeholder="$ type a command..."
          placeholderTextColor={catppuccin.overlay0}
          autoCapitalize="none"
          autoCorrect={false}
          returnKeyType="send"
          onSubmitEditing={handleSend}
          blurOnSubmit={false}
        />
        <Pressable style={styles.sendButton} onPress={handleSend}>
          <Text style={styles.sendText}>Send</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: catppuccin.base,
  },
  headerRight: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  statusDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
  },
  headerAgent: {
    fontSize: 13,
    color: catppuccin.subtext0,
  },
  terminal: {
    flex: 1,
  },
  webview: {
    flex: 1,
    backgroundColor: 'transparent',
  },
  tabBar: {
    flexDirection: 'row',
    backgroundColor: catppuccin.mantle,
    borderTopWidth: 1,
    borderTopColor: catppuccin.surface0,
  },
  tab: {
    flex: 1,
    paddingVertical: 10,
    alignItems: 'center',
  },
  tabActive: {
    borderBottomWidth: 2,
    borderBottomColor: catppuccin.lavender,
  },
  tabText: {
    fontSize: 14,
    fontWeight: '500',
    color: catppuccin.overlay0,
  },
  tabTextActive: {
    color: catppuccin.lavender,
  },
  inputBar: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: catppuccin.mantle,
    paddingHorizontal: 12,
    paddingVertical: 8,
    gap: 8,
  },
  input: {
    flex: 1,
    backgroundColor: catppuccin.surface0,
    color: catppuccin.text,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 14,
    fontFamily: 'Menlo',
  },
  sendButton: {
    backgroundColor: catppuccin.lavender,
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 8,
  },
  sendText: {
    color: catppuccin.base,
    fontSize: 14,
    fontWeight: '600',
  },
});
