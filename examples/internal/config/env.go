package config

import "os"

// Env reads a Clara Core example setting with the former prefix as fallback.
func Env(name string) string {
	if value, ok := os.LookupEnv("CLARA_CORE_" + name); ok {
		return value
	}
	return os.Getenv("CLARY_" + name)
}
