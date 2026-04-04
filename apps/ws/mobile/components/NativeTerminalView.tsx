import { useRef, useState, useCallback, forwardRef, useImperativeHandle, memo } from 'react';
import {
  View,
  Text,
  StyleSheet,
} from 'react-native';
import { FlashList } from '@shopify/flash-list';
import { parseTerminalContent, type ParsedLine } from '../lib/ansi-native';
import type { PaneFrame } from '../lib/types';
import { catppuccin } from '../lib/theme';

const MAX_BUFFER_LINES = 5000;

export interface NativeTerminalHandle {
  pushFrame: (frame: PaneFrame) => void;
  clear: () => void;
}

/**
 * Merge a new frame into the existing line buffer based on the scroll indicator.
 */
function mergeFrame(buffer: string[], frame: PaneFrame): string[] {
  const frameLines = frame.content.split('\n');

  if (frame.scrolled === -1) {
    if (frame.rows === 0 || buffer.length === 0) {
      return frameLines;
    }
    const historyEnd = Math.max(0, buffer.length - frame.rows);
    const merged = buffer.slice(0, historyEnd).concat(frameLines);
    return merged.length > MAX_BUFFER_LINES
      ? merged.slice(merged.length - MAX_BUFFER_LINES)
      : merged;
  }

  if (frame.scrolled === 0) {
    const historyEnd = Math.max(0, buffer.length - frameLines.length);
    const merged = buffer.slice(0, historyEnd).concat(frameLines);
    return merged.length > MAX_BUFFER_LINES
      ? merged.slice(merged.length - MAX_BUFFER_LINES)
      : merged;
  }

  const newLines = frameLines.slice(frameLines.length - frame.scrolled);
  const merged = buffer.concat(newLines);
  return merged.length > MAX_BUFFER_LINES
    ? merged.slice(merged.length - MAX_BUFFER_LINES)
    : merged;
}

/**
 * Parse only the lines that changed between prev and next buffers.
 * Returns a new parsedLines array reusing unchanged ParsedLine references.
 */
function incrementalParse(
  prevBuffer: string[],
  nextBuffer: string[],
  prevParsed: ParsedLine[],
): ParsedLine[] {
  // Find where buffers diverge from the start
  const minLen = Math.min(prevBuffer.length, nextBuffer.length);
  let firstDiff = 0;
  while (firstDiff < minLen && prevBuffer[firstDiff] === nextBuffer[firstDiff]) {
    firstDiff++;
  }

  // If nothing changed and same length, return prev reference
  if (firstDiff === minLen && prevBuffer.length === nextBuffer.length) {
    return prevParsed;
  }

  // Reuse unchanged prefix, re-parse only the changed tail
  const reused = prevParsed.slice(0, firstDiff);
  const changedRaw = nextBuffer.slice(firstDiff).join('\n');
  const changedParsed = parseTerminalContent(changedRaw);
  return reused.concat(changedParsed);
}

export const NativeTerminalView = forwardRef<NativeTerminalHandle>(
  function NativeTerminalView(_props, ref) {
    const listRef = useRef<FlashList<ParsedLine>>(null);
    const bufferRef = useRef<string[]>([]);
    const prevBufferRef = useRef<string[]>([]);
    const userScrolledUpRef = useRef(false);
    const [parsedLines, setParsedLines] = useState<ParsedLine[]>([]);

    useImperativeHandle(ref, () => ({
      pushFrame(frame: PaneFrame) {
        const prev = bufferRef.current;
        const next = mergeFrame(prev, frame);
        bufferRef.current = next;
        if (!userScrolledUpRef.current) {
          const prevParsedSnapshot = prevBufferRef.current;
          prevBufferRef.current = next;
          setParsedLines(prevParsed =>
            incrementalParse(prevParsedSnapshot, next, prevParsed),
          );
          // Auto-scroll after React processes the update
          requestAnimationFrame(() => {
            if (!userScrolledUpRef.current) {
              listRef.current?.scrollToEnd({ animated: false });
            }
          });
        }
      },
      clear() {
        bufferRef.current = [];
        prevBufferRef.current = [];
        setParsedLines([]);
      },
    }), []);

    const flushBuffer = useCallback(() => {
      const content = bufferRef.current.join('\n');
      const parsed = parseTerminalContent(content);
      prevBufferRef.current = bufferRef.current;
      setParsedLines(parsed);
      requestAnimationFrame(() => {
        listRef.current?.scrollToEnd({ animated: false });
      });
    }, []);

    const handleScroll = useCallback(
      (event: { nativeEvent: { layoutMeasurement: { height: number }; contentOffset: { y: number }; contentSize: { height: number } } }) => {
        const { layoutMeasurement, contentOffset, contentSize } = event.nativeEvent;
        const distanceFromBottom =
          contentSize.height - contentOffset.y - layoutMeasurement.height;
        const wasScrolledUp = userScrolledUpRef.current;
        const isScrolledUp = distanceFromBottom > 40;
        userScrolledUpRef.current = isScrolledUp;

        if (wasScrolledUp && !isScrolledUp) {
          flushBuffer();
        }
      },
      [flushBuffer],
    );

    const renderItem = useCallback(
      ({ item }: { item: ParsedLine }) => <TerminalLine line={item} />,
      [],
    );

    return (
      <View style={styles.container}>
        <FlashList
          ref={listRef}
          data={parsedLines}
          renderItem={renderItem}
          estimatedItemSize={18}
          onScroll={handleScroll}
          scrollEventThrottle={32}
          showsVerticalScrollIndicator
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.scrollContent}
        />
      </View>
    );
  },
);

const TerminalLine = memo(function TerminalLine({ line }: { line: ParsedLine }) {
  if (line.spans.length === 0) {
    return <Text style={styles.line}>{'\n'}</Text>;
  }
  return (
    <Text style={styles.line}>
      {line.spans.map((span, i) => (
        <Text key={i} style={span.style}>
          {span.text}
        </Text>
      ))}
    </Text>
  );
});

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: catppuccin.base,
  },
  scrollContent: {
    paddingHorizontal: 6,
    paddingVertical: 4,
  },
  line: {
    fontFamily: 'Menlo',
    fontSize: 13,
    lineHeight: 18,
    color: catppuccin.text,
  },
});
