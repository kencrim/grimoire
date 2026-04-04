import type { AgentStatus } from './types';

/**
 * Merge an incoming agent list with existing state.
 * The incoming list is authoritative for which agents exist (removals are honoured),
 * but optional display fields (color, shader) are preserved from existing state
 * when missing in the incoming data.
 */
export function mergeAgentList(prev: AgentStatus[], incoming: AgentStatus[]): AgentStatus[] {
  const prevById = new Map(prev.map(a => [a.id, a]));
  return incoming.map(fresh => {
    const existing = prevById.get(fresh.id);
    if (!existing) return fresh;
    return {
      ...existing,
      ...fresh,
      color: fresh.color ?? existing.color,
      shader: fresh.shader ?? existing.shader,
    };
  });
}
