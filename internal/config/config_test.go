package config

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func files(m map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		s, ok := m[name]
		if !ok {
			return nil, errors.New("no such file")
		}
		return []byte(s), nil
	}
}

var base = map[string]string{
	"DOKSEO_SYNC_DATA":     "/data",
	"DOKSEO_SYNC_USERNAME": "reader",
	"DOKSEO_SYNC_PASSWORD": "hunter2",
}

func withBase(extra map[string]string) map[string]string {
	m := map[string]string{}
	for k, v := range base {
		m[k] = v
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestLoadDefaults(t *testing.T) {
	got, err := load(nil, env(base), files(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := Config{Listen: "127.0.0.1:8080", DataDir: "/data", Credentials: Credentials{Username: "reader", Password: "hunter2"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("load = %+v, want %+v", got, want)
	}
}

func TestLoadFlagOverEnv(t *testing.T) {
	args := []string{"--listen", ":9000", "--data", "/flag", "--username", "flagger"}
	got, err := load(args, env(withBase(map[string]string{"DOKSEO_SYNC_LISTEN": ":1"})), files(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Listen != ":9000" || got.DataDir != "/flag" || got.Credentials.Username != "flagger" {
		t.Errorf("load = %+v, want the flag values", got)
	}
}

func TestLoadOrigins(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want []string
	}{
		{"repeated flag", []string{"--cors-origin", "https://a.example", "--cors-origin", "http://b.example:8080"}, "", []string{"https://a.example", "http://b.example:8080"}},
		{"comma env", nil, " https://a.example, ,https://b.example ,", []string{"https://a.example", "https://b.example"}},
		{"flag over env", []string{"--cors-origin", "https://flag.example"}, "https://env.example", []string{"https://flag.example"}},
		{"none", nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := load(tt.args, env(withBase(map[string]string{"DOKSEO_SYNC_CORS_ORIGINS": tt.env})), files(nil))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if !reflect.DeepEqual(got.AllowedOrigins, tt.want) {
				t.Errorf("AllowedOrigins = %q, want %q", got.AllowedOrigins, tt.want)
			}
		})
	}
}

func TestLoadRejectsInvalidOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
	}{
		{"path", "https://a.example/app"},
		{"trailing slash", "https://a.example/"},
		{"no scheme", "a.example"},
		{"no scheme with port", "a.example:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load([]string{"--cors-origin", tt.origin}, env(base), files(nil))
			if err == nil {
				t.Errorf("load accepted origin %q", tt.origin)
			}
		})
	}
}

func TestLoadRequiresDataAndUsername(t *testing.T) {
	tests := []struct {
		name string
		drop string
	}{
		{"data", "DOKSEO_SYNC_DATA"},
		{"username", "DOKSEO_SYNC_USERNAME"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := withBase(nil)
			delete(m, tt.drop)
			if _, err := load(nil, env(m), files(nil)); err == nil {
				t.Errorf("load succeeded without %s", tt.drop)
			}
		})
	}
}

func TestLoadPassword(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		files   map[string]string
		want    string
		wantErr bool
	}{
		{"from env", nil, map[string]string{"DOKSEO_SYNC_PASSWORD": "hunter2"}, nil, "hunter2", false},
		{"from file flag", []string{"--password-file", "/pw"}, nil, map[string]string{"/pw": "s3cret\n"}, "s3cret", false},
		{"from file env", nil, map[string]string{"DOKSEO_SYNC_PASSWORD_FILE": "/pw"}, map[string]string{"/pw": "s3cret"}, "s3cret", false},
		{"trims only one newline", []string{"--password-file", "/pw"}, nil, map[string]string{"/pw": "s3cret\n\n"}, "s3cret\n", false},
		{"both sources", []string{"--password-file", "/pw"}, map[string]string{"DOKSEO_SYNC_PASSWORD": "hunter2"}, map[string]string{"/pw": "s3cret"}, "", true},
		{"neither source", nil, nil, nil, "", true},
		{"empty file", []string{"--password-file", "/pw"}, nil, map[string]string{"/pw": "\n"}, "", true},
		{"missing file", []string{"--password-file", "/gone"}, nil, nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]string{"DOKSEO_SYNC_DATA": "/data", "DOKSEO_SYNC_USERNAME": "reader"}
			for k, v := range tt.env {
				m[k] = v
			}
			got, err := load(tt.args, env(m), files(tt.files))
			if (err != nil) != tt.wantErr {
				t.Fatalf("load error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got.Credentials.Password != tt.want {
				t.Errorf("Password = %q, want %q", got.Credentials.Password, tt.want)
			}
		})
	}
}

func TestLoadErrorsLeavePasswordOut(t *testing.T) {
	const password = "hunter2"
	tests := []struct {
		name string
		args []string
	}{
		{"both sources", []string{"--password-file", "/pw"}},
		{"bad origin", []string{"--cors-origin", "https://a.example/path"}},
		{"unknown flag", []string{"--password", password}},
		{"extra argument", []string{password}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(tt.args, env(withBase(nil)), files(map[string]string{"/pw": password}))
			if err == nil {
				t.Fatal("load succeeded")
			}
			if strings.Contains(err.Error(), password) {
				t.Errorf("error %q contains the password", err)
			}
		})
	}
}

func TestConfigStringLeavesPasswordOut(t *testing.T) {
	cfg, err := load(nil, env(base), files(nil))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, s := range []string{cfg.String(), cfg.Credentials.String(), cfg.Credentials.GoString()} {
		if strings.Contains(s, "hunter2") {
			t.Errorf("%q contains the password", s)
		}
	}
}

func TestLoadHelpReturnsErrHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		_, err := load([]string{arg}, env(base), files(nil))
		if !errors.Is(err, flag.ErrHelp) {
			t.Errorf("load(%s) error = %v, want flag.ErrHelp", arg, err)
		}
	}
}
