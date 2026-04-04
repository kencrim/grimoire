import { useEffect, useRef, useState, useCallback, memo } from 'react';
import {
  View,
  Text,
  TextInput,
  Pressable,
  Image,
  StyleSheet,
  Keyboard,
  Alert,
  ScrollView,
  KeyboardAvoidingView,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useLocalSearchParams, Stack, router } from 'expo-router';
import { SymbolView } from 'expo-symbols';
import { BottomSheetModal } from '@gorhom/bottom-sheet';
import * as Haptics from 'expo-haptics';
import * as ImagePicker from 'expo-image-picker';
import {
  ExpoSpeechRecognitionModule,
  useSpeechRecognitionEvent,
} from 'expo-speech-recognition';
import { useRelay } from '../_layout';
import { AnimatedIconButton } from '../../components/AnimatedIconButton';
import { SkillsSheet } from '../../components/SkillsSheet';
import { NativeTerminalView, type NativeTerminalHandle } from '../../components/NativeTerminalView';
import type { Skill } from '../../lib/types';
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
  const terminalRef = useRef<NativeTerminalHandle>(null);
  const [pendingImages, setPendingImages] = useState<PendingImage[]>([]);
  const [uploading, setUploading] = useState(false);
  const [recognizing, setRecognizing] = useState(false);
  const [skills, setSkills] = useState<Skill[]>([]);
  const skillsRef = useRef<BottomSheetModal>(null);

  // Fetch available skills on mount
  useEffect(() => {
    if (!client || !id) return;
    client.getSkills(id).then(setSkills).catch(() => {});
  }, [client, id]);

  const handleSkills = useCallback(() => {
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    skillsRef.current?.present();
  }, []);

  const handleSkillSelect = useCallback((skill: Skill, argument?: string) => {
    skillsRef.current?.dismiss();
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    const cmd = argument ? `/${skill.name} ${argument}` : `/${skill.name}`;
    client?.sendPaneInput({ type: 'input_submit', data: cmd });
  }, [client]);

  // Speech recognition — streams transcription into the input field
  useSpeechRecognitionEvent('end', () => setRecognizing(false));
  useSpeechRecognitionEvent('error', () => setRecognizing(false));
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
      setRecognizing(false);
      ExpoSpeechRecognitionModule.stop();
      return;
    }
    if (!micPermitted.current) {
      Alert.alert('Permission required', 'Microphone and speech recognition access is needed.');
      return;
    }
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    setRecognizing(true);
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
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
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

  // Connect pane WebSocket and push frames directly to terminal (no parent re-render)
  useEffect(() => {
    if (!client || !agentRef) return;

    terminalRef.current?.clear();
    client.connectPane(agentRef, activeTab);

    const unsub = client.onPane((frame) => {
      terminalRef.current?.pushFrame(frame);
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

    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    const newImages = result.assets.map((asset) => ({
      uri: asset.uri,
      fileName: asset.fileName ?? `image-${Date.now()}.png`,
      mimeType: asset.mimeType,
    }));
    setPendingImages((prev) => [...prev, ...newImages]);
  }, []);

  const removeImage = useCallback((index: number) => {
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    setPendingImages((prev) => prev.filter((_, i) => i !== index));
  }, []);

  // Send text + any pending images
  const handleSend = useCallback(async () => {
    if (!client) return;
    if (!inputText.trim() && pendingImages.length === 0) return;

    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
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
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  };

  const insets = useSafeAreaInsets();
  const hasContent = inputText.trim().length > 0 || pendingImages.length > 0;

  const statusColor =
    agent?.status === 'alive'
      ? catppuccin.green
      : agent?.status === 'idle'
        ? catppuccin.yellow
        : catppuccin.overlay0;

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={process.env.EXPO_OS === 'ios' ? 'padding' : undefined}
      keyboardVerticalOffset={insets.top + 44}
    >
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
        <NativeTerminalView ref={terminalRef} />
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

      <ComposeBar
        inputText={inputText}
        setInputText={setInputText}
        pendingImages={pendingImages}
        removeImage={removeImage}
        handleImagePick={handleImagePick}
        handleSkills={handleSkills}
        handleMic={handleMic}
        recognizing={recognizing}
        handleSend={handleSend}
        hasContent={hasContent}
        uploading={uploading}
        bottomInset={insets.bottom}
      />
      <SkillsSheet ref={skillsRef} skills={skills} onSelect={handleSkillSelect} />
    </KeyboardAvoidingView>
  );
}

interface ComposeBarProps {
  inputText: string;
  setInputText: (text: string) => void;
  pendingImages: PendingImage[];
  removeImage: (index: number) => void;
  handleImagePick: () => void;
  handleSkills: () => void;
  handleMic: () => void;
  recognizing: boolean;
  handleSend: () => void;
  hasContent: boolean;
  uploading: boolean;
  bottomInset: number;
}

const ComposeBar = memo(function ComposeBar(props: ComposeBarProps) {
  const {
    inputText, setInputText, pendingImages, removeImage,
    handleImagePick, handleSkills, handleMic, recognizing,
    handleSend, hasContent, uploading, bottomInset,
  } = props;

  return (
    <View style={styles.compose}>
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
                  <SymbolView name="xmark.circle.fill" size={18} tintColor={catppuccin.red} />
                </Pressable>
              </View>
            ))}
          </ScrollView>
        )}

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

        <View style={styles.actionsRow}>
          <View style={styles.actionsLeft}>
            <AnimatedIconButton onPress={handleImagePick} hitSlop={8} style={styles.attachButton}>
              <SymbolView name="plus" size={18} tintColor={catppuccin.overlay1} />
            </AnimatedIconButton>
            <AnimatedIconButton onPress={handleSkills} hitSlop={8} style={styles.skillsButton}>
              <SymbolView name="sparkles" size={18} tintColor={catppuccin.overlay1} />
            </AnimatedIconButton>
          </View>
          <View style={styles.actionsRight}>
            <AnimatedIconButton
              onPress={handleMic}
              hitSlop={8}
              style={[styles.micButton, recognizing && styles.micButtonActive]}
            >
              <SymbolView
                name="mic.fill"
                size={18}
                tintColor={recognizing ? catppuccin.red : catppuccin.overlay1}
                animationSpec={{ effect: { type: 'bounce' } }}
              />
            </AnimatedIconButton>
            <AnimatedIconButton
              style={[styles.sendButton, !hasContent && styles.sendButtonDisabled]}
              onPress={handleSend}
              disabled={!hasContent || uploading}
            >
              <SymbolView
                name="arrow.up"
                size={16}
                tintColor={hasContent ? catppuccin.base : catppuccin.surface2}
              />
            </AnimatedIconButton>
          </View>
        </View>

        <View style={{ height: bottomInset }} />
    </View>
  );
});

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
    borderCurve: 'continuous',
    overflow: 'visible',
  },
  thumbnailImage: {
    width: 56,
    height: 56,
    borderRadius: 8,
    borderCurve: 'continuous',
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: catppuccin.surface2,
  },
  thumbnailRemove: {
    position: 'absolute',
    top: -6,
    right: -6,
    backgroundColor: catppuccin.mantle,
    borderRadius: 9,
    borderCurve: 'continuous',
  },
  inputWrapper: {
    marginTop: 12,
    borderWidth: 1,
    borderColor: catppuccin.surface2,
    borderRadius: 22,
    borderCurve: 'continuous',
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
  actionsLeft: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  attachButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    borderCurve: 'continuous',
    borderWidth: 1,
    borderColor: catppuccin.surface2,
    alignItems: 'center',
    justifyContent: 'center',
  },
  skillsButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    borderCurve: 'continuous',
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
    borderCurve: 'continuous',
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
    borderCurve: 'continuous',
    backgroundColor: catppuccin.lavender,
    alignItems: 'center',
    justifyContent: 'center',
  },
  sendButtonDisabled: {
    backgroundColor: catppuccin.surface0,
  },
});
