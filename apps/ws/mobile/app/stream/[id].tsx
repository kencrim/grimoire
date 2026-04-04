import { useEffect, useRef, useState, useCallback, useMemo } from 'react';
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
import {
  ExpoSpeechRecognitionModule,
  useSpeechRecognitionEvent,
} from 'expo-speech-recognition';
import { useRelay } from '../_layout';
import { NativeTerminalView } from '../../components/NativeTerminalView';
import type { PaneFrame } from '../../lib/types';
import { catppuccin } from '../../lib/theme';

type PaneTab = 'agent' | 'terminal';

interface PendingImage {
  uri: string;
  fileName: string;
  mimeType?: string;
}

export default function StreamScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { client, agents } = useRelay();
  const [activeTab, setActiveTab] = useState<PaneTab>('agent');
  const [inputText, setInputText] = useState('');
  const [latestFrame, setLatestFrame] = useState<PaneFrame | null>(null);
  const [pendingImages, setPendingImages] = useState<PendingImage[]>([]);
  const [uploading, setUploading] = useState(false);
  const [recognizing, setRecognizing] = useState(false);

  // Speech recognition — streams transcription into the input field
  useSpeechRecognitionEvent('start', () => setRecognizing(true));
  useSpeechRecognitionEvent('end', () => setRecognizing(false));
  useSpeechRecognitionEvent('result', (event) => {
    const transcript = event.results[0]?.transcript ?? '';
    if (transcript) {
      setInputText(transcript);
    }
  });

  // Request mic permissions once on mount
  const micPermitted = useRef(false);
  useEffect(() => {
    ExpoSpeechRecognitionModule.requestPermissionsAsync().then((result) => {
      micPermitted.current = result.granted;
    });
  }, []);

  const handleMic = useCallback(() => {
    if (recognizing) {
      ExpoSpeechRecognitionModule.stop();
      return;
    }
    if (!micPermitted.current) {
      Alert.alert('Permission required', 'Microphone and speech recognition access is needed.');
      return;
    }
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    ExpoSpeechRecognitionModule.start({
      lang: 'en-US',
      interimResults: true,
      continuous: true,
      addsPunctuation: true,
    });
  }, [recognizing]);

  // Find the agent to get pane info
  const agent = agents.find((a) => a.id === id);
  const displayName = id?.includes('/') ? id.split('/').pop() : id;

  const handleActions = useCallback(() => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    Alert.alert(
      'Kill workstream?',
      `This will destroy the worktree and tmux session for "${displayName}" and all its children.`,
      [
        { text: 'Cancel', style: 'cancel' },
        {
          text: 'Kill',
          style: 'destructive',
          onPress: () => {
            client?.killAgent(id ?? '').then(() => router.back());
          },
        },
      ],
    );
  }, [client, id, displayName]);

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
      fileName: asset.fileName ?? `image-${Date.now()}.png`,
      mimeType: asset.mimeType,
    }));
    setPendingImages((prev) => [...prev, ...newImages]);
  }, []);

  const removeImage = useCallback((index: number) => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    setPendingImages((prev) => prev.filter((_, i) => i !== index));
  }, []);

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

    if (images.length > 0) {
      setUploading(true);
      try {
        // Single batch request — no concurrent upload issues
        const paths = await client.uploadImages(images);
        // Send each path as its own submitted line so the agent sees each image,
        // then send the text message last
        for (const p of paths) {
          client.sendPaneInput({ type: 'input_submit', data: p });
        }
        if (text) {
          client.sendPaneInput({ type: 'input_submit', data: text });
        }
      } catch (err) {
        Alert.alert('Upload failed', String(err));
      }
      setUploading(false);
    } else if (text) {
      client.sendPaneInput({ type: 'input_submit', data: text });
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
      <Stack.Screen options={{ title: displayName ?? 'Terminal' }} />
      <Stack.Toolbar placement="right">
        <Stack.Toolbar.Menu icon="ellipsis">
          <Stack.Toolbar.MenuAction
            icon="xmark.circle"
            destructive
            onPress={handleActions}
          >
            Kill Workstream
          </Stack.Toolbar.MenuAction>
        </Stack.Toolbar.Menu>
      </Stack.Toolbar>

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

        {/* Input field — full width with border outline */}
        <View style={styles.inputWrapper}>
          <TextInput
            style={styles.input}
            value={inputText}
            onChangeText={setInputText}
            placeholder="Message..."
            placeholderTextColor={catppuccin.overlay0}
            autoCapitalize="none"
            autoCorrect={false}
            multiline
            returnKeyType="default"
            blurOnSubmit={false}
          />
        </View>

        {/* Action buttons row */}
        <View style={styles.actionsRow}>
          <Pressable onPress={handleImagePick} hitSlop={8} style={styles.attachButton}>
            <FontAwesome name="plus" size={18} color={catppuccin.overlay1} />
          </Pressable>
          <View style={styles.actionsRight}>
            <Pressable
              onPress={handleMic}
              hitSlop={8}
              style={[styles.micButton, recognizing && styles.micButtonActive]}
            >
              <FontAwesome
                name="microphone"
                size={18}
                color={recognizing ? catppuccin.red : catppuccin.overlay1}
              />
            </Pressable>
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
        </View>

        {/* Safe area spacer */}
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
    paddingHorizontal: 16,
  },
  thumbnailRow: {
    maxHeight: 72,
    marginHorizontal: -16,
  },
  thumbnailContent: {
    paddingHorizontal: 16,
    paddingTop: 10,
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
  inputWrapper: {
    marginTop: 12,
    borderWidth: 1,
    borderColor: catppuccin.surface2,
    borderRadius: 22,
    backgroundColor: 'transparent',
  },
  input: {
    color: catppuccin.text,
    paddingHorizontal: 18,
    paddingTop: 12,
    paddingBottom: 12,
    fontSize: 16,
    lineHeight: 22,
    maxHeight: 120,
  },
  actionsRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingTop: 10,
    paddingBottom: 8,
  },
  attachButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    borderWidth: 1,
    borderColor: catppuccin.surface2,
    alignItems: 'center',
    justifyContent: 'center',
  },
  actionsRight: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  micButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    borderWidth: 1,
    borderColor: catppuccin.surface2,
    alignItems: 'center',
    justifyContent: 'center',
  },
  micButtonActive: {
    borderColor: catppuccin.red,
    backgroundColor: 'rgba(243, 139, 168, 0.15)',
  },
  sendButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: catppuccin.lavender,
    alignItems: 'center',
    justifyContent: 'center',
  },
  sendButtonDisabled: {
    backgroundColor: catppuccin.surface0,
  },
});
