package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// valid returns a Config that passes Validate in a dev environment.
func valid() Config {
	c := Default()
	c.ServiceName, c.TrustDomain, c.Env = "paperless", "example.org", "dev"
	c.Authz.Source, c.Authz.Path = "file", "policy.yaml"
	c.DB.DSN = "postgres://paperless_app:x@db/paperless?sslmode=disable"
	c.Valkey.Addresses = []string{"127.0.0.1:6379"}
	c.KEK.Path = "deploy/kek.dev"
	c.ObjectStore.Endpoint = "localhost:9000"
	c.ObjectStore.Bucket = "paperless"
	c.Gateway.Issuer = "https://localhost:8443"
	return c
}

func production(c *Config) {
	c.Env = "production"
	c.DB.DSN = "postgres://u:p@db/paperless?sslmode=verify-full"
	c.Valkey.AllowPlaintext = false
	c.ObjectStore.UseSSL = true
}

func TestDefaultsAreSecure(t *testing.T) {
	c := Default()
	if c.Valkey.AllowPlaintext {
		t.Fatal("valkey plaintext must be opt-in")
	}
	if c.KEK.Source != "file" {
		t.Fatalf("kek source default %q", c.KEK.Source)
	}
	if c.ObjectStore.Region != "us-east-1" || c.ObjectStore.PresignTTL != 300 {
		t.Fatalf("object store defaults %+v", c.ObjectStore)
	}
	if c.Extract.TimeoutSeconds != 60 || c.Extract.MaxBytes != 32<<20 {
		t.Fatalf("extract defaults %+v", c.Extract)
	}
	if c.Uploads.MaxSizeBytes != 100<<20 {
		t.Fatalf("uploads default %+v", c.Uploads)
	}
	if !c.Events.Enabled || c.Gateway.Service != "gateway" {
		t.Fatalf("events/gateway defaults %+v %+v", c.Events, c.Gateway)
	}
	if c.Jobs.Workers != 5 || c.Jobs.IntervalSeconds != 5 || c.Jobs.LeaseSeconds != 300 ||
		c.Jobs.MaxRetries != 3 || c.Jobs.RetryDelaySeconds != 60 || c.Jobs.BackoffMultiplier != 2.0 ||
		c.Jobs.JobTimeoutSeconds != 300 || c.Jobs.CleanupDays != 30 {
		t.Fatalf("jobs defaults %+v", c.Jobs)
	}
	if c.Limits.BackupMaxBytes != 32<<20 || c.Limits.SearchMaxLen != 512 {
		t.Fatalf("limits defaults %+v", c.Limits)
	}
}

func TestValidateAcceptsDevAndProduction(t *testing.T) {
	c := valid()
	if err := c.Validate(); err != nil {
		t.Fatalf("dev config should validate: %v", err)
	}
	production(&c)
	if err := c.Validate(); err != nil {
		t.Fatalf("production config should validate: %v", err)
	}
	// verify-ca is also accepted in production.
	c.DB.DSN = "postgres://u:p@db/paperless?sslmode=verify-ca"
	if err := c.Validate(); err != nil {
		t.Fatalf("verify-ca should validate: %v", err)
	}
	// KEK via env is accepted.
	c.KEK = KEK{Source: "env", Env: "PAPERLESS_KEK"}
	if err := c.Validate(); err != nil {
		t.Fatalf("kek env should validate: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Config){
		"service_name":          func(c *Config) { c.ServiceName = "" },
		"db.dsn required":       func(c *Config) { c.DB.DSN = "" },
		"db.dsn prod ssl":       func(c *Config) { production(c); c.DB.DSN = "postgres://u:p@db/x?sslmode=disable" },
		"valkey addresses":      func(c *Config) { c.Valkey.Addresses = nil },
		"valkey plaintext":      func(c *Config) { production(c); c.Valkey.AllowPlaintext = true },
		"kek file path":         func(c *Config) { c.KEK = KEK{Source: "file"} },
		"kek env":               func(c *Config) { c.KEK = KEK{Source: "env"} },
		"kek source":            func(c *Config) { c.KEK = KEK{Source: "vault"} },
		"object store missing":  func(c *Config) { c.ObjectStore.Endpoint = "" },
		"object bucket missing": func(c *Config) { c.ObjectStore.Bucket = "" },
		"presign ttl low":       func(c *Config) { c.ObjectStore.PresignTTL = 10 },
		"presign ttl high":      func(c *Config) { c.ObjectStore.PresignTTL = 4000 },
		"uploads too small":     func(c *Config) { c.Uploads.MaxSizeBytes = 1 },
		"uploads too large":     func(c *Config) { c.Uploads.MaxSizeBytes = 6 << 30 },
		"extract timeout low":   func(c *Config) { c.Extract.TimeoutSeconds = 0 },
		"extract timeout high":  func(c *Config) { c.Extract.TimeoutSeconds = 4000 },
		"jobs workers low":      func(c *Config) { c.Jobs.Workers = 0 },
		"jobs workers high":     func(c *Config) { c.Jobs.Workers = 100 },
		"jobs interval":         func(c *Config) { c.Jobs.IntervalSeconds = 0 },
		"jobs lease":            func(c *Config) { c.Jobs.LeaseSeconds = 1 },
		"jobs max retries":      func(c *Config) { c.Jobs.MaxRetries = -1 },
		"jobs job timeout":      func(c *Config) { c.Jobs.JobTimeoutSeconds = 1 },
		"jobs cleanup days":     func(c *Config) { c.Jobs.CleanupDays = 0 },
		"gateway service":       func(c *Config) { c.Gateway.Service = "" },
		"gateway issuer":        func(c *Config) { c.Gateway.Issuer = "http://insecure" },
		"gateway issuer empty":  func(c *Config) { c.Gateway.Issuer = "" },
		"backup max bytes":      func(c *Config) { c.Limits.BackupMaxBytes = 1 << 20 },
		"search max len":        func(c *Config) { c.Limits.SearchMaxLen = 1 },
		"framework invalid":     func(c *Config) { c.TrustDomain = "" },
	}
	for name, mut := range cases {
		c := valid()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestWarnings(t *testing.T) {
	c := valid()
	c.Valkey.AllowPlaintext = true
	c.ObjectStore.UseSSL = false
	w := c.Warnings()
	joined := strings.Join(w, "\n")
	if !strings.Contains(joined, "valkey.allow_plaintext") {
		t.Fatalf("expected valkey plaintext warning: %v", w)
	}
	if !strings.Contains(joined, "object_store.use_ssl") {
		t.Fatalf("expected object store TLS warning: %v", w)
	}
	// A fully secure config surfaces neither module warning.
	secure := valid()
	secure.ObjectStore.UseSSL = true
	for _, line := range secure.Warnings() {
		if strings.Contains(line, "valkey.allow_plaintext") || strings.Contains(line, "object_store.use_ssl") {
			t.Fatalf("secure config should not warn: %q", line)
		}
	}
}

func TestDurationHelpers(t *testing.T) {
	c := valid()
	if c.Interval() != 5*time.Second {
		t.Fatalf("Interval %v", c.Interval())
	}
	if c.Lease() != 300*time.Second {
		t.Fatalf("Lease %v", c.Lease())
	}
	if c.JobTimeout() != 300*time.Second {
		t.Fatalf("JobTimeout %v", c.JobTimeout())
	}
	if c.CleanupWindow() != 30*24*time.Hour {
		t.Fatalf("CleanupWindow %v", c.CleanupWindow())
	}
	if c.RetryDelay() != 60*time.Second {
		t.Fatalf("RetryDelay %v", c.RetryDelay())
	}
	if c.ExtractTimeout() != 60*time.Second {
		t.Fatalf("ExtractTimeout %v", c.ExtractTimeout())
	}
	if c.PresignTTL() != 300*time.Second {
		t.Fatalf("PresignTTL %v", c.PresignTTL())
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	body := "db:\n  max_conns: 32\njobs:\n  workers: 8\nobject_store:\n  bucket: docs\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DB.MaxConns != 32 || c.Jobs.Workers != 8 || c.ObjectStore.Bucket != "docs" {
		t.Fatalf("loaded %+v", c)
	}
	// Defaults preserved for untouched fields.
	if c.ObjectStore.PresignTTL != 300 {
		t.Fatalf("default not preserved: %+v", c.ObjectStore)
	}
	// Missing file.
	if _, err := Load(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Fatal("missing file: expected error")
	}
	// Unknown field is rejected.
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("not_a_field: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("unknown field: expected error")
	}
}
