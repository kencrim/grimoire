import { View, Text, StyleSheet } from 'react-native';
import { AnimatedIconButton } from './AnimatedIconButton';
import { PixelDaemon } from './PixelDaemon';
import { hex } from '../lib/theme';

interface DaemonSectionHeaderProps {
  name: string;
  host: string;
  port: number;
  connected: boolean;
  agentCount: number;
  collapsed: boolean;
  onToggle: () => void;
}

export function DaemonSectionHeader({
  name,
  host,
  port,
  connected,
  agentCount,
  collapsed,
  onToggle,
}: DaemonSectionHeaderProps) {
  return (
    <AnimatedIconButton style={styles.container} onPress={onToggle} pressScale={0.98}>
      {/* Pixel daemon icon with status badge */}
      <View style={styles.iconContainer}>
        <PixelDaemon size={24} color={hex.text} />
        <View style={[styles.statusBadge, connected ? styles.badgeGreen : styles.badgeRed]} />
      </View>

      <View style={styles.info}>
        <Text style={styles.name}>{name}</Text>
        <Text style={styles.host}>
          {host}:{port}
        </Text>
      </View>

      <View style={styles.trailing}>
        {agentCount > 0 && (
          <View style={styles.countBadge}>
            <Text style={styles.countText}>{agentCount}</Text>
          </View>
        )}
        <Text style={styles.chevron}>{collapsed ? '\u25B6' : '\u25BC'}</Text>
      </View>
    </AnimatedIconButton>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 4,
    paddingTop: 8,
    paddingBottom: 8,
    marginBottom: 4,
  },
  iconContainer: {
    width: 30,
    height: 28,
    justifyContent: 'center',
    alignItems: 'center',
    marginRight: 10,
  },
  statusBadge: {
    position: 'absolute',
    top: 0,
    right: 0,
    width: 8,
    height: 8,
    borderRadius: 4,
    borderWidth: 1.5,
    borderColor: hex.base,
  },
  badgeGreen: {
    backgroundColor: hex.green,
  },
  badgeRed: {
    backgroundColor: hex.red,
  },
  info: {
    flex: 1,
  },
  name: {
    fontSize: 14,
    fontFamily: 'SpaceGrotesk_700Bold',
    color: hex.text,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
  },
  host: {
    fontSize: 11,
    fontFamily: 'JetBrainsMono_400Regular',
    color: hex.overlay0,
    marginTop: 1,
  },
  trailing: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  countBadge: {
    backgroundColor: hex.surface1,
    borderRadius: 8,
    paddingHorizontal: 6,
    paddingVertical: 2,
  },
  countText: {
    fontSize: 11,
    fontFamily: 'JetBrainsMono_500Medium',
    color: hex.subtext0,
  },
  chevron: {
    fontSize: 10,
    color: hex.overlay0,
  },
});
