// Hex industrial palette — dark obsidian + safety orange
export const hex = {
  base: '#161616',
  mantle: '#111111',
  crust: '#0c0c0c',
  surface0: '#1e1e1e',
  surface1: '#282828',
  surface2: '#363636',
  overlay0: '#767676',
  overlay1: '#7a7a7a',
  text: '#e0e0e0',
  subtext0: '#8a8a8a',
  subtext1: '#aaaaaa',
  accent: '#ff8c00',
  blue: '#60a5fa',
  sapphire: '#38bdf8',
  sky: '#7dd3fc',
  teal: '#2dd4bf',
  green: '#4ade80',
  yellow: '#fbbf24',
  peach: '#ffb77d',
  maroon: '#fb7185',
  red: '#f87171',
  mauve: '#c084fc',
  pink: '#f472b6',
  flamingo: '#fda4af',
  rosewater: '#fecdd3',
} as const;

// Workstream theme definitions — mirrors daemon's WorkstreamThemes
export const workstreamThemes = [
  { shader: 'starfield.glsl', border: '#60a5fa', tint: '#161620', label: 'starfield' },
  { shader: 'inside-the-matrix.glsl', border: '#4ade80', tint: '#141a14', label: 'matrix' },
  { shader: 'sparks-from-fire.glsl', border: '#ffb77d', tint: '#1a1410', label: 'embers' },
  { shader: 'just-snow.glsl', border: '#38bdf8', tint: '#141820', label: 'snow' },
  { shader: 'gears-and-belts.glsl', border: '#fbbf24', tint: '#1a1a10', label: 'gears' },
  { shader: 'cubes.glsl', border: '#c084fc', tint: '#1a1420', label: 'cubes' },
  { shader: 'animated-gradient-shader.glsl', border: '#2dd4bf', tint: '#101a18', label: 'gradient' },
] as const;

// xterm.js theme matching the Hex palette
export const xtermTheme = {
  background: hex.base,
  foreground: hex.text,
  cursor: hex.accent,
  cursorAccent: hex.base,
  selectionBackground: hex.surface2,
  selectionForeground: hex.text,
  black: hex.surface1,
  red: hex.red,
  green: hex.green,
  yellow: hex.yellow,
  blue: hex.blue,
  magenta: hex.pink,
  cyan: hex.teal,
  white: hex.subtext1,
  brightBlack: hex.surface2,
  brightRed: '#fca5a5',
  brightGreen: '#86efac',
  brightYellow: '#fde68a',
  brightBlue: '#93c5fd',
  brightMagenta: '#f9a8d4',
  brightCyan: '#5eead4',
  brightWhite: '#f5f5f5',
};

export function themeByBorder(border: string) {
  return workstreamThemes.find((t) => t.border === border) ?? workstreamThemes[0];
}
