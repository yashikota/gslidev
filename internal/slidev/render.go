package slidev

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

type RenderOptions struct {
	ChromePath string
	SlidevPath string
}

type mediaSnapshot struct {
	Kind   string  `json:"kind"`
	Source string  `json:"source"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type pageSnapshot struct {
	Width  float64         `json:"width"`
	Height float64         `json:"height"`
	Media  []mediaSnapshot `json:"media"`
}

// RenderFile renders every Slidev page. The source file is never modified.
func RenderFile(ctx context.Context, input string, deck *Deck, opts RenderOptions) error {
	slidevPath := opts.SlidevPath
	if slidevPath == "" {
		slidevPath = os.Getenv("GSLIDEV_SLIDEV")
	}
	if slidevPath == "" {
		slidevPath = findSlidev(input)
	}
	if slidevPath == "" {
		return fmt.Errorf("slidev executable not found; install Slidev, set --slidev-path, or add @slidev/cli to the project")
	}
	absoluteInput, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, slidevPath, filepath.Base(absoluteInput), "--port", strconv.Itoa(port), "--host", "127.0.0.1")
	cmd.Dir = filepath.Dir(absoluteInput)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start slidev: %w", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	if err := waitForServer(ctx, baseURL); err != nil {
		return err
	}

	allocatorOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocatorOptions = append(allocatorOptions, chromedp.Headless, chromedp.NoSandbox, chromedp.DisableGPU)
	if opts.ChromePath != "" {
		allocatorOptions = append(allocatorOptions, chromedp.ExecPath(opts.ChromePath))
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	if err := chromedp.Run(browserCtx, emulation.SetDeviceMetricsOverride(1920, 1080, 1, false)); err != nil {
		return fmt.Errorf("configure Chrome: %w", err)
	}

	for i := range deck.Slides {
		pageCtx, cancelPage := context.WithTimeout(browserCtx, 60*time.Second)
		var snap pageSnapshot
		var screenshot []byte
		actions := chromedp.Tasks{
			chromedp.Navigate(fmt.Sprintf("%s/%d", baseURL, i+1)),
			chromedp.WaitReady("body"),
			chromedp.Sleep(500 * time.Millisecond),
			chromedp.Evaluate(preparePageScript, &snap),
			chromedp.Screenshot(`[data-gslidev-root="true"]`, &screenshot, chromedp.NodeVisible),
		}
		if err := chromedp.Run(pageCtx, actions); err != nil {
			cancelPage()
			return fmt.Errorf("capture slide %d: %w", i+1, err)
		}
		cancelPage()
		if snap.Width <= 0 || snap.Height <= 0 {
			return fmt.Errorf("capture slide %d: slide root was not found", i+1)
		}
		deck.Slides[i].Screenshot = screenshot
		for _, media := range snap.Media {
			if media.Kind != "gif" {
				continue
			}
			data, mime, err := fetchMedia(ctx, media.Source)
			if err != nil {
				return fmt.Errorf("fetch GIF on slide %d: %w", i+1, err)
			}
			deck.Slides[i].GIFs = append(deck.Slides[i].GIFs, Media{
				Data: data, MIME: mime, Source: media.Source,
				X: media.X / snap.Width, Y: media.Y / snap.Height,
				Width: media.Width / snap.Width, Height: media.Height / snap.Height,
			})
		}
	}
	return nil
}

func findSlidev(input string) string {
	if path, err := exec.LookPath("slidev"); err == nil {
		return path
	}
	absoluteInput, err := filepath.Abs(input)
	if err != nil {
		return ""
	}
	local := filepath.Join(filepath.Dir(absoluteInput), "node_modules", ".bin", "slidev")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return ""
}

const preparePageScript = `(() => {
  const root = document.querySelector('.slidev-page') ||
    document.querySelector('[data-slidev-slide]') ||
    document.querySelector('#page-root');
  if (!root) throw new Error('Slidev page root not found');
  document.querySelectorAll('[data-gslidev-root="true"]').forEach((el) => el.removeAttribute('data-gslidev-root'));
  root.setAttribute('data-gslidev-root', 'true');
  const rootRect = root.getBoundingClientRect();
  const media = [];
  root.querySelectorAll('img, video').forEach((el) => {
    const source = el.currentSrc || el.src || el.getAttribute('src') || '';
    const gif = el.tagName === 'IMG' && (/^data:image\/gif/i.test(source) || /\.gif(?:[?#]|$)/i.test(source));
    const video = el.tagName === 'VIDEO';
    if (!gif && !video) return;
    const rect = el.getBoundingClientRect();
    media.push({kind: gif ? 'gif' : 'video', source, x: rect.left - rootRect.left, y: rect.top - rootRect.top, width: rect.width, height: rect.height});
    el.style.visibility = 'hidden';
  });
  return {width: rootRect.width, height: rootRect.height, media};
})()`

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForServer(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode < 500 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for Slidev at %s", baseURL)
		case <-ticker.C:
		}
	}
}

func fetchMedia(ctx context.Context, source string) ([]byte, string, error) {
	if strings.HasPrefix(source, "data:") {
		return decodeDataURL(source)
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme == "" {
		return nil, "", fmt.Errorf("invalid media URL %q", source)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("media URL returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 50<<20))
	if err != nil {
		return nil, "", err
	}
	mime := strings.Split(response.Header.Get("Content-Type"), ";")[0]
	if mime == "" {
		mime = "image/gif"
	}
	return data, mime, nil
}

func decodeDataURL(source string) ([]byte, string, error) {
	parts := strings.SplitN(source, ",", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid data URL")
	}
	header, data := parts[0], parts[1]
	mime := strings.TrimPrefix(strings.TrimPrefix(header, "data:"), " ")
	if !strings.Contains(mime, ";base64") {
		return []byte(data), mime, nil
	}
	mime = strings.TrimSuffix(mime, ";base64")
	decoded, err := base64.StdEncoding.DecodeString(data)
	return decoded, mime, err
}
