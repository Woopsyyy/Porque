// Minecraft/itzg console output carries ANSI SGR escape codes. The console view
// renders plain text, so strip them. Kept as its own unit so it can be tested.
// eslint-disable-next-line no-control-regex
const ANSI_RE = /\x1b\[[0-9;]*m/g;

export function stripAnsi(input: string): string {
  return input.replace(ANSI_RE, "");
}

// Split a raw log chunk into clean, non-empty lines, filtering out RCON connection spam.
export function toLines(chunk: string): string[] {
  return stripAnsi(chunk)
    .split(/\r?\n/)
    .filter((l) => l.length > 0 && !l.includes("[RCON Listener") && !l.includes("[RCON Client"));
}
