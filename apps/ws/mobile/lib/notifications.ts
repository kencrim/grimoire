import type { StreamEvent } from './types';

// Notifications are disabled — expo-notifications requires a paid Apple Developer account
// for the push entitlement. Local notifications can be re-enabled once we have one.

export async function requestNotificationPermissions(): Promise<boolean> {
  return false;
}

export function notifyAgentEvent(_event: StreamEvent): void {
  // no-op
}
