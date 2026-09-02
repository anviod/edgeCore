package update

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeTestTar(t *testing.T, path string, binName string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho edgeCore test binary\n")
	if err := tw.WriteHeader(&tar.Header{Name: binName, Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeHeadFile(t *testing.T, path string, head []byte) {
	t.Helper()
	if err := os.WriteFile(path, head, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseLocalName(t *testing.T) {
	cases := []struct {
		name, format string
		wantVer      string
		wantArch     string
		wantOK       bool
	}{
		{"edgeCore-0.1.0-linux-arm64.tar.gz", "tar.gz", "0.1.0", "arm64", true},
		{"edgeCore-0.1.0-linux-amd64.tar.gz", "tar.gz", "0.1.0", "amd64", true},
		{"edgeCore-v0.1.0-arm64.deb", "deb", "0.1.0", "arm64", true},
		{"edgeCore-v0.1.0-arm64.rpm", "rpm", "0.1.0", "arm64", true},
		{"edgeCore-v0.1.0-SNAPSHOT-a02b75d8-arm64.deb", "deb", "0.1.0-SNAPSHOT-a02b75d8", "arm64", true},
		{"random-file.txt", "txt", "", "", false},
		{"edgeCore-0.1.0-linux-arm64.zip", "tar.gz", "", "", false},
		{"edgeCore-0.1.0-arm64.exe", "deb", "", "", false},
	}
	for _, c := range cases {
		ver, arch, ok := parseLocalName(c.name, c.format)
		if ok != c.wantOK || ver != c.wantVer || arch != c.wantArch {
			t.Errorf("parseLocalName(%q,%q)=%q,%q,%v want %q,%q,%v",
				c.name, c.format, ver, arch, ok, c.wantVer, c.wantArch, c.wantOK)
		}
	}
}

func TestValidateLocalTarGz(t *testing.T) {
	m := NewManager("", "", nil)
	dir := t.TempDir()
	p := filepath.Join(dir, "edgeCore-0.1.0-linux-amd64.tar.gz")

	writeTestTar(t, p, "edgeCore")
	pkg, err := m.ValidateLocal(p)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Compatible != packageArchCompat("amd64") {
		t.Fatalf("compatible mismatch: got %v, packageArchCompat(amd64)=%v (platform %s/%s)",
			pkg.Compatible, packageArchCompat("amd64"), runtime.GOOS, runtime.GOARCH)
	}
	if pkg.Version != "0.1.0" || pkg.Format != "tar.gz" {
		t.Fatalf("unexpected pkg: %+v", pkg)
	}
}

func TestValidateLocalInvalidContent(t *testing.T) {
	m := NewManager("", "", nil)
	dir := t.TempDir()
	p := filepath.Join(dir, "edgeCore-0.1.0-linux-amd64.tar.gz")
	// 写一个非法的 gzip
	if err := os.WriteFile(p, []byte("not a gzip file at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg, err := m.ValidateLocal(p)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Compatible {
		t.Fatalf("expected incompatible for bad gzip, got %+v", pkg)
	}
}

func TestValidateLocalDebRpmMagic(t *testing.T) {
	m := NewManager("", "", nil)
	dir := t.TempDir()

	deb := filepath.Join(dir, "edgeCore-v0.1.0-arm64.deb")
	writeHeadFile(t, deb, []byte("!<arch>\n"))
	p, err := m.ValidateLocal(deb)
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != "deb" || p.Version != "0.1.0" || p.Arch != "arm64" {
		t.Fatalf("unexpected deb pkg: %+v", p)
	}

	rpm := filepath.Join(dir, "edgeCore-v0.1.0-arm64.rpm")
	writeHeadFile(t, rpm, []byte{0xED, 0xAB, 0xEE, 0xDB})
	p2, err := m.ValidateLocal(rpm)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Format != "rpm" {
		t.Fatalf("unexpected rpm pkg: %+v", p2)
	}
}

func TestArchIncompatibleFilenameStillParsed(t *testing.T) {
	m := NewManager("", "", nil)
	dir := t.TempDir()
	// arm7 包在当前常见平台（amd64/arm64）下不兼容，但文件名应仍可解析出版本与架构
	p := filepath.Join(dir, "edgeCore-0.1.0-linux-arm7.tar.gz")
	writeTestTar(t, p, "edgeCore")
	pkg, err := m.ValidateLocal(p)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Version != "0.1.0" || pkg.Format != "tar.gz" || pkg.Arch != "arm7" {
		t.Fatalf("unexpected pkg: %+v", pkg)
	}
	if pkg.Compatible != packageArchCompat("arm7") {
		t.Fatalf("compatible mismatch: %v vs %v", pkg.Compatible, packageArchCompat("arm7"))
	}
	if !pkg.Compatible && pkg.Reason == "" {
		t.Fatal("incompatible package should carry a reason")
	}
}