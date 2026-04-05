import { View, Text, StyleSheet, RefreshControl, ActionSheetIOS } from 'react-native';
import { FlashList } from '@shopify/flash-list';
import { router } from 'expo-router';
import { useDaemons } from '../_layout';
import { hex } from '../../lib/theme';
import { AnimatedIconButton } from '../../components/AnimatedIconButton';
import { StreamTreeItem } from '../../components/StreamTree';
import { DaemonSectionHeader } from '../../components/DaemonSectionHeader';
import { ExpandingFab } from '../../components/ExpandingFab';
import type { AgentStatus, StreamNode } from '../../lib/types';
import { useCallback, useMemo, useState } from 'react';

type ListItem =
  | { type: 'header'; daemonId: string; name: string; host: string; port: number; connected: boolean; agentCount: number }
  | { type: 'agent'; node: StreamNode; daemonId: string; isLast: boolean };

export default function DaemonsScreen() {
  const { daemons, daemonOrder, refreshAgents, resolveAgent } = useDaemons();
  const [refreshing, setRefreshing] = useState(false);
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  const toggleCollapse = useCallback((daemonId: string) => {
    setCollapsed(prev => {
      const next = new Set(prev);
      if (next.has(daemonId)) {
        next.delete(daemonId);
      } else {
        next.add(daemonId);
      }
      return next;
    });
  }, []);

  // Build a flat list with section headers and agent nodes
  const listItems = useMemo(() => {
    const items: ListItem[] = [];
    for (const id of daemonOrder) {
      const state = daemons.get(id);
      if (!state) continue;

      items.push({
        type: 'header',
        daemonId: id,
        name: state.entry.name,
        host: state.entry.config.host,
        port: state.entry.config.port,
        connected: state.connected,
        agentCount: state.agents.length,
      });

      if (!collapsed.has(id)) {
        const nodes = flattenTree(state.agents, id);
        for (let i = 0; i < nodes.length; i++) {
          const node = nodes[i];
          // "last" = no subsequent sibling at the same depth before a shallower node
          let isLast = true;
          for (let j = i + 1; j < nodes.length; j++) {
            if (nodes[j].depth === node.depth) { isLast = false; break; }
            if (nodes[j].depth < node.depth) break;
          }
          items.push({ type: 'agent', node, daemonId: id, isLast });
        }
      }
    }
    return items;
  }, [daemons, daemonOrder, collapsed]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await refreshAgents();
    setRefreshing(false);
  }, [refreshAgents]);

  const handleKill = useCallback((qualifiedId: string) => {
    const resolved = resolveAgent(qualifiedId);
    if (resolved) {
      resolved.daemon.client?.killAgent(resolved.agent.id);
    }
  }, [resolveAgent]);

  const handleNewWorkstream = useCallback(() => {
    const connectedDaemons = daemonOrder
      .map(id => daemons.get(id)!)
      .filter(d => d?.connected);

    if (connectedDaemons.length === 0) {
      router.push('/(tabs)/manage');
      return;
    }
    if (connectedDaemons.length === 1) {
      router.push(`/create?daemonId=${connectedDaemons[0].entry.id}`);
      return;
    }

    // Multiple connected — let user pick
    const options = [...connectedDaemons.map(d => d.entry.name), 'Cancel'];
    ActionSheetIOS.showActionSheetWithOptions(
      {
        options,
        cancelButtonIndex: options.length - 1,
        title: 'Create workstream on...',
      },
      (index) => {
        if (index < connectedDaemons.length) {
          router.push(`/create?daemonId=${connectedDaemons[index].entry.id}`);
        }
      },
    );
  }, [daemons, daemonOrder]);

  const handleManageDaemons = useCallback(() => {
    router.push('/(tabs)/manage');
  }, []);

  return (
    <View style={styles.container}>
      {daemonOrder.length === 0 ? (
        <View style={styles.empty}>
          <Text style={styles.emptyTitle}>No daemons</Text>
          <Text style={styles.emptySubtitle}>
            Add a daemon from the Manage tab to get started.
          </Text>
          <AnimatedIconButton
            style={styles.emptyButton}
            onPress={() => router.push('/(tabs)/manage')}
            pressScale={0.97}
          >
            <Text style={styles.emptyButtonText}>Add Daemon</Text>
          </AnimatedIconButton>
        </View>
      ) : (
        <>
          <FlashList
            data={listItems}
            renderItem={({ item }) => {
              if (item.type === 'header') {
                return (
                  <DaemonSectionHeader
                    name={item.name}
                    host={item.host}
                    port={item.port}
                    connected={item.connected}
                    agentCount={item.agentCount}
                    collapsed={collapsed.has(item.daemonId)}
                    onToggle={() => toggleCollapse(item.daemonId)}
                  />
                );
              }
              return (
                <StreamTreeItem
                  node={item.node}
                  isLast={item.isLast}
                  onKill={handleKill}
                />
              );
            }}
            getItemType={(item) => item.type}
            estimatedItemSize={56}
            keyExtractor={(item) =>
              item.type === 'header' ? `header-${item.daemonId}` : item.node.id
            }
            refreshControl={
              <RefreshControl
                refreshing={refreshing}
                onRefresh={onRefresh}
                tintColor={hex.accent}
              />
            }
            contentContainerStyle={styles.listContent}
          />
          <ExpandingFab
            actions={[
              { icon: 'workstream', onPress: handleNewWorkstream },
              { icon: 'daemon', onPress: handleManageDaemons },
            ]}
          />
        </>
      )}
    </View>
  );
}

// Convert flat agent list into tree-ordered flat list with depth info
function flattenTree(agents: AgentStatus[], daemonId: string): StreamNode[] {
  const byId = new Map<string, AgentStatus>();
  for (const a of agents) {
    byId.set(a.id, a);
  }

  const roots = agents.filter((a) => !a.parent_id || !byId.has(a.parent_id));
  const result: StreamNode[] = [];

  function walk(agent: AgentStatus, depth: number) {
    const name = agent.id.includes('/') ? agent.id.split('/').pop()! : agent.id;
    result.push({
      id: `${daemonId}::${agent.id}`,
      name,
      agent: agent.agent,
      status: agent.status,
      color: agent.color,
      parentId: agent.parent_id,
      paneId: agent.pane_id,
      children: [],
      depth,
    });

    const children = agents.filter((a) => a.parent_id === agent.id);
    for (const child of children) {
      walk(child, depth + 1);
    }
  }

  for (const root of roots) {
    walk(root, 0);
  }

  return result;
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: hex.base,
  },
  listContent: {
    padding: 16,
  },
  empty: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
    padding: 24,
  },
  emptyTitle: {
    fontSize: 20,
    fontFamily: 'SpaceGrotesk_700Bold',
    color: hex.text,
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 14,
    color: hex.subtext0,
    textAlign: 'center',
    marginBottom: 24,
  },
  emptyButton: {
    backgroundColor: hex.accent,
    paddingHorizontal: 24,
    paddingVertical: 12,
    borderRadius: 0,
  },
  emptyButtonText: {
    color: hex.base,
    fontSize: 16,
    fontFamily: 'SpaceGrotesk_600SemiBold',
  },
});
