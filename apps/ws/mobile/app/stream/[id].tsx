import { useEffect, useState, useCallback, useMemo } from 'react';
import {
  View,
  Text,
  TextInput,
  Pressable,
  Image,
  StyleSheet,
  Keyboard,
  Platform,
  Alert,
  ScrollView,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, Stack, router } from 'expo-router';
import FontAwesome from '@expo/vector-icons/FontAwesome';
import * as Haptics from 'expo-haptics';
import * as ImagePicker from 'expo-image-picker';
import { useRelay } from '../_layout';
import { showWorkstreamActions } from '../../components/StreamTree';
import { NativeTerminalView } from '../../components/NativeTerminalView';
import type { PaneFrame } from '../../lib/types';
import { catppuccin } from '../../lib/theme';

type PaneTab = 'agent' | 'terminal';

interface PendingImage {
  uri: string;
  filename: string;
}

export default function StreamScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { client, agents } = useRelay();
  const [activeTab, setActiveTab] = useState<PaneTab>('agent');
  const [inputText, setInputText] = useState('');
  const [latestFrame, setLatestFrame] = useState<PaneFrame | null>(null);
  const [pendingImages, setPendingImages] = useState<PendingImage[]>([]);
  const [uploading, setUploading] = useState(false);

  // Find the agent to get pane info
  const agent = agents.find((a) => a.id === id);
  const displayName = id?.includes('/') ? id.split('/').pop() : id;

  // Build a StreamNode for the action sheet helper
  const streamNode = useMemo(() => ({
    id: id ?? '',
    name: displayName ?? '',
    agent: agent?.agent ?? '',
    status: agent?.status ?? '',
    color: agent?.color,
    children: [],
    depth: 0,
  }), [id, displayName, agent]);

  const handleActions = useCallback(() => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    showWorkstreamActions(streamNode, () => {
      client?.killAgent(streamNode.id).then(() => {
        router.back();
      });
    });
  }, [streamNode, client]);

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

  // Pick images — stage them without sending
  const handleImagePick = useCallback(async () => {
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images'],
      allowsMultipleSelection: true,
      quality: 0.8,
    });

    if (result.canceled || !result.assets.length) return;

    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    const newImages = result.assets.map((asset) => ({
      uri: asset.uri,
      filename: asset.fileName ?? `image-${Date.now()}.png`,
    }));
    setPendingImages((prev) => [...prev, ...newImages]);
  }, []);

  const removeImage = useCallback((index: number) => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    setPendingImages((prev) => prev.filter((_, i) => i !== index));
  }, []);

  // Small delay to let tmux process each line
  const delay = (ms: number) => new Promise((r) => setTimeout(r, ms));

  // Send text + any pending images
  const handleSend = useCallback(async () => {
    if (!client) return;
    if (!inputText.trim() && pendingImages.length === 0) return;

    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    const text = inputText.trim();
    const images = [...pendingImages];

    // Clear UI immediately
    setInputText('');
    setPendingImages([]);
    Keyboard.dismiss();

    // Upload all images in parallel, then send paths + text
    if (images.length > 0) {
      setUploading(true);
      try {
        const paths = await Promise.all(
          images.map((img) => client.uploadImage(img.uri, img.filename)),
        );
        // All paths + text on one line, space-separated
        const combined = [...paths, ...(text ? [text] : [])].join(' ');
        client.sendPaneInput({ type: 'input', data: combined });
        await delay(50);
        client.sendPaneInput({ type: 'special', data: 'Enter' });
      } catch (err) {
        Alert.alert('Upload failed', String(err));
      }
      setUploading(false);
    } else if (text) {
      // Text only — no images
      client.sendPaneInput({ type: 'input', data: text });
      await delay(50);
      client.sendPaneInput({ type: 'special', data: 'Enter' });
    }
  }, [inputText, pendingImages, client]);

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
  const hasContent = inputText.trim().length > 0 || pendingImages.length > 0;

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
              <Pressable onPress={handleActions} hitSlop={8}>
                <FontAwesome name="ellipsis-h" size={18} color={catppuccin.subtext0} />
              </Pressable>
            </View>
          ),
        }}
      />

      {/* Terminal */}
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

      {/* Compose area */}
      <View style={styles.compose}>
        {/* Image thumbnails */}
        {pendingImages.length > 0 && (
          <ScrollView
            horizontal
            showsHorizontalScrollIndicator={false}
            style={styles.thumbnailRow}
            contentContainerStyle={styles.thumbnailContent}
          >
            {pendingImages.map((img, i) => (
              <View key={`${img.uri}-${i}`} style={styles.thumbnail}>
                <Image source={{ uri: img.uri }} style={styles.thumbnailImage} />
                <Pressable
                  style={styles.thumbnailRemove}
                  onPress={() => removeImage(i)}
                  hitSlop={6}
                >
                  <FontAwesome name="times-circle" size={18} color={catppuccin.red} />
                </Pressable>
              </View>
            ))}
          </ScrollView>
        )}

        {/* Input row */}
        <View style={styles.inputRow}>
          <Pressable onPress={handleImagePick} hitSlop={6} style={styles.attachButton}>
            <FontAwesome name="plus" size={18} color={catppuccin.overlay1} />
          </Pressable>
          <TextInput
            style={styles.input}
            value={inputText}
            onChangeText={setInputText}
            placeholder="Message..."
            placeholderTextColor={catppuccin.overlay0}
            autoCapitalize="none"
            autoCorrect={false}
            returnKeyType="send"
            onSubmitEditing={handleSend}
            blurOnSubmit={false}
          />
          <Pressable
            style={[styles.sendButton, !hasContent && styles.sendButtonDisabled]}
            onPress={handleSend}
            disabled={!hasContent || uploading}
          >
            <FontAwesome
              name="arrow-up"
              size={16}
              color={hasContent ? catppuccin.base : catppuccin.surface2}
            />
          </Pressable>
        </View>

        {/* Safe area spacer — keeps input row centered in the footer */}
        <View style={{ height: bottomPadding }} />
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
    gap: 8,
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
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: catppuccin.surface0,
  },
  tab: {
    flex: 1,
    paddingVertical: 8,
    alignItems: 'center',
  },
  tabActive: {
    borderBottomWidth: 2,
    borderBottomColor: catppuccin.lavender,
  },
  tabText: {
    fontSize: 13,
    fontWeight: '500',
    color: catppuccin.overlay0,
  },
  tabTextActive: {
    color: catppuccin.lavender,
  },
  // Compose area
  compose: {
    backgroundColor: catppuccin.mantle,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: catppuccin.surface0,
  },
  thumbnailRow: {
    maxHeight: 72,
  },
  thumbnailContent: {
    paddingHorizontal: 12,
    paddingTop: 8,
    gap: 8,
  },
  thumbnail: {
    position: 'relative',
    width: 56,
    height: 56,
    borderRadius: 8,
    overflow: 'visible',
  },
  thumbnailImage: {
    width: 56,
    height: 56,
    borderRadius: 8,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: catppuccin.surface2,
  },
  thumbnailRemove: {
    position: 'absolute',
    top: -6,
    right: -6,
    backgroundColor: catppuccin.mantle,
    borderRadius: 9,
  },
  inputRow: {
    flexDirection: 'row',
    alignItems: 'flex-end',
    paddingHorizontal: 16,
    paddingTop: 12,
    paddingBottom: 12,
    gap: 10,
  },
  attachButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: catppuccin.surface0,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 2,
  },
  input: {
    flex: 1,
    backgroundColor: catppuccin.surface0,
    color: catppuccin.text,
    borderRadius: 20,
    paddingHorizontal: 16,
    paddingTop: 10,
    paddingBottom: 10,
    fontSize: 16,
    lineHeight: 22,
    maxHeight: 120,
  },
  sendButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: catppuccin.lavender,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 2,
  },
  sendButtonDisabled: {
    backgroundColor: catppuccin.surface0,
  },
  sendText: {
    color: catppuccin.base,
    fontSize: 14,
    fontWeight: '600',
  },
});
