// Catppuccin Mocha palette — matches the daemon's WorkstreamThemes
export const catppuccin = {
  base: '#1e1e2e',
  mantle: '#181825',
  crust: '#11111b',
  surface0: '#313244',
  surface1: '#45475a',
  surface2: '#585b70',
  overlay0: '#6c7086',
  overlay1: '#7f849c',
  text: '#cdd6f4',
  subtext0: '#a6adc8',
  subtext1: '#bac2de',
  lavender: '#b4befe',
  blue: '#89b4fa',
  sapphire: '#74c7ec',
  sky: '#89dceb',
  teal: '#94e2d5',
  green: '#a6e3a1',
  yellow: '#f9e2af',
  peach: '#fab387',
  maroon: '#eba0ac',
  red: '#f38ba8',
  mauve: '#cba6f7',
  pink: '#f5c2e7',
  flamingo: '#f2cdcd',
  rosewater: '#f5e0dc',
} as const;

// Workstream theme definitions — mirrors daemon's WorkstreamThemes
export const workstreamThemes = [
  { shader: 'starfield.glsl', border: '#b4befe', tint: '#1e1e3a', label: 'starfield' },
  { shader: 'inside-the-matrix.glsl', border: '#a6e3a1', tint: '#1e2e1e', label: 'matrix' },
  { shader: 'sparks-from-fire.glsl', border: '#fab387', tint: '#2e1e1e', label: 'embers' },
  { shader: 'just-snow.glsl', border: '#89b4fa', tint: '#1e2636', label: 'snow' },
  { shader: 'gears-and-belts.glsl', border: '#f9e2af', tint: '#2a2a1e', label: 'gears' },
  { shader: 'cubes.glsl', border: '#cba6f7', tint: '#261e30', label: 'cubes' },
  { shader: 'animated-gradient-shader.glsl', border: '#94e2d5', tint: '#1e2e2a', label: 'gradient' },
] as const;

// xterm.js theme matching Catppuccin Mocha
export const xtermTheme = {
  background: catppuccin.base,
  foreground: catppuccin.text,
  cursor: catppuccin.rosewater,
  cursorAccent: catppuccin.base,
  selectionBackground: catppuccin.surface2,
  selectionForeground: catppuccin.text,
  black: catppuccin.surface1,
  red: catppuccin.red,
  green: catppuccin.green,
  yellow: catppuccin.yellow,
  blue: catppuccin.blue,
  magenta: catppuccin.pink,
  cyan: catppuccin.teal,
  white: catppuccin.subtext1,
  brightBlack: catppuccin.surface2,
  brightRed: catppuccin.red,
  brightGreen: catppuccin.green,
  brightYellow: catppuccin.yellow,
  brightBlue: catppuccin.blue,
  brightMagenta: catppuccin.pink,
  brightCyan: catppuccin.teal,
  brightWhite: catppuccin.text,
};

export function themeByBorder(border: string) {
  return workstreamThemes.find((t) => t.border === border) ?? workstreamThemes[0];
}
