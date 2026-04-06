import { useState, useCallback, memo } from 'react';
import { View, Text, Pressable, StyleSheet, ScrollView } from 'react-native';
import * as Haptics from 'expo-haptics';
import { hex } from '../lib/theme';
import type { PaneInputMsg } from '../lib/types';

interface ExtraKeysBarProps {
  onKey: (input: PaneInputMsg) => void;
}

interface KeyDef {
  label: string;
  action: () => PaneInputMsg;
  /** If true, key is a sticky modifier (CTRL) */
  isModifier?: boolean;
  /** Wider key */
  wide?: boolean;
}

export const ExtraKeysBar = memo(function ExtraKeysBar({ onKey }: ExtraKeysBarProps) {
  const [ctrlActive, setCtrlActive] = useState(false);

  const sendSpecial = useCallback((key: string) => {
    return { type: 'special' as const, data: key };
  }, []);

  const sendInput = useCallback((data: string) => {
    return { type: 'input' as const, data };
  }, []);

  const handleKeyPress = useCallback((keyDef: KeyDef) => {
    if (process.env.EXPO_OS === 'ios') {
      Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    }

    if (keyDef.isModifier) {
      setCtrlActive(prev => !prev);
      return;
    }

    const msg = keyDef.action();

    // If CTRL is active, wrap the key as a ctrl sequence
    if (ctrlActive && msg.type === 'input' && msg.data.length === 1) {
      setCtrlActive(false);
      onKey({ type: 'special', data: `C-${msg.data}` });
      return;
    }

    if (ctrlActive) {
      setCtrlActive(false);
    }

    onKey(msg);
  }, [ctrlActive, onKey]);

  const keys: KeyDef[] = [
    { label: 'ESC', action: () => sendSpecial('Escape'), wide: true },
    { label: 'TAB', action: () => sendSpecial('Tab'), wide: true },
    { label: 'CTRL', action: () => sendSpecial(''), isModifier: true, wide: true },
    { label: '^C', action: () => sendSpecial('C-c') },
    { label: '^Z', action: () => sendSpecial('C-z') },
    { label: '|', action: () => sendInput('|') },
    { label: '~', action: () => sendInput('~') },
    { label: '-', action: () => sendInput('-') },
    { label: '/', action: () => sendInput('/') },
    { label: '\u2190', action: () => sendSpecial('Left') },
    { label: '\u2191', action: () => sendSpecial('Up') },
    { label: '\u2193', action: () => sendSpecial('Down') },
    { label: '\u2192', action: () => sendSpecial('Right') },
  ];

  return (
    <View style={styles.container}>
      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={styles.scrollContent}
        keyboardShouldPersistTaps="always"
      >
        {keys.map((keyDef) => (
          <Pressable
            key={keyDef.label}
            style={({ pressed }) => [
              styles.key,
              keyDef.wide && styles.keyWide,
              keyDef.isModifier && ctrlActive && styles.keyActive,
              pressed && styles.keyPressed,
            ]}
            onPress={() => handleKeyPress(keyDef)}
          >
            <Text
              style={[
                styles.keyLabel,
                keyDef.isModifier && ctrlActive && styles.keyLabelActive,
              ]}
            >
              {keyDef.label}
            </Text>
          </Pressable>
        ))}
      </ScrollView>
    </View>
  );
});

const styles = StyleSheet.create({
  container: {
    backgroundColor: hex.mantle,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: hex.surface0,
  },
  scrollContent: {
    paddingHorizontal: 8,
    paddingVertical: 6,
    gap: 6,
  },
  key: {
    minWidth: 36,
    height: 32,
    borderRadius: 6,
    backgroundColor: hex.surface0,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: hex.surface2,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: 8,
  },
  keyWide: {
    minWidth: 44,
    paddingHorizontal: 10,
  },
  keyActive: {
    backgroundColor: hex.accent,
    borderColor: hex.accent,
  },
  keyPressed: {
    backgroundColor: hex.surface1,
  },
  keyLabel: {
    fontFamily: 'JetBrainsMono_400Regular',
    fontSize: 13,
    color: hex.text,
  },
  keyLabelActive: {
    color: hex.base,
    fontWeight: 'bold',
  },
});
