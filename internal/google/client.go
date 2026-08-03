package google

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	oauthgoogle "golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/slides/v1"
)

type Client struct {
	slides *slides.Service
	drive  *drive.Service
}

var profilePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]*$`)

func New(ctx context.Context, profile string) (*Client, error) {
	httpClient, err := httpClient(ctx, profile)
	if err != nil {
		return nil, err
	}
	slidesService, err := slides.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}
	driveService, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}
	return &Client{slides: slidesService, drive: driveService}, nil
}

func httpClient(ctx context.Context, profile string) (*http.Client, error) {
	if accessToken := os.Getenv("GSLIDEV_ACCESS_TOKEN"); accessToken != "" {
		return retryClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken}))), nil
	}

	credentialsPath, tokenPath, err := credentialPaths(profile)
	if err != nil {
		return nil, err
	}
	credentials, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read OAuth credentials %s: %w", credentialsPath, err)
	}
	cfg, err := oauthgoogle.ConfigFromJSON(credentials, slides.PresentationsScope, slides.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("parse OAuth credentials: %w", err)
	}

	token, err := readToken(tokenPath)
	if err == nil && token.Valid() && time.Until(token.Expiry) > time.Minute {
		return retryClient(cfg.Client(ctx, token)), nil
	}
	if err == nil && token.RefreshToken != "" {
		refreshed, refreshErr := cfg.TokenSource(ctx, token).Token()
		if refreshErr == nil {
			if err := writeToken(tokenPath, refreshed); err != nil {
				return nil, err
			}
			return retryClient(cfg.Client(ctx, refreshed)), nil
		}
	}

	newToken, err := authorize(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := writeToken(tokenPath, newToken); err != nil {
		return nil, err
	}
	return retryClient(cfg.Client(ctx, newToken)), nil
}

func retryClient(client *http.Client) *http.Client {
	retry := retryablehttp.NewClient()
	retry.HTTPClient = client
	retry.RetryMax = 8
	retry.RetryWaitMin = time.Second
	retry.RetryWaitMax = 20 * time.Second
	return retry.StandardClient()
}

func credentialPaths(profile string) (string, string, error) {
	if !profilePattern.MatchString(profile) {
		return "", "", fmt.Errorf("invalid profile name %q", profile)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	dataDir := filepath.Join(dataHome, "gslidev")
	stateDir := filepath.Join(stateHome, "gslidev")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", "", err
	}
	for _, app := range []struct {
		dataDir  string
		stateDir string
	}{
		{dataDir: dataDir, stateDir: stateDir},
		{dataDir: filepath.Join(dataHome, "deck"), stateDir: filepath.Join(stateHome, "deck")},
	} {
		credentials := filepath.Join(app.dataDir, "credentials.json")
		token := filepath.Join(app.stateDir, "token.json")
		if profile != "" {
			profileCredentials := filepath.Join(app.dataDir, "credentials-"+profile+".json")
			if _, err := os.Stat(profileCredentials); err == nil {
				credentials = profileCredentials
			}
			token = filepath.Join(app.stateDir, "token-"+profile+".json")
		}
		if _, err := os.Stat(credentials); err == nil {
			return credentials, token, nil
		}
	}
	return filepath.Join(dataDir, "credentials.json"), filepath.Join(stateDir, "token.json"), nil
}

func readToken(path string) (*oauth2.Token, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	token := new(oauth2.Token)
	if err := json.NewDecoder(file).Decode(token); err != nil {
		return nil, err
	}
	return token, nil
}

func writeToken(path string, token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".token-*")
	if err != nil {
		return fmt.Errorf("create token cache: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		temporary.Close()
		os.Remove(temporaryName)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write token cache: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace token cache: %w", err)
	}
	return nil
}

func authorize(ctx context.Context, cfg *oauth2.Config) (*oauth2.Token, error) {
	verifierBytes := make([]byte, 64)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	stateBytes := make([]byte, 24)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, err
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for OAuth callback: %w", err)
	}
	defer listener.Close()
	cfg.RedirectURL = "http://" + listener.Addr().String() + "/"

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	server := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "invalid OAuth state", http.StatusBadRequest)
			return
		}
		if oauthError := r.URL.Query().Get("error"); oauthError != "" {
			errCh <- fmt.Errorf("OAuth authorization failed: %s", oauthError)
			_, _ = w.Write([]byte("Authorization failed. You may close this tab."))
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "authorization code is missing", http.StatusBadRequest)
			return
		}
		codeCh <- code
		_, _ = w.Write([]byte("Authorization complete. You may close this tab."))
	})
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"))
	if err := browser.OpenURL(authURL); err != nil {
		_ = server.Shutdown(ctx)
		return nil, fmt.Errorf("open OAuth browser: %w", err)
	}

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		_ = server.Shutdown(ctx)
		return nil, err
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return nil, ctx.Err()
	}
	if err := server.Shutdown(ctx); err != nil {
		return nil, err
	}
	token, err := cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange OAuth code: %w", err)
	}
	return token, nil
}
