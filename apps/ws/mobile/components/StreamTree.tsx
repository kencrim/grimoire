import { View, Text, StyleSheet, ActionSheetIOS, Alert } from 'react-native';
import { router } from 'expo-router';
import * as Haptics from 'expo-haptics';
import { hex } from '../lib/theme';
import { AnimatedIconButton } from './AnimatedIconButton';
import type { StreamNode } from '../lib/types';

interface StreamTreeItemProps {
  node: StreamNode;
  isLast?: boolean;
  onKill?: (id: string) => void;
}

const STATUS_COLORS: Record<string, string> = {
  alive: hex.green,
  running: hex.green,
  idle: hex.yellow,
  exited: hex.overlay0,
  blocked: hex.red,
  done: hex.blue,
};

const AGENT_LABELS: Record<string, string> = {
  amp: 'amp',
  claude: 'claude',
  codex: 'codex',
};

function confirmKill(name: string, onConfirm: () => void) {
  Alert.alert(
    'Kill workstream?',
    `This will destroy the worktree and tmux session for "${name}" and all its children.`,
    [
      { text: 'Cancel', style: 'cancel' },
      { text: 'Kill', style: 'destructive', onPress: onConfirm },
    ],
  );
}

export function showWorkstreamActions(
  node: StreamNode,
  onKill: () => void,
) {
  const actions = ['Kill Workstream', 'Cancel'];
  const destructiveIndex = 0;
  const cancelIndex = actions.length - 1;

  if (process.env.EXPO_OS === 'ios') {
    ActionSheetIOS.showActionSheetWithOptions(
      {
        options: actions,
        destructiveButtonIndex: destructiveIndex,
        cancelButtonIndex: cancelIndex,
        title: node.name,
      },
      (index) => {
        if (index === destructiveIndex) {
          confirmKill(node.name, onKill);
        }
      },
    );
  } else {
    confirmKill(node.name, onKill);
  }
}

const GUTTER = 24;
const INDENT = 20;

export function StreamTreeItem({ node, isLast = false, onKill }: StreamTreeItemProps) {
  const statusColor = node.color ?? STATUS_COLORS[node.status] ?? hex.overlay0;
  const gutterWidth = GUTTER + node.depth * INDENT;

  const handlePress = () => {
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    router.push(`/stream/${encodeURIComponent(node.id)}`);
  };

  const handleLongPress = () => {
    if (!onKill) return;
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    showWorkstreamActions(node, () => onKill(node.id));
  };

  return (
    <View style={styles.row}>
      {/* Connector gutter */}
      <View style={{ width: gutterWidth }}>
        {/* Vertical line — full height if not last, half if last */}
        <View
          style={[
            styles.vLine,
            {
              left: GUTTER / 2 + node.depth * INDENT - INDENT / 2 - 0.5,
              top: 0,
              height: isLast ? '50%' : '100%',
            },
          ]}
        />
        {/* Horizontal branch */}
        <View
          style={[
            styles.hLine,
            {
              left: GUTTER / 2 + node.depth * INDENT - INDENT / 2 - 0.5,
              top: '50%',
              width: gutterWidth - (GUTTER / 2 + node.depth * INDENT - INDENT / 2),
            },
          ]}
        />
      </View>

      {/* Content */}
      <AnimatedIconButton
        style={styles.item}
        onPress={handlePress}
        onLongPress={handleLongPress}
        pressScale={0.97}
      >
        <View style={[styles.dot, { backgroundColor: statusColor }]} />
        <Text style={styles.name} numberOfLines={1}>
          {node.name}
        </Text>
        <View style={styles.spacer} />
        <View style={[styles.badge, { borderColor: statusColor + '40' }]}>
          <Text style={[styles.badgeText, { color: statusColor }]}>
            {AGENT_LABELS[node.agent] ?? node.agent}
          </Text>
        </View>
      </AnimatedIconButton>
    </View>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: 'row',
    alignItems: 'stretch',
  },
  vLine: {
    position: 'absolute',
    width: 1,
    backgroundColor: hex.surface2,
  },
  hLine: {
    position: 'absolute',
    height: 1,
    backgroundColor: hex.surface2,
  },
  item: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: 12,
    paddingRight: 4,
  },
  dot: {
    width: 10,
    height: 10,
    borderRadius: 5,
    marginRight: 10,
  },
  name: {
    fontSize: 16,
    fontFamily: 'SpaceGrotesk_600SemiBold',
    color: hex.text,
    flexShrink: 1,
  },
  spacer: {
    flex: 1,
  },
  badge: {
    borderWidth: 1,
    borderRadius: 0,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  badgeText: {
    fontSize: 12,
    fontFamily: 'JetBrainsMono_500Medium',
  },
});
