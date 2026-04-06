import { memo, type ComponentProps } from 'react';
import { View, Text, StyleSheet } from 'react-native';
import { SymbolView } from 'expo-symbols';
import * as Haptics from 'expo-haptics';
import { AnimatedIconButton } from './AnimatedIconButton';
import { hex } from '../lib/theme';

type SymbolName = ComponentProps<typeof SymbolView>['name'];

interface TerminalControlsProps {
  onKey: (key: string) => void;
}

function DpadButton({ icon, onPress }: { icon: SymbolName; onPress: () => void }) {
  return (
    <AnimatedIconButton onPress={onPress} style={styles.dpadBtn} pressScale={0.85}>
      <SymbolView name={icon} size={16} tintColor={hex.text} />
    </AnimatedIconButton>
  );
}

function UtilButton({ label, onPress, accent }: { label: string; onPress: () => void; accent?: boolean }) {
  return (
    <AnimatedIconButton
      onPress={onPress}
      style={[styles.utilBtn, accent && styles.utilBtnAccent]}
      pressScale={0.88}
    >
      <Text style={[styles.utilLabel, accent && styles.utilLabelAccent]}>{label}</Text>
    </AnimatedIconButton>
  );
}

export const TerminalControls = memo(function TerminalControls({ onKey }: TerminalControlsProps) {
  const sendKey = (key: string) => {
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Rigid);
    onKey(key);
  };

  return (
    <View style={styles.container}>
      {/* D-pad */}
      <View style={styles.dpad}>
        <View style={styles.dpadTopRow}>
          <DpadButton icon="chevron.up" onPress={() => sendKey('Up')} />
        </View>
        <View style={styles.dpadMidRow}>
          <DpadButton icon="chevron.left" onPress={() => sendKey('Left')} />
          <View style={styles.dpadCenter} />
          <DpadButton icon="chevron.right" onPress={() => sendKey('Right')} />
        </View>
        <View style={styles.dpadBotRow}>
          <DpadButton icon="chevron.down" onPress={() => sendKey('Down')} />
        </View>
      </View>

      {/* Divider */}
      <View style={styles.divider} />

      {/* Utility keys */}
      <View style={styles.utilGrid}>
        <View style={styles.utilRow}>
          <UtilButton label="Esc" onPress={() => sendKey('Escape')} />
          <UtilButton label="Tab" onPress={() => sendKey('Tab')} />
        </View>
        <View style={styles.utilRow}>
          <UtilButton label="⌃C" onPress={() => sendKey('C-c')} />
          <UtilButton label="Enter" onPress={() => sendKey('Enter')} accent />
        </View>
      </View>
    </View>
  );
});

const DPAD_BTN = 44;
const DPAD_GAP = 2;

const styles = StyleSheet.create({
  container: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: 10,
    paddingHorizontal: 4,
    gap: 16,
  },
  // D-pad cluster
  dpad: {
    alignItems: 'center',
    gap: DPAD_GAP,
  },
  dpadTopRow: {
    flexDirection: 'row',
    justifyContent: 'center',
  },
  dpadMidRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: DPAD_GAP,
  },
  dpadBotRow: {
    flexDirection: 'row',
    justifyContent: 'center',
  },
  dpadCenter: {
    width: DPAD_BTN,
    height: DPAD_BTN,
  },
  dpadBtn: {
    width: DPAD_BTN,
    height: DPAD_BTN,
    borderRadius: 8,
    borderCurve: 'continuous',
    backgroundColor: hex.surface1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  // Divider
  divider: {
    width: StyleSheet.hairlineWidth,
    height: '70%',
    backgroundColor: hex.surface2,
  },
  // Utility key grid
  utilGrid: {
    flex: 1,
    gap: DPAD_GAP,
  },
  utilRow: {
    flexDirection: 'row',
    gap: DPAD_GAP,
  },
  utilBtn: {
    flex: 1,
    height: 44,
    borderRadius: 8,
    borderCurve: 'continuous',
    backgroundColor: hex.surface1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  utilBtnAccent: {
    backgroundColor: hex.accent,
  },
  utilLabel: {
    fontFamily: 'JetBrainsMono_500Medium',
    fontSize: 13,
    color: hex.text,
  },
  utilLabelAccent: {
    color: hex.base,
  },
});
