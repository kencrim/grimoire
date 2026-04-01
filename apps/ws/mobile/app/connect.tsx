import { useState, useEffect } from 'react';
import {
  View,
  Text,
  Pressable,
  StyleSheet,
  ActivityIndicator,
  ScrollView,
} from 'react-native';
import { Redirect } from 'expo-router';
import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { useRelay } from './_layout';
import { catppuccin } from '../lib/theme';
import { parseGrimoireUri } from '../lib/relay-client';
import { discoverDaemons, saveTailscaleConfig, type DiscoveredDaemon } from '../lib/discovery';

export default function ConnectScreen() {
  const { connected, connect, connectFromUri, config } = useRelay();
  const [permission, requestPermission] = useCameraPermissions();
  const [showScanner, setShowScanner] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [discovered, setDiscovered] = useState<DiscoveredDaemon[]>([]);
  const [scanning, setScanning] = useState(false);

  useEffect(() => {
    if (!connected) {
      runDiscovery();
    }
  }, []);

  if (connected) {
    return <Redirect href="/(tabs)" />;
  }

  const runDiscovery = async () => {
    setScanning(true);
    try {
      const results = await discoverDaemons(config);
      setDiscovered(results);
    } catch {
      // ignore
    }
    setScanning(false);
  };

  const handleDiscoveredConnect = async (daemon: DiscoveredDaemon) => {
    setLoading(true);
    setError('');
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);

    if (daemon.type === 'tailscale') {
      saveTailscaleConfig({ host: daemon.host, port: daemon.port, token: daemon.token });
    }

    const ok = await connect({
      host: daemon.host,
      port: daemon.port,
      token: daemon.token,
    });

    setLoading(false);
    if (!ok) {
      setError(`Could not connect to ${daemon.host}`);
    }
  };

  const handleQrScanned = async (data: string) => {
    setShowScanner(false);
    setLoading(true);
    setError('');

    const parsed = parseGrimoireUri(data);
    if (!parsed) {
      setError('Invalid QR code. Expected grimoire:// URI.');
      setLoading(false);
      return;
    }

    if (parsed.host.includes('.ts.net')) {
      saveTailscaleConfig({ host: parsed.host, port: parsed.port, token: parsed.token });
    }

    const ok = await connectFromUri(data);
    setLoading(false);
    if (!ok) {
      setError('Could not connect to daemon. Is it running?');
    }
  };

  if (showScanner) {
    if (!permission?.granted) {
      return (
        <View style={styles.container}>
          <Text style={styles.title}>Camera Permission</Text>
          <Text style={styles.subtitle}>We need camera access to scan QR codes.</Text>
          <Pressable style={styles.button} onPress={requestPermission}>
            <Text style={styles.buttonText}>Grant Permission</Text>
          </Pressable>
          <Pressable style={styles.linkButton} onPress={() => setShowScanner(false)}>
            <Text style={styles.linkText}>Back</Text>
          </Pressable>
        </View>
      );
    }

    return (
      <View style={styles.container}>
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
        <Pressable style={styles.linkButton} onPress={() => setShowScanner(false)}>
          <Text style={styles.linkText}>Back</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <ScrollView
      contentContainerStyle={styles.scrollContent}
      keyboardShouldPersistTaps="handled"
    >
      <Text style={styles.logo}>grimoire</Text>
      <Text style={styles.subtitle}>Connect to your relay daemon</Text>

      {/* Discovered daemons */}
      {discovered.length > 0 && (
        <View style={styles.discoveredSection}>
          <Text style={styles.sectionLabel}>Discovered</Text>
          {discovered.map((d) => (
            <Pressable
              key={d.host}
              style={({ pressed }) => [
                styles.discoveredItem,
                pressed && styles.discoveredItemPressed,
              ]}
              onPress={() => handleDiscoveredConnect(d)}
              disabled={loading}
            >
              <View style={styles.discoveredDot} />
              <View style={styles.discoveredInfo}>
                <Text style={styles.discoveredLabel}>{d.label}</Text>
                <Text style={styles.discoveredHost}>
                  {d.host}:{d.port}
                </Text>
              </View>
              <Text style={styles.discoveredType}>{d.type}</Text>
            </Pressable>
          ))}
        </View>
      )}

      {scanning && (
        <View style={styles.scanningRow}>
          <ActivityIndicator size="small" color={catppuccin.lavender} />
          <Text style={styles.scanningText}>Scanning network...</Text>
        </View>
      )}

      {!scanning && discovered.length === 0 && (
        <Text style={styles.noDiscovered}>No daemons found on network</Text>
      )}

      <Pressable style={styles.rescanButton} onPress={runDiscovery} disabled={scanning}>
        <Text style={styles.rescanText}>Rescan</Text>
      </Pressable>

      {error ? <Text style={styles.error}>{error}</Text> : null}

      <View style={styles.divider}>
        <View style={styles.dividerLine} />
        <Text style={styles.dividerText}>or</Text>
        <View style={styles.dividerLine} />
      </View>

      <Pressable
        style={[styles.scanButton, loading && styles.buttonDisabled]}
        onPress={() => setShowScanner(true)}
        disabled={loading}
      >
        <Text style={styles.scanButtonText}>Scan QR Code</Text>
      </Pressable>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: catppuccin.base,
    justifyContent: 'center',
    alignItems: 'center',
    padding: 24,
  },
  scrollContent: {
    flexGrow: 1,
    alignItems: 'center',
    paddingHorizontal: 24,
    paddingTop: 80,
    paddingBottom: 40,
  },
  logo: {
    fontSize: 32,
    fontWeight: '700',
    color: catppuccin.lavender,
    marginBottom: 6,
  },
  title: {
    fontSize: 24,
    fontWeight: '700',
    color: catppuccin.text,
    marginBottom: 8,
  },
  subtitle: {
    fontSize: 14,
    color: catppuccin.subtext0,
    textAlign: 'center',
    marginBottom: 32,
  },
  discoveredSection: {
    width: '100%',
    marginBottom: 24,
  },
  sectionLabel: {
    fontSize: 13,
    fontWeight: '600',
    color: catppuccin.subtext0,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
    marginBottom: 8,
    marginLeft: 4,
  },
  discoveredItem: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: catppuccin.surface0,
    borderRadius: 10,
    paddingHorizontal: 14,
    paddingVertical: 12,
    marginBottom: 8,
  },
  discoveredItemPressed: {
    backgroundColor: catppuccin.surface1,
  },
  discoveredDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: catppuccin.green,
    marginRight: 10,
  },
  discoveredInfo: {
    flex: 1,
  },
  discoveredLabel: {
    fontSize: 15,
    fontWeight: '500',
    color: catppuccin.text,
  },
  discoveredHost: {
    fontSize: 12,
    color: catppuccin.subtext0,
    marginTop: 2,
    fontFamily: 'Menlo',
  },
  discoveredType: {
    fontSize: 11,
    color: catppuccin.overlay0,
    textTransform: 'uppercase',
  },
  scanningRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    marginBottom: 12,
  },
  scanningText: {
    fontSize: 13,
    color: catppuccin.subtext0,
  },
  noDiscovered: {
    fontSize: 13,
    color: catppuccin.overlay0,
    marginBottom: 8,
  },
  rescanButton: {
    marginBottom: 24,
  },
  rescanText: {
    fontSize: 14,
    color: catppuccin.lavender,
  },
  divider: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: 24,
    width: '100%',
  },
  dividerLine: {
    flex: 1,
    height: 1,
    backgroundColor: catppuccin.surface1,
  },
  dividerText: {
    color: catppuccin.overlay0,
    fontSize: 12,
    marginHorizontal: 12,
  },
  scanButton: {
    backgroundColor: catppuccin.lavender,
    paddingHorizontal: 32,
    paddingVertical: 14,
    borderRadius: 12,
  },
  scanButtonText: {
    color: catppuccin.base,
    fontSize: 16,
    fontWeight: '600',
  },
  button: {
    backgroundColor: catppuccin.lavender,
    paddingHorizontal: 32,
    paddingVertical: 14,
    borderRadius: 12,
    marginTop: 8,
    width: '100%',
    alignItems: 'center',
  },
  buttonDisabled: {
    opacity: 0.6,
  },
  buttonText: {
    color: catppuccin.base,
    fontSize: 16,
    fontWeight: '600',
  },
  linkButton: {
    marginTop: 16,
  },
  linkText: {
    color: catppuccin.lavender,
    fontSize: 14,
  },
  error: {
    color: catppuccin.red,
    fontSize: 13,
    marginBottom: 12,
  },
  cameraContainer: {
    width: 280,
    height: 280,
    borderRadius: 16,
    overflow: 'hidden',
    marginBottom: 24,
  },
  camera: {
    flex: 1,
  },
});
