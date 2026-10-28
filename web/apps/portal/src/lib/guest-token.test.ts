import { describe, expect, it } from 'vitest'
import { parseGuestToken } from '@/lib/guest-token'

describe('parseGuestToken', () => {
  it('lee el token del fragmento con o sin almohadilla', () => {
    expect(parseGuestToken('#t=abc.def-ghi')).toBe('abc.def-ghi')
    expect(parseGuestToken('t=abc')).toBe('abc')
  })

  it('recorta espacios y decodifica el valor', () => {
    expect(parseGuestToken('#t=%20abc%20')).toBe('abc')
  })

  it('no inventa un token cuando el fragmento está vacío o no lo contiene', () => {
    expect(parseGuestToken('')).toBeNull()
    expect(parseGuestToken('#')).toBeNull()
    expect(parseGuestToken('#otro=1')).toBeNull()
    expect(parseGuestToken('#t=')).toBeNull()
    expect(parseGuestToken('#t=%20%20')).toBeNull()
  })

  it('tolera otros parámetros junto al token', () => {
    expect(parseGuestToken('#a=1&t=xyz&b=2')).toBe('xyz')
  })
})
