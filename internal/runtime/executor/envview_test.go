package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadEnvView_FileFillsGaps(t *testing.T) {
	p := writeEnvFile(t, "FROM_FILE_ONLY=filevalue\n")
	v, missing, err := loadEnvView(p)
	if err != nil || missing {
		t.Fatalf("loadEnvView: missing=%v err=%v", missing, err)
	}
	if got, ok := v.lookup("FROM_FILE_ONLY"); !ok || got != "filevalue" {
		t.Errorf("lookup = %q,%v; want filevalue,true", got, ok)
	}
	if v.source != p {
		t.Errorf("source = %q, want %q", v.source, p)
	}
}

func TestLoadEnvView_HostWins(t *testing.T) {
	t.Setenv("ENVVIEW_CLASH", "fromhost")
	p := writeEnvFile(t, "ENVVIEW_CLASH=fromfile\n")
	v, _, err := loadEnvView(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, shadowed := v.ambient["ENVVIEW_CLASH"]; shadowed {
		t.Error("host-set var must not enter the ambient map")
	}
	if got, _ := v.lookup("ENVVIEW_CLASH"); got != "fromhost" {
		t.Errorf("lookup = %q, want fromhost", got)
	}
}

func TestLoadEnvView_AbsentFileDegrades(t *testing.T) {
	v, missing, err := loadEnvView(filepath.Join(t.TempDir(), "no-such.env"))
	if err != nil {
		t.Fatalf("absent file must not error: %v", err)
	}
	if !missing {
		t.Error("missing flag should be true")
	}
	if _, ok := v.lookup("ANYTHING_AT_ALL_XYZ"); ok {
		t.Error("empty view resolved a name")
	}
}

func TestLoadEnvView_NoPathDeclared(t *testing.T) {
	v, missing, err := loadEnvView("")
	if err != nil || missing {
		t.Fatalf("empty path: missing=%v err=%v", missing, err)
	}
	if v.source != "" {
		t.Errorf("source = %q, want empty", v.source)
	}
}

func TestLoadEnvView_UnparseableErrors(t *testing.T) {
	// godotenv returns an error for an unterminated quoted value.
	// Content: KEY="unterminated  — verified empirically to produce
	// "unterminated quoted value" from godotenv v1.5.1.
	p := writeEnvFile(t, "KEY=\"unterminated")
	if _, _, err := loadEnvView(p); err == nil {
		t.Error("unparseable env_file must error")
	}
}
