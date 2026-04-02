import { useEffect, useRef, useState, createContext, useContext, useCallback } from 'react';
import { Stack, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { StyleSheet } from 'react-native';
import * as SecureStore from 'expo-secure-store';
import { RelayClient, parseGrimoireUri, checkDaemonHealth } from '../lib/relay-client';
import type { AgentStatus, ConnectionConfig, StreamEvent } from '../lib/types';
import { catppuccin } from '../lib/theme';
import {
  requestNotificationPermissions,
  notifyAgentEvent,
  addNotificationResponseListener,
  registerPushToken,
} from '../lib/notifications';
import { getSavedTailscaleConfig } from '../lib/discovery';

interface RelayContextValue {
  client: RelayClient | null;
  connected: boolean;
  agents: AgentStatus[];
  connect: (config: ConnectionConfig) => Promise<boolean>;
  connectFromUri: (uri: string) => Promise<boolean>;
  disconnect: () => void;
  refreshAgents: () => Promise<void>;
  config: ConnectionConfig | null;
  ready: boolean;
}

const RelayContext = createContext<RelayContextValue>({
  client: null,
  connected: false,
  agents: [],
  connect: async () => false,
  connectFromUri: async () => false,
  disconnect: () => {},
  refreshAgents: async () => {},
  config: null,
  ready: false,
});

export function useRelay() {
  return useContext(RelayContext);
}

const STORE_KEY = 'grimoire_connection';

export default function RootLayout() {
  const [client, setClient] = useState<RelayClient | null>(null);
  const [connected, setConnected] = useState(false);
  const [agents, setAgents] = useState<AgentStatus[]>([]);
  const [config, setConfig] = useState<ConnectionConfig | null>(null);
  const [ready, setReady] = useState(false);
  const clientRef = useRef<RelayClient | null>(null);
  const router = useRouter();

  // Restore saved connection on mount
  useEffect(() => {
    SecureStore.getItemAsync(STORE_KEY)
      .then(async (stored) => {
        if (stored) {
          try {
            const saved: ConnectionConfig = JSON.parse(stored);
            const ok = await connectToConfig(saved);
            if (ok) return;
          } catch {
            // ignore corrupt data
          }
        }
        // Primary config failed or missing — try saved Tailscale config
        const tsConfig = await getSavedTailscaleConfig();
        if (tsConfig) {
          await connectToConfig(tsConfig);
        }
      })
      .finally(() => {
        setReady(true);
      });

    requestNotificationPermissions();

    // Navigate to the agent's stream when user taps a notification
    const sub = addNotificationResponseListener((agentId) => {
      router.push(`/stream/${agentId}`);
    });

    return () => {
      sub.remove();
      clientRef.current?.dispose();
    };
  }, []);

  const connectToConfig = useCallback(async (cfg: ConnectionConfig): Promise<boolean> => {
    // Check health first
    const healthy = await checkDaemonHealth(cfg);
    if (!healthy) return false;

    // Dispose old client
    clientRef.current?.dispose();

    const newClient = new RelayClient(cfg);

    newClient.onStatus((isConnected) => {
      setConnected(isConnected);
    });

    newClient.onStreams((event: StreamEvent) => {
      if (event.type === 'snapshot' && Array.isArray(event.data)) {
        setAgents(event.data);
      } else if (event.type === 'agent_spawned' && !Array.isArray(event.data)) {
        setAgents((prev) => [...prev, event.data as AgentStatus]);
      } else if (event.type === 'agent_killed' && !Array.isArray(event.data)) {
        const killed = event.data as AgentStatus;
        setAgents((prev) => prev.filter((a) => a.id !== killed.id));
      } else if (event.type === 'status_changed' && !Array.isArray(event.data)) {
        const updated = event.data as AgentStatus;
        setAgents((prev) => prev.map((a) => (a.id === updated.id ? { ...a, ...updated } : a)));
      }

      // Fire local notification when app is backgrounded
      if (event.type !== 'snapshot') {
        notifyAgentEvent(event);
      }
    });

    newClient.connectStreams();

    clientRef.current = newClient;
    setClient(newClient);
    setConfig(cfg);

    // Persist config — may fail if device is locked (Keychain unavailable)
    try {
      await SecureStore.setItemAsync(STORE_KEY, JSON.stringify(cfg));
    } catch {
      // Keychain write rejected (e.g. device locked during background restore)
    }

    // Register push token so daemon can send remote notifications
    registerPushToken(cfg).catch(() => {});

    return true;
  }, []);

  const connectFromUri = useCallback(async (uri: string): Promise<boolean> => {
    const parsed = parseGrimoireUri(uri);
    if (!parsed) return false;
    return connectToConfig(parsed);
  }, [connectToConfig]);

  const refreshAgents = useCallback(async () => {
    if (!clientRef.current) return;
    try {
      const fresh = await clientRef.current.getStatus();
      if (Array.isArray(fresh)) {
        setAgents(fresh);
      }
    } catch {
      // ignore — WebSocket will recover on its own
    }
  }, []);

  const disconnect = useCallback(() => {
    clientRef.current?.dispose();
    clientRef.current = null;
    setClient(null);
    setConnected(false);
    setAgents([]);
    setConfig(null);
    SecureStore.deleteItemAsync(STORE_KEY);
  }, []);

  return (
    <RelayContext.Provider
      value={{
        client,
        connected,
        agents,
        connect: connectToConfig,
        connectFromUri,
        disconnect,
        refreshAgents,
        config,
        ready,
      }}
    >
      <StatusBar style="light" />
      <Stack
        initialRouteName="connect"
        screenOptions={{
          headerStyle: { backgroundColor: catppuccin.mantle },
          headerTintColor: catppuccin.text,
          headerTitleStyle: { fontWeight: '600' },
          contentStyle: { backgroundColor: catppuccin.base },
        }}
      >
        <Stack.Screen name="connect" options={{ title: 'Connect', headerShown: false }} />
        <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
        <Stack.Screen
          name="stream/[id]"
          options={{
            title: 'Terminal',
            headerBackTitle: 'Back',
          }}
        />
      </Stack>
    </RelayContext.Provider>
  );
}
