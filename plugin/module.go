package plugin

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdkplugin "github.com/bomly-dev/bomly-sdk/plugin"
)

// Name is the plugin's identity. It MUST equal the "id" field in
// bomly-plugin.json — Bomly refuses to load a plugin whose manifest id and
// runtime descriptor name disagree. It is also the descriptor name the Bomly
// CLI composition keys on when it embeds this matcher.
const Name = "depsdev-license-matcher"

// displayName is the human-readable matcher name.
const displayName = "deps.dev License Matcher"

// moduleConfig is the JSON configuration accepted in managed execution under
// plugins.matchers.depsdev-license-matcher. Durations are Go duration strings
// ("24h", "30m").
type moduleConfig struct {
	APIBase  string `json:"api_base"`
	CacheDir string `json:"cache_dir"`
	CacheTTL string `json:"cache_ttl"`
}

// moduleDescriptor is the matcher's static registration data, shared by the
// embedded Descriptor method and the managed Module constructor.
func moduleDescriptor() sdkplugin.MatcherDescriptor {
	descriptor := (&Checker{}).Descriptor()
	descriptor.ConfigSchema = sdkplugin.MustConfigSchemaFor(moduleConfig{})
	return descriptor
}

// configFromHost builds the matcher Config from the host-provided JSON block.
func configFromHost(host sdkplugin.HostContext) (Config, error) {
	var raw moduleConfig
	if err := host.DecodeConfig(&raw); err != nil {
		return Config{}, fmt.Errorf("decode deps.dev matcher configuration: %w", err)
	}
	cfg := DefaultConfig()
	cfg.Logger = host.Logger()
	cfg.HTTPClientProvider = host.HTTPClient()
	if strings.TrimSpace(raw.APIBase) != "" {
		cfg.APIBase = raw.APIBase
	}
	if strings.TrimSpace(raw.CacheDir) != "" {
		cfg.CacheDir = raw.CacheDir
	}
	if trimmed := strings.TrimSpace(raw.CacheTTL); trimmed != "" {
		parsed, err := time.ParseDuration(trimmed)
		if err != nil {
			return Config{}, fmt.Errorf("invalid cache_ttl: %w", err)
		}
		if parsed > 0 {
			cfg.CacheTTL = parsed
		}
	}
	return cfg, nil
}

// Module packages the matcher for both execution modes: Bomly can embed it
// in-process or serve it as a managed plugin subprocess (see
// cmd/bomly-plugin-depsdev-license-matcher).
func Module() sdkplugin.Module {
	return sdkplugin.Module{
		Kind: sdkplugin.PluginKindMatcher,
		Matcher: &sdkplugin.MatcherModule{
			Descriptor: moduleDescriptor(),
			New: func(_ context.Context, host sdkplugin.HostContext) (sdkplugin.Matcher, error) {
				cfg, err := configFromHost(host)
				if err != nil {
					return nil, err
				}
				return New(cfg)
			},
		},
	}
}
