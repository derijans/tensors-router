package cook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFilenameOnDiskReusesTheExistingExtensionCase(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "Model.KCPPS"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	filename, err := ConfigFilenameOnDisk(configDir, "Model.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "Model.KCPPS" {
		t.Fatalf("filename %q would create a second config with id Model", filename)
	}
}

func TestConfigFilenameOnDiskKeepsTheRequestedNameWithoutAMatch(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "Other.KCPPS"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, configDir := range []string{configDir, filepath.Join(configDir, "missing")} {
		filename, err := ConfigFilenameOnDisk(configDir, "Model.kcpps")
		if err != nil {
			t.Fatal(err)
		}
		if filename != "Model.kcpps" {
			t.Fatalf("filename %q does not keep the requested name in %s", filename, configDir)
		}
	}
}
