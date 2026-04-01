import { useRef, useState, useCallback, useEffect } from 'react';
import {
  View,
  Text,
  ScrollView,
  StyleSheet,
  NativeSyntheticEvent,
  NativeScrollEvent,
} from 'react-native';
import { parseTerminalContent, type ParsedLine } from '../lib/ansi-native';
import type { PaneFrame } from '../lib/types';
import { catppuccin } from '../lib/theme';

const MAX_BUFFER_LINES = 5000;

interface NativeTerminalViewProps {
  frame: PaneFrame | null;
}

/**
 * Merge a new frame into the existing line buffer based on the scroll indicator.
 *
 * The server sends two kinds of scrolled=-1 frames:
 *  1. The initial history snapshot (capture-pane -S -1000) which has rows=0
 *     because the StreamTo method doesn't set Cols/Rows.
 *  2. Subsequent "line-count mismatch" frames where the server couldn't detect
 *     scroll direction — these have rows>0 and are just the visible pane.
 *
 *  scrolled === -1, rows === 0  → initial history: replace everything
 *  scrolled === -1, rows  > 0  → visible-pane refresh: preserve history
 *  scrolled === 0               → in-place update: replace visible portion
 *  scrolled  > 0                → N new lines scrolled in: append to history
 */
function mergeFrame(buffer: string[], frame: PaneFrame): string[] {
  const frameLines = frame.content.split('\n');

  if (frame.scrolled === -1) {
    if (frame.rows === 0 || buffer.length === 0) {
      // Initial history snapshot or very first frame — replace everything
      return frameLines;
    }
    // Visible-pane-only refresh — preserve history, replace last rows lines
    const historyEnd = Math.max(0, buffer.length - frame.rows);
    const merged = buffer.slice(0, historyEnd).concat(frameLines);
    return merged.length > MAX_BUFFER_LINES
      ? merged.slice(merged.length - MAX_BUFFER_LINES)
      : merged;
  }

  if (frame.scrolled === 0) {
    // Replace the last visible chunk (same size as the incoming frame)
    const historyEnd = Math.max(0, buffer.length - frameLines.length);
    const merged = buffer.slice(0, historyEnd).concat(frameLines);
    return merged.length > MAX_BUFFER_LINES
      ? merged.slice(merged.length - MAX_BUFFER_LINES)
      : merged;
  }

  // scrolled > 0: new lines appeared at the bottom
  const newLines = frameLines.slice(frameLines.length - frame.scrolled);
  const merged = buffer.concat(newLines);
  return merged.length > MAX_BUFFER_LINES
    ? merged.slice(merged.length - MAX_BUFFER_LINES)
    : merged;
}

export function NativeTerminalView({ frame }: NativeTerminalViewProps) {
  const scrollRef = useRef<ScrollView>(null);
  const bufferRef = useRef<string[]>([]);
  const userScrolledUpRef = useRef(false);
  const [parsedLines, setParsedLines] = useState<ParsedLine[]>([]);

  // Process every frame into the buffer; only re-render when not scrolled up.
  useEffect(() => {
    if (!frame) return;

    bufferRef.current = mergeFrame(bufferRef.current, frame);

    if (!userScrolledUpRef.current) {
      setParsedLines(parseTerminalContent(bufferRef.current.join('\n')));
    }
  }, [frame]);

  // When user scrolls back to bottom, render the latest buffer state.
  const flushBuffer = useCallback(() => {
    setParsedLines(parseTerminalContent(bufferRef.current.join('\n')));
    requestAnimationFrame(() => {
      scrollRef.current?.scrollToEnd({ animated: false });
    });
  }, []);

  const handleScroll = useCallback(
    (event: NativeSyntheticEvent<NativeScrollEvent>) => {
      const { layoutMeasurement, contentOffset, contentSize } = event.nativeEvent;
      const distanceFromBottom =
        contentSize.height - contentOffset.y - layoutMeasurement.height;
      const wasScrolledUp = userScrolledUpRef.current;
      const isScrolledUp = distanceFromBottom > 40;
      userScrolledUpRef.current = isScrolledUp;

      // User just scrolled back to bottom — flush buffered content
      if (wasScrolledUp && !isScrolledUp) {
        flushBuffer();
      }
    },
    [flushBuffer],
  );

  const handleContentSizeChange = useCallback(
    (_w: number, h: number) => {
      if (!userScrolledUpRef.current) {
        scrollRef.current?.scrollToEnd({ animated: false });
      }
    },
    [],
  );

  return (
    <View style={styles.container}>
      <ScrollView
        ref={scrollRef}
        style={styles.scroll}
        contentContainerStyle={styles.scrollContent}
        onScroll={handleScroll}
        onContentSizeChange={handleContentSizeChange}
        scrollEventThrottle={16}
        showsVerticalScrollIndicator
        showsHorizontalScrollIndicator={false}
        bounces
        indicatorStyle="white"
      >
        {parsedLines.map((line, i) => (
          <TerminalLine key={i} line={line} />
        ))}
      </ScrollView>
    </View>
  );
}

function TerminalLine({ line }: { line: ParsedLine }) {
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
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: catppuccin.base,
  },
  scroll: {
    flex: 1,
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
