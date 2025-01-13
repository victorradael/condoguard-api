package main

import (
    "fmt"
    "github.com/victorradael/condoguard/internal/config"
)

func main() {
    redisAddr := config.GetEnv("REDIS_ADDR", "localhost:6379")
    fmt.Printf("Redis Address: %s\n", redisAddr)
} 