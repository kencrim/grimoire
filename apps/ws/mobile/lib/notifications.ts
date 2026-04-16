import * as Notifications from 'expo-notifications';
import Constants from 'expo-constants';
import { Platform } from 'react-native';
import type { StreamEvent, AgentStatus, ConnectionConfig } from './types';

// Configure foreground notification behavior — show alerts even when
// the app is open so the user sees when any agent finishes.
try {
  Notifications.setNotificationHandler({
    handleNotification: async () => ({
      shouldShowBanner: true,
      shouldShowList: true,
      shouldPlaySound: true,
      shouldSetBadge: false,
    }),
  });
} catch (e) {
  console.warn('[notifications] Failed to set handler — native module may not be linked:', e);
}

export async function requestNotificationPermissions(): Promise<boolean> {
  if (Platform.OS === 'web') return false;

  const { status: existing } = await Notifications.getPermissionsAsync();
  if (existing === 'granted') return true;

  const { status } = await Notifications.requestPermissionsAsync();
  return status === 'granted';
}

// Returns the handler for notification taps. The caller should wire this
// into the router to navigate to the appropriate stream screen.
export function addNotificationResponseListener(
  onTap: (agentId: string) => void,
): Notifications.Subscription {
  return Notifications.addNotificationResponseReceivedListener((response) => {
    const agentId = response.notification.request.content.data?.agentId;
    if (typeof agentId === 'string') {
      onTap(agentId);
    }
  });
}

// Fire a local notification for relevant agent events.
// Only notifies for status transitions to "idle" (agent finished thinking).
export function notifyAgentEvent(event: StreamEvent): void {
  if (event.type !== 'status_changed' || Array.isArray(event.data)) return;

  const agent = event.data as AgentStatus;
  if (agent.status !== 'idle') return;

  const name = agent.id.includes('/') ? agent.id.split('/').pop() : agent.id;
  const agentLabel = agent.agent ? ` (${agent.agent})` : '';

  Notifications.scheduleNotificationAsync({
    content: {
      title: `${name} is ready`,
      body: `${agent.id}${agentLabel} is waiting for input`,
      data: { agentId: agent.id },
    },
    trigger: null, // fire immediately
  });
}

// Register Expo push token with the daemon so it can send remote
// notifications even when the app is closed.
export async function registerPushToken(config: ConnectionConfig): Promise<void> {
  if (Platform.OS === 'web') return;

  const granted = await requestNotificationPermissions();
  console.log('[push] permission granted:', granted);
  if (!granted) return;

  const projectId =
    Constants.expoConfig?.extra?.eas?.projectId ?? '406ce5c5-a435-4cd8-8db6-1cea79c52c4d';
  console.log('[push] requesting token with projectId:', projectId);
  const tokenData = await Notifications.getExpoPushTokenAsync({ projectId });
  const token = tokenData.data;
  console.log('[push] got token:', token);

  const url = `http://${config.host}:${config.port}/api/push-token?token=${config.token}`;
  console.log('[push] registering with daemon at:', `${config.host}:${config.port}`);
  const resp = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  });
  console.log('[push] daemon responded:', resp.status);
}
