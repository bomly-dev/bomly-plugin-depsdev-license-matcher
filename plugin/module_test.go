package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	sdk "github.com/bomly-dev/bomly-sdk"
	"github.com/bomly-dev/bomly-sdk/conformance"
	"go.uber.org/zap"
)

// testHost is a minimal HostContext for unit tests.
type testHost struct {
	config json.RawMessage
}

func (h testHost) Logger() *zap.Logger                 { return zap.NewNop() }
func (h testHost) HTTPClient() *sdk.HTTPClientProvider { return nil }
func (h testHost) Runtime() sdk.RuntimeInfo {
	return sdk.RuntimeInfo{Execution: sdk.ExecutionEmbedded}
}

func (h testHost) DecodeConfig(v any) error {
	payload := h.config
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	return json.Unmarshal(payload, v)
}

// TestModuleConstructsChecker checks the managed constructor honors the JSON
// config block.
func TestModuleConstructsChecker(t *testing.T) {
	cacheDir := filepath.ToSlash(filepath.Join(t.TempDir(), "cache"))
	cfg := fmt.Sprintf(`{"api_base":"https://depsdev.example/v3alpha","cache_dir":%q,"cache_ttl":"12h"}`, cacheDir)
	component, err := Module().Matcher.New(context.Background(), testHost{config: json.RawMessage(cfg)})
	if err != nil {
		t.Fatalf("construct matcher: %v", err)
	}
	checker, ok := component.(*Checker)
	if !ok {
		t.Fatalf("unexpected component type %T", component)
	}
	if checker.config.APIBase != "https://depsdev.example/v3alpha" {
		t.Fatalf("api base = %q", checker.config.APIBase)
	}
}

// newDepsDevServer answers every versionbatch entry with an MIT license.
func newDepsDevServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req versionBatchRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := versionBatchResponse{Responses: make([]versionBatchResult, len(req.Requests))}
		for i := range resp.Responses {
			resp.Responses[i] = versionBatchResult{Version: depsDevVersion{Licenses: []string{"MIT"}}}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	return server
}

func newDeltaGraphAndRegistry(t *testing.T) (*sdk.Graph, *sdk.PackageRegistry) {
	t.Helper()
	graph := sdk.New()
	dep := sdk.NewDependencyRef("left-pad", "1.3.0")
	dep.PURL = "pkg:npm/left-pad@1.3.0"
	dep.Ecosystem = sdk.EcosystemNPM
	if err := graph.AddNode(dep); err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	registry := sdk.NewPackageRegistry()
	registry.Add(&sdk.Package{
		Coordinates: sdk.Coordinates{
			PURL:      "pkg:npm/left-pad@1.3.0",
			Name:      "left-pad",
			Version:   "1.3.0",
			Ecosystem: sdk.EcosystemNPM,
		},
	})
	return graph, registry
}

func newDeltaChecker(t *testing.T, apiBase string) *Checker {
	t.Helper()
	checker, err := New(Config{APIBase: apiBase, CacheDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return checker
}

// TestMatchDeltaEquivalence is the delta-protocol contract check: when the
// request sets AcceptPackageUpdates, Match must leave the request registry
// untouched and return deltas that — applied through the host's own merge,
// sdk.ApplyPackageUpdates — reproduce the registry the legacy full-registry
// path produces.
func TestMatchDeltaEquivalence(t *testing.T) {
	server := newDepsDevServer(t)

	legacyGraph, legacyRegistry := newDeltaGraphAndRegistry(t)
	legacy, err := newDeltaChecker(t, server.URL).Match(context.Background(), sdk.MatchRequest{
		Graph:    legacyGraph,
		Registry: legacyRegistry,
	})
	if err != nil {
		t.Fatalf("legacy Match() error = %v", err)
	}

	deltaGraph, deltaRegistry := newDeltaGraphAndRegistry(t)
	delta, err := newDeltaChecker(t, server.URL).Match(context.Background(), sdk.MatchRequest{
		Graph:                deltaGraph,
		Registry:             deltaRegistry,
		AcceptPackageUpdates: true,
	})
	if err != nil {
		t.Fatalf("delta Match() error = %v", err)
	}

	if delta.Registry != nil {
		t.Fatal("delta path must not return a registry")
	}
	if len(delta.PackageUpdates) != 1 {
		t.Fatalf("expected 1 package update, got %d", len(delta.PackageUpdates))
	}
	update := delta.PackageUpdates[0]
	if update.PURL != "pkg:npm/left-pad@1.3.0" || !update.Matched {
		t.Fatalf("unexpected update %#v", update)
	}
	if len(update.Vulnerabilities) != 0 || update.Scorecard != nil {
		t.Fatalf("update must carry only the mutated fields, got %#v", update)
	}
	if len(update.Licenses) != 1 || update.Licenses[0].Value != "MIT" {
		t.Fatalf("unexpected licenses %#v", update.Licenses)
	}

	// The matcher must not have enriched the request registry in delta mode.
	for _, pkg := range deltaRegistry.All() {
		if pkg.Matched || len(pkg.Licenses) != 0 {
			t.Fatalf("delta path mutated request registry package %#v", pkg)
		}
	}

	merged := sdk.ApplyPackageUpdates(deltaRegistry, delta.PackageUpdates)
	if diff := registryDiff(legacy.Registry, merged); diff != "" {
		t.Fatalf("merged delta registry differs from legacy registry: %s", diff)
	}
	if legacy.MatcherStats != delta.MatcherStats {
		t.Fatalf("matcher stats diverge: legacy %#v, delta %#v", legacy.MatcherStats, delta.MatcherStats)
	}
}

// registryDiff deep-compares two registries package by package.
func registryDiff(want, got *sdk.PackageRegistry) string {
	wantPkgs := want.All()
	gotPkgs := got.All()
	if len(wantPkgs) != len(gotPkgs) {
		return fmt.Sprintf("package count %d != %d", len(gotPkgs), len(wantPkgs))
	}
	for _, wantPkg := range wantPkgs {
		gotPkg, ok := got.Get(wantPkg.PURL)
		if !ok {
			return fmt.Sprintf("missing package %s", wantPkg.PURL)
		}
		if !reflect.DeepEqual(wantPkg, gotPkg) {
			return fmt.Sprintf("package %s differs: want %#v, got %#v", wantPkg.PURL, wantPkg, gotPkg)
		}
	}
	return ""
}

// TestConformance runs the SDK conformance suite against the module,
// including the bomly-plugin.json identity cross-check.
func TestConformance(t *testing.T) {
	conformance.Test(t, conformance.Config{
		Module:       Module(),
		ManifestPath: filepath.Join("..", "bomly-plugin.json"),
		SampleConfig: json.RawMessage(`{"api_base":"https://api.deps.dev/v3alpha","cache_ttl":"24h"}`),
	})
}

// TestProbeBinary builds the real plugin binary and probes it over the
// managed HashiCorp gRPC transport, asserting the served descriptor equals
// the in-process one.
func TestProbeBinary(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not available; skipping managed-transport probe")
	}
	binaryPath := filepath.Join(t.TempDir(), "bomly-plugin-depsdev-license-matcher")
	build := exec.Command(goBinary, "build", "-o", binaryPath, "./cmd/bomly-plugin-depsdev-license-matcher")
	build.Dir = ".."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build plugin binary: %v\n%s", err, output)
	}
	conformance.ProbeBinary(t, binaryPath, conformance.WithModule(Module()))
}
