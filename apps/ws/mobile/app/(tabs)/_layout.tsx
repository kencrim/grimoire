import { Tabs } from 'expo-router';
import { SymbolView } from 'expo-symbols';
import { hex } from '../../lib/theme';
import { PixelDaemon } from '../../components/PixelDaemon';

export default function TabLayout() {
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
          title: 'Daemons',
          tabBarIcon: ({ color }) => <PixelDaemon size={22} color={color} />,
        }}
      />
      <Tabs.Screen
        name="manage"
        options={{
          title: 'Manage',
          tabBarIcon: ({ color }) => <SymbolView name="plus.circle" size={20} tintColor={color} />,
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
