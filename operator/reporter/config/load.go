package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const defaultHomeDir = ".akt-reporter"

// FilePath returns the configuration file of a home directory.
func FilePath(home string) string {
	return filepath.Join(home, "config", "reporter.toml")
}

// envName returns the environment variable of a key: the prefix, then the key upper-cased with "." and "-" as "_".
func envName(key string) string {
	return EnvPrefix + "_" + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(key))
}

func parseEnviron(environ []string) map[string]string {
	env := make(map[string]string, len(environ))

	for _, kv := range environ {
		if name, value, ok := strings.Cut(kv, "="); ok {
			env[name] = value
		}
	}

	return env
}

// ResolveHome returns the home directory: --home, then AKT_REPORTER_HOME, then $HOME/.akt-reporter.
func ResolveHome(flags *pflag.FlagSet, environ []string) (string, error) {
	if f := flags.Lookup(FlagHome); f != nil && f.Changed && f.Value.String() != "" {
		return f.Value.String(), nil
	}

	env := parseEnviron(environ)
	if home := env[EnvPrefix+"_HOME"]; home != "" {
		return home, nil
	}

	user := env["HOME"]
	if user == "" {
		var err error
		if user, err = os.UserHomeDir(); err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
	}

	return filepath.Join(user, defaultHomeDir), nil
}

// Load reads the configuration with the precedence flag, environment, file, default. flags must come from
// AddFlags and be parsed. It does not validate values; see Resolve. Unknown keys in the file are errors.
// Unknown AKT_REPORTER_* variables are returned as warnings.
func Load(flags *pflag.FlagSet, environ []string) (Config, []string, error) {
	home, err := ResolveHome(flags, environ)
	if err != nil {
		return Config{}, nil, err
	}

	v := viper.New()
	v.SetConfigType("toml")

	known := make(map[string]bool)

	walk(reflect.ValueOf(Default()), "", func(key string, f reflect.Value) {
		known[key] = true
		v.SetDefault(key, f.Interface())
	})

	path := FilePath(home)

	switch data, err := os.ReadFile(path); { //nolint:gosec // the operator names the home directory
	case err == nil:
		if err := checkFileKeys(path, data, known); err != nil {
			return Config{}, nil, err
		}

		if err := v.ReadConfig(bytes.NewReader(data)); err != nil {
			return Config{}, nil, fmt.Errorf("%s: %w", path, err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return Config{}, nil, fmt.Errorf("read %s: %w", path, err)
	}

	env := parseEnviron(environ)

	if err := bindFlags(flags, v, known, env); err != nil {
		return Config{}, nil, err
	}

	if err := checkDurations(v); err != nil {
		return Config{}, nil, err
	}

	cfg := Default()
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, nil, fmt.Errorf("invalid configuration: %w", err)
	}

	if cfg.KeyringDir == "" {
		cfg.KeyringDir = home
	}

	return cfg, unknownEnv(environ, known), nil
}

// checkDurations rejects a duration key whose value is not a string, such as the TOML integer max-age = 60, which
// would otherwise decode as 60 nanoseconds.
func checkDurations(v *viper.Viper) error {
	var errs []error

	walk(reflect.ValueOf(Default()), "", func(key string, f reflect.Value) {
		if _, ok := f.Interface().(time.Duration); !ok {
			return
		}

		switch val := v.Get(key).(type) {
		case string, time.Duration:
		default:
			errs = append(errs, fmt.Errorf("%s: must be a duration string such as \"60s\", got %v", key, val))
		}
	})

	return errors.Join(errs...)
}

// checkFileKeys fails for every key of the file that is not a configuration key.
func checkFileKeys(path string, data []byte, known map[string]bool) error {
	fv := viper.New()
	fv.SetConfigType("toml")

	if err := fv.ReadConfig(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	var errs []error

	keys := fv.AllKeys()
	slices.Sort(keys)

	for _, key := range keys {
		if !known[key] {
			errs = append(errs, fmt.Errorf("%s: unknown key %q", path, key))
		}
	}

	return errors.Join(errs...)
}

// bindFlags is chain-sdk's bindFlags (go/cli/configs.go) for a bare flag set. Viper puts a changed flag above the
// file and defaults. Environment values come from environ instead of the process, and are applied above the file
// only when the flag is unset, so the order is flag, environment, file, default.
func bindFlags(flags *pflag.FlagSet, v *viper.Viper, known map[string]bool, env map[string]string) error {
	var err error

	flags.VisitAll(func(f *pflag.Flag) {
		if err != nil || !known[f.Name] {
			return
		}

		if err = v.BindPFlag(f.Name, f); err != nil {
			return
		}

		if val := env[envName(f.Name)]; val != "" && !f.Changed {
			v.Set(f.Name, val)
		}
	})

	return err
}

// unknownEnv warns about AKT_REPORTER_* variables that are not keys: platforms add their own, for example
// AKT_REPORTER_SERVICE_HOST in Kubernetes.
func unknownEnv(environ []string, known map[string]bool) []string {
	names := map[string]bool{EnvPrefix + "_HOME": true}
	for key := range known {
		names[envName(key)] = true
	}

	var warnings []string

	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, EnvPrefix+"_") && !names[name] {
			warnings = append(warnings, fmt.Sprintf("ignoring unknown environment variable %s", name))
		}
	}

	return warnings
}
