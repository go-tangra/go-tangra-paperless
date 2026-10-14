package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/config"
	"github.com/go-tangra/go-tangra/v4/authz"
	"gopkg.in/yaml.v3"
)

var placeholderRE = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*)\}`)

// joinValues are values of the shape the gateway fills in: core values,
// generated secrets and the host inputs.
var joinValues = map[string]string{
	"TRUST_DOMAIN": "infra.example.org", "GATEWAY_ISSUER": "https://portal.example.org:8443",
	"LCM_ENROLL_URL": "https://portal.example.org:8443/api/lcm/v1/enroll", "LCM_GRPC": "portal.example.org:9945",
	"GATEWAY_GRPC": "portal.example.org:9643", "AUTH_GRPC": "portal.example.org:9543", "MESH_TENANT_ID": "00000000-0000-0000-0000-000000000001",
	"MODULE_IMAGE": "ghcr.io/go-tangra/go-tangra-paperless", "MODULE_VERSION": "4.5.0",
	"GEN_PASSWORD_1": strings.Repeat("a", 48), "GEN_PASSWORD_2": strings.Repeat("b", 48), "GEN_PASSWORD_3": strings.Repeat("c", 48),
	"GEN_KEY_1":             "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
	"MODULE_ADVERTISE_HOST": "paperless.example.org", "MODULE_BIND_IP": "10.0.0.5",
	"VALKEY_ADDRESS": "valkey.example.org:6380", "VALKEY_PASSWORD": strings.Repeat("e", 32), "VALKEY_CA_FILE": "/etc/tangra/valkey-ca.pem",
}

func render(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "bundle", name))
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	rendered := placeholderRE.ReplaceAllStringFunc(string(raw), func(m string) string {
		k := placeholderRE.FindStringSubmatch(m)[1]
		v, ok := joinValues[k]
		if !ok {
			missing = append(missing, k)
		}
		return v
	})
	if len(missing) > 0 {
		t.Fatalf("%s uses placeholders the gateway does not fill: %v", name, missing)
	}
	return rendered
}

// The descriptor's host inputs accept values of the shape operators give,
// and the bundle has no private Valkey: events must reach the core's bus.
func TestCatalogueDescriptorHostInputs(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "tangra-module.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Bundle struct {
			TLSHosts []string `yaml:"tls_hosts"`
		} `yaml:"bundle"`
		HostInputs []struct {
			Key     string `yaml:"key"`
			Pattern string `yaml:"pattern"`
		} `yaml:"host_inputs"`
	}
	if err := yaml.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, in := range d.HostInputs {
		keys[in.Key] = true
		if !regexp.MustCompile(in.Pattern).MatchString(joinValues[in.Key]) {
			t.Fatalf("host input %s refuses %q", in.Key, joinValues[in.Key])
		}
	}
	for _, k := range []string{"MODULE_ADVERTISE_HOST", "MODULE_BIND_IP", "VALKEY_ADDRESS", "VALKEY_PASSWORD", "VALKEY_CA_FILE"} {
		if !keys[k] {
			t.Fatalf("host input %s missing", k)
		}
	}
	if strings.Join(d.Bundle.TLSHosts, ",") != "paperless-db" {
		t.Fatalf("tls_hosts %v", d.Bundle.TLSHosts)
	}
}

// The join bundle's templates, rendered with values of the shape the gateway
// fills in, are a valid production configuration and policy, the event bus is
// the core's Valkey over TLS, and the compose file uses only values the
// gateway writes to .env.
func TestJoinBundleRendersToValidProductionConfig(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"config.yaml", "policy.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(render(t, name)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	compose := render(t, "compose.yaml")
	if strings.Contains(compose, "valkey/valkey") || !strings.Contains(compose, "/etc/tangra/valkey-ca.pem:/tls/valkey-ca.pem:ro") {
		t.Fatal("compose must not run a private Valkey and must mount the core's Valkey CA")
	}
	cfg, err := config.Load(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("rendered config invalid: %v", err)
	}
	if !cfg.IsProduction() || cfg.Gateway.Issuer != "https://portal.example.org:8443" || cfg.Valkey.AllowPlaintext || cfg.KEK.Env != "PAPERLESS_KEK" {
		t.Fatalf("%+v %+v %+v", cfg.Gateway, cfg.Valkey.AllowPlaintext, cfg.KEK)
	}
	if len(cfg.Valkey.Addresses) != 1 || cfg.Valkey.Addresses[0] != "valkey.example.org:6380" || cfg.Valkey.Username != "paperless" ||
		cfg.Valkey.Password != joinValues["VALKEY_PASSWORD"] || cfg.Valkey.CAFile != "/tls/valkey-ca.pem" {
		t.Fatalf("%+v", cfg.Valkey.Addresses)
	}
	pol, _ := os.ReadFile(filepath.Join(dir, "policy.yaml"))
	if _, err := authz.Load(bytes.NewReader(pol)); err != nil {
		t.Fatalf("rendered policy invalid: %v", err)
	}
	if !strings.Contains(string(pol), "spiffe://infra.example.org/svc/gateway") || !strings.Contains(string(pol), "spiffe://infra.example.org/svc/asset") {
		t.Fatal(string(pol))
	}
}
