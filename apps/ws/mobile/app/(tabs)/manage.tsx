import { useState, useEffect, useMemo } from 'react';
import {
  View,
  Text,
  StyleSheet,
  ActivityIndicator,
  ScrollView,
  Alert,
} from 'react-native';
import { toast } from 'sonner-native';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { SymbolView } from 'expo-symbols';
import * as Haptics from 'expo-haptics';
import { useDaemons } from '../_layout';
import { hex } from '../../lib/theme';
import { parseHexUri } from '../../lib/relay-client';
import { discoverDaemons, saveTailscaleConfig, type DiscoveredDaemon } from '../../lib/discovery';
import { AnimatedIconButton } from '../../components/AnimatedIconButton';
import type { ConnectionConfig } from '../../lib/types';

export default function ManageScreen() {
  const {
    daemons,
    daemonOrder,
    addDaemon,
    removeDaemon,
    connectDaemon,
    disconnectDaemon,
    connectFromUri,
  } = useDaemons();

  const [permission, requestPermission] = useCameraPermissions();
  const [showScanner, setShowScanner] = useState(false);
  const [loading, setLoading] = useState(false);
  const [allDiscovered, setAllDiscovered] = useState<DiscoveredDaemon[]>([]);
  const [scanning, setScanning] = useState(false);

  // Live-filter discovered: hide anything already saved (by host:port OR name)
  const existingKeys = useMemo(() => {
    const hosts = new Set<string>();
    const names = new Set<string>();
    for (const id of daemonOrder) {
      const d = daemons.get(id);
      if (d) {
        hosts.add(`${d.entry.config.host}:${d.entry.config.port}`);
        names.add(d.entry.name);
      }
    }
    return { hosts, names };
  }, [daemons, daemonOrder]);

  const discovered = useMemo(
    () => allDiscovered.filter(d =>
      !existingKeys.hosts.has(`${d.host}:${d.port}`) && !existingKeys.names.has(d.label)
    ),
    [allDiscovered, existingKeys],
  );

  useEffect(() => {
    runDiscovery();
  }, []);

  const runDiscovery = async () => {
    const savedConfigs: ConnectionConfig[] = daemonOrder
      .map(id => daemons.get(id)?.entry.config)
      .filter(Boolean) as ConnectionConfig[];

    setScanning(true);
    try {
      const results = await discoverDaemons(savedConfigs);
      setAllDiscovered(results);
    } catch {
      // ignore
    }
    setScanning(false);
  };

  const handleDiscoveredConnect = async (daemon: DiscoveredDaemon) => {
    setLoading(true);
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);

    if (daemon.type === 'tailscale') {
      saveTailscaleConfig({ host: daemon.host, port: daemon.port, token: daemon.token });
    }

    const entry = addDaemon(daemon.label, {
      host: daemon.host,
      port: daemon.port,
      token: daemon.token,
    });

    const ok = await connectDaemon(entry.id);
    setLoading(false);
    if (!ok) {
      toast.error('Connection failed', { description: `Could not connect to ${daemon.host}` });
    }
  };

  const handleQrScanned = async (data: string) => {
    setShowScanner(false);
    setLoading(true);

    const parsed = parseHexUri(data);
    if (!parsed) {
      toast.error('Invalid QR code', { description: 'Expected hex:// URI.' });
      setLoading(false);
      return;
    }

    const ok = await connectFromUri(data);
    setLoading(false);
    if (!ok) {
      toast.error('Connection failed', { description: 'Could not connect to daemon. Is it running?' });
    }
  };

  const handleRemove = (daemonId: string, name: string) => {
    Alert.alert('Remove daemon?', `Remove "${name}" from your saved daemons.`, [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Remove',
        style: 'destructive',
        onPress: () => removeDaemon(daemonId),
      },
    ]);
  };

  const handleToggleConnection = async (daemonId: string, connected: boolean) => {
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    if (connected) {
      disconnectDaemon(daemonId);
    } else {
      setLoading(true);
      const ok = await connectDaemon(daemonId);
      setLoading(false);
      if (!ok) {
        toast.error('Connection failed', { description: 'Could not reconnect. Is the daemon running?' });
      }
    }
  };

  if (showScanner) {
    if (!permission?.granted) {
      return (
        <View style={styles.centered}>
          <Text style={styles.title}>Camera Permission</Text>
          <Text style={styles.subtitle}>We need camera access to scan QR codes.</Text>
          <AnimatedIconButton style={styles.primaryButton} onPress={requestPermission} pressScale={0.97}>
            <Text style={styles.primaryButtonText}>Grant Permission</Text>
          </AnimatedIconButton>
          <AnimatedIconButton style={styles.linkButton} onPress={() => setShowScanner(false)} pressScale={0.92}>
            <Text style={styles.linkText}>Back</Text>
          </AnimatedIconButton>
        </View>
      );
    }

    return (
      <View style={styles.centered}>
        <Text style={styles.title}>Scan QR Code</Text>
        <Text style={styles.subtitle}>
          Point at the QR code shown by{'\n'}`ws daemon connect`
        </Text>
        <View style={styles.cameraContainer}>
          <CameraView
            style={styles.camera}
            facing="back"
            barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
            onBarcodeScanned={(result) => handleQrScanned(result.data)}
          />
        </View>
        <AnimatedIconButton style={styles.linkButton} onPress={() => setShowScanner(false)} pressScale={0.92}>
          <Text style={styles.linkText}>Back</Text>
        </AnimatedIconButton>
      </View>
    );
  }

  return (
    <ScrollView
      contentContainerStyle={styles.scrollContent}
      keyboardShouldPersistTaps="handled"
    >
      {/* Saved daemons */}
      {daemonOrder.length > 0 && (
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>Your Daemons</Text>
          {daemonOrder.map(id => {
            const state = daemons.get(id);
            if (!state) return null;
            return (
              <View key={id} style={styles.row}>
                <AnimatedIconButton
                  style={styles.rowMain}
                  onPress={() => handleToggleConnection(id, state.connected)}
                  pressScale={0.97}
                >
                  <View style={[styles.dot, state.connected ? styles.dotGreen : styles.dotRed]} />
                  <View style={styles.rowInfo}>
                    <Text style={styles.rowName}>{state.entry.name}</Text>
                    <Text style={styles.rowHost}>
                      {state.entry.config.host}:{state.entry.config.port}
                    </Text>
                  </View>
                  <Text style={[styles.rowAction, state.connected && styles.rowActionDisconnect]}>
                    {state.connected ? 'Disconnect' : 'Connect'}
                  </Text>
                </AnimatedIconButton>
                <AnimatedIconButton
                  style={styles.rowRemove}
                  onPress={() => handleRemove(id, state.entry.name)}
                  pressScale={0.85}
                  hitSlop={4}
                >
                  <SymbolView name="trash" size={16} tintColor={hex.overlay0} />
                </AnimatedIconButton>
              </View>
            );
          })}
        </View>
      )}

      {/* Discovered daemons (filtered to exclude already-saved) */}
      {discovered.length > 0 && (
        <View style={styles.section}>
          <Text style={styles.sectionLabel}>Discovered</Text>
          {discovered.map((d) => (
            <AnimatedIconButton
              key={`${d.host}:${d.port}`}
              style={styles.rowDiscovered}
              onPress={() => handleDiscoveredConnect(d)}
              disabled={loading}
              pressScale={0.97}
            >
              <View style={[styles.dot, styles.dotYellow]} />
              <View style={styles.rowInfo}>
                <Text style={styles.rowName}>{d.label}</Text>
                <Text style={styles.rowHost}>
                  {d.host}:{d.port}
                </Text>
              </View>
              <Text style={styles.rowAction}>Add</Text>
            </AnimatedIconButton>
          ))}
        </View>
      )}

      {scanning && (
        <View style={styles.scanningRow}>
          <ActivityIndicator size="small" color={hex.accent} />
          <Text style={styles.scanningText}>Scanning network...</Text>
        </View>
      )}

      {!scanning && discovered.length === 0 && daemonOrder.length === 0 && (
        <Text style={styles.noDiscovered}>No daemons found on network</Text>
      )}

      <AnimatedIconButton style={styles.rescanButton} onPress={runDiscovery} disabled={scanning} pressScale={0.92}>
        <Text style={styles.rescanText}>Rescan</Text>
      </AnimatedIconButton>

      <View style={styles.divider}>
        <View style={styles.dividerLine} />
        <Text style={styles.dividerText}>or</Text>
        <View style={styles.dividerLine} />
      </View>

      <AnimatedIconButton
        style={[styles.primaryButton, loading && styles.buttonDisabled]}
        onPress={() => setShowScanner(true)}
        disabled={loading}
        pressScale={0.97}
      >
        <Text style={styles.primaryButtonText}>Scan QR Code</Text>
      </AnimatedIconButton>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  centered: {
    flex: 1,
    backgroundColor: hex.base,
    justifyContent: 'center',
    alignItems: 'center',
    padding: 24,
  },
  scrollContent: {
    flexGrow: 1,
    backgroundColor: hex.base,
    paddingHorizontal: 16,
    paddingTop: 16,
    paddingBottom: 40,
  },
  title: {
    fontSize: 24,
    fontFamily: 'SpaceGrotesk_700Bold',
    color: hex.text,
    letterSpacing: -0.5,
    marginBottom: 8,
  },
  subtitle: {
    fontSize: 14,
    color: hex.subtext0,
    textAlign: 'center',
    marginBottom: 32,
  },
  section: {
    marginBottom: 24,
  },
  sectionLabel: {
    fontSize: 13,
    fontFamily: 'SpaceGrotesk_600SemiBold',
    color: hex.subtext0,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
    marginBottom: 8,
    marginLeft: 4,
  },
  // Shared row styles for both sections
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: hex.surface0,
    marginBottom: 8,
  },
  rowMain: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    paddingHorizontal: 14,
    paddingVertical: 12,
  },
  rowDiscovered: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: hex.surface0,
    marginBottom: 8,
    paddingHorizontal: 14,
    paddingVertical: 12,
  },
  rowRemove: {
    paddingHorizontal: 14,
    paddingVertical: 12,
    justifyContent: 'center',
    alignItems: 'center',
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    marginRight: 10,
  },
  dotGreen: {
    backgroundColor: hex.green,
  },
  dotRed: {
    backgroundColor: hex.red,
  },
  dotYellow: {
    backgroundColor: hex.yellow,
  },
  rowInfo: {
    flex: 1,
  },
  rowName: {
    fontSize: 15,
    fontFamily: 'SpaceGrotesk_600SemiBold',
    color: hex.text,
  },
  rowHost: {
    fontSize: 12,
    color: hex.subtext0,
    marginTop: 2,
    fontFamily: 'JetBrainsMono_400Regular',
  },
  rowAction: {
    fontSize: 12,
    color: hex.accent,
    fontFamily: 'SpaceGrotesk_600SemiBold',
    textTransform: 'uppercase',
  },
  rowActionDisconnect: {
    color: hex.overlay0,
  },
  scanningRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 12,
    justifyContent: 'center',
  },
  scanningText: {
    fontSize: 13,
    color: hex.subtext0,
  },
  noDiscovered: {
    fontSize: 13,
    color: hex.overlay0,
    marginBottom: 8,
    textAlign: 'center',
  },
  rescanButton: {
    marginBottom: 24,
    alignSelf: 'center',
  },
  rescanText: {
    fontSize: 14,
    color: hex.accent,
  },
  divider: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: 24,
  },
  dividerLine: {
    flex: 1,
    height: 1,
    backgroundColor: hex.surface1,
  },
  dividerText: {
    color: hex.overlay0,
    fontSize: 12,
    marginHorizontal: 12,
  },
  primaryButton: {
    backgroundColor: hex.accent,
    paddingHorizontal: 32,
    paddingVertical: 14,
    borderRadius: 0,
    alignSelf: 'center',
  },
  primaryButtonText: {
    color: hex.base,
    fontSize: 16,
    fontFamily: 'SpaceGrotesk_600SemiBold',
  },
  buttonDisabled: {
    opacity: 0.6,
  },
  linkButton: {
    marginTop: 16,
  },
  linkText: {
    color: hex.accent,
    fontSize: 14,
  },
  cameraContainer: {
    width: 280,
    height: 280,
    borderRadius: 0,
    overflow: 'hidden',
    marginBottom: 24,
  },
  camera: {
    flex: 1,
  },
});
