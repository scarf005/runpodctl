package configpath

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const appName = "runpod"

// Result describes the configuration file selected by Load.
type Result struct {
	Path   string
	Legacy bool
}

// Current returns the native per-user configuration path for runpodctl.
// os.UserConfigDir follows XDG_CONFIG_HOME on unix and the native application
// data directory on windows and darwin.
func Current() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to find user config directory: %w", err)
	}
	return filepath.Join(dir, appName, "config.toml"), nil
}

func legacyPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to find user home directory: %w", err)
	}
	return []string{
		filepath.Join(home, ".runpod", "config.toml"),
		filepath.Join(home, ".runpod.yaml"),
	}, nil
}

// Load selects the current config first, then the two historical files. An
// existing file is authoritative: parse and permission errors are returned and
// never cause a fallback or a replacement file to be written.
func Load(v *viper.Viper) (*Result, error) {
	return load(v, true)
}

// LoadExisting applies Load's selection and parsing rules without creating a
// config file when none exists. This is used by read-only callers such as e2e
// tests, which must not change the user's filesystem just by loading settings.
func LoadExisting(v *viper.Viper) (*Result, error) {
	return load(v, false)
}

func load(v *viper.Viper, create bool) (*Result, error) {
	if v == nil {
		return nil, fmt.Errorf("viper instance is nil")
	}

	current, err := Current()
	if err != nil {
		return nil, err
	}
	v.AutomaticEnv()

	exists, err := pathExists(current)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect config file %s: %w", current, err)
	}
	if exists {
		if err := read(v, current, "toml"); err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", current, err)
		}
		return &Result{Path: current}, nil
	}

	legacy, err := legacyPaths()
	if err != nil {
		return nil, err
	}
	for _, path := range legacy {
		exists, err := pathExists(path)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect config file %s: %w", path, err)
		}
		if !exists {
			continue
		}

		typeName := "toml"
		if filepath.Ext(path) == ".yaml" {
			typeName = "yaml"
		}
		if err := read(v, path, typeName); err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}
		return &Result{Path: path, Legacy: true}, nil
	}

	if !create {
		return nil, nil
	}
	v.SetConfigFile(current)
	v.SetConfigType("toml")
	if _, err := Save(v); err != nil {
		return nil, fmt.Errorf("failed to create config file %s: %w", current, err)
	}
	return &Result{Path: current}, nil
}

// Save writes the current viper settings to the native config path. Existing
// files are parsed before they are overwritten, so a malformed file can never
// be replaced by a command that saves settings. New files use an exclusive
// create to avoid a race overwriting a file created by another process.
func Save(v *viper.Viper) (string, error) {
	if v == nil {
		return "", fmt.Errorf("viper instance is nil")
	}
	path, err := Current()
	if err != nil {
		return "", err
	}

	exists, err := pathExists(path)
	if err != nil {
		return "", fmt.Errorf("failed to inspect config file %s: %w", path, err)
	}
	if exists {
		if err := read(v, path, "toml"); err != nil {
			return "", fmt.Errorf("refusing to overwrite config file %s: %w", path, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	v.SetConfigPermissions(0600)
	if exists {
		err = v.WriteConfigAs(path)
	} else {
		err = v.SafeWriteConfigAs(path)
	}
	if err != nil {
		return "", err
	}
	// Keep the permission guarantee for files that existed before this save.
	if err := os.Chmod(path, 0600); err != nil {
		return "", fmt.Errorf("failed to secure config file: %w", err)
	}
	return path, nil
}

func read(v *viper.Viper, path, typeName string) error {
	v.SetConfigFile(path)
	v.SetConfigType(typeName)
	return v.ReadInConfig()
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
