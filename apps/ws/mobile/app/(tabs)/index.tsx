import { View, StyleSheet, RefreshControl } from 'react-native';
import { FlashList } from '@shopify/flash-list';
import { useRelay } from '../_layout';
import { catppuccin } from '../../lib/theme';
import { StreamTreeItem } from '../../components/StreamTree';
import { ConnectionBanner } from '../../components/ConnectionBanner';
import type { AgentStatus, StreamNode } from '../../lib/types';
import { useCallback, useMemo, useState } from 'react';

export default function StreamsScreen() {
  const { agents, connected, config, refreshAgents } = useRelay();
  const [refreshing, setRefreshing] = useState(false);

  // Build tree from flat agent list
  const flatNodes = useMemo(() => flattenTree(agents), [agents]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await refreshAgents();
    setRefreshing(false);
  }, [refreshAgents]);

  return (
    <View style={styles.container}>
      <ConnectionBanner connected={connected} host={config?.host} port={config?.port} />
      <FlashList
        data={flatNodes}
        renderItem={({ item }) => <StreamTreeItem node={item} />}
        estimatedItemSize={56}
        keyExtractor={(item) => item.id}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={onRefresh}
            tintColor={catppuccin.lavender}
          />
        }
        contentContainerStyle={styles.listContent}
      />
    </View>
  );
}

// Convert flat agent list into tree-ordered flat list with depth info
function flattenTree(agents: AgentStatus[]): StreamNode[] {
  const byId = new Map<string, AgentStatus>();
  for (const a of agents) {
    byId.set(a.id, a);
  }

  // Find roots (no parent or parent not in list)
  const roots = agents.filter((a) => !a.parent_id || !byId.has(a.parent_id));

  const result: StreamNode[] = [];

  function walk(agent: AgentStatus, depth: number) {
    const name = agent.id.includes('/') ? agent.id.split('/').pop()! : agent.id;
    result.push({
      id: agent.id,
      name,
      agent: agent.agent,
      status: agent.status,
      color: agent.color,
      parentId: agent.parent_id,
      paneId: agent.pane_id,
      children: [],
      depth,
    });

    // Find children
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
    backgroundColor: catppuccin.base,
  },
  listContent: {
    padding: 16,
  },
});
