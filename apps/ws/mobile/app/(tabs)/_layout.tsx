import { Tabs } from 'expo-router';
import { Redirect } from 'expo-router';
import { SymbolView } from 'expo-symbols';
import { useRelay } from '../_layout';
import { hex } from '../../lib/theme';

export default function TabLayout() {
  const { connected, ready } = useRelay();

  // Redirect to connect screen if not connected (and done loading)
  if (ready && !connected) {
    return <Redirect href="/connect" />;
  }

  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: hex.accent,
        tabBarInactiveTintColor: hex.overlay0,
        tabBarStyle: {
          backgroundColor: hex.mantle,
          borderTopColor: hex.surface0,
        },
        headerStyle: { backgroundColor: hex.mantle },
        headerTintColor: hex.text,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: 'Streams',
          tabBarIcon: ({ color }) => <SymbolView name="square.grid.2x2" size={20} tintColor={color} />,
        }}
      />
      <Tabs.Screen
        name="settings"
        options={{
          title: 'Settings',
          tabBarIcon: ({ color }) => <SymbolView name="gearshape" size={20} tintColor={color} />,
        }}
      />
    </Tabs>
  );
}
