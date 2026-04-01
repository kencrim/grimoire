import { useEffect, useState, useCallback } from 'react';
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
import * as Haptics from 'expo-haptics';
import { useRelay } from '../_layout';
import { NativeTerminalView } from '../../components/NativeTerminalView';
import type { PaneFrame } from '../../lib/types';
import { catppuccin } from '../../lib/theme';

type PaneTab = 'agent' | 'terminal';

export default function StreamScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { client, agents } = useRelay();
  const [activeTab, setActiveTab] = useState<PaneTab>('agent');
  const [inputText, setInputText] = useState('');
  const [latestFrame, setLatestFrame] = useState<PaneFrame | null>(null);

  // Find the agent to get pane info
  const agent = agents.find((a) => a.id === id);
  const displayName = id?.includes('/') ? id.split('/').pop() : id;

  // The agent ID is what we pass to the daemon — it resolves the pane internally
  const agentRef = id ?? '';

  // Connect pane WebSocket and receive frames
  useEffect(() => {
    if (!client || !agentRef) return;

    setLatestFrame(null);
    client.connectPane(agentRef, activeTab);

    const unsub = client.onPane((frame: PaneFrame) => {
      setLatestFrame(frame);
    });

    return () => {
      unsub();
      client.disconnectPane();
    };
  }, [client, agentRef, activeTab]);

  // Handle send button for the text input bar
  const handleSend = useCallback(() => {
    if (!inputText.trim() || !client) return;

    // Send as literal text + Enter
    client.sendPaneInput({ type: 'input', data: inputText });
    client.sendPaneInput({ type: 'special', data: 'Enter' });

    setInputText('');
    Keyboard.dismiss();
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  }, [inputText, client]);

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

      {/* Native terminal renderer */}
      <View style={styles.terminal}>
        <NativeTerminalView frame={latestFrame} />
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
