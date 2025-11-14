# Bug Fixes for v0.1.0

**Date**: 2025-01-15
**Version**: v0.1.0 post-release fixes
**Based on**: Code review feedback

## Summary

This document records bug fixes identified during code review after the v0.1.0 release. These issues were caught before production deployment and have been addressed.

## Issues Fixed

### 1. CLI --refresh Flag Not Working ⚠️ CRITICAL

**File**: `cmd/k8s-monitor/main.go:87-89`

**Problem**:
```go
if refresh, _ := cmd.Flags().GetInt("refresh"); refresh > 0 {
    config.RefreshInterval = config.RefreshInterval  // ❌ Assigns to itself!
}
```

The `--refresh` flag was being read from command line but then assigned back to itself, meaning user input was completely ignored.

**Impact**: HIGH
- Users cannot control refresh interval via CLI
- Only config file or default (10s) would work
- Critical for operations where different refresh rates are needed

**Fix**:
```go
if refresh, _ := cmd.Flags().GetInt("refresh"); refresh > 0 {
    config.RefreshInterval = time.Duration(refresh) * time.Second  // ✅ Correct
}
```

**Added Import**: `"time"` package to `cmd/k8s-monitor/main.go`

**Testing**:
```bash
# Before fix: Always uses 10s regardless of flag
./k8s-monitor console --refresh 5  # Would still refresh every 10s

# After fix: Correctly uses 5s
./k8s-monitor console --refresh 5  # Refreshes every 5s ✓
```

---

### 2. YAML Configuration File Not Parsed Correctly ⚠️ HIGH

**Files**:
- `internal/app/config.go` (LoadConfig function)
- `config/default.yaml`

**Problem**:

The configuration file uses nested YAML structure:
```yaml
cluster:
  kubeconfig: ""
  context: ""
refresh:
  interval: 10s
  timeout: 5s
ui:
  color_mode: auto
logging:
  level: info
```

But the code was using `viper.Unmarshal()` with flat mapstructure tags:
```go
type Config struct {
    Kubeconfig string `mapstructure:"kubeconfig"`  // ❌ Expects flat key
    // ...
}
```

This caused ALL config file settings to be ignored, only defaults and CLI flags would work.

**Impact**: HIGH
- Config file completely non-functional
- Users cannot customize settings via YAML
- Documentation advertises YAML config but it doesn't work

**Fix**:

Replaced `viper.Unmarshal()` with explicit nested key mapping:

```go
// Before: Used Unmarshal with flat tags
viper.SetDefault("refresh_interval", "10s")  // ❌ Wrong key format
var config Config
viper.Unmarshal(&config)

// After: Use nested key format
viper.SetDefault("refresh.interval", "10s")  // ✅ Matches YAML structure
config.RefreshInterval = viper.GetDuration("refresh.interval")
config.Kubeconfig = viper.GetString("cluster.kubeconfig")
// ... explicit mapping for all fields
```

**Testing**:
```bash
# Create test config
cat > test-config.yaml <<EOF
refresh:
  interval: 30s
ui:
  color_mode: never
logging:
  level: debug
EOF

# Before fix: Would ignore all settings, use defaults
./k8s-monitor console --config test-config.yaml

# After fix: Correctly loads refresh=30s, no-color, debug logging ✓
```

---

### 3. --no-color Flag Not Propagated to UI ⚠️ MEDIUM

**Files**:
- `cmd/k8s-monitor/main.go`
- `internal/app/config.go`

**Problem**:

The `--no-color` flag was defined in CLI but never passed to the UI layer:

```go
consoleCmd.Flags().BoolP("no-color", "", false, "disable color output")
// ... but never used
```

The UI would always use colors regardless of flag setting.

**Impact**: MEDIUM
- Cannot disable colors for piping/scripting
- Issues with terminals that don't support ANSI colors
- Accessibility problem for users with color blindness

**Fix**:

1. Added `NoColor` field to Config struct:
```go
type Config struct {
    // ...
    NoColor     bool   `mapstructure:"no_color"`
}
```

2. Added flag handling in `cmd/k8s-monitor/main.go`:
```go
if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
    config.NoColor = true
    config.ColorMode = "never"
}
```

3. Added to YAML config defaults:
```go
viper.SetDefault("ui.no_color", false)
config.NoColor = viper.GetBool("ui.no_color")
```

**Note**: UI layer implementation will be completed in follow-up commit. The config is now properly passed through.

**Testing**:
```bash
# Before fix: Always shows colors
./k8s-monitor console --no-color  # Still has colors ❌

# After fix: Config correctly set (UI implementation pending)
./k8s-monitor console --no-color  # Will respect flag ✓
```

---

### 4. Timeout and MaxConcurrent Config Not Used 📝 LOW

**File**: Multiple datasource files

**Problem**:

Config fields `Timeout` and `MaxConcurrent` are defined and loaded but never actually applied to:
- API Server client
- kubelet client
- Data source aggregator

**Impact**: LOW
- Performance tuning not possible
- Cannot adjust concurrency for different cluster sizes
- Cannot tune timeouts for slow networks

**Status**: NOTED
This is not a bug but missing feature implementation. The configuration infrastructure is in place, but the values need to be wired into the HTTP clients and concurrency control.

**Planned Fix** (for v0.1.1 or v0.2):
- Pass `config.Timeout` to kubelet HTTP client
- Pass `config.MaxConcurrent` to aggregated data source
- Implement rate limiting/connection pooling

---

### 5. kubelet Direct Mode TLS Insecurity 🔒 SECURITY NOTE

**File**: `internal/datasource/kubelet.go:116`

**Code**:
```go
Transport: &http.Transport{
    TLSClientConfig: &tls.Config{
        InsecureSkipVerify: true,  // ⚠️ Security risk
    },
},
```

**Problem**:

kubelet direct access mode (currently unimplemented) has placeholder code that disables TLS verification. This is a security risk if direct mode is ever enabled.

**Impact**: CURRENTLY NONE (feature not implemented)
- Direct mode is not accessible via configuration
- Only proxy mode through API Server is used
- Code is dormant placeholder

**Recommendation**:

When implementing direct mode in the future:
1. Implement proper certificate handling
2. Load kubelet CA from kubeconfig
3. Support custom CA bundles
4. Make `InsecureSkipVerify` a config option (default: false)
5. Warn users if insecure mode is enabled

**Current Mitigation**:
- Feature is not exposed to users
- Documentation states "proxy mode only"
- Direct mode requires additional implementation before use

---

## Testing Results

### Unit Tests
```
=== RUN   TestTTLCache
--- PASS: TestTTLCache (1.10s)
=== RUN   TestTTLCacheInvalidate
--- PASS: TestTTLCacheInvalidate (0.00s)
=== RUN   TestTTLCacheSetTTL
--- PASS: TestTTLCacheSetTTL (0.00s)
=== RUN   TestTTLCacheIsExpired
--- PASS: TestTTLCacheIsExpired (0.15s)
PASS - internal/cache

=== RUN   TestBuildClusterSummary
--- PASS: TestBuildClusterSummary (0.00s)
... (6/6 tests passed)
PASS - internal/datasource

Total: 10/10 tests passing ✓
```

### Compilation
```
go build -o ./bin/k8s-monitor ./cmd/k8s-monitor/
✓ Success - no warnings, no errors
```

### Manual Testing
- ✓ `--refresh` flag now changes interval
- ✓ Config file YAML parsing works
- ✓ `--no-color` flag accepted (UI impl pending)
- ✓ Backwards compatible with existing usage

---

## Impact Assessment

| Issue | Severity | User Impact | Fixed |
|-------|----------|-------------|-------|
| --refresh flag broken | HIGH | Users stuck with 10s refresh | ✅ Yes |
| YAML config ignored | HIGH | Config files don't work | ✅ Yes |
| --no-color ignored | MEDIUM | Can't disable colors | ✅ Partial* |
| Timeout config unused | LOW | Can't tune performance | 📝 Noted |
| TLS insecurity | INFO | Dormant code risk | 📝 Documented |

\* Config propagation complete, UI implementation pending

---

## Recommendations

### For v0.1.1 (Patch Release)

1. ✅ **DONE**: Fix --refresh flag (CRITICAL)
2. ✅ **DONE**: Fix YAML config parsing (CRITICAL)
3. 🔄 **IN PROGRESS**: Complete --no-color UI implementation
4. 📋 **TODO**: Wire up Timeout and MaxConcurrent configs
5. 📋 **TODO**: Add integration test for config loading

### For v0.2 (Minor Release)

1. Implement proper kubelet direct mode with certificates
2. Add config validation on startup
3. Add `--validate-config` command
4. Improve error messages for config problems

### For Documentation

1. Update README with corrected config examples
2. Add troubleshooting section for config issues
3. Document current limitation: proxy mode only
4. Add example configs for common scenarios

---

## Files Changed

```
cmd/k8s-monitor/main.go:
  - Added time import
  - Fixed refresh flag assignment (line 88)
  - Added no-color flag handling (lines 92-95)

internal/app/config.go:
  - Removed unused fmt import
  - Added NoColor field to Config struct
  - Changed to explicit nested key mapping
  - Updated all viper.SetDefault() calls to use nested keys
  - Replaced Unmarshal with explicit GetString/GetDuration calls
```

---

## Credits

**Reported by**: Code Review Team
**Fixed by**: Development Team
**Review Date**: 2025-01-15
**Fix Date**: 2025-01-15

---

## Changelog Entry

```markdown
## [v0.1.1] - Unreleased

### Fixed
- CLI `--refresh` flag now correctly overrides default refresh interval
- YAML configuration file parsing now works with nested structure
- `--no-color` flag is now properly passed to configuration system
- Added missing `time` package import to main.go

### Known Issues
- Timeout and MaxConcurrent config values defined but not yet applied
- UI layer not yet using NoColor config (implementation pending)
- kubelet direct mode has insecure TLS placeholder (feature not active)
```

---

**Status**: Ready for v0.1.1 patch release
