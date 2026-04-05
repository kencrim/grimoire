import { View, Text, StyleSheet } from 'react-native';
import { useDaemons } from '../_layout';
import { hex } from '../../lib/theme';

export default function SettingsScreen() {
  const { daemons, daemonOrder } = useDaemons();

  const connectedCount = daemonOrder.filter(id => daemons.get(id)?.connected).length;
  const totalCount = daemonOrder.length;

  return (
    <View style={styles.container}>
      <View style={styles.section}>
        <Text style={styles.sectionTitle}>Daemons</Text>
        <View style={styles.card}>
          <View style={styles.row}>
            <Text style={styles.label}>Connected</Text>
            <View style={styles.statusRow}>
              <View style={[styles.dot, connectedCount > 0 ? styles.dotGreen : styles.dotRed]} />
              <Text style={styles.value}>
                {connectedCount} of {totalCount}
              </Text>
            </View>
          </View>
        </View>
      </View>

      <View style={styles.section}>
        <Text style={styles.sectionTitle}>About</Text>
        <View style={styles.card}>
          <View style={styles.row}>
            <Text style={styles.label}>App</Text>
            <Text style={styles.value}>Hex</Text>
          </View>
          <View style={styles.separator} />
          <View style={styles.row}>
            <Text style={styles.label}>Version</Text>
            <Text style={styles.value}>0.2.0</Text>
          </View>
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: hex.base,
    padding: 16,
  },
  section: {
    marginBottom: 24,
  },
  sectionTitle: {
    fontSize: 13,
    fontFamily: 'SpaceGrotesk_600SemiBold',
    color: hex.subtext0,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
    marginBottom: 8,
    marginLeft: 4,
  },
  card: {
    backgroundColor: hex.surface0,
    borderRadius: 0,
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
    fontFamily: 'SpaceGrotesk_400Regular',
    color: hex.text,
  },
  value: {
    fontSize: 13,
    fontFamily: 'JetBrainsMono_400Regular',
    color: hex.subtext0,
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
    backgroundColor: hex.green,
  },
  dotRed: {
    backgroundColor: hex.red,
  },
  separator: {
    height: 1,
    backgroundColor: hex.surface1,
    marginLeft: 16,
  },
});
