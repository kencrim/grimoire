import { useCallback, useMemo, forwardRef } from 'react';
import { Text, Pressable, StyleSheet, View } from 'react-native';
import {
  BottomSheetModal,
  BottomSheetBackdrop,
  BottomSheetView,
} from '@gorhom/bottom-sheet';
import type { BottomSheetBackdropProps } from '@gorhom/bottom-sheet';
import { hex } from '../lib/theme';

export interface ActionSheetItem {
  label: string;
  onPress: () => void;
  destructive?: boolean;
}

interface ActionSheetProps {
  title?: string;
  items: ActionSheetItem[];
  onDismiss?: () => void;
}

export const ActionSheet = forwardRef<BottomSheetModal, ActionSheetProps>(
  ({ title, items, onDismiss }, ref) => {
    const renderBackdrop = useCallback(
      (props: BottomSheetBackdropProps) => (
        <BottomSheetBackdrop
          {...props}
          disappearsOnIndex={-1}
          appearsOnIndex={0}
          pressBehavior="close"
        />
      ),
      [],
    );

    return (
      <BottomSheetModal
        ref={ref}
        enableDynamicSizing
        enablePanDownToClose
        backdropComponent={renderBackdrop}
        backgroundStyle={styles.background}
        handleIndicatorStyle={styles.handle}
        onDismiss={onDismiss}
      >
        <BottomSheetView style={styles.content}>
          {title && <Text style={styles.title}>{title}</Text>}
          {items.map((item, i) => (
            <Pressable
              key={i}
              style={({ pressed }) => [styles.item, pressed && styles.itemPressed]}
              onPress={item.onPress}
            >
              <Text
                style={[styles.itemText, item.destructive && styles.itemTextDestructive]}
              >
                {item.label}
              </Text>
            </Pressable>
          ))}
          <View style={styles.spacer} />
        </BottomSheetView>
      </BottomSheetModal>
    );
  },
);

const styles = StyleSheet.create({
  background: {
    backgroundColor: hex.mantle,
  },
  handle: {
    backgroundColor: hex.surface2,
  },
  content: {
    paddingHorizontal: 16,
    paddingBottom: 32,
  },
  title: {
    fontSize: 14,
    fontWeight: '600',
    color: hex.subtext0,
    textAlign: 'center',
    marginBottom: 12,
  },
  item: {
    paddingVertical: 14,
    paddingHorizontal: 16,
    borderRadius: 0,
  },
  itemPressed: {
    backgroundColor: hex.surface0,
  },
  itemText: {
    fontSize: 16,
    color: hex.text,
  },
  itemTextDestructive: {
    color: hex.red,
  },
  spacer: {
    height: 8,
  },
});
