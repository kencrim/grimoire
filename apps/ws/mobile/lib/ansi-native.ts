import Anser from 'anser';
import { TextStyle } from 'react-native';
import { catppuccin, xtermTheme } from './theme';

export interface StyledSpan {
  text: string;
  style: TextStyle;
}

export interface ParsedLine {
  spans: StyledSpan[];
}

// Map anser's class names for the 16 standard colors to Catppuccin,
// matching the xtermTheme mapping used by the xterm.js renderer.
const FG_CLASS_MAP: Record<string, string> = {
  'ansi-black': xtermTheme.black,
  'ansi-red': xtermTheme.red,
  'ansi-green': xtermTheme.green,
  'ansi-yellow': xtermTheme.yellow,
  'ansi-blue': xtermTheme.blue,
  'ansi-magenta': xtermTheme.magenta,
  'ansi-cyan': xtermTheme.cyan,
  'ansi-white': xtermTheme.white,
  'ansi-bright-black': xtermTheme.brightBlack,
  'ansi-bright-red': xtermTheme.brightRed,
  'ansi-bright-green': xtermTheme.brightGreen,
  'ansi-bright-yellow': xtermTheme.brightYellow,
  'ansi-bright-blue': xtermTheme.brightBlue,
  'ansi-bright-magenta': xtermTheme.brightMagenta,
  'ansi-bright-cyan': xtermTheme.brightCyan,
  'ansi-bright-white': xtermTheme.brightWhite,
};

const BG_CLASS_MAP: Record<string, string> = { ...FG_CLASS_MAP };

// 6×6×6 color cube steps for 256-color palette indices 16–231.
const CUBE_STEPS = [0, 95, 135, 175, 215, 255];

function palette256ToHex(index: number): string {
  if (index < 16) return catppuccin.text; // shouldn't happen — anser uses class names
  if (index < 232) {
    // 6×6×6 color cube
    const ci = index - 16;
    const b = CUBE_STEPS[ci % 6];
    const g = CUBE_STEPS[Math.floor(ci / 6) % 6];
    const r = CUBE_STEPS[Math.floor(ci / 36)];
    return rgbToHex(r, g, b);
  }
  // Grayscale ramp 232–255
  const v = 8 + (index - 232) * 10;
  return rgbToHex(v, v, v);
}

function rgbToHex(r: number, g: number, b: number): string {
  return '#' + ((1 << 24) | (r << 16) | (g << 8) | b).toString(16).slice(1);
}

function rgbStringToHex(rgb: string): string {
  const parts = rgb.split(',').map((s) => parseInt(s.trim(), 10));
  return rgbToHex(parts[0], parts[1], parts[2]);
}

function resolveColor(
  className: string | null,
  truecolor: string | null,
  classMap: Record<string, string>,
): string | undefined {
  if (truecolor) return rgbStringToHex(truecolor);
  if (!className) return undefined;
  if (classMap[className]) return classMap[className];
  // 256-color palette: "ansi-palette-NNN"
  const paletteMatch = className.match(/^ansi-palette-(\d+)$/);
  if (paletteMatch) return palette256ToHex(parseInt(paletteMatch[1], 10));
  return undefined;
}

// Strip non-SGR escape sequences (cursor movement, screen clearing, etc.)
// but preserve SGR sequences (colors/styles) which start with \x1b[ and end with m.
const NON_SGR_RE = /\x1b\[[^m]*?[A-LN-Za-ln-z]/g;

export function parseTerminalContent(content: string): ParsedLine[] {
  // Strip non-SGR control codes and normalize line endings
  const cleaned = content.replace(NON_SGR_RE, '').replace(/\r\n?/g, '\n');

  // Parse with anser — use_classes gives us named colors for the 16 standard
  // colors so we can map them to Catppuccin.
  const chunks = Anser.ansiToJson(cleaned, { use_classes: true });

  // Split anser chunks into lines. A single chunk may span multiple lines.
  const lines: ParsedLine[] = [];
  let currentSpans: StyledSpan[] = [];

  for (const chunk of chunks) {
    if (!chunk.content) continue;

    const fg = resolveColor(chunk.fg, chunk.fg_truecolor, FG_CLASS_MAP);
    const bg = resolveColor(chunk.bg, chunk.bg_truecolor, BG_CLASS_MAP);
    const decorations: string[] = chunk.decorations ?? [];

    // anser already swaps fg/bg for reverse video, so we just use them directly.
    const style: TextStyle = {};
    if (fg) style.color = fg;
    if (bg) style.backgroundColor = bg;
    if (decorations.includes('bold')) style.fontWeight = 'bold';
    if (decorations.includes('italic')) style.fontStyle = 'italic';
    if (decorations.includes('underline')) style.textDecorationLine = 'underline';
    if (decorations.includes('dim')) style.opacity = 0.5;

    const parts = chunk.content.split('\n');
    for (let i = 0; i < parts.length; i++) {
      if (i > 0) {
        lines.push({ spans: currentSpans });
        currentSpans = [];
      }
      if (parts[i]) {
        currentSpans.push({ text: parts[i], style });
      }
    }
  }

  // Push trailing line
  lines.push({ spans: currentSpans });

  return lines;
}
