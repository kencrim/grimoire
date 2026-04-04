import { type ReactNode, useCallback } from 'react';
import { Pressable, type ViewStyle } from 'react-native';
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
} from 'react-native-reanimated';

const ReanimatedPressable = Animated.createAnimatedComponent(Pressable);

const PRESS_SPRING = { damping: 15, stiffness: 400, mass: 0.3 };
const RELEASE_SPRING = { damping: 12, stiffness: 200, mass: 0.3 };

interface AnimatedIconButtonProps {
  onPress: () => void;
  onLongPress?: () => void;
  style?: ViewStyle | readonly (ViewStyle | false | undefined)[];
  disabled?: boolean;
  hitSlop?: number;
  /** Press-in scale factor. Defaults to 0.82 for small buttons; use ~0.97 for large ones. */
  pressScale?: number;
  children: ReactNode;
}

export function AnimatedIconButton({
  onPress,
  onLongPress,
  style,
  disabled,
  hitSlop,
  pressScale = 0.82,
  children,
}: AnimatedIconButtonProps) {
  const scale = useSharedValue(1);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }));

  const handlePressIn = useCallback(() => {
    scale.value = withSpring(pressScale, PRESS_SPRING);
  }, [scale, pressScale]);

  const handlePressOut = useCallback(() => {
    scale.value = withSpring(1, RELEASE_SPRING);
  }, [scale]);

  return (
    <ReanimatedPressable
      onPress={onPress}
      onLongPress={onLongPress}
      onPressIn={handlePressIn}
      onPressOut={handlePressOut}
      style={[animatedStyle, style]}
      disabled={disabled}
      hitSlop={hitSlop}
    >
      {children}
    </ReanimatedPressable>
  );
}
