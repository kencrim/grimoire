import { View, StyleSheet } from 'react-native';

interface PixelDaemonProps {
  size?: number;
  color?: string;
}

// 9x9 pixel grid for a Space Invaders-style daemon
// 1 = filled, 0 = empty
const GRID = [
  [0, 0, 1, 0, 0, 0, 1, 0, 0],
  [0, 0, 0, 1, 0, 1, 0, 0, 0],
  [0, 0, 1, 1, 1, 1, 1, 0, 0],
  [0, 1, 1, 0, 1, 0, 1, 1, 0],
  [1, 1, 1, 1, 1, 1, 1, 1, 1],
  [1, 0, 1, 1, 1, 1, 1, 0, 1],
  [1, 0, 1, 0, 0, 0, 1, 0, 1],
  [0, 0, 0, 1, 0, 1, 0, 0, 0],
];

export function PixelDaemon({ size = 20, color = '#cdd6f4' }: PixelDaemonProps) {
  const px = size / GRID[0].length;

  return (
    <View style={[styles.grid, { width: size, height: px * GRID.length }]}>
      {GRID.map((row, y) =>
        row.map((cell, x) =>
          cell ? (
            <View
              key={`${x}-${y}`}
              style={{
                position: 'absolute',
                left: x * px,
                top: y * px,
                width: px,
                height: px,
                backgroundColor: color,
              }}
            />
          ) : null,
        ),
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  grid: {
    position: 'relative',
  },
});
