import { useCallback, useRef, useState } from 'react';
import { View, Pressable, StyleSheet, Animated } from 'react-native';
import { SymbolView } from 'expo-symbols';
import * as Haptics from 'expo-haptics';
import { PixelDaemon } from './PixelDaemon';
import { hex } from '../lib/theme';

interface FabAction {
  icon: 'daemon' | 'workstream';
  onPress: () => void;
}

interface ExpandingFabProps {
  actions: FabAction[];
}

export function ExpandingFab({ actions }: ExpandingFabProps) {
  const [open, setOpen] = useState(false);
  const animation = useRef(new Animated.Value(0)).current;

  const toggle = useCallback(() => {
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    const toValue = open ? 0 : 1;
    Animated.spring(animation, {
      toValue,
      useNativeDriver: true,
      friction: 6,
      tension: 100,
    }).start();
    setOpen(!open);
  }, [open, animation]);

  const handleAction = useCallback((action: FabAction) => {
    // Close first, then fire action
    Animated.spring(animation, {
      toValue: 0,
      useNativeDriver: true,
      friction: 6,
      tension: 100,
    }).start();
    setOpen(false);
    if (process.env.EXPO_OS === 'ios') Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    action.onPress();
  }, [animation]);

  // Main button: rotate 45° and scale down slightly when open
  const rotation = animation.interpolate({
    inputRange: [0, 1],
    outputRange: ['0deg', '45deg'],
  });
  const scale = animation.interpolate({
    inputRange: [0, 1],
    outputRange: [1, 0.85],
  });

  // Backdrop fade
  const backdropOpacity = animation.interpolate({
    inputRange: [0, 1],
    outputRange: [0, 0.4],
  });

  return (
    <>
      {/* Backdrop */}
      {open && (
        <Animated.View
          style={[StyleSheet.absoluteFill, { backgroundColor: '#000', opacity: backdropOpacity }]}
        >
          <Pressable style={StyleSheet.absoluteFill} onPress={toggle} />
        </Animated.View>
      )}

      <View style={styles.container} pointerEvents="box-none">
        {/* Sub-buttons — stacked above the main FAB */}
        {actions.map((action, i) => {
          const offset = (i + 1) * 64;
          const translateY = animation.interpolate({
            inputRange: [0, 1],
            outputRange: [0, -offset],
          });
          const itemScale = animation.interpolate({
            inputRange: [0, 0.5, 1],
            outputRange: [0, 0, 1],
          });
          const itemOpacity = animation.interpolate({
            inputRange: [0, 0.4, 1],
            outputRange: [0, 0, 1],
          });

          return (
            <Animated.View
              key={i}
              style={[
                styles.subButton,
                {
                  transform: [{ translateY }, { scale: itemScale }],
                  opacity: itemOpacity,
                },
              ]}
            >
              <Pressable
                style={styles.subButtonInner}
                onPress={() => handleAction(action)}
              >
                {action.icon === 'daemon' ? (
                  <PixelDaemon size={20} color={hex.base} />
                ) : (
                  <SymbolView name="text.badge.plus" size={20} tintColor={hex.base} />
                )}
              </Pressable>
            </Animated.View>
          );
        })}

        {/* Main FAB */}
        <Pressable onPress={toggle}>
          <Animated.View
            style={[
              styles.fab,
              { transform: [{ rotate: rotation }, { scale }] },
            ]}
          >
            <SymbolView name="plus" size={24} tintColor={hex.base} />
          </Animated.View>
        </Pressable>
      </View>
    </>
  );
}

const styles = StyleSheet.create({
  container: {
    position: 'absolute',
    bottom: 24,
    right: 24,
    alignItems: 'center',
  },
  fab: {
    width: 56,
    height: 56,
    borderRadius: 28,
    borderCurve: 'continuous',
    backgroundColor: hex.accent,
    alignItems: 'center',
    justifyContent: 'center',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.3,
    shadowRadius: 6,
    elevation: 8,
  },
  subButton: {
    position: 'absolute',
    bottom: 0,
  },
  subButtonInner: {
    width: 44,
    height: 44,
    borderRadius: 22,
    borderCurve: 'continuous',
    backgroundColor: hex.surface2,
    alignItems: 'center',
    justifyContent: 'center',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.25,
    shadowRadius: 4,
    elevation: 6,
  },
});
