package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// createTestLayer creates a v1.Layer from a map of filename to content.
func createTestLayer(t *testing.T, fileContents map[string]string) v1.Layer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range fileContents {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	layer, err := tarball.LayerFromReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return layer
}

// createTestImage creates a v1.Image with the given layers.
func createTestImage(t *testing.T, layers ...v1.Layer) v1.Image {
	t.Helper()
	img, err := mutate.AppendLayers(empty.Image, layers...)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestInspectImage(t *testing.T) {
	layer1 := createTestLayer(t, map[string]string{"a.txt": "hello"})
	layer2 := createTestLayer(t, map[string]string{"b.txt": "world"})
	img := createTestImage(t, layer1, layer2)

	var buf bytes.Buffer
	cfg := &config{out: &buf}

	if err := inspectImage(cfg, img); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Header + 2 layer lines
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), output)
	}
	if !strings.Contains(lines[0], "N") || !strings.Contains(lines[0], "Layer") || !strings.Contains(lines[0], "Size") {
		t.Errorf("unexpected header: %s", lines[0])
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[1]), "1") {
		t.Errorf("expected line 1 to start with '1': %s", lines[1])
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[2]), "2") {
		t.Errorf("expected line 2 to start with '2': %s", lines[2])
	}
}

func TestFiles(t *testing.T) {
	layer := createTestLayer(t, map[string]string{
		"app/main.go":  "package main",
		"app/utils.go": "package utils",
		"README.md":    "# Project",
	})

	var buf bytes.Buffer
	cfg := &config{out: &buf}

	if err := files(cfg, layer); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	for _, name := range []string{"app/main.go", "app/utils.go", "README.md"} {
		if !strings.Contains(output, name) {
			t.Errorf("expected %q in output:\n%s", name, output)
		}
	}
	if !strings.Contains(output, "Mode") || !strings.Contains(output, "Size") || !strings.Contains(output, "Name") {
		t.Errorf("expected header in output:\n%s", output)
	}
}

func TestFilesSorted(t *testing.T) {
	layer := createTestLayer(t, map[string]string{
		"small.txt":  "x",
		"medium.txt": "xxxx",
		"large.txt":  strings.Repeat("x", 100),
	})

	var buf bytes.Buffer
	cfg := &config{out: &buf, sort: true}

	if err := files(cfg, layer); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	largeIdx := strings.Index(output, "large.txt")
	mediumIdx := strings.Index(output, "medium.txt")
	smallIdx := strings.Index(output, "small.txt")

	if largeIdx < 0 || mediumIdx < 0 || smallIdx < 0 {
		t.Fatalf("missing files in output:\n%s", output)
	}
	if !(largeIdx < mediumIdx && mediumIdx < smallIdx) {
		t.Errorf("files not sorted by size (large=%d, medium=%d, small=%d):\n%s",
			largeIdx, mediumIdx, smallIdx, output)
	}
}

func TestExtractToStdout(t *testing.T) {
	layer := createTestLayer(t, map[string]string{
		"hello.txt": "hello world",
	})
	img := createTestImage(t, layer)

	var buf bytes.Buffer
	cfg := &config{
		files: []string{"hello.txt"},
		out:   &buf,
	}

	if err := extractFromImage(cfg, img); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

func TestExtractLastLayerWins(t *testing.T) {
	layer1 := createTestLayer(t, map[string]string{
		"config.json": `{"version": 1}`,
	})
	layer2 := createTestLayer(t, map[string]string{
		"config.json": `{"version": 2}`,
	})
	img := createTestImage(t, layer1, layer2)

	var buf bytes.Buffer
	cfg := &config{
		files: []string{"config.json"},
		out:   &buf,
	}

	if err := extractFromImage(cfg, img); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); got != `{"version": 2}` {
		t.Errorf("expected version 2 from last layer, got %q", got)
	}
}

func TestExtractFileNotFound(t *testing.T) {
	layer := createTestLayer(t, map[string]string{
		"exists.txt": "content",
	})
	img := createTestImage(t, layer)

	var buf bytes.Buffer
	cfg := &config{
		files: []string{"nonexistent.txt"},
		out:   &buf,
	}

	err := extractFromImage(cfg, img)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestExtractToOutputDir(t *testing.T) {
	layer := createTestLayer(t, map[string]string{
		"app/config.yaml": "key: value",
	})
	img := createTestImage(t, layer)

	dir := t.TempDir()
	cfg := &config{
		files:     []string{"app/config.yaml"},
		outputDir: dir,
		out:       &bytes.Buffer{},
	}

	if err := extractFromImage(cfg, img); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "app", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "key: value" {
		t.Errorf("expected %q, got %q", "key: value", string(data))
	}
}

func TestLsImage(t *testing.T) {
	layer1 := createTestLayer(t, map[string]string{"layer1.txt": "a"})
	layer2 := createTestLayer(t, map[string]string{"layer2.txt": "b"})
	img := createTestImage(t, layer1, layer2)

	var buf bytes.Buffer
	cfg := &config{out: &buf}

	if err := lsImage(cfg, img); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "layer1.txt") {
		t.Errorf("expected layer1.txt in output:\n%s", output)
	}
	if !strings.Contains(output, "layer2.txt") {
		t.Errorf("expected layer2.txt in output:\n%s", output)
	}
}

func TestLsImageWithLayerID(t *testing.T) {
	layer1 := createTestLayer(t, map[string]string{"layer1.txt": "a"})
	layer2 := createTestLayer(t, map[string]string{"layer2.txt": "b"})
	img := createTestImage(t, layer1, layer2)

	var buf bytes.Buffer
	cfg := &config{
		layerIDs: []string{"1"},
		out:      &buf,
	}

	if err := lsImage(cfg, img); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "layer1.txt") {
		t.Errorf("expected layer1.txt in output:\n%s", output)
	}
	if strings.Contains(output, "layer2.txt") {
		t.Errorf("did not expect layer2.txt for layer 1 only:\n%s", output)
	}
}
