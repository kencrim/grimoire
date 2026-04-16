import { useEffect } from 'react';
import { Stack, router } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { BottomSheetModalProvider } from '@gorhom/bottom-sheet';
import { Toaster } from 'sonner-native';
import { useFonts } from 'expo-font';
import {
  SpaceGrotesk_400Regular,
  SpaceGrotesk_600SemiBold,
  SpaceGrotesk_700Bold,
} from '@expo-google-fonts/space-grotesk';
import {
  JetBrainsMono_400Regular,
  JetBrainsMono_500Medium,
} from '@expo-google-fonts/jetbrains-mono';
import { DaemonManagerProvider, useDaemons } from '../lib/daemon-context';
import { requestNotificationPermissions, addNotificationResponseListener } from '../lib/notifications';
import { hex } from '../lib/theme';

export { useDaemons } from '../lib/daemon-context';

/** Listens for notification taps and navigates to the agent's stream screen. */
function NotificationNavigator() {
  const { allAgents } = useDaemons();

  useEffect(() => {
    // Request permissions early so the token is ready for daemon connections
    requestNotificationPermissions()
      .then((granted) => console.log('[notifications] permission granted:', granted))
      .catch((err) => console.error('[notifications] permission error:', err));

    const sub = addNotificationResponseListener((agentId) => {
      // Find the qualified ID (daemonId::agentId) for this agent
      const match = allAgents.find(a => a.id === agentId);
      if (match) {
        router.push(`/stream/${match.qualifiedId}`);
      }
    });
    return () => sub.remove();
  }, [allAgents]);

  return null;
}

export default function RootLayout() {
  const [fontsLoaded] = useFonts({
    SpaceGrotesk_400Regular,
    SpaceGrotesk_600SemiBold,
    SpaceGrotesk_700Bold,
    JetBrainsMono_400Regular,
    JetBrainsMono_500Medium,
  });

  if (!fontsLoaded) return null;

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <BottomSheetModalProvider>
        <DaemonManagerProvider>
          <NotificationNavigator />
          <StatusBar style="light" />
          <Stack
            initialRouteName="(tabs)"
            screenOptions={{
              headerStyle: { backgroundColor: hex.mantle },
              headerTintColor: hex.text,
              headerTitleStyle: { fontFamily: 'SpaceGrotesk_600SemiBold' },
              contentStyle: { backgroundColor: hex.base },
            }}
          >
            <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
            <Stack.Screen
              name="stream/[id]"
              options={{
                title: 'Terminal',
                headerBackTitle: 'Back',
              }}
            />
            <Stack.Screen
              name="create"
              options={{
                title: 'New Workstream',
                presentation: 'modal',
                headerBackTitle: 'Cancel',
              }}
            />
          </Stack>
        </DaemonManagerProvider>
        <Toaster
          position="top-center"
          duration={4000}
          visibleToasts={3}
          swipeToDismissDirection="up"
          gap={8}
          theme="dark"
          toastOptions={{
            style: {
              backgroundColor: hex.surface0,
              borderColor: hex.surface2,
              borderWidth: 1,
            },
            titleStyle: {
              fontFamily: 'SpaceGrotesk_600SemiBold',
              color: hex.text,
              fontSize: 14,
            },
            descriptionStyle: {
              fontFamily: 'SpaceGrotesk_400Regular',
              color: hex.subtext1,
              fontSize: 13,
            },
          }}
        />
      </BottomSheetModalProvider>
    </GestureHandlerRootView>
  );
}
