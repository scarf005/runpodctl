package configpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestCurrentUsesXDGConfigHomeOnLinux(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("APPDATA", t.TempDir())

	path, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		want := filepath.Join(xdg, "runpod", "config.toml")
		if path != want {
			t.Fatalf("path = %q, want %q", path, want)
		}
	}
	if filepath.Base(path) != "config.toml" || filepath.Base(filepath.Dir(path)) != "runpod" {
		t.Fatalf("path = %q, want runpod/config.toml", path)
	}
}

func TestCurrentFallsBackToDefaultConfigDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", t.TempDir())

	path, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		want := filepath.Join(home, ".config", "runpod", "config.toml")
		if path != want {
			t.Fatalf("path = %q, want %q", path, want)
		}
	}
}

func TestCurrentUsesNativePlatformConfigDirectory(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	appData := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("APPDATA", appData)

	path, err := Current()
	if err != nil {
		t.Fatal(err)
	}

	var want string
	switch runtime.GOOS {
	case "darwin":
		want = filepath.Join(home, "Library", "Application Support", "runpod", "config.toml")
	case "windows":
		want = filepath.Join(appData, "runpod", "config.toml")
	default:
		t.Skipf("native path assertion is platform-specific (running on %s)", runtime.GOOS)
	}
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestLoadPrefersCurrentConfig(t *testing.T) {
	home, _ := isolatedConfigEnv(t)
	current := currentConfigPath(t)
	legacy := filepath.Join(home, ".runpod", "config.toml")
	writeFile(t, current, "apiKey = \"current-key\"\n")
	writeFile(t, legacy, "apiKey = \"legacy-key\"\n")

	v := viper.New()
	result, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if result.Legacy || result.Path != current {
		t.Fatalf("result = %+v, want current config", result)
	}
	if got := v.GetString("apiKey"); got != "current-key" {
		t.Fatalf("api key = %q, want current-key", got)
	}
}

func TestLoadFallsBackToLegacyWithoutWritingIt(t *testing.T) {
	home, _ := isolatedConfigEnv(t)
	legacy := filepath.Join(home, ".runpod", "config.toml")
	contents := "apiKey = \"legacy-key\"\n"
	writeFile(t, legacy, contents)

	v := viper.New()
	result, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Legacy || result.Path != legacy {
		t.Fatalf("result = %+v, want legacy config", result)
	}
	if got := v.GetString("apiKey"); got != "legacy-key" {
		t.Fatalf("api key = %q, want legacy-key", got)
	}
	if got := readFile(t, legacy); got != contents {
		t.Fatalf("legacy config changed: %q", got)
	}
}

func TestLoadDoesNotFallbackFromMalformedCurrentConfig(t *testing.T) {
	home, _ := isolatedConfigEnv(t)
	current := currentConfigPath(t)
	legacy := filepath.Join(home, ".runpod", "config.toml")
	bad := "apiKey = [not valid toml\n"
	writeFile(t, current, bad)
	writeFile(t, legacy, "apiKey = \"legacy-key\"\n")

	v := viper.New()
	_, err := Load(v)
	if err == nil {
		t.Fatal("expected malformed current config to fail")
	}
	if !strings.Contains(err.Error(), current) {
		t.Fatalf("error = %q, want current path", err)
	}
	if got := readFile(t, current); got != bad {
		t.Fatalf("malformed current config changed: %q", got)
	}
	if got := v.GetString("apiKey"); got != "" {
		t.Fatalf("api key = %q, want no fallback value", got)
	}
}

func TestLoadFallsBackToLegacyYAML(t *testing.T) {
	home, _ := isolatedConfigEnv(t)
	legacy := filepath.Join(home, ".runpod.yaml")
	writeFile(t, legacy, "apiKey: legacy-key\n")

	v := viper.New()
	result, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Legacy || result.Path != legacy {
		t.Fatalf("result = %+v, want legacy yaml config", result)
	}
	if got := v.GetString("apiKey"); got != "legacy-key" {
		t.Fatalf("api key = %q, want legacy-key", got)
	}
}

func TestLoadDoesNotCreateCurrentConfigForMalformedLegacy(t *testing.T) {
	home, _ := isolatedConfigEnv(t)
	legacy := filepath.Join(home, ".runpod.yaml")
	writeFile(t, legacy, "apiKey: [not valid yaml\n")

	v := viper.New()
	_, err := Load(v)
	if err == nil {
		t.Fatal("expected malformed legacy config to fail")
	}
	current := currentConfigPath(t)
	if _, statErr := os.Stat(current); !os.IsNotExist(statErr) {
		t.Fatalf("current config was created after legacy read failure: %v", statErr)
	}
}

func TestLoadExistingDoesNotCreateConfig(t *testing.T) {
	_, _ = isolatedConfigEnv(t)

	result, err := LoadExisting(viper.New())
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("result = %+v, want no config", result)
	}
	current := currentConfigPath(t)
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("current config was created: %v", err)
	}
}

func TestLoadExistingRejectsMalformedCurrentConfig(t *testing.T) {
	home, _ := isolatedConfigEnv(t)
	current := currentConfigPath(t)
	writeFile(t, current, "apiKey = [not valid toml\n")
	writeFile(t, filepath.Join(home, ".runpod.yaml"), "apiKey: legacy-key\n")

	_, err := LoadExisting(viper.New())
	if err == nil {
		t.Fatal("expected malformed current config to fail")
	}
	if !strings.Contains(err.Error(), current) {
		t.Fatalf("error = %q, want current path", err)
	}
}

func TestSaveRefusesMalformedCurrentConfig(t *testing.T) {
	_, _ = isolatedConfigEnv(t)
	current := currentConfigPath(t)
	bad := "apiKey = [not valid toml\n"
	writeFile(t, current, bad)

	v := viper.New()
	v.Set("apiKey", "new-key")
	if _, err := Save(v); err == nil {
		t.Fatal("expected save to reject malformed current config")
	}
	if got := readFile(t, current); got != bad {
		t.Fatalf("malformed current config changed: %q", got)
	}
}

func TestSavePreservesExplicitOverrideWithExistingCurrentConfig(t *testing.T) {
	_, _ = isolatedConfigEnv(t)
	current := currentConfigPath(t)
	writeFile(t, current, "apiKey = \"old-key\"\n")

	v := viper.New()
	v.Set("apiKey", "new-key")
	if _, err := Save(v); err != nil {
		t.Fatal(err)
	}
	if got := viperValue(t, current, "apiKey"); got != "new-key" {
		t.Fatalf("saved api key = %q, want new-key", got)
	}
}

func TestSaveMigratesWithoutOverwritingLegacy(t *testing.T) {
	home, _ := isolatedConfigEnv(t)
	legacy := filepath.Join(home, ".runpod", "config.toml")
	contents := "apiKey = \"legacy-key\"\n"
	writeFile(t, legacy, contents)

	v := viper.New()
	if _, err := Load(v); err != nil {
		t.Fatal(err)
	}
	v.Set("apiKey", "new-key")
	current, err := Save(v)
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, legacy); got != contents {
		t.Fatalf("legacy config changed: %q", got)
	}
	if got := viperValue(t, current, "apiKey"); got != "new-key" {
		t.Fatalf("saved api key = %q, want new-key", got)
	}
	if mode := fileMode(t, current); runtime.GOOS != "windows" && mode.Perm() != 0600 {
		t.Fatalf("config mode = %o, want 600", mode.Perm())
	}
}

func isolatedConfigEnv(t *testing.T) (home, xdg string) {
	t.Helper()
	home = t.TempDir()
	xdg = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APIKEY", "")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("APPDATA", t.TempDir())
	return home, xdg
}

func currentConfigPath(t *testing.T) string {
	t.Helper()
	path, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

func viperValue(t *testing.T, path, key string) string {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	if err := v.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	return v.GetString(key)
}
