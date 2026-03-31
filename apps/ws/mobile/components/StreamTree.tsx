import { Pressable, View, Text, StyleSheet } from 'react-native';
import { router } from 'expo-router';
import * as Haptics from 'expo-haptics';
import { catppuccin } from '../lib/theme';
import type { StreamNode } from '../lib/types';

interface StreamTreeItemProps {
  node: StreamNode;
}

const STATUS_COLORS: Record<string, string> = {
  alive: catppuccin.green,
  running: catppuccin.green,
  idle: catppuccin.yellow,
  exited: catppuccin.overlay0,
  blocked: catppuccin.red,
  done: catppuccin.blue,
};

const AGENT_LABELS: Record<string, string> = {
  amp: 'amp',
  claude: 'claude',
  codex: 'codex',
};

export function StreamTreeItem({ node }: StreamTreeItemProps) {
  const statusColor = node.color ?? STATUS_COLORS[node.status] ?? catppuccin.overlay0;

  const handlePress = () => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    router.push(`/stream/${encodeURIComponent(node.id)}`);
  };

  const handleLongPress = () => {
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    // TODO: show action sheet (kill, send message)
  };

  return (
    <Pressable
      style={({ pressed }) => [styles.item, pressed && styles.itemPressed]}
      onPress={handlePress}
      onLongPress={handleLongPress}
    >
      {/* Indent based on depth */}
      {node.depth > 0 && <View style={{ width: node.depth * 20 }} />}

      {/* Tree connector */}
      {node.depth > 0 && (
        <View style={styles.connector}>
          <Text style={styles.connectorText}>
            {node.depth > 0 ? '├─' : ''}
          </Text>
        </View>
      )}

      {/* Status dot */}
      <View style={[styles.dot, { backgroundColor: statusColor }]} />

      {/* Name */}
      <Text style={styles.name} numberOfLines={1}>
        {node.name}
      </Text>

      {/* Spacer */}
      <View style={styles.spacer} />

      {/* Agent badge */}
      <View style={[styles.badge, { borderColor: statusColor + '40' }]}>
        <Text style={[styles.badgeText, { color: statusColor }]}>
          {AGENT_LABELS[node.agent] ?? node.agent}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  item: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: 12,
    paddingHorizontal: 4,
    borderRadius: 8,
  },
  itemPressed: {
    backgroundColor: catppuccin.surface0,
  },
  connector: {
    marginRight: 4,
  },
  connectorText: {
    color: catppuccin.surface2,
    fontSize: 12,
    fontFamily: 'Menlo',
  },
  dot: {
    width: 10,
    height: 10,
    borderRadius: 5,
    marginRight: 10,
  },
  name: {
    fontSize: 16,
    fontWeight: '500',
    color: catppuccin.text,
    flexShrink: 1,
  },
  spacer: {
    flex: 1,
  },
  badge: {
    borderWidth: 1,
    borderRadius: 6,
    paddingHorizontal: 8,
    paddingVertical: 3,
  },
  badgeText: {
    fontSize: 12,
    fontWeight: '500',
  },
});
