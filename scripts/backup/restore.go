package main

import (
    "flag"
    "log"
    "os"
    "os/exec"
    "github.com/joho/godotenv"
)

func main() {
    if err := godotenv.Load(); err != nil {
        log.Printf("Warning: .env file not found")
    }

    // Parse command line arguments
    backupFile := flag.String("file", "", "Backup file to restore")
    flag.Parse()

    if *backupFile == "" {
        log.Fatal("Please specify a backup file using -file flag")
    }

    // Check if backup file exists
    if _, err := os.Stat(*backupFile); os.IsNotExist(err) {
        log.Fatalf("Backup file does not exist: %s", *backupFile)
    }

    // Get MongoDB URI from environment
    mongoURI := os.Getenv("MONGODB_URI")
    if mongoURI == "" {
        log.Fatal("MONGODB_URI environment variable not set")
    }

    // Create mongorestore command
    cmd := exec.Command("mongorestore",
        "--uri", mongoURI,
        "--gzip",
        "--archive="+*backupFile,
        "--drop", // Drop existing collections before restore
    )

    // Execute restore
    if output, err := cmd.CombinedOutput(); err != nil {
        log.Fatal("Restore failed:", string(output))
    }

    log.Printf("Restore completed successfully from: %s", *backupFile)
} 