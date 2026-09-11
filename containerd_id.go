package main

import "strings"

func sanitizeContainerdID(value string) string {
	value = strings.TrimSpace(value)

	var builder strings.Builder

	for _, runeValue := range value {
		switch {
		case runeValue >= 'a' && runeValue <= 'z':
			builder.WriteRune(runeValue)
		case runeValue >= 'A' && runeValue <= 'Z':
			builder.WriteRune(runeValue)
		case runeValue >= '0' && runeValue <= '9':
			builder.WriteRune(runeValue)
		case runeValue == '.', runeValue == '_', runeValue == '-':
			builder.WriteRune(runeValue)
		case runeValue == ' ':
			builder.WriteRune('-')
		}
	}

	return strings.Trim(builder.String(), ".-_")
}
