package backend

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	qobuzAccountCacheFile = "qobuz-account.json"
	qobuzOAuthSuccessHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Qobuz Authentication</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0b0f19; color: #f8fafc; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
    .card { background: #151d30; border: 1px solid #23314d; border-radius: 16px; padding: 40px; text-align: center; max-width: 440px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
    .icon { font-size: 48px; margin-bottom: 16px; }
    h2 { margin: 0 0 12px; color: #38bdf8; font-size: 24px; }
    p { margin: 0; color: #94a3b8; font-size: 15px; line-height: 1.6; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">✨</div>
    <h2>Connected to Qobuz!</h2>
    <p>Your Qobuz account has been successfully linked with SpotiFLAC.<br>You may now close this tab and return to the app.</p>
  </div>
</body>
</html>`

	qobuzOAuthErrorHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Qobuz Authentication</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0b0f19; color: #f8fafc; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
    .card { background: #151d30; border: 1px solid #451a1a; border-radius: 16px; padding: 40px; text-align: center; max-width: 440px; box-shadow: 0 10px 30px rgba(0,0,0,0.5); }
    .icon { font-size: 48px; margin-bottom: 16px; }
    h2 { margin: 0 0 12px; color: #f87171; font-size: 24px; }
    p { margin: 0; color: #94a3b8; font-size: 15px; line-height: 1.6; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">⚠️</div>
    <h2>Authentication Failed</h2>
    <p>%s<br>Please return to SpotiFLAC and try again.</p>
  </div>
</body>
</html>`
)

var (
	qobuzPrivateKeyPattern = regexp.MustCompile(`privateKey:\s*"(?P<key>[A-Za-z0-9]{6,30})"`)

	oauthServerMu     sync.Mutex
	activeOAuthServer *http.Server
)

type QobuzAccount struct {
	UserID        int64  `json:"user_id"`
	UserAuthToken string `json:"user_auth_token"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	Subscription  string `json:"subscription"`
	CountryCode   string `json:"country_code,omitempty"`
	Connected     bool   `json:"connected"`
}

type qobuzLoginUserPayload struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	CountryCode   string `json:"country_code"`
	UserAuthToken string `json:"user_auth_token"`
	Credential    struct {
		Parameters struct {
			ShortLabel string `json:"short_label"`
			Label      string `json:"label"`
			Hires      bool   `json:"hires"`
			Lossless   bool   `json:"lossless"`
		} `json:"parameters"`
	} `json:"credential"`
}

type qobuzUserLoginResponse struct {
	User          qobuzLoginUserPayload `json:"user"`
	UserAuthToken string                `json:"user_auth_token"`
	Status        string                `json:"status,omitempty"`
	Message       string                `json:"message,omitempty"`
}

type qobuzOAuthCallbackResponse struct {
	Token   string `json:"token"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

func qobuzAccountCachePath() (string, error) {
	appDir, err := EnsureAppDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(appDir, qobuzAccountCacheFile), nil
}

func GetQobuzAccount() (*QobuzAccount, error) {
	cachePath, err := qobuzAccountCachePath()
	if err != nil {
		return &QobuzAccount{Connected: false}, err
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &QobuzAccount{Connected: false}, nil
		}
		return &QobuzAccount{Connected: false}, err
	}

	var acc QobuzAccount
	if err := json.Unmarshal(data, &acc); err != nil {
		return &QobuzAccount{Connected: false}, err
	}

	if strings.TrimSpace(acc.UserAuthToken) == "" {
		acc.Connected = false
	}

	return &acc, nil
}

func SaveQobuzAccount(acc *QobuzAccount) error {
	if acc == nil {
		return errors.New("qobuz account cannot be nil")
	}

	cachePath, err := qobuzAccountCachePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(acc, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(cachePath, data, 0o600)
}

func ClearQobuzAccount() error {
	cachePath, err := qobuzAccountCachePath()
	if err != nil {
		return err
	}
	_ = os.Remove(cachePath)
	return nil
}

func parseQobuzSubscriptionLabel(params struct {
	ShortLabel string `json:"short_label"`
	Label      string `json:"label"`
	Hires      bool   `json:"hires"`
	Lossless   bool   `json:"lossless"`
}) string {
	if strings.TrimSpace(params.ShortLabel) != "" {
		return strings.TrimSpace(params.ShortLabel)
	}
	if strings.TrimSpace(params.Label) != "" {
		return strings.TrimSpace(params.Label)
	}
	if params.Hires {
		return "Studio (Hi-Res)"
	}
	if params.Lossless {
		return "Lossless (16-bit)"
	}
	return "Active"
}

func fetchQobuzUserProfile(token string, creds *qobuzAPICredentials) (*QobuzAccount, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("empty auth token")
	}
	if creds == nil {
		var err error
		creds, err = getQobuzAPICredentials(false)
		if err != nil {
			return nil, err
		}
	}

	// Try 1: user/login with token
	loginReqURL := fmt.Sprintf("%s/user/login?app_id=%s&user_auth_token=%s", qobuzAPIBaseURL, url.QueryEscape(creds.AppID), url.QueryEscape(token))
	req, err := http.NewRequest(http.MethodGet, loginReqURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", qobuzDefaultUA)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-App-Id", creds.AppID)
		req.Header.Set("X-User-Auth-Token", token)

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var res qobuzUserLoginResponse
				if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && (res.User.ID > 0 || res.User.Email != "") {
					sub := parseQobuzSubscriptionLabel(res.User.Credential.Parameters)
					displayName := firstNonEmptyQobuzValue(res.User.DisplayName, res.User.Email)
					return &QobuzAccount{
						UserID:        res.User.ID,
						UserAuthToken: token,
						Email:         res.User.Email,
						DisplayName:   displayName,
						Subscription:  sub,
						CountryCode:   res.User.CountryCode,
						Connected:     true,
					}, nil
				}
			}
		}
	}

	// Try 2: user/get with X-User-Auth-Token
	getReqURL := fmt.Sprintf("%s/user/get?app_id=%s", qobuzAPIBaseURL, url.QueryEscape(creds.AppID))
	getReq, err := http.NewRequest(http.MethodGet, getReqURL, nil)
	if err == nil {
		getReq.Header.Set("User-Agent", qobuzDefaultUA)
		getReq.Header.Set("Accept", "application/json")
		getReq.Header.Set("X-App-Id", creds.AppID)
		getReq.Header.Set("X-User-Auth-Token", token)

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(getReq)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var res qobuzUserLoginResponse
				if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && (res.User.ID > 0 || res.User.Email != "") {
					sub := parseQobuzSubscriptionLabel(res.User.Credential.Parameters)
					displayName := firstNonEmptyQobuzValue(res.User.DisplayName, res.User.Email)
					return &QobuzAccount{
						UserID:        res.User.ID,
						UserAuthToken: token,
						Email:         res.User.Email,
						DisplayName:   displayName,
						Subscription:  sub,
						CountryCode:   res.User.CountryCode,
						Connected:     true,
					}, nil
				}
			}
		}
	}

	return &QobuzAccount{
		UserAuthToken: token,
		Subscription:  "Connected",
		Connected:     true,
	}, nil
}

func LoginQobuzWithCredentials(identifier, password string) (*QobuzAccount, error) {
	trimmedID := strings.TrimSpace(identifier)
	trimmedPwd := strings.TrimSpace(password)
	if trimmedID == "" || trimmedPwd == "" {
		return nil, errors.New("username/email and password are required")
	}

	creds, err := getQobuzAPICredentials(false)
	if err != nil {
		return nil, fmt.Errorf("failed to load Qobuz credentials: %w", err)
	}

	pwdSum := md5.Sum([]byte(trimmedPwd))
	md5Pwd := hex.EncodeToString(pwdSum[:])

	// Attempt variants of user/login parameters
	attempts := []url.Values{
		{"username": {trimmedID}, "password": {md5Pwd}},
		{"email": {trimmedID}, "password": {md5Pwd}},
		{"username": {trimmedID}, "password": {trimmedPwd}},
		{"email": {trimmedID}, "password": {trimmedPwd}},
	}

	var lastErr error
	client := &http.Client{Timeout: 20 * time.Second}

	for _, attemptParams := range attempts {
		req, err := newQobuzSignedRequestWithCredentials(http.MethodGet, "user/login", attemptParams, creds)
		if err != nil {
			lastErr = err
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			var errPayload struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(body, &errPayload)
			msg := strings.TrimSpace(errPayload.Message)
			if msg == "" {
				msg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, previewQobuzResponseBody(body, 200))
			}
			lastErr = errors.New(msg)
			continue
		}

		var loginResp qobuzUserLoginResponse
		if err := json.Unmarshal(body, &loginResp); err != nil {
			lastErr = fmt.Errorf("failed to parse login response: %w", err)
			continue
		}

		token := firstNonEmptyQobuzValue(loginResp.UserAuthToken, loginResp.User.UserAuthToken)
		if token == "" {
			lastErr = errors.New("no authentication token received from Qobuz")
			continue
		}

		sub := parseQobuzSubscriptionLabel(loginResp.User.Credential.Parameters)
		displayName := firstNonEmptyQobuzValue(loginResp.User.DisplayName, loginResp.User.Email, trimmedID)

		account := &QobuzAccount{
			UserID:        loginResp.User.ID,
			UserAuthToken: token,
			Email:         loginResp.User.Email,
			DisplayName:   displayName,
			Subscription:  sub,
			CountryCode:   loginResp.User.CountryCode,
			Connected:     true,
		}

		if err := SaveQobuzAccount(account); err != nil {
			fmt.Printf("Warning: failed to persist Qobuz account: %v\n", err)
		}

		return account, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("qobuz login failed: %w", lastErr)
	}
	return nil, errors.New("qobuz login failed: invalid credentials or unsupported method")
}

func LoginQobuzWithToken(token string, userID int64) (*QobuzAccount, error) {
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return nil, errors.New("user auth token cannot be empty")
	}

	account, err := fetchQobuzUserProfile(trimmedToken, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	if userID > 0 && account.UserID == 0 {
		account.UserID = userID
	}

	account.Connected = true
	if err := SaveQobuzAccount(account); err != nil {
		fmt.Printf("Warning: failed to persist Qobuz account: %v\n", err)
	}

	return account, nil
}

func scrapeQobuzOAuthPrivateKey(client *http.Client) string {
	req, err := http.NewRequest(http.MethodGet, "https://play.qobuz.com/login", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", qobuzDefaultUA)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	bundleMatch := qobuzOpenBundleScriptPattern.FindStringSubmatch(string(html))
	if len(bundleMatch) < 2 {
		return ""
	}

	bundleURL := bundleMatch[1]
	if strings.HasPrefix(bundleURL, "/") {
		bundleURL = "https://play.qobuz.com" + bundleURL
	}

	bundleReq, err := http.NewRequest(http.MethodGet, bundleURL, nil)
	if err != nil {
		return ""
	}
	bundleReq.Header.Set("User-Agent", qobuzDefaultUA)

	bundleResp, err := client.Do(bundleReq)
	if err != nil {
		return ""
	}
	defer bundleResp.Body.Close()

	if bundleResp.StatusCode != http.StatusOK {
		return ""
	}

	bundleBody, err := io.ReadAll(bundleResp.Body)
	if err != nil {
		return ""
	}

	keyMatch := qobuzPrivateKeyPattern.FindStringSubmatch(string(bundleBody))
	if len(keyMatch) >= 2 {
		return strings.TrimSpace(keyMatch[1])
	}

	return ""
}

func CancelQobuzOAuth() error {
	oauthServerMu.Lock()
	defer oauthServerMu.Unlock()

	if activeOAuthServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = activeOAuthServer.Shutdown(ctx)
		activeOAuthServer = nil
	}
	return nil
}

func StartQobuzOAuth(onSuccess func(*QobuzAccount), onError func(error)) (string, error) {
	_ = CancelQobuzOAuth()

	creds, err := getQobuzAPICredentials(false)
	if err != nil {
		return "", fmt.Errorf("failed to get Qobuz API credentials: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to allocate local port for OAuth: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)
	authURL := fmt.Sprintf("https://www.qobuz.com/signin/oauth?ext_app_id=%s&redirect_url=%s",
		url.QueryEscape(creds.AppID),
		url.QueryEscape(redirectURL),
	)

	mux := http.NewServeMux()
	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	oauthServerMu.Lock()
	activeOAuthServer = server
	oauthServerMu.Unlock()

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			code = strings.TrimSpace(r.URL.Query().Get("code_autorisation"))
		}

		if code == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, qobuzOAuthErrorHTML, "No authorization code was found in the response.")
			if onError != nil {
				onError(errors.New("no authorization code received from Qobuz"))
			}
			go func() {
				time.Sleep(500 * time.Millisecond)
				_ = CancelQobuzOAuth()
			}()
			return
		}

		// Exchange code for token
		httpClient := &http.Client{Timeout: 20 * time.Second}
		privateKey := scrapeQobuzOAuthPrivateKey(httpClient)

		callbackURL := fmt.Sprintf("%s/oauth/callback", qobuzAPIBaseURL)
		reqValues := url.Values{
			"code":   {code},
			"app_id": {creds.AppID},
		}
		if privateKey != "" {
			reqValues.Set("private_key", privateKey)
		}

		exchangeReq, err := http.NewRequest(http.MethodGet, callbackURL+"?"+reqValues.Encode(), nil)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, qobuzOAuthErrorHTML, "Failed to create token exchange request.")
			if onError != nil {
				onError(err)
			}
			go func() {
				time.Sleep(500 * time.Millisecond)
				_ = CancelQobuzOAuth()
			}()
			return
		}

		exchangeReq.Header.Set("User-Agent", qobuzDefaultUA)
		exchangeReq.Header.Set("Accept", "application/json")
		exchangeReq.Header.Set("X-App-Id", creds.AppID)

		resp, err := httpClient.Do(exchangeReq)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, qobuzOAuthErrorHTML, fmt.Sprintf("Token exchange error: %v", err))
			if onError != nil {
				onError(err)
			}
			go func() {
				time.Sleep(500 * time.Millisecond)
				_ = CancelQobuzOAuth()
			}()
			return
		}
		defer resp.Body.Close()

		var cbResp qobuzOAuthCallbackResponse
		if err := json.NewDecoder(resp.Body).Decode(&cbResp); err != nil || cbResp.Token == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			msg := "Token exchange failed."
			if cbResp.Message != "" {
				msg = cbResp.Message
			}
			_, _ = fmt.Fprintf(w, qobuzOAuthErrorHTML, msg)
			if onError != nil {
				onError(errors.New(msg))
			}
			go func() {
				time.Sleep(500 * time.Millisecond)
				_ = CancelQobuzOAuth()
			}()
			return
		}

		// Retrieve full profile with token
		account, err := fetchQobuzUserProfile(cbResp.Token, creds)
		if err != nil {
			account = &QobuzAccount{
				UserAuthToken: cbResp.Token,
				Subscription:  "Active",
				Connected:     true,
			}
		}

		if err := SaveQobuzAccount(account); err != nil {
			fmt.Printf("Warning: failed to persist Qobuz account: %v\n", err)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(qobuzOAuthSuccessHTML))

		if onSuccess != nil {
			onSuccess(account)
		}

		go func() {
			time.Sleep(1 * time.Second)
			_ = CancelQobuzOAuth()
		}()
	})

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if onError != nil {
				onError(err)
			}
		}
	}()

	// Automatic timeout after 5 minutes
	go func() {
		time.Sleep(5 * time.Minute)
		_ = CancelQobuzOAuth()
	}()

	return authURL, nil
}

func (q *QobuzDownloader) getQobuzUserAccountDownloadURL(trackID int64, quality string, userAuthToken string) (string, error) {
	qualityCode := strings.TrimSpace(quality)
	switch qualityCode {
	case "5", "6", "7", "27":
	default:
		qualityCode = "6"
	}

	creds, err := getQobuzAPICredentials(false)
	if err != nil {
		return "", fmt.Errorf("failed to get Qobuz credentials: %w", err)
	}

	params := url.Values{
		"track_id":        {fmt.Sprintf("%d", trackID)},
		"format_id":       {qualityCode},
		"intent":          {"stream"},
		"user_auth_token": {userAuthToken},
	}

	req, err := newQobuzSignedRequestWithCredentials(http.MethodGet, "track/getFileUrl", params, creds)
	if err != nil {
		return "", fmt.Errorf("failed to create signed getFileUrl request: %w", err)
	}
	req.Header.Set("X-User-Auth-Token", userAuthToken)

	resp, err := q.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute getFileUrl: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errPayload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &errPayload)
		errMsg := strings.TrimSpace(errPayload.Message)
		if errMsg == "" {
			errMsg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, previewQobuzResponseBody(body, 256))
		}
		return "", fmt.Errorf("qobuz getFileUrl error: %s", errMsg)
	}

	streamURL := extractQobuzStreamingURL(body)
	if streamURL == "" {
		return "", errors.New("no streamable URL in qobuz getFileUrl response")
	}

	return streamURL, nil
}

