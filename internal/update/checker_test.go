package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func fakeReleaseJSON(tag string, assets ...Asset) string {
	body := fmt.Sprintf(`{"tag_name":%q,"name":%q,"body":"release notes","html_url":"https://example.com/releases/tag/%s","published_at":"2026-09-02T00:00:00Z"}`, tag, tag, tag)
	if len(assets) > 0 {
		body = body[:len(body)-1] + `,"assets":[`
		for i, a := range assets {
			if i > 0 {
				body += ","
			}
			body += fmt.Sprintf(`{"name":%q,"size":%d,"browser_download_url":%q}`, a.Name, a.Size, a.BrowserDownloadURL)
		}
		body += "]}"
	}
	return body
}

func TestFetchLatestAndArchiveAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fakeReleaseJSON("v0.1.0",
			Asset{Name: "edgeCore-0.1.0-linux-arm64.tar.gz", Size: 1000, BrowserDownloadURL: "https://dl/x"},
			Asset{Name: "SHA256SUMS", Size: 10, BrowserDownloadURL: "https://dl/sums"},
		)))
	}))
	defer srv.Close()

	c := NewChecker(srv.URL)
	rel, err := c.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if rel.TagName != "v0.1.0" {
		t.Errorf("tag=%q", rel.TagName)
	}
	if Euclidean := FindAsset(rel, "SHA256SUMS"); Euclidean == nil {
		t.Errorf("SHA256SUMS asset not found")
	}
}

func TestVerifySHA256FromFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello upgrade")
	sumPath := filepath.Join(dir, "SHA256SUMS")
	archPath := filepath.Join(dir, "edgeCore-0.1.0-linux-arm64.tar.gz")
	if err := os.WriteFile(archPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	line := fmt.Sprintf("%s  edgeCore-0.1.0-linux-arm64.tar.gz\n%s  other\n", hex.EncodeToString(digest[:]), strings_Repeat("0", 64))
	if err := os.WriteFile(sumPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := verifySHA256FromFile(sumPath, archPath, "edgeCore-0.1.0-linux-arm64.tar.gz")
	if err != nil {
		t.Fatalf("verify err: %v", err)
	}
	if !ok {
		t.Errorf("expected valid sha256 match")
	}
}

func strings_Repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}