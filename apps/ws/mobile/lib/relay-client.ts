import type {
  AgentStatus,
  ConnectionConfig,
  Envelope,
  PaneFrame,
  PaneInputMsg,
  StreamEvent,
} from './types';

type StreamsCallback = (event: StreamEvent) => void;
type PaneCallback = (frame: PaneFrame) => void;
type StatusCallback = (connected: boolean) => void;

const RECONNECT_DELAYS = [1000, 2000, 5000, 10000];

export class RelayClient {
  private config: ConnectionConfig;
  private streamsWs: WebSocket | null = null;
  private paneWs: WebSocket | null = null;
  private relayWs: WebSocket | null = null;
  private activePaneRef: string | null = null;
  private activePaneType: 'agent' | 'terminal' = 'agent';

  private streamsCallbacks = new Set<StreamsCallback>();
  private paneCallbacks = new Set<PaneCallback>();
  private statusCallbacks = new Set<StatusCallback>();
  private reconnectAttempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private disposed = false;

  constructor(config: ConnectionConfig) {
    this.config = config;
  }

  private get baseUrl(): string {
    return `ws://${this.config.host}:${this.config.port}`;
  }

  private get authParam(): string {
    return `token=${this.config.token}`;
  }

  // --- Connection status ---

  onStatus(cb: StatusCallback): () => void {
    this.statusCallbacks.add(cb);
    return () => this.statusCallbacks.delete(cb);
  }

  private notifyStatus(connected: boolean) {
    for (const cb of this.statusCallbacks) {
      cb(connected);
    }
  }

  // --- Streams (DAG state) ---

  connectStreams(): void {
    if (this.streamsWs) return;

    const url = `${this.baseUrl}/ws/streams?${this.authParam}`;
    const ws = new WebSocket(url);

    ws.onopen = () => {
      this.reconnectAttempt = 0;
      this.notifyStatus(true);
    };

    ws.onmessage = (event) => {
      try {
        const data: StreamEvent = JSON.parse(event.data);
        for (const cb of this.streamsCallbacks) {
          cb(data);
        }
      } catch {
        // ignore malformed messages
      }
    };

    ws.onclose = () => {
      this.streamsWs = null;
      this.notifyStatus(false);
      this.scheduleReconnect(() => this.connectStreams());
    };

    ws.onerror = () => {
      ws.close();
    };

    this.streamsWs = ws;
  }

  onStreams(cb: StreamsCallback): () => void {
    this.streamsCallbacks.add(cb);
    return () => this.streamsCallbacks.delete(cb);
  }

  // --- Pane terminal I/O ---

  connectPane(paneRef: string, paneType: 'agent' | 'terminal' = 'agent'): void {
    this.disconnectPane();
    this.activePaneRef = paneRef;
    this.activePaneType = paneType;

    const url = `${this.baseUrl}/ws/panes/${encodeURIComponent(paneRef)}?${this.authParam}&pane=${paneType}`;
    const ws = new WebSocket(url);

    ws.onmessage = (event) => {
      try {
        const data: PaneFrame = JSON.parse(event.data);
        for (const cb of this.paneCallbacks) {
          cb(data);
        }
      } catch {
        // ignore
      }
    };

    ws.onclose = () => {
      this.paneWs = null;
      // Auto-reconnect if we still have an active pane ref
      if (this.activePaneRef && !this.disposed) {
        this.scheduleReconnect(() => {
          if (this.activePaneRef) {
            this.connectPane(this.activePaneRef, this.activePaneType);
          }
        });
      }
    };

    ws.onerror = () => {
      ws.close();
    };

    this.paneWs = ws;
  }

  disconnectPane(): void {
    this.activePaneRef = null;
    this.activePaneType = 'agent';
    if (this.paneWs) {
      this.paneWs.close();
      this.paneWs = null;
    }
  }

  sendPaneInput(input: PaneInputMsg): void {
    if (this.paneWs?.readyState === WebSocket.OPEN) {
      this.paneWs.send(JSON.stringify(input));
    }
  }

  onPane(cb: PaneCallback): () => void {
    this.paneCallbacks.add(cb);
    return () => this.paneCallbacks.delete(cb);
  }

  // --- Relay protocol (send, spawn, kill, status) ---

  async relay(envelope: Envelope): Promise<Record<string, unknown>> {
    return new Promise((resolve, reject) => {
      const url = `${this.baseUrl}/ws/relay?${this.authParam}`;
      const ws = new WebSocket(url);

      ws.onopen = () => {
        ws.send(JSON.stringify(envelope));
      };

      ws.onmessage = (event) => {
        try {
          resolve(JSON.parse(event.data));
        } catch {
          reject(new Error('Invalid response'));
        }
        ws.close();
      };

      ws.onerror = () => {
        reject(new Error('Relay connection failed'));
        ws.close();
      };

      // Timeout after 10 seconds
      setTimeout(() => {
        reject(new Error('Relay timeout'));
        ws.close();
      }, 10000);
    });
  }

  // Convenience methods for common relay operations

  async getStatus(): Promise<AgentStatus[]> {
    const resp = await this.relay({
      action: 'status',
      payload: { agent_id: 'all' },
    });
    return resp as unknown as AgentStatus[];
  }

  async killAgent(agentId: string): Promise<{ killed: string[]; status: string }> {
    const resp = await this.relay({
      action: 'kill',
      payload: { agent_id: agentId },
    });
    return resp as unknown as { killed: string[]; status: string };
  }

  async sendMessage(from: string, to: string, content: string, type = 'task'): Promise<void> {
    await this.relay({
      action: 'send',
      payload: { from, to, type, content, time: new Date().toISOString() },
    });
  }

  async spawnAgent(opts: {
    name: string;
    repo?: string;
    agent?: string;
    task?: string;
    parentId?: string;
  }): Promise<{ agent_id: string; status: string }> {
    const resp = await this.relay({
      action: 'spawn',
      payload: {
        name: opts.name,
        repo: opts.repo ?? '',
        agent: opts.agent ?? 'claude',
        task: opts.task ?? '',
        parent_id: opts.parentId ?? '',
      },
    });
    return resp as unknown as { agent_id: string; status: string };
  }

  async fetchRepos(): Promise<{ name: string; path: string }[]> {
    const resp = await fetch(
      `http://${this.config.host}:${this.config.port}/api/repos?${this.authParam}`,
    );
    return resp.json();
  }

  async uploadImage(uri: string, filename: string): Promise<string> {
    const formData = new FormData();
    formData.append('image', {
      uri,
      name: filename,
      type: 'image/png',
    } as unknown as Blob);

    const resp = await fetch(
      `http://${this.config.host}:${this.config.port}/api/upload?${this.authParam}`,
      { method: 'POST', body: formData },
    );

    if (!resp.ok) {
      throw new Error(`Upload failed: ${resp.status}`);
    }

    const data = await resp.json();
    return data.path;
  }

  // --- Lifecycle ---

  private scheduleReconnect(fn: () => void): void {
    if (this.disposed) return;

    const delay = RECONNECT_DELAYS[Math.min(this.reconnectAttempt, RECONNECT_DELAYS.length - 1)];
    this.reconnectAttempt++;

    this.reconnectTimer = setTimeout(fn, delay);
  }

  dispose(): void {
    this.disposed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
    }
    this.streamsWs?.close();
    this.paneWs?.close();
    this.relayWs?.close();
    this.streamsCallbacks.clear();
    this.paneCallbacks.clear();
    this.statusCallbacks.clear();
  }
}

// --- Health check ---

export async function checkDaemonHealth(config: ConnectionConfig): Promise<boolean> {
  try {
    const resp = await fetch(`http://${config.host}:${config.port}/api/health`);
    const data = await resp.json();
    return data.status === 'ok';
  } catch {
    return false;
  }
}

// --- Parse grimoire:// URI from QR code ---

export function parseGrimoireUri(uri: string): ConnectionConfig | null {
  // grimoire://192.168.1.5:8077?token=abc123...
  const match = uri.match(/^grimoire:\/\/([^:]+):(\d+)\?token=(.+)$/);
  if (!match) return null;
  return {
    host: match[1],
    port: parseInt(match[2], 10),
    token: match[3],
  };
}
