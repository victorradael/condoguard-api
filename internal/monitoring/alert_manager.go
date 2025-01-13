package monitoring

import (
    "bytes"
    "encoding/json"
    "net/http"
    "time"
)

type AlertLevel string

const (
    AlertLevelInfo    AlertLevel = "info"
    AlertLevelWarning AlertLevel = "warning"
    AlertLevelError   AlertLevel = "error"
)

type Alert struct {
    Level     AlertLevel  `json:"level"`
    Title     string      `json:"title"`
    Message   string      `json:"message"`
    Tags      []string    `json:"tags"`
    Timestamp time.Time   `json:"timestamp"`
}

type AlertManager struct {
    webhookURL string
    client     *http.Client
}

func NewAlertManager(webhookURL string) *AlertManager {
    return &AlertManager{
        webhookURL: webhookURL,
        client: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

func (am *AlertManager) SendAlert(alert Alert) error {
    alert.Timestamp = time.Now()

    payload, err := json.Marshal(alert)
    if err != nil {
        return err
    }

    req, err := http.NewRequest("POST", am.webhookURL, bytes.NewBuffer(payload))
    if err != nil {
        return err
    }

    req.Header.Set("Content-Type", "application/json")
    
    resp, err := am.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    return nil
} 