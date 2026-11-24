package cloudsync

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Client interface abstracts Firestore synchronisation operations.
type Client interface {
	SyncStatus(ctx context.Context, payload PublicStatusPayload) error
	SyncDigest(ctx context.Context, payload PublicDigestPayload) error
	GetPendingActions(ctx context.Context) ([]ActionRequest, error)
	UpdateAction(ctx context.Context, action ActionRequest) error
	IsDryRun() bool
}

// ServiceAccountKey matches the Google Cloud Service Account JSON structure.
type ServiceAccountKey struct {
	Type                    string `json:"type"`
	ProjectID               string `json:"project_id"`
	PrivateKeyID            string `json:"private_key_id"`
	PrivateKey              string `json:"private_key"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	AuthURI                 string `json:"auth_uri"`
	TokenURI                string `json:"token_uri"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
}

// RESTClient communicates with Firestore via standard library HTTP and OAuth2 JWT bearer tokens.
type RESTClient struct {
	projectID  string
	sa         *ServiceAccountKey
	parsedKey  *rsa.PrivateKey
	httpClient *http.Client
	baseURL    string

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// DryRunClient is a no-op client used when credentials are not configured or invalid.
type DryRunClient struct{}

func (d *DryRunClient) SyncStatus(ctx context.Context, payload PublicStatusPayload) error {
	return nil
}

func (d *DryRunClient) SyncDigest(ctx context.Context, payload PublicDigestPayload) error {
	return nil
}

func (d *DryRunClient) GetPendingActions(ctx context.Context) ([]ActionRequest, error) {
	return nil, nil
}

func (d *DryRunClient) UpdateAction(ctx context.Context, action ActionRequest) error {
	return nil
}

func (d *DryRunClient) IsDryRun() bool {
	return true
}

// NewClient constructs a Client based on credentials path or JSON content.
// If credentials are empty or the file is missing, it returns a DryRunClient without failing.
func NewClient(credPath, projectID string) (Client, error) {
	credPath = strings.TrimSpace(credPath)
	if credPath == "" {
		log.Printf("[cloudsync] No credentials configured -> running in DRY-RUN mode")
		return &DryRunClient{}, nil
	}

	var data []byte
	var err error

	// If credPath begins with '{', treat as raw JSON content
	if strings.HasPrefix(credPath, "{") {
		data = []byte(credPath)
	} else {
		// Clean and verify file path
		cleanPath := filepath.Clean(credPath)
		data, err = os.ReadFile(cleanPath)
		if err != nil {
			log.Printf("[cloudsync] Credential file not found at %s -> running in DRY-RUN mode", cleanPath)
			return &DryRunClient{}, nil
		}
	}

	var sa ServiceAccountKey
	if err := json.Unmarshal(data, &sa); err != nil {
		log.Printf("[cloudsync] Failed to parse service account JSON: %v -> running in DRY-RUN mode", err)
		return &DryRunClient{}, nil
	}

	if sa.ProjectID != "" && projectID == "" {
		projectID = sa.ProjectID
	}

	parsedKey, err := parseRSAPrivateKey(sa.PrivateKey)
	if err != nil {
		log.Printf("[cloudsync] Failed to parse RSA private key: %v -> running in DRY-RUN mode", err)
		return &DryRunClient{}, nil
	}

	return &RESTClient{
		projectID:  projectID,
		sa:         &sa,
		parsedKey:  parsedKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    fmt.Sprintf("https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents", projectID),
	}, nil
}

func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("failed to decode PEM block containing private key")
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("key is not RSA private key")
	}

	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func (c *RESTClient) IsDryRun() bool {
	return false
}

func (c *RESTClient) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-5*time.Minute)) {
		return c.accessToken, nil
	}

	now := time.Now().UTC().Unix()
	headerJSON := `{"alg":"RS256","typ":"JWT"}`
	claimMap := map[string]interface{}{
		"iss":   c.sa.ClientEmail,
		"scope": "https://www.googleapis.com/auth/datastore",
		"aud":   "https://oauth2.googleapis.com/token",
		"exp":   now + 3600,
		"iat":   now,
	}
	claimJSON, err := json.Marshal(claimMap)
	if err != nil {
		return "", err
	}

	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	claimB64 := base64.RawURLEncoding.EncodeToString(claimJSON)
	signInput := headerB64 + "." + claimB64

	hashed := sha256.Sum256([]byte(signInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.parsedKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	jwtToken := signInput + "." + sigB64

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", jwtToken)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth2 token error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", err
	}

	c.accessToken = res.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
	return c.accessToken, nil
}

// SyncStatus uploads the sanitized status payload to sentinel_public/status.
func (c *RESTClient) SyncStatus(ctx context.Context, payload PublicStatusPayload) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("cloudsync: auth failed: %w", err)
	}

	fields := map[string]interface{}{
		"updated_at": map[string]interface{}{"stringValue": payload.UpdatedAt},
		"device": map[string]interface{}{
			"mapValue": map[string]interface{}{
				"fields": map[string]interface{}{
					"battery_level":     map[string]interface{}{"integerValue": payload.Device.BatteryLevel},
					"is_charging":       map[string]interface{}{"booleanValue": payload.Device.IsCharging},
					"ram_used_percent":  map[string]interface{}{"doubleValue": payload.Device.RAMUsedPercent},
					"disk_used_percent": map[string]interface{}{"doubleValue": payload.Device.DiskUsedPercent},
					"uptime_seconds":    map[string]interface{}{"integerValue": payload.Device.UptimeSeconds},
					"uptime_hours":      map[string]interface{}{"doubleValue": payload.Device.UptimeHours},
				},
			},
		},
		"services": map[string]interface{}{
			"mapValue": map[string]interface{}{
				"fields": map[string]interface{}{
					"server":    map[string]interface{}{"stringValue": payload.Services.Server},
					"gatus":     map[string]interface{}{"stringValue": payload.Services.Gatus},
					"tailscale": map[string]interface{}{"stringValue": payload.Services.Tailscale},
				},
			},
		},
	}
	if payload.Message != "" {
		fields["message"] = map[string]interface{}{"stringValue": payload.Message}
	}

	docPayload := map[string]interface{}{"fields": fields}
	docBytes, err := json.Marshal(docPayload)
	if err != nil {
		return err
	}

	docURL := fmt.Sprintf("%s/sentinel_public/status", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "PATCH", docURL, bytes.NewReader(docBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sync status network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync status failed (HTTP %d): %s", resp.StatusCode, string(b))
	}

	return nil
}

// SyncDigest uploads the sanitized digest payload to sentinel_public/digest.
func (c *RESTClient) SyncDigest(ctx context.Context, payload PublicDigestPayload) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("cloudsync: auth failed: %w", err)
	}

	fields := map[string]interface{}{
		"updated_at": map[string]interface{}{"stringValue": payload.UpdatedAt},
	}
	if payload.SummaryMD != "" {
		fields["summary_md"] = map[string]interface{}{"stringValue": payload.SummaryMD}
	}

	if payload.Gold != nil {
		fields["gold"] = map[string]interface{}{
			"mapValue": map[string]interface{}{
				"fields": map[string]interface{}{
					"world_spot": map[string]interface{}{"doubleValue": payload.Gold.WorldSpot},
					"bar_buy":    map[string]interface{}{"doubleValue": payload.Gold.BarBuy},
					"bar_sell":   map[string]interface{}{"doubleValue": payload.Gold.BarSell},
					"updated_at": map[string]interface{}{"stringValue": payload.Gold.UpdatedAt},
				},
			},
		}
	}

	docPayload := map[string]interface{}{"fields": fields}
	docBytes, err := json.Marshal(docPayload)
	if err != nil {
		return err
	}

	docURL := fmt.Sprintf("%s/sentinel_public/digest", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "PATCH", docURL, bytes.NewReader(docBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sync digest network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync digest failed (HTTP %d): %s", resp.StatusCode, string(b))
	}

	return nil
}

// GetPendingActions queries sentinel_actions collection for any pending requests.
func (c *RESTClient) GetPendingActions(ctx context.Context) ([]ActionRequest, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("cloudsync: auth failed: %w", err)
	}

	docURL := fmt.Sprintf("%s/sentinel_actions", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", docURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get actions failed (HTTP %d): %s", resp.StatusCode, string(b))
	}

	var res struct {
		Documents []struct {
			Name   string `json:"name"`
			Fields map[string]struct {
				StringValue string `json:"stringValue"`
			} `json:"fields"`
			CreateTime string `json:"createTime"`
		} `json:"documents"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	actions := make([]ActionRequest, 0, len(res.Documents))
	for _, doc := range res.Documents {
		actionType := doc.Fields["action"].StringValue
		status := doc.Fields["status"].StringValue
		if status != "pending" {
			continue
		}

		parts := strings.Split(doc.Name, "/")
		id := parts[len(parts)-1]

		reqTime, _ := time.Parse(time.RFC3339, doc.Fields["requested_at"].StringValue)
		if reqTime.IsZero() {
			reqTime, _ = time.Parse(time.RFC3339, doc.CreateTime)
		}

		prompt := doc.Fields["prompt"].StringValue
		model := doc.Fields["model"].StringValue

		actions = append(actions, ActionRequest{
			ID:          id,
			Action:      actionType,
			Status:      status,
			Prompt:      prompt,
			Model:       model,
			RequestedAt: reqTime,
		})
	}

	return actions, nil
}

// UpdateAction updates an action document with completion/failure status.
func (c *RESTClient) UpdateAction(ctx context.Context, action ActionRequest) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("cloudsync: auth failed: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	fields := map[string]interface{}{
		"action":       map[string]interface{}{"stringValue": action.Action},
		"status":       map[string]interface{}{"stringValue": action.Status},
		"requested_at": map[string]interface{}{"stringValue": action.RequestedAt.UTC().Format(time.RFC3339)},
		"completed_at": map[string]interface{}{"stringValue": now},
		"result":       map[string]interface{}{"stringValue": action.Result},
	}

	docPayload := map[string]interface{}{"fields": fields}
	docBytes, err := json.Marshal(docPayload)
	if err != nil {
		return err
	}

	docURL := fmt.Sprintf("%s/sentinel_actions/%s", c.baseURL, action.ID)
	req, err := http.NewRequestWithContext(ctx, "PATCH", docURL, bytes.NewReader(docBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update action failed (HTTP %d): %s", resp.StatusCode, string(b))
	}

	return nil
}
