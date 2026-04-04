// Types matching the daemon's wire protocol

export interface AgentStatus {
  id: string;
  status: string; // alive, idle, exited
  agent: string; // claude, amp, codex
  parent_id?: string;
  color?: string;
  shader?: string;
  session?: string;
  pane_id?: string;
}

export interface StreamEvent {
  type: 'snapshot' | 'agent_spawned' | 'agent_killed' | 'status_changed';
  data: AgentStatus[] | AgentStatus;
}

export interface PaneFrame {
  type: 'frame';
  content: string;
  cols: number;
  rows: number;
  scrolled: number; // -1 = full snapshot, 0 = in-place update, >0 = lines scrolled off top
}

export interface PaneInputMsg {
  type: 'input' | 'input_submit' | 'special' | 'resize';
  data: string;
  cols?: number;
  rows?: number;
}

export interface Envelope {
  action: 'send' | 'spawn' | 'status' | 'kill' | 'register' | 'unregister';
  payload: Record<string, unknown>;
}

export interface Skill {
  name: string;
  description: string;
  source: 'plugin' | 'project' | 'user';
  argument_hint?: string;
}

export interface ConnectionConfig {
  host: string;
  port: number;
  token: string;
}

// DAG tree node for display
export interface StreamNode {
  id: string;
  name: string;
  agent: string;
  status: string;
  color?: string;
  parentId?: string;
  paneId?: string;
  children: StreamNode[];
  depth: number;
}
