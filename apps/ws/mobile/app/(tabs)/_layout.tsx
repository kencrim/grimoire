import { Tabs } from 'expo-router';
import { Redirect } from 'expo-router';
import FontAwesome from '@expo/vector-icons/FontAwesome';
import { useRelay } from '../_layout';
import { catppuccin } from '../../lib/theme';

export default function TabLayout() {
  const { connected, ready } = useRelay();

  // Redirect to connect screen if not connected (and done loading)
  if (ready && !connected) {
    return <Redirect href="/connect" />;
  }

  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: catppuccin.lavender,
        tabBarInactiveTintColor: catppuccin.overlay0,
        tabBarStyle: {
          backgroundColor: catppuccin.mantle,
          borderTopColor: catppuccin.surface0,
        },
        headerStyle: { backgroundColor: catppuccin.mantle },
        headerTintColor: catppuccin.text,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: 'Streams',
          tabBarIcon: ({ color }) => <FontAwesome name="sitemap" size={20} color={color} />,
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: 'Settings',
          tabBarIcon: ({ color }) => <FontAwesome name="cog" size={20} color={color} />,
        }}
      />
    </Tabs>
  );
}
