// El enlace de invitado lleva el token en el fragmento (`#t=…`). El fragmento
// nunca se envía al servidor: se lee solo en el navegador.
export function parseGuestToken(hash: string): string | null {
  const raw = hash.startsWith('#') ? hash.slice(1) : hash
  const value = new URLSearchParams(raw).get('t')?.trim()
  return value ? value : null
}
