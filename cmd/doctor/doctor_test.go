package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/runpod/runpodctl/internal/configenv"
	"github.com/runpod/runpodctl/internal/configpath"

	"github.com/spf13/viper"
)

func TestCheckAPIKeyMigratesLegacyConfig(t *testing.T) {
	tests := []struct {
		name         string
		path         func(home string) string
		contents     string
		storedKey    string
		environment  string
		automaticEnv string
	}{
		{
			name:        "toml",
			path:        func(home string) string { return filepath.Join(home, ".runpod", "config.toml") },
			contents:    "apiKey = \"legacy-toml-key\"\n",
			storedKey:   "legacy-toml-key",
			environment: "environment-override-key",
		},
		{
			name:         "toml with automatic env override",
			path:         func(home string) string { return filepath.Join(home, ".runpod", "config.toml") },
			contents:     "apiKey = \"legacy-key\"\n",
			storedKey:    "legacy-key",
			automaticEnv: "environment-only-key",
		},
		{
			name:      "yaml",
			path:      func(home string) string { return filepath.Join(home, ".runpod.yaml") },
			contents:  "apiKey: legacy-yaml-key\n",
			storedKey: "legacy-yaml-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home, _ := isolatedDoctorConfigEnv(t)
			resetDoctorViper(t)
			t.Setenv(configenv.APIKeyEnv, tt.environment)
			t.Setenv("APIKEY", tt.automaticEnv)

			legacy := tt.path(home)
			writeDoctorFile(t, legacy, tt.contents)
			if _, err := configpath.Load(viper.GetViper()); err != nil {
				t.Fatal(err)
			}

			result := checkAPIKey()
			if result.Status != "pass" || !result.Fixed {
				t.Fatalf("result = %+v, want migrated pass", result)
			}
			current, err := configpath.Current()
			if err != nil {
				t.Fatal(err)
			}
			if got := readDoctorFile(t, legacy); got != tt.contents {
				t.Fatalf("legacy config changed: %q", got)
			}
			if got := doctorConfigValue(t, current); got != tt.storedKey {
				t.Fatalf("current api key = %q, want %q", got, tt.storedKey)
			}
			if tt.automaticEnv != "" && configenv.APIKey() != tt.automaticEnv {
				t.Fatal("migration changed the active environment credential")
			}
		})
	}
}

func TestCheckAPIKeyDoesNotMigrateEnvironmentOnlyLegacyCredential(t *testing.T) {
	home, _ := isolatedDoctorConfigEnv(t)
	resetDoctorViper(t)
	t.Setenv(configenv.APIKeyEnv, "")
	t.Setenv("APIKEY", "environment-only-key")
	legacy := filepath.Join(home, ".runpod", "config.toml")
	writeDoctorFile(t, legacy, "apiUrl = \"https://api.runpod.io/graphql\"\n")
	if _, err := configpath.Load(viper.GetViper()); err != nil {
		t.Fatal(err)
	}

	result := checkAPIKey()
	if result.Status != "pass" || result.Fixed {
		t.Fatalf("result = %+v, want unchanged pass", result)
	}
	current, err := configpath.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("current config exists: %v", err)
	}
}

func TestCheckAPIKeyDoesNotPersistEnvironmentCredential(t *testing.T) {
	_, _ = isolatedDoctorConfigEnv(t)
	resetDoctorViper(t)
	t.Setenv(configenv.APIKeyEnv, "environment-only-key")

	result := checkAPIKey()
	if result.Status != "pass" || result.Fixed {
		t.Fatalf("result = %+v, want unchanged pass", result)
	}
	current, err := configpath.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("current config exists: %v", err)
	}
}

func TestCheckAPIKeyDoesNotRewriteCurrentConfig(t *testing.T) {
	_, _ = isolatedDoctorConfigEnv(t)
	resetDoctorViper(t)
	t.Setenv(configenv.APIKeyEnv, "environment-override-key")

	current, err := configpath.Current()
	if err != nil {
		t.Fatal(err)
	}
	contents := "# preserve this comment\napiKey = \"native-key\"\n"
	writeDoctorFile(t, current, contents)
	if _, err := configpath.Load(viper.GetViper()); err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(current)
	if err != nil {
		t.Fatal(err)
	}
	beforeModTime := beforeInfo.ModTime()

	result := checkAPIKey()
	if result.Status != "pass" || result.Fixed {
		t.Fatalf("result = %+v, want unchanged pass", result)
	}
	if got := readDoctorFile(t, current); got != contents {
		t.Fatalf("current config changed: %q", got)
	}
	afterInfo, err := os.Stat(current)
	if err != nil {
		t.Fatal(err)
	}
	if !afterInfo.ModTime().Equal(beforeModTime) {
		t.Fatalf("current config mtime changed: %s -> %s", beforeModTime, afterInfo.ModTime())
	}
}

func resetDoctorViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
}

func isolatedDoctorConfigEnv(t *testing.T) (home, xdg string) {
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

func writeDoctorFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func readDoctorFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func doctorConfigValue(t *testing.T, path string) string {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	if err := v.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	return v.GetString("apiKey")
}
