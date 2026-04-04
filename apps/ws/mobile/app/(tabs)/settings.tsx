import { View, Text, StyleSheet, Alert } from 'react-native';
import { useRouter } from 'expo-router';
import { useRelay } from '../_layout';
import { catppuccin } from '../../lib/theme';
import { AnimatedIconButton } from '../../components/AnimatedIconButton';

export default function SettingsScreen() {
  const { config, connected, disconnect } = useRelay();
  const router = useRouter();

  const handleDisconnect = () => {
    Alert.alert('Disconnect', 'This will disconnect from the relay daemon.', [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Disconnect',
        style: 'destructive',
        onPress: () => {
          disconnect();
          router.replace('/connect');
        },
      },
    ]);
  };

  return (
    <View style={styles.container}>
      <View style={styles.section}>
        <Text style={styles.sectionTitle}>Connection</Text>
        <View style={styles.card}>
          <View style={styles.row}>
            <Text style={styles.label}>Status</Text>
            <View style={styles.statusRow}>
              <View style={[styles.dot, connected ? styles.dotGreen : styles.dotRed]} />
              <Text style={styles.value}>{connected ? 'Connected' : 'Disconnected'}</Text>
            </View>
          </View>
          {config && (
            <>
              <View style={styles.separator} />
              <View style={styles.row}>
                <Text style={styles.label}>Host</Text>
                <Text style={styles.value}>
                  {config.host}:{config.port}
                </Text>
              </View>
              <View style={styles.separator} />
              <View style={styles.row}>
                <Text style={styles.label}>Token</Text>
                <Text style={styles.value}>{config.token.slice(0, 12)}...</Text>
              </View>
            </>
          )}
        </View>
      </View>

      {connected ? (
        <AnimatedIconButton style={styles.disconnectButton} onPress={handleDisconnect} pressScale={0.97}>
          <Text style={styles.disconnectText}>Disconnect</Text>
        </AnimatedIconButton>
      ) : (
        <AnimatedIconButton style={styles.disconnectButton} onPress={() => router.replace('/connect')} pressScale={0.97}>
          <Text style={styles.reconnectText}>Reconnect</Text>
        </AnimatedIconButton>
      )}

      <View style={styles.section}>
        <Text style={styles.sectionTitle}>About</Text>
        <View style={styles.card}>
          <View style={styles.row}>
            <Text style={styles.label}>App</Text>
            <Text style={styles.value}>Grimoire Mobile</Text>
          </View>
          <View style={styles.separator} />
          <View style={styles.row}>
            <Text style={styles.label}>Version</Text>
            <Text style={styles.value}>0.1.0</Text>
          </View>
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: catppuccin.base,
    padding: 16,
  },
  section: {
    marginBottom: 24,
  },
  sectionTitle: {
    fontSize: 13,
    fontWeight: '600',
    color: catppuccin.subtext0,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
    marginBottom: 8,
    marginLeft: 4,
  },
  card: {
    backgroundColor: catppuccin.surface0,
    borderRadius: 12,
    borderCurve: 'continuous',
    overflow: 'hidden',
  },
  row: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingVertical: 14,
  },
  label: {
    fontSize: 15,
    color: catppuccin.text,
  },
  value: {
    fontSize: 15,
    color: catppuccin.subtext0,
  },
  statusRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: 4,
  },
  dotGreen: {
    backgroundColor: catppuccin.green,
  },
  dotRed: {
    backgroundColor: catppuccin.red,
  },
  separator: {
    height: 1,
    backgroundColor: catppuccin.surface1,
    marginLeft: 16,
  },
  disconnectButton: {
    backgroundColor: catppuccin.surface0,
    borderRadius: 12,
    borderCurve: 'continuous',
    paddingVertical: 14,
    alignItems: 'center',
    marginBottom: 24,
  },
  disconnectText: {
    color: catppuccin.red,
    fontSize: 16,
    fontWeight: '600',
  },
  reconnectText: {
    color: catppuccin.lavender,
    fontSize: 16,
    fontWeight: '600',
  },
});
