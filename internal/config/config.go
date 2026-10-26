// Package config reads the dokseo-sync settings from flags and the environment.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

const defaultListen = "127.0.0.1:8080"

// Credentials is the username and password of the account. Its String method
// leaves the password out.
type Credentials struct {
	Username string
	Password string
}

// String returns the username and a redacted password.
func (c Credentials) String() string {
	return fmt.Sprintf("{Username:%s Password:[redacted]}", c.Username)
}

// GoString returns the same text as String, so %#v leaves the password out too.
func (c Credentials) GoString() string {
	return c.String()
}

// Config is the validated server configuration.
type Config struct {
	// Listen is the address the server listens on.
	Listen string
	// DataDir is the directory that holds dokseo-sync.db.
	DataDir string
	// AllowedOrigins lists the browser origins that may call the API.
	AllowedOrigins []string
	// Credentials is the account that POST /v1/devices accepts.
	Credentials Credentials
}

type origins []string

func (o *origins) String() string {
	return strings.Join(*o, ",")
}

func (o *origins) Set(v string) error {
	*o = append(*o, v)
	return nil
}

// Load reads the configuration from args, which exclude the program name, and
// falls back to getenv for every setting a flag leaves unset.
func Load(args []string, getenv func(string) string) (Config, error) {
	return load(args, getenv, os.ReadFile)
}

func load(args []string, getenv func(string) string, readFile func(string) ([]byte, error)) (Config, error) {
	fs := flag.NewFlagSet("dokseo-sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	listen := fs.String("listen", "", "address to listen on (default "+defaultListen+")")
	data := fs.String("data", "", "directory for dokseo-sync.db")
	var flagOrigins origins
	fs.Var(&flagOrigins, "cors-origin", "allowed browser origin, repeatable")
	username := fs.String("username", "", "account username")
	passwordFile := fs.String("password-file", "", "file that holds the account password")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, errors.New("unexpected positional argument")
	}

	cfg := Config{
		Listen:  pick(*listen, getenv("DOKSEO_SYNC_LISTEN"), defaultListen),
		DataDir: pick(*data, getenv("DOKSEO_SYNC_DATA"), ""),
	}
	cfg.Credentials.Username = pick(*username, getenv("DOKSEO_SYNC_USERNAME"), "")
	if cfg.DataDir == "" {
		return Config{}, errors.New("data directory is required: set --data or DOKSEO_SYNC_DATA")
	}
	if cfg.Credentials.Username == "" {
		return Config{}, errors.New("username is required: set --username or DOKSEO_SYNC_USERNAME")
	}

	rawOrigins := []string(flagOrigins)
	if len(rawOrigins) == 0 {
		rawOrigins = strings.Split(getenv("DOKSEO_SYNC_CORS_ORIGINS"), ",")
	}
	for _, o := range rawOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if err := checkOrigin(o); err != nil {
			return Config{}, err
		}
		cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
	}

	password, err := loadPassword(pick(*passwordFile, getenv("DOKSEO_SYNC_PASSWORD_FILE"), ""), getenv("DOKSEO_SYNC_PASSWORD"), readFile)
	if err != nil {
		return Config{}, err
	}
	cfg.Credentials.Password = password
	return cfg, nil
}

// String returns the configuration with the password redacted.
func (c Config) String() string {
	return fmt.Sprintf("{Listen:%s DataDir:%s AllowedOrigins:%v Credentials:%s}", c.Listen, c.DataDir, c.AllowedOrigins, c.Credentials)
}

func pick(flagValue, envValue, fallback string) string {
	if flagValue != "" {
		return flagValue
	}
	if envValue != "" {
		return envValue
	}
	return fallback
}

func checkOrigin(o string) error {
	u, err := url.Parse(o)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.Opaque != "" || strings.HasSuffix(o, "/") {
		return fmt.Errorf("invalid cors origin %q: want scheme://host[:port] with no path", o)
	}
	return nil
}

func loadPassword(file, direct string, readFile func(string) ([]byte, error)) (string, error) {
	switch {
	case file != "" && direct != "":
		return "", errors.New("set only one of DOKSEO_SYNC_PASSWORD and a password file")
	case file == "" && direct == "":
		return "", errors.New("password is required: set DOKSEO_SYNC_PASSWORD or --password-file")
	case file == "":
		return direct, nil
	}
	b, err := readFile(file)
	if err != nil {
		return "", fmt.Errorf("read password file: %w", err)
	}
	password := strings.TrimSuffix(string(b), "\n")
	if password == "" {
		return "", errors.New("password file is empty")
	}
	return password, nil
}

// Usage describes the flags and the environment variables that back them.
const Usage = `usage: dokseo-sync [flags]

Each flag falls back to an environment variable. A flag wins over the variable.

  --listen ADDR          address to listen on (DOKSEO_SYNC_LISTEN, default 127.0.0.1:8080)
  --data DIR             directory for dokseo-sync.db (DOKSEO_SYNC_DATA, required)
  --cors-origin ORIGIN   allowed browser origin, repeatable (DOKSEO_SYNC_CORS_ORIGINS, comma-separated)
  --username NAME        account username (DOKSEO_SYNC_USERNAME, required)
  --password-file PATH   file that holds the account password (DOKSEO_SYNC_PASSWORD_FILE)

The password comes from DOKSEO_SYNC_PASSWORD or from the password file; set exactly one.
`
