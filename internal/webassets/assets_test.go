package webassets

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEmbeddedAssetsMatchDeterministicWebBuild(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not available")
	}

	repoRoot := mustRepoRoot(t)
	tempRoot := t.TempDir()
	buildScript := filepath.ToSlash(filepath.Join(repoRoot, "web", "scripts", "web-assets.mjs"))

	cmd := exec.Command(
		"node",
		"--no-warnings",
		"--input-type=module",
		"-e",
		fmt.Sprintf(`const { buildWebAssets } = await import(%q); await buildWebAssets(process.argv[1]);`, "file://"+buildScript),
		tempRoot,
	)
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("web build failed: %v\n%s", err, output)
	}

	for _, name := range []string{"index.html", "assets/waypoint.css", "assets/waypoint.js"} {
		generated, err := os.ReadFile(filepath.Join(tempRoot, name))
		if err != nil {
			t.Fatalf("read generated %s: %v", name, err)
		}
		embedded, err := fs.ReadFile(Assets, filepath.ToSlash(filepath.Join("dist", name)))
		if err != nil {
			t.Fatalf("read embedded %s: %v", name, err)
		}
		if string(generated) != string(embedded) {
			t.Fatalf("%s drifted from embedded output", name)
		}
	}
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
