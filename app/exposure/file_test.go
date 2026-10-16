package exposure

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUnitEncodeGolden(t *testing.T) {
	f := &File{
		Name: "demo",
		Exposures: []Exposure{
			{
				Name:        "beta_note",
				Type:        "notebook",
				URL:         "https://example.com/b",
				Label:       "Beta",
				Description: "line one\nline two\n",
				OwnerName:   "analytics",
				OwnerEmail:  "@analytics",
			},
			{
				Name:        "alpha_board",
				Type:        "dashboard",
				URL:         "https://example.com/a",
				Label:       "Alpha",
				Description: "short desc",
				OwnerName:   "analytics",
				OwnerEmail:  "@analytics",
				DependsOn:   []string{"ref('orders_day')", "source('raw', 'clicks')"},
				Maturity:    "high",
				Tags:        []string{"qlik", "prod"},
			},
		},
	}
	got, err := f.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "exposure.golden.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !yamlEqual(t, got, want) {
		t.Fatalf("YAML mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestUnitEncodeOmitsEmptyMaturityTags(t *testing.T) {
	f := &File{
		Name: "x",
		Exposures: []Exposure{{
			Name: "solo", Type: "analysis", URL: "u", Label: "l",
			Description: "d", OwnerName: "n", OwnerEmail: "e",
		}},
	}
	raw, err := f.Encode()
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if bytes.Contains(raw, []byte("maturity:")) {
		t.Fatalf("unexpected maturity:\n%s", s)
	}
	if bytes.Contains(raw, []byte("tags:")) {
		t.Fatalf("unexpected tags:\n%s", s)
	}
}

func TestUnitWrite(t *testing.T) {
	dir := t.TempDir()
	f := &File{
		Name: "stream_demo",
		Exposures: []Exposure{{
			Name: "solo", Type: "dashboard", URL: "u", Label: "l",
			Description: "d", OwnerName: "n", OwnerEmail: "e",
			DependsOn: []string{"ref('orders_day')"},
		}},
	}
	path, err := f.Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(dir, "stream_demo.yml")
	if path != wantPath {
		t.Fatalf("path %q want %q", path, wantPath)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["version"] != 2 {
		t.Fatalf("version = %v", doc["version"])
	}
}

func yamlEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var da, db any
	if err := yaml.Unmarshal(a, &da); err != nil {
		t.Fatalf("unmarshal a: %v", err)
	}
	if err := yaml.Unmarshal(b, &db); err != nil {
		t.Fatalf("unmarshal b: %v", err)
	}
	ra, err := yaml.Marshal(da)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := yaml.Marshal(db)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(ra, rb)
}
