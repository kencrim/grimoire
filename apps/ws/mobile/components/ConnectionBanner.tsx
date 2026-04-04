import { View, Text, StyleSheet } from 'react-native';
import { hex } from '../lib/theme';

interface ConnectionBannerProps {
  connected: boolean;
  host?: string;
  port?: number;
}

export function ConnectionBanner({ connected, host, port }: ConnectionBannerProps) {
  if (!host) return null;

  return (
    <View style={[styles.banner, connected ? styles.bannerConnected : styles.bannerDisconnected]}>
      <View style={[styles.dot, connected ? styles.dotGreen : styles.dotRed]} />
      <Text style={styles.text}>
        {connected
          ? `Connected to ${host}:${port}`
          : `Reconnecting to ${host}:${port}...`}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  banner: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingVertical: 8,
    gap: 8,
  },
  bannerConnected: {
    backgroundColor: hex.mantle,
  },
  bannerDisconnected: {
    backgroundColor: hex.surface0,
  },
  dot: {
    width: 6,
    height: 6,
    borderRadius: 3,
  },
  dotGreen: {
    backgroundColor: hex.green,
  },
  dotRed: {
    backgroundColor: hex.red,
  },
  text: {
    fontSize: 12,
    fontFamily: 'JetBrainsMono_400Regular',
    color: hex.subtext0,
  },
});
