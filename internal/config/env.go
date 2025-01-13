package config

import (
    "os"
    "strconv"
    "time"
)

// GetEnv retorna o valor de uma variável de ambiente ou um valor padrão se não existir
func GetEnv(key, defaultValue string) string {
    if value, exists := os.LookupEnv(key); exists {
        return value
    }
    return defaultValue
}

// GetEnvInt retorna o valor inteiro de uma variável de ambiente ou um valor padrão
func GetEnvInt(key string, defaultValue int) int {
    if value, exists := os.LookupEnv(key); exists {
        if intValue, err := strconv.Atoi(value); err == nil {
            return intValue
        }
    }
    return defaultValue
}

// GetEnvBool retorna o valor booleano de uma variável de ambiente ou um valor padrão
func GetEnvBool(key string, defaultValue bool) bool {
    if value, exists := os.LookupEnv(key); exists {
        if boolValue, err := strconv.ParseBool(value); err == nil {
            return boolValue
        }
    }
    return defaultValue
}

// GetEnvDuration retorna o valor de duração de uma variável de ambiente ou um valor padrão
func GetEnvDuration(key string, defaultValue time.Duration) time.Duration {
    if value, exists := os.LookupEnv(key); exists {
        if duration, err := time.ParseDuration(value); err == nil {
            return duration
        }
    }
    return defaultValue
} 