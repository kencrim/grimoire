import { useState, useEffect, useCallback } from 'react';
import {
  View,
  Text,
  TextInput,
  StyleSheet,
  ScrollView,
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
} from 'react-native';
import { Stack, router } from 'expo-router';
import * as Haptics from 'expo-haptics';
import { useRelay } from './_layout';
import { hex } from '../lib/theme';
import { AnimatedIconButton } from '../components/AnimatedIconButton';

const AGENTS = ['claude', 'amp', 'codex'] as const;

interface RepoEntry {
  name: string;
  path: string;
}

export default function CreateScreen() {
  const { client } = useRelay();
  const [name, setName] = useState('');
  const [repos, setRepos] = useState<RepoEntry[]>([]);
  const [selectedRepo, setSelectedRepo] = useState<string>('');
  const [selectedAgent, setSelectedAgent] = useState<string>('claude');
  const [task, setTask] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadingRepos, setLoadingRepos] = useState(true);

  useEffect(() => {
    if (!client) return;
    client
      .fetchRepos()
      .then((data) => {
        setRepos(data);
        if (data.length === 1) {
          setSelectedRepo(data[0].name);
        }
      })
      .catch(() => {
        setRepos([]);
      })
      .finally(() => setLoadingRepos(false));
  }, [client]);

  const handleCreate = useCallback(async () => {
    if (!client || !name.trim()) return;

    setLoading(true);
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);

    try {
      await client.spawnAgent({
        name: name.trim(),
        repo: selectedRepo || undefined,
        agent: selectedAgent,
        task: task.trim() || undefined,
      });
      router.back();
    } catch (err) {
      Alert.alert('Failed to create workstream', String(err));
    } finally {
      setLoading(false);
    }
  }, [client, name, selectedRepo, selectedAgent, task]);

  const canCreate = name.trim().length > 0 && !loading;

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={process.env.EXPO_OS === 'ios' ? 'padding' : undefined}
    >
      <Stack.Screen
        options={{
          title: 'New Workstream',
          presentation: 'modal',
          headerRight: () => (
            <AnimatedIconButton
              onPress={handleCreate}
              disabled={!canCreate}
              style={[styles.createPressable, !canCreate && styles.createButtonDisabled]}
              pressScale={0.92}
            >
              {loading ? (
                <ActivityIndicator size="small" color={hex.accent} />
              ) : (
                <Text style={styles.createButton}>Create</Text>
              )}
            </AnimatedIconButton>
          ),
        }}
      />

      <ScrollView
        contentContainerStyle={styles.form}
        keyboardShouldPersistTaps="handled"
      >
        {/* Name */}
        <Text style={styles.label}>Name</Text>
        <TextInput
          style={styles.input}
          value={name}
          onChangeText={setName}
          placeholder="e.g. auth-refactor"
          placeholderTextColor={hex.overlay0}
          autoCapitalize="none"
          autoCorrect={false}
          autoFocus
        />

        {/* Repo */}
        <Text style={styles.label}>Repository</Text>
        {loadingRepos ? (
          <ActivityIndicator
            size="small"
            color={hex.accent}
            style={styles.repoLoader}
          />
        ) : repos.length === 0 ? (
          <Text style={styles.hint}>
            No repos registered. Run: ws repo add {'<name>'} {'<path>'}
          </Text>
        ) : (
          <View style={styles.chipRow}>
            {repos.map((repo) => (
              <AnimatedIconButton
                key={repo.name}
                style={[
                  styles.chip,
                  selectedRepo === repo.name && styles.chipSelected,
                ]}
                onPress={() => setSelectedRepo(repo.name)}
                pressScale={0.92}
              >
                <Text
                  style={[
                    styles.chipText,
                    selectedRepo === repo.name && styles.chipTextSelected,
                  ]}
                >
                  {repo.name}
                </Text>
              </AnimatedIconButton>
            ))}
          </View>
        )}

        {/* Agent */}
        <Text style={styles.label}>Agent</Text>
        <View style={styles.chipRow}>
          {AGENTS.map((agent) => (
            <AnimatedIconButton
              key={agent}
              style={[
                styles.chip,
                selectedAgent === agent && styles.chipSelected,
              ]}
              onPress={() => setSelectedAgent(agent)}
              pressScale={0.92}
            >
              <Text
                style={[
                  styles.chipText,
                  selectedAgent === agent && styles.chipTextSelected,
                ]}
              >
                {agent}
              </Text>
            </AnimatedIconButton>
          ))}
        </View>

        {/* Task */}
        <Text style={styles.label}>Initial Task (optional)</Text>
        <TextInput
          style={[styles.input, styles.taskInput]}
          value={task}
          onChangeText={setTask}
          placeholder="Describe what the agent should work on..."
          placeholderTextColor={hex.overlay0}
          multiline
          textAlignVertical="top"
        />
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: hex.base,
  },
  form: {
    padding: 20,
    gap: 8,
  },
  label: {
    fontSize: 13,
    fontFamily: 'SpaceGrotesk_600SemiBold',
    color: hex.subtext0,
    textTransform: 'uppercase',
    letterSpacing: 0.5,
    marginTop: 16,
    marginBottom: 4,
  },
  input: {
    backgroundColor: hex.surface0,
    color: hex.text,
    borderRadius: 0,
    paddingHorizontal: 14,
    paddingVertical: 12,
    fontSize: 16,
    fontFamily: 'JetBrainsMono_400Regular',
  },
  taskInput: {
    minHeight: 100,
    fontFamily: undefined,
  },
  chipRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  chip: {
    borderWidth: 1,
    borderColor: hex.surface2,
    borderRadius: 0,
    paddingHorizontal: 14,
    paddingVertical: 8,
  },
  chipSelected: {
    borderColor: hex.accent,
    backgroundColor: hex.accent + '20',
  },
  chipText: {
    fontSize: 14,
    fontWeight: '500',
    color: hex.overlay1,
  },
  chipTextSelected: {
    color: hex.accent,
  },
  hint: {
    fontSize: 13,
    color: hex.overlay0,
    fontStyle: 'italic',
  },
  repoLoader: {
    alignSelf: 'flex-start',
    marginVertical: 8,
  },
  createPressable: {
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  createButton: {
    fontSize: 16,
    fontWeight: '600',
    color: hex.accent,
  },
  createButtonDisabled: {
    opacity: 0.4,
  },
});
