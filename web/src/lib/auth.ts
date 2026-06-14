// Lightweight token store: module-level state mirrored to localStorage, with a
// subscription so React (and the API layer) can react to login/logout.
const KEY = "porque_token";

let token: string | null = localStorage.getItem(KEY);
type Listener = (t: string | null) => void;
const listeners = new Set<Listener>();

export function getToken(): string | null {
  return token;
}

export function setToken(next: string | null): void {
  token = next;
  if (next) localStorage.setItem(KEY, next);
  else localStorage.removeItem(KEY);
  listeners.forEach((l) => l(next));
}

export function clearToken(): void {
  setToken(null);
}

export function onTokenChange(l: Listener): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}
