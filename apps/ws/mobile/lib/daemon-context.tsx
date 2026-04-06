import { createContext, useContext, useEffect, useRef, useState, useCallback, useMemo } from 'react';
import * as SecureStore from 'expo-secure-store';
import { randomUUID } from 'expo-crypto';
import { toast } from 'sonner-native';
import { RelayClient, parseHexUri, checkDaemonHealth } from './relay-client';
import { mergeAgentList } from './agents';
import { getSavedTailscaleConfig, saveTailscaleConfig } from './discovery';
import type { AgentStatus, ConnectionConfig, DaemonEntry, QualifiedAgent, StreamEvent } from './types';

// --- Toast helpers ---

function agentDisplayName(id: string): string {
  return id.includes('/') ? id.split('/').pop()! : id;
}

function notifyStreamEvent(event: StreamEvent): void {
  if (event.type === 'status_changed' && !Array.isArray(event.data)) {
    const agent = event.data as AgentStatus;
    if (agent.status === 'idle') {
      toast.success(`${agentDisplayName(agent.id)} is ready`, {
        description: 'Waiting for input',
      });
    }
  } else if (event.type === 'agent_spawned' && !Array.isArray(event.data)) {
    const agent = event.data as AgentStatus;
    toast(`${agentDisplayName(agent.id)} spawned`, {
      description: agent.agent ?? 'workstream',
    });
  } else if (event.type === 'agent_killed' && !Array.isArray(event.data)) {
    const agent = event.data as AgentStatus;
    toast(`${agentDisplayName(agent.id)} ended`);
  }
}

// --- DaemonState (lives here to avoid circular import with RelayClient) ---

export interface DaemonState {
  entry: DaemonEntry;
  client: RelayClient | null;
  connected: boolean;
  agents: AgentStatus[];
}

// --- Context shape ---

interface DaemonManagerContextValue {
  daemons: Map<string, DaemonState>;
  daemonOrder: string[];
  allAgents: QualifiedAgent[];
  ready: boolean;
  anyConnected: boolean;

  addDaemon: (name: string, config: ConnectionConfig) => DaemonEntry;
  removeDaemon: (daemonId: string) => void;
  renameDaemon: (daemonId: string, newName: string) => void;
  connectDaemon: (daemonId: string) => Promise<boolean>;
  disconnectDaemon: (daemonId: string) => void;
  connectFromUri: (uri: string) => Promise<boolean>;
  refreshAgents: (daemonId?: string) => Promise<void>;
  resolveAgent: (qualifiedId: string) => { daemon: DaemonState; agent: AgentStatus } | null;
  getClient: (daemonId: string) => RelayClient | null;
}

const DaemonManagerContext = createContext<DaemonManagerContextValue>({
  daemons: new Map(),
  daemonOrder: [],
  allAgents: [],
  ready: false,
  anyConnected: false,
  addDaemon: () => { throw new Error('No provider'); },
  removeDaemon: () => {},
  renameDaemon: () => {},
  connectDaemon: async () => false,
  disconnectDaemon: () => {},
  connectFromUri: async () => false,
  refreshAgents: async () => {},
  resolveAgent: () => null,
  getClient: () => null,
});

export function useDaemons() {
  return useContext(DaemonManagerContext);
}

// --- Persistence ---

const STORE_KEY = 'hex_daemons';
const OLD_STORE_KEY = 'hex_connection';

async function persistEntries(entries: DaemonEntry[]): Promise<void> {
  try {
    await SecureStore.setItemAsync(STORE_KEY, JSON.stringify(entries));
  } catch {
    // Keychain write rejected (e.g. device locked)
  }
}

async function loadEntries(): Promise<DaemonEntry[]> {
  const stored = await SecureStore.getItemAsync(STORE_KEY);
  if (stored) {
    try {
      return JSON.parse(stored);
    } catch {
      return [];
    }
  }

  // Migrate from old single-daemon format
  const old = await SecureStore.getItemAsync(OLD_STORE_KEY);
  if (old) {
    try {
      const config: ConnectionConfig = JSON.parse(old);
      const migrated: DaemonEntry = {
        id: randomUUID(),
        name: 'Default',
        config,
        addedAt: new Date().toISOString(),
      };
      await persistEntries([migrated]);
      await SecureStore.deleteItemAsync(OLD_STORE_KEY);
      return [migrated];
    } catch {
      // ignore corrupt data
    }
  }

  return [];
}

// --- Provider ---

export function DaemonManagerProvider({ children }: { children: React.ReactNode }) {
  const [daemonMap, setDaemonMap] = useState<Map<string, DaemonState>>(new Map());
  const [daemonOrder, setDaemonOrder] = useState<string[]>([]);
  const [ready, setReady] = useState(false);

  // Mutable ref mirrors state for use in WebSocket callbacks (avoids stale closures)
  const mapRef = useRef<Map<string, DaemonState>>(new Map());
  // Suppress toasts during initial restore so we don't spam on app launch
  const initialLoadDoneRef = useRef(false);

  // Keep ref in sync
  useEffect(() => {
    mapRef.current = daemonMap;
  }, [daemonMap]);

  // --- Derived ---

  const allAgents = useMemo(() => {
    const result: QualifiedAgent[] = [];
    for (const [daemonId, state] of daemonMap) {
      for (const agent of state.agents) {
        result.push({
          ...agent,
          daemonId,
          qualifiedId: `${daemonId}::${agent.id}`,
        });
      }
    }
    return result;
  }, [daemonMap]);

  const anyConnected = useMemo(() => {
    for (const state of daemonMap.values()) {
      if (state.connected) return true;
    }
    return false;
  }, [daemonMap]);

  // --- Helpers ---

  const getEntries = useCallback((): DaemonEntry[] => {
    return Array.from(mapRef.current.values()).map(s => s.entry);
  }, []);

  const updateDaemon = useCallback((daemonId: string, updater: (state: DaemonState) => DaemonState) => {
    setDaemonMap(prev => {
      const state = prev.get(daemonId);
      if (!state) return prev;
      const next = new Map(prev);
      next.set(daemonId, updater(state));
      return next;
    });
  }, []);

  // --- Wire up a RelayClient for a daemon ---

  const wireClient = useCallback((daemonId: string, client: RelayClient) => {
    client.onStatus((isConnected) => {
      updateDaemon(daemonId, s => {
        const name = s.entry.name;
        // Only toast on transitions, not initial state
        if (s.connected && !isConnected) {
          toast.error(`Disconnected from ${name}`);
        } else if (!s.connected && isConnected) {
          toast.success(`Connected to ${name}`);
        }
        return { ...s, connected: isConnected };
      });
    });

    client.onStreams((event: StreamEvent) => {
      notifyStreamEvent(event);
      updateDaemon(daemonId, s => {
        if (event.type === 'snapshot' && Array.isArray(event.data)) {
          return { ...s, agents: mergeAgentList(s.agents, event.data) };
        } else if (event.type === 'agent_spawned' && !Array.isArray(event.data)) {
          return { ...s, agents: [...s.agents, event.data as AgentStatus] };
        } else if (event.type === 'agent_killed' && !Array.isArray(event.data)) {
          const killed = event.data as AgentStatus;
          return { ...s, agents: s.agents.filter(a => a.id !== killed.id) };
        } else if (event.type === 'status_changed' && !Array.isArray(event.data)) {
          const updated = event.data as AgentStatus;
          return { ...s, agents: s.agents.map(a => a.id === updated.id ? { ...a, ...updated } : a) };
        }
        return s;
      });
    });

    client.connectStreams();
  }, [updateDaemon]);

  // --- Public API ---

  const addDaemon = useCallback((name: string, config: ConnectionConfig): DaemonEntry => {
    // Dedup: same host:port OR same name (same daemon at different IPs)
    for (const state of mapRef.current.values()) {
      if (
        (state.entry.config.host === config.host && state.entry.config.port === config.port) ||
        state.entry.name === name
      ) {
        return state.entry;
      }
    }

    const entry: DaemonEntry = {
      id: randomUUID(),
      name,
      config,
      addedAt: new Date().toISOString(),
    };

    const state: DaemonState = { entry, client: null, connected: false, agents: [] };

    // Eagerly update the ref so connectDaemon can read it immediately
    mapRef.current = new Map(mapRef.current);
    mapRef.current.set(entry.id, state);

    setDaemonMap(prev => {
      const next = new Map(prev);
      next.set(entry.id, state);
      return next;
    });
    setDaemonOrder(prev => [...prev, entry.id]);

    // Persist — getEntries() already includes the new entry via mapRef
    setTimeout(() => {
      persistEntries(getEntries());
    }, 0);

    return entry;
  }, [getEntries]);

  const removeDaemon = useCallback((daemonId: string) => {
    const state = mapRef.current.get(daemonId);
    if (state?.client) {
      state.client.dispose();
    }

    setDaemonMap(prev => {
      const next = new Map(prev);
      next.delete(daemonId);
      return next;
    });
    setDaemonOrder(prev => prev.filter(id => id !== daemonId));

    // Persist after removal
    setTimeout(() => {
      const entries = getEntries().filter(e => e.id !== daemonId);
      persistEntries(entries);
    }, 0);
  }, [getEntries]);

  const renameDaemon = useCallback((daemonId: string, newName: string) => {
    updateDaemon(daemonId, s => ({
      ...s,
      entry: { ...s.entry, name: newName },
    }));

    setTimeout(() => {
      const entries = getEntries().map(e =>
        e.id === daemonId ? { ...e, name: newName } : e
      );
      persistEntries(entries);
    }, 0);
  }, [updateDaemon, getEntries]);

  const connectDaemon = useCallback(async (daemonId: string): Promise<boolean> => {
    const state = mapRef.current.get(daemonId);
    if (!state) return false;

    const healthy = await checkDaemonHealth(state.entry.config);
    if (!healthy) return false;

    // Dispose old client if any
    state.client?.dispose();

    const client = new RelayClient(state.entry.config);
    updateDaemon(daemonId, s => ({ ...s, client }));
    wireClient(daemonId, client);

    return true;
  }, [updateDaemon, wireClient]);

  const disconnectDaemon = useCallback((daemonId: string) => {
    const state = mapRef.current.get(daemonId);
    if (!state) return;

    state.client?.dispose();
    updateDaemon(daemonId, s => ({
      ...s,
      client: null,
      connected: false,
      agents: [],
    }));
  }, [updateDaemon]);

  const connectFromUri = useCallback(async (uri: string): Promise<boolean> => {
    const parsed = parseHexUri(uri);
    if (!parsed) return false;

    if (parsed.host.includes('.ts.net')) {
      saveTailscaleConfig(parsed);
    }

    // Check if we already have a daemon with this host:port
    for (const state of mapRef.current.values()) {
      if (state.entry.config.host === parsed.host && state.entry.config.port === parsed.port) {
        // Update token and reconnect
        updateDaemon(state.entry.id, s => ({
          ...s,
          entry: { ...s.entry, config: parsed },
        }));
        return connectDaemon(state.entry.id);
      }
    }

    // New daemon
    const label = parsed.host.includes('.ts.net')
      ? `Tailscale (${parsed.host.split('.')[0]})`
      : parsed.host;
    const entry = addDaemon(label, parsed);
    return connectDaemon(entry.id);
  }, [addDaemon, connectDaemon, updateDaemon]);

  const refreshAgents = useCallback(async (daemonId?: string) => {
    const targets = daemonId
      ? [mapRef.current.get(daemonId)].filter(Boolean) as DaemonState[]
      : Array.from(mapRef.current.values());

    await Promise.allSettled(
      targets.map(async (state) => {
        if (!state.client) return;
        try {
          const fresh = await state.client.getStatus();
          if (Array.isArray(fresh)) {
            updateDaemon(state.entry.id, s => ({
              ...s,
              agents: mergeAgentList(s.agents, fresh),
            }));
          }
        } catch {
          // ignore — WebSocket will recover
        }
      }),
    );
  }, [updateDaemon]);

  const resolveAgent = useCallback((qualifiedId: string): { daemon: DaemonState; agent: AgentStatus } | null => {
    const sepIdx = qualifiedId.indexOf('::');
    if (sepIdx === -1) return null;
    const daemonId = qualifiedId.substring(0, sepIdx);
    const agentId = qualifiedId.substring(sepIdx + 2);
    const state = mapRef.current.get(daemonId);
    if (!state) return null;
    const agent = state.agents.find(a => a.id === agentId);
    if (!agent) return null;
    return { daemon: state, agent };
  }, []);

  const getClient = useCallback((daemonId: string): RelayClient | null => {
    return mapRef.current.get(daemonId)?.client ?? null;
  }, []);

  // --- Restore on mount ---

  useEffect(() => {
    (async () => {
      const entries = await loadEntries();

      if (entries.length === 0) {
        setReady(true);
        return;
      }

      // Initialize state for all entries
      const initial = new Map<string, DaemonState>();
      const order: string[] = [];
      for (const entry of entries) {
        initial.set(entry.id, { entry, client: null, connected: false, agents: [] });
        order.push(entry.id);
      }
      setDaemonMap(initial);
      setDaemonOrder(order);
      mapRef.current = initial;

      // Connect all concurrently
      await Promise.allSettled(
        entries.map(async (entry) => {
          const healthy = await checkDaemonHealth(entry.config);
          if (!healthy) return;

          const client = new RelayClient(entry.config);

          // Update the map with the client before wiring
          setDaemonMap(prev => {
            const next = new Map(prev);
            const state = next.get(entry.id);
            if (state) {
              next.set(entry.id, { ...state, client });
            }
            return next;
          });

          // Wire callbacks
          client.onStatus((isConnected) => {
            setDaemonMap(prev => {
              const next = new Map(prev);
              const s = next.get(entry.id);
              if (!s) return prev;
              // Toast on disconnect after initial load
              if (initialLoadDoneRef.current) {
                if (s.connected && !isConnected) {
                  toast.error(`Disconnected from ${entry.name}`);
                } else if (!s.connected && isConnected) {
                  toast.success(`Connected to ${entry.name}`);
                }
              }
              next.set(entry.id, { ...s, connected: isConnected });
              return next;
            });
          });

          client.onStreams((event: StreamEvent) => {
            if (initialLoadDoneRef.current) {
              notifyStreamEvent(event);
            }
            setDaemonMap(prev => {
              const next = new Map(prev);
              const s = next.get(entry.id);
              if (!s) return prev;

              let newAgents: AgentStatus[];
              if (event.type === 'snapshot' && Array.isArray(event.data)) {
                newAgents = mergeAgentList(s.agents, event.data);
              } else if (event.type === 'agent_spawned' && !Array.isArray(event.data)) {
                newAgents = [...s.agents, event.data as AgentStatus];
              } else if (event.type === 'agent_killed' && !Array.isArray(event.data)) {
                const killed = event.data as AgentStatus;
                newAgents = s.agents.filter(a => a.id !== killed.id);
              } else if (event.type === 'status_changed' && !Array.isArray(event.data)) {
                const updated = event.data as AgentStatus;
                newAgents = s.agents.map(a => a.id === updated.id ? { ...a, ...updated } : a);
              } else {
                return prev;
              }

              next.set(entry.id, { ...s, agents: newAgents });
              return next;
            });
          });

          client.connectStreams();
        }),
      );

      initialLoadDoneRef.current = true;
      setReady(true);
    })();

    return () => {
      for (const state of mapRef.current.values()) {
        state.client?.dispose();
      }
    };
  }, []);

  const value = useMemo<DaemonManagerContextValue>(() => ({
    daemons: daemonMap,
    daemonOrder,
    allAgents,
    ready,
    anyConnected,
    addDaemon,
    removeDaemon,
    renameDaemon,
    connectDaemon,
    disconnectDaemon,
    connectFromUri,
    refreshAgents,
    resolveAgent,
    getClient,
  }), [
    daemonMap, daemonOrder, allAgents, ready, anyConnected,
    addDaemon, removeDaemon, renameDaemon, connectDaemon, disconnectDaemon,
    connectFromUri, refreshAgents, resolveAgent, getClient,
  ]);

  return (
    <DaemonManagerContext.Provider value={value}>
      {children}
    </DaemonManagerContext.Provider>
  );
}
