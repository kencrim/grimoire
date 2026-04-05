import * as MdnsDiscovery from '../modules/mdns-discovery';
import * as SecureStore from 'expo-secure-store';
import type { ConnectionConfig } from './types';

const TAILSCALE_HOST_KEY = 'hex_ts_host';
const MDNS_SCAN_DURATION = 5000;

type DebugCallback = (msg: string) => void;

export interface DiscoveredDaemon {
  host: string;
  port: number;
  token: string;
  label: string;
  type: 'lan' | 'tailscale' | 'saved';
}

// discoverDaemons scans for hex relay daemons via mDNS and saved connections.
export async function discoverDaemons(
  savedConfigs: ConnectionConfig[],
  onDebug?: DebugCallback
): Promise<DiscoveredDaemon[]> {
  const log = onDebug ?? (() => {});
  const found: DiscoveredDaemon[] = [];

  log('Starting mDNS scan + saved config probe');

  const [mdnsResults] = await Promise.allSettled([
    scanMDNS(log),
    probeSavedConnections(savedConfigs, found, log),
  ]);

  if (mdnsResults.status === 'fulfilled') {
    found.push(...mdnsResults.value);
  } else {
    log(`mDNS scan failed: ${mdnsResults.reason}`);
  }

  // Deduplicate by host:port AND by service name (same daemon can appear
  // at multiple IPs — LAN, link-local, Tailscale)
  const seenHostPort = new Set<string>();
  const seenLabel = new Set<string>();
  return found.filter((d) => {
    const hostKey = `${d.host}:${d.port}`;
    if (seenHostPort.has(hostKey)) return false;
    // Same service name at a different IP = same daemon
    if (seenLabel.has(d.label)) return false;
    seenHostPort.add(hostKey);
    seenLabel.add(d.label);
    return true;
  });
}

function scanMDNS(log: DebugCallback): Promise<DiscoveredDaemon[]> {
  return new Promise((resolve) => {
    const results: DiscoveredDaemon[] = [];

    const foundSub = MdnsDiscovery.onServiceFound((service) => {
      log(`mDNS found: ${service.name} @ ${service.host}:${service.port}`);
      log(`  TXT: ${JSON.stringify(service.txt)}`);
      log(`  Addresses: ${service.addresses.join(', ')}`);

      const host =
        service.addresses.find((a) => a.includes('.') && !a.includes(':')) ?? service.host;
      const token = service.txt?.token ?? '';
      const tailscaleHost = service.txt?.tailscale ?? '';

      if (host && service.port > 0) {
        const isTailscale = !!tailscaleHost || host.startsWith('100.') || host.includes('.ts.net');
        results.push({
          host,
          port: service.port,
          token,
          label: service.name,
          type: isTailscale ? 'tailscale' : 'lan',
        });

        if (tailscaleHost && token) {
          saveTailscaleConfig({ host: tailscaleHost, port: service.port, token });
        }
      }
    });

    const errorSub = MdnsDiscovery.onScanError((err) => {
      log(`mDNS error: ${err.message}`);
    });

    log('mDNS scanning for _hex._tcp');
    MdnsDiscovery.startScan('_hex._tcp');

    setTimeout(() => {
      MdnsDiscovery.stopScan();
      foundSub.remove();
      errorSub.remove();
      log(`mDNS scan complete: ${results.length} services`);
      resolve(results);
    }, MDNS_SCAN_DURATION);
  });
}

async function probeSavedConnections(
  savedConfigs: ConnectionConfig[],
  found: DiscoveredDaemon[],
  log: DebugCallback
): Promise<void> {
  const checks: Promise<void>[] = [];
  const probed = new Set<string>();

  for (const cfg of savedConfigs) {
    const key = `${cfg.host}:${cfg.port}`;
    if (probed.has(key)) continue;
    probed.add(key);

    log(`Probing saved: ${cfg.host}:${cfg.port}`);
    checks.push(
      probeHost(cfg.host, cfg.port).then((ok) => {
        log(`Saved probe ${cfg.host}: ${ok ? 'reachable' : 'unreachable'}`);
        if (ok) {
          found.push({
            host: cfg.host,
            port: cfg.port,
            token: cfg.token,
            label: `Saved (${cfg.host})`,
            type: cfg.host.includes('.ts.net') ? 'tailscale' : 'saved',
          });
        }
      })
    );
  }

  // Probe saved Tailscale config independently — works even after disconnect
  const tsConfig = await getSavedTailscaleConfig();
  if (tsConfig && !probed.has(`${tsConfig.host}:${tsConfig.port}`)) {
    log(`Probing Tailscale: ${tsConfig.host}:${tsConfig.port}`);
    checks.push(
      probeHost(tsConfig.host, tsConfig.port).then((ok) => {
        log(`Tailscale probe ${tsConfig.host}: ${ok ? 'reachable' : 'unreachable'}`);
        if (ok) {
          found.push({
            host: tsConfig.host,
            port: tsConfig.port,
            token: tsConfig.token,
            label: `Tailscale (${tsConfig.host})`,
            type: 'tailscale',
          });
        }
      })
    );
  }

  await Promise.allSettled(checks);
}

async function probeHost(host: string, port: number): Promise<boolean> {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 2000);
    const resp = await fetch(`http://${host}:${port}/api/health`, {
      signal: controller.signal,
    });
    clearTimeout(timeout);
    if (resp.ok) {
      const data = await resp.json();
      return data.status === 'ok';
    }
  } catch {
    // not reachable
  }
  return false;
}

// Save a complete Tailscale connection config so the app can reach the
// daemon over Tailscale even after disconnect wipes the primary config.
export async function saveTailscaleConfig(config: ConnectionConfig): Promise<void> {
  if (config.host.includes('.ts.net') || config.host.startsWith('100.')) {
    await SecureStore.setItemAsync(TAILSCALE_HOST_KEY, JSON.stringify(config));
  }
}

export async function getSavedTailscaleConfig(): Promise<ConnectionConfig | null> {
  const stored = await SecureStore.getItemAsync(TAILSCALE_HOST_KEY);
  if (!stored) return null;
  try {
    // Handle legacy format (plain hostname string)
    if (!stored.startsWith('{')) return null;
    return JSON.parse(stored);
  } catch {
    return null;
  }
}
