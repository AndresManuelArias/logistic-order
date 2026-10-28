package postgres

import (
	"encoding/json"
	"strconv"
)

// nullIfEmpty convierte un string apuntado en nil para SQL NULL.
func nullIfEmpty(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

// toJSONb serializa un mapa a JSON para columnas jsonb (NULL si vacío).
func toJSONb(m map[string]any) any {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return string(b)
}

// itoa convierte un índice de argumento SQL a texto.
func itoa(n int) string { return strconv.Itoa(n) }
