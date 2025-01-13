package main

import (
    "fmt"
    "log"
    "os"
    "os/exec"
    "time"
    "github.com/joho/godotenv"
)

func main() {
    if err := godotenv.Load(); err != nil {
        log.Printf("Warning: .env file not found")
    }

    // Create backup directory if it doesn't exist
    backupDir := "backups"
    if err := os.MkdirAll(backupDir, 0755); err != nil {
        log.Fatal("Failed to create backup directory:", err)
    }

    // Generate backup filename with timestamp
    timestamp := time.Now().Format("2006-01-02-15-04-05")
    filename := fmt.Sprintf("%s/backup-%s.gz", backupDir, timestamp)

    // Get MongoDB URI from environment
    mongoURI := os.Getenv("MONGODB_URI")
    if mongoURI == "" {
        log.Fatal("MONGODB_URI environment variable not set")
    }

    // Create mongodump command
    cmd := exec.Command("mongodump",
        "--uri", mongoURI,
        "--gzip",
        "--archive="+filename,
    )

    // Execute backup
    if output, err := cmd.CombinedOutput(); err != nil {
        log.Fatal("Backup failed:", string(output))
    }

    log.Printf("Backup completed successfully: %s", filename)

    // Clean up old backups (keep last 7 days)
    cleanOldBackups(backupDir, 7)
}

func cleanOldBackups(backupDir string, daysToKeep int) {
    files, err := os.ReadDir(backupDir)
    if err != nil {
        log.Printf("Failed to read backup directory: %v", err)
        return
    }

    cutoffTime := time.Now().AddDate(0, 0, -daysToKeep)

    for _, file := range files {
        info, err := file.Info()
        if err != nil {
            continue
        }

        if info.ModTime().Before(cutoffTime) {
            path := fmt.Sprintf("%s/%s", backupDir, file.Name())
            if err := os.Remove(path); err != nil {
                log.Printf("Failed to remove old backup %s: %v", path, err)
            } else {
                log.Printf("Removed old backup: %s", path)
            }
        }
    }
} 