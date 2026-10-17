package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/yusufarbc/vantage/config"
	log "github.com/yusufarbc/vantage/logger"
)

var conf *config.NotificationConfig

// slackWebhookHost is the only host a Slack incoming webhook is ever served
// from. Pinning to it keeps the outbound notification request from being
// redirected to an arbitrary (e.g. internal) address if the configured value
// is ever malformed or tampered with.
const slackWebhookHost = "hooks.slack.com"

// safeSlackURL validates the admin-configured Slack webhook and returns a URL
// rebuilt with a constant scheme and host. Only the path/query (which carry
// Slack's per-workspace secret) are taken from the configured value, so the
// request can only ever be sent to https://hooks.slack.com and never be
// redirected to an attacker- or misconfiguration-chosen address.
func safeSlackURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() != slackWebhookHost {
		return "", false
	}
	safe := url.URL{
		Scheme:   "https",
		Host:     slackWebhookHost,
		Path:     u.Path,
		RawQuery: u.RawQuery,
	}
	return safe.String(), true
}

// Setup initializes the notifier package with the provided configuration.
func Setup(c *config.NotificationConfig) {
	conf = c
}

// SendAlert sends a notification alert for a finding.
func SendAlert(toolName, severity, name, target string) {
	if conf == nil || !conf.Enabled {
		return
	}

	// Only alert on high/critical findings
	sevUpper := strings.ToUpper(severity)
	if sevUpper != "CRITICAL" && sevUpper != "HIGH" {
		return
	}

	msg := fmt.Sprintf("🚨 *VANTAGE ALERT* 🚨\n\n*Severity:* %s\n*Tool:* %s\n*Finding:* %s\n*Target:* %s\n*Time:* %s",
		sevUpper, toolName, name, target, time.Now().Format("2006-01-02 15:04:05"))

	// 1. Try Notify binary if configured
	if conf.NotifyConfig != "" {
		go callNotify(msg)
	}

	// 2. Fallback/Direct pushing to Slack
	if conf.SlackWebhook != "" {
		go sendSlack(msg)
	}

	// 3. Direct pushing to Telegram
	if conf.TelegramToken != "" && conf.TelegramChatID != "" {
		go sendTelegram(msg)
	}
}

func callNotify(msg string) {
	// Example: echo "msg" | notify -config config/notify.yaml
	cmd := exec.Command("notify", "-config", conf.NotifyConfig, "-silent")
	cmd.Stdin = bytes.NewBufferString(msg)
	if err := cmd.Run(); err != nil {
		log.Errorf("notify tool error: %v", err)
	}
}

func sendSlack(msg string) {
	payload := map[string]string{"text": msg}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("slack notification marshal error: %v", err)
		return
	}
	endpoint, ok := safeSlackURL(conf.SlackWebhook)
	if !ok {
		log.Errorf("slack notification error: webhook must be an https URL on %s", slackWebhookHost)
		return
	}
	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Errorf("slack notification error: %v", err)
		return
	}
	defer resp.Body.Close()
}

func sendTelegram(msg string) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", conf.TelegramToken)
	payload := map[string]string{
		"chat_id":    conf.TelegramChatID,
		"text":       msg,
		"parse_mode": "Markdown",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("telegram notification marshal error: %v", err)
		return
	}
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Errorf("telegram notification error: %v", err)
		return
	}
	defer resp.Body.Close()
}
