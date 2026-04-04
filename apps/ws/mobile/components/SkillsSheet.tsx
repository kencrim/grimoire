import { useCallback, useState } from 'react';
import { Text, View, Pressable, TextInput, StyleSheet } from 'react-native';
import {
  BottomSheetModal,
  BottomSheetBackdrop,
  BottomSheetFlatList,
} from '@gorhom/bottom-sheet';
import type { BottomSheetBackdropProps } from '@gorhom/bottom-sheet';
import { forwardRef } from 'react';
import { SymbolView } from 'expo-symbols';
import { hex } from '../lib/theme';
import { AnimatedIconButton } from './AnimatedIconButton';
import type { Skill } from '../lib/types';

const SOURCE_LABELS: Record<string, string> = {
  plugin: 'plugin',
  project: 'project',
  user: 'user',
};

interface SkillsSheetProps {
  skills: Skill[];
  onSelect: (skill: Skill, argument?: string) => void;
}

export const SkillsSheet = forwardRef<BottomSheetModal, SkillsSheetProps>(
  ({ skills, onSelect }, ref) => {
    const [activeSkill, setActiveSkill] = useState<Skill | null>(null);
    const [argument, setArgument] = useState('');

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

    const handleTap = useCallback((skill: Skill) => {
      if (skill.argument_hint) {
        setActiveSkill(skill);
        setArgument('');
      } else {
        onSelect(skill);
      }
    }, [onSelect]);

    const handleSubmitArgument = useCallback(() => {
      if (!activeSkill) return;
      onSelect(activeSkill, argument.trim());
      setActiveSkill(null);
      setArgument('');
    }, [activeSkill, argument, onSelect]);

    const handleBack = useCallback(() => {
      setActiveSkill(null);
      setArgument('');
    }, []);

    const handleDismiss = useCallback(() => {
      setActiveSkill(null);
      setArgument('');
    }, []);

    const renderItem = useCallback(
      ({ item }: { item: Skill }) => (
        <Pressable
          style={({ pressed }) => [styles.item, pressed && styles.itemPressed]}
          onPress={() => handleTap(item)}
        >
          <View style={styles.itemHeader}>
            <Text style={styles.itemName}>/{item.name}</Text>
            <View style={styles.itemMeta}>
              {item.argument_hint ? (
                <Text style={styles.itemArg}>{item.argument_hint}</Text>
              ) : null}
              <Text style={styles.itemSource}>{SOURCE_LABELS[item.source] ?? item.source}</Text>
            </View>
          </View>
          {item.description ? (
            <Text style={styles.itemDesc} numberOfLines={2}>
              {item.description}
            </Text>
          ) : null}
        </Pressable>
      ),
      [handleTap],
    );

    return (
      <BottomSheetModal
        ref={ref}
        snapPoints={['50%', '80%']}
        enablePanDownToClose
        backdropComponent={renderBackdrop}
        backgroundStyle={styles.background}
        handleIndicatorStyle={styles.handle}
        onDismiss={handleDismiss}
      >
        {activeSkill ? (
          <View style={styles.argContainer}>
            <View style={styles.argHeader}>
              <AnimatedIconButton onPress={handleBack} style={styles.backButton} pressScale={0.85}>
                <SymbolView name="chevron.left" size={16} tintColor={hex.accent} />
              </AnimatedIconButton>
              <View style={styles.argTitleWrap}>
                <Text style={styles.argTitle}>/{activeSkill.name}</Text>
                {activeSkill.description ? (
                  <Text style={styles.argDesc} numberOfLines={1}>{activeSkill.description}</Text>
                ) : null}
              </View>
            </View>
            <View style={styles.argInputRow}>
              <TextInput
                style={styles.argInput}
                value={argument}
                onChangeText={setArgument}
                placeholder={activeSkill.argument_hint ?? 'Enter argument...'}
                placeholderTextColor={hex.overlay0}
                autoCapitalize="none"
                autoCorrect={false}
                autoFocus
                returnKeyType="send"
                onSubmitEditing={handleSubmitArgument}
              />
              <AnimatedIconButton
                onPress={handleSubmitArgument}
                style={styles.argSend}
              >
                <SymbolView name="arrow.up" size={16} tintColor={hex.base} />
              </AnimatedIconButton>
            </View>
          </View>
        ) : (
          <>
            <View style={styles.header}>
              <Text style={styles.title}>Skills</Text>
              <Text style={styles.subtitle}>{skills.length} available</Text>
            </View>
            <BottomSheetFlatList
              data={skills}
              keyExtractor={(item) => `${item.source}-${item.name}`}
              renderItem={renderItem}
              contentContainerStyle={styles.list}
              initialNumToRender={15}
            />
          </>
        )}
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
  header: {
    paddingHorizontal: 20,
    paddingBottom: 12,
  },
  title: {
    fontSize: 18,
    fontWeight: '700',
    color: hex.text,
  },
  subtitle: {
    fontSize: 13,
    color: hex.overlay0,
    marginTop: 2,
  },
  list: {
    paddingHorizontal: 12,
    paddingBottom: 32,
  },
  item: {
    paddingVertical: 12,
    paddingHorizontal: 14,
    borderRadius: 0,
    marginBottom: 2,
  },
  itemPressed: {
    backgroundColor: hex.surface0,
  },
  itemHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  itemMeta: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  itemName: {
    fontSize: 16,
    fontWeight: '600',
    color: hex.accent,
    fontFamily: 'JetBrainsMono_400Regular',
    flexShrink: 1,
  },
  itemArg: {
    fontSize: 12,
    color: hex.yellow,
    fontFamily: 'JetBrainsMono_400Regular',
  },
  itemSource: {
    fontSize: 11,
    color: hex.overlay0,
    textTransform: 'uppercase',
  },
  itemDesc: {
    fontSize: 14,
    color: hex.subtext0,
    marginTop: 3,
    lineHeight: 19,
  },
  // Argument input view
  argContainer: {
    paddingHorizontal: 20,
    paddingBottom: 32,
  },
  argHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    marginBottom: 16,
    gap: 12,
  },
  backButton: {
    width: 32,
    height: 32,
    borderRadius: 16,
    borderCurve: 'continuous',
    backgroundColor: hex.surface0,
    alignItems: 'center',
    justifyContent: 'center',
  },
  argTitleWrap: {
    flex: 1,
  },
  argTitle: {
    fontSize: 18,
    fontWeight: '700',
    color: hex.accent,
    fontFamily: 'JetBrainsMono_400Regular',
  },
  argDesc: {
    fontSize: 13,
    color: hex.subtext0,
    marginTop: 2,
  },
  argInputRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
  },
  argInput: {
    flex: 1,
    backgroundColor: hex.surface0,
    color: hex.text,
    borderRadius: 0,
    paddingHorizontal: 14,
    paddingVertical: 12,
    fontSize: 16,
    fontFamily: 'JetBrainsMono_400Regular',
  },
  argSend: {
    width: 36,
    height: 36,
    borderRadius: 18,
    borderCurve: 'continuous',
    backgroundColor: hex.accent,
    alignItems: 'center',
    justifyContent: 'center',
  },
});
