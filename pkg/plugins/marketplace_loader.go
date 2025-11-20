package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// MarketplaceConfig defines marketplace enablement and additional registries.
type MarketplaceConfig struct {
	EnabledPlugins         map[string]bool              `json:"enabledPlugins,omitempty"`
	ExtraKnownMarketplaces map[string]MarketplaceSource `json:"extraKnownMarketplaces,omitempty"`
}

// MarketplaceSource describes how to reach a marketplace or plugin source.
type MarketplaceSource struct {
	Source string `json:"source"`
	Repo   string `json:"repo,omitempty"`
	URL    string `json:"url,omitempty"`
	Path   string `json:"path,omitempty"`
}

// MarketplaceManifest captures the structure of .claude-plugin/marketplace.json.
type MarketplaceManifest struct {
	Name    string                   `json:"name"`
	Plugins []MarketplacePluginEntry `json:"plugins"`
}

// MarketplacePluginEntry links a plugin name to its source.
type MarketplacePluginEntry struct {
	Name        string
	Description string
	Version     string
	Source      MarketplaceSource
}

// LoadMarketplace resolves enabled plugins from marketplaces, supporting github,
// generic git, and local directories. Only plugins explicitly enabled (value=true)
// are loaded.
func LoadMarketplace(cfg *MarketplaceConfig) ([]*ClaudePlugin, error) {
	if cfg == nil {
		return nil, errors.New("marketplace config is nil")
	}
	requested, err := groupRequestedPlugins(cfg.EnabledPlugins)
	if err != nil {
		return nil, err
	}
	if len(requested) == 0 {
		return nil, nil
	}
	known := mergeMarketplaceSources(defaultMarketplaces(), cfg.ExtraKnownMarketplaces)

	var plugins []*ClaudePlugin
	for marketName, pluginNames := range requested {
		source, ok := known[marketName]
		if !ok {
			return nil, fmt.Errorf("marketplace %s is not configured", marketName)
		}
		manifest, root, err := loadMarketplaceManifest(source)
		if err != nil {
			return nil, fmt.Errorf("load marketplace %s: %w", marketName, err)
		}
		for _, name := range pluginNames {
			entry, ok := manifest.PluginByName(name)
			if !ok {
				return nil, fmt.Errorf("plugin %s not found in marketplace %s", name, marketName)
			}
			plugin, err := loadPluginFromSource(entry.Source, root)
			if err != nil {
				return nil, fmt.Errorf("plugin %s@%s: %w", name, marketName, err)
			}
			plugins = append(plugins, plugin)
		}
	}
	return plugins, nil
}

// PluginByName finds a plugin entry by name.
func (m MarketplaceManifest) PluginByName(name string) (*MarketplacePluginEntry, bool) {
	for i := range m.Plugins {
		if m.Plugins[i].Name == name {
			return &m.Plugins[i], true
		}
	}
	return nil, false
}

func groupRequestedPlugins(enabled map[string]bool) (map[string][]string, error) {
	grouped := make(map[string][]string)
	for key, on := range enabled {
		if !on {
			continue
		}
		plugin, market, err := parsePluginKey(key)
		if err != nil {
			return nil, err
		}
		grouped[market] = append(grouped[market], plugin)
	}
	for k := range grouped {
		sort.Strings(grouped[k])
	}
	return grouped, nil
}

func parsePluginKey(key string) (plugin, market string, err error) {
	parts := strings.Split(strings.TrimSpace(key), "@")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("plugin key %q must be formatted as plugin@marketplace", key)
	}
	plugin = strings.TrimSpace(parts[0])
	market = strings.TrimSpace(parts[1])
	if plugin == "" || market == "" {
		return "", "", fmt.Errorf("plugin key %q must include plugin and marketplace", key)
	}
	return plugin, market, nil
}

func mergeMarketplaceSources(base, extra map[string]MarketplaceSource) map[string]MarketplaceSource {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make(map[string]MarketplaceSource, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func loadMarketplaceManifest(source MarketplaceSource) (*MarketplaceManifest, string, error) {
	localRoot, err := materializeSource(source)
	if err != nil {
		return nil, "", err
	}
	manifestPath := filepath.Join(localRoot, ".claude-plugin", "marketplace.json")
	if _, err := os.Stat(manifestPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			alt := filepath.Join(localRoot, "marketplace.json")
			manifestPath = alt
			if _, err := os.Stat(manifestPath); err != nil {
				return nil, "", fmt.Errorf("marketplace manifest missing under %s", localRoot)
			}
		} else {
			return nil, "", err
		}
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, "", err
	}
	var manifest MarketplaceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, "", fmt.Errorf("decode marketplace.json: %w", err)
	}
	manifest.normalize()
	return &manifest, localRoot, nil
}

func (m *MarketplaceManifest) normalize() {
	if m == nil {
		return
	}
	m.Name = strings.TrimSpace(m.Name)
	for i := range m.Plugins {
		m.Plugins[i].Name = strings.TrimSpace(m.Plugins[i].Name)
		m.Plugins[i].Description = strings.TrimSpace(m.Plugins[i].Description)
		m.Plugins[i].Version = strings.TrimSpace(m.Plugins[i].Version)
	}
}

func loadPluginFromSource(src MarketplaceSource, baseDir string) (*ClaudePlugin, error) {
	switch src.Source {
	case "directory":
		if strings.TrimSpace(src.Path) == "" {
			return nil, errors.New("directory source path is required")
		}
		dir := src.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(baseDir, dir)
		}
		return LoadPluginFromDir(dir)
	case "github":
		if strings.TrimSpace(src.Repo) == "" {
			return nil, errors.New("github source repo is required")
		}
		url := fmt.Sprintf("https://github.com/%s.git", src.Repo)
		cloneDir, err := cloneGitRepo(url)
		if err != nil {
			return nil, err
		}
		return LoadPluginFromDir(cloneDir)
	case "git":
		if strings.TrimSpace(src.URL) == "" {
			return nil, errors.New("git source url is required")
		}
		cloneDir, err := cloneGitRepo(src.URL)
		if err != nil {
			return nil, err
		}
		return LoadPluginFromDir(cloneDir)
	default:
		return nil, fmt.Errorf("unsupported source %q", src.Source)
	}
}

func materializeSource(src MarketplaceSource) (string, error) {
	switch src.Source {
	case "directory":
		if strings.TrimSpace(src.Path) == "" {
			return "", errors.New("directory source path is required")
		}
		if !filepath.IsAbs(src.Path) {
			return filepath.Abs(src.Path)
		}
		return src.Path, nil
	case "github":
		if strings.TrimSpace(src.Repo) == "" {
			return "", errors.New("github source repo is required")
		}
		url := fmt.Sprintf("https://github.com/%s.git", src.Repo)
		return cloneGitRepo(url)
	case "git":
		if strings.TrimSpace(src.URL) == "" {
			return "", errors.New("git source url is required")
		}
		return cloneGitRepo(src.URL)
	default:
		return "", fmt.Errorf("unsupported source %q", src.Source)
	}
}

func cloneGitRepo(url string) (string, error) {
	tmp, err := os.MkdirTemp("", "claude-marketplace-")
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "clone", "--depth=1", url, tmp)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git clone %s: %w", url, err)
	}
	return tmp, nil
}

func defaultMarketplaces() map[string]MarketplaceSource {
	return map[string]MarketplaceSource{}
}

// UnmarshalJSON supports either a structured source object or a simple string
// path (treated as directory source relative to marketplace root).
func (e *MarketplacePluginEntry) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Version     string          `json:"version"`
		Source      json.RawMessage `json:"source"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Name = strings.TrimSpace(raw.Name)
	e.Description = strings.TrimSpace(raw.Description)
	e.Version = strings.TrimSpace(raw.Version)
	if len(raw.Source) == 0 {
		return errors.New("source is required")
	}
	if raw.Source[0] == '"' {
		var path string
		if err := json.Unmarshal(raw.Source, &path); err != nil {
			return err
		}
		e.Source = MarketplaceSource{Source: "directory", Path: path}
		return nil
	}
	var src MarketplaceSource
	if err := json.Unmarshal(raw.Source, &src); err != nil {
		return err
	}
	if err := validateMarketplaceSource(&src); err != nil {
		return err
	}
	e.Source = src
	return nil
}

func validateMarketplaceSource(src *MarketplaceSource) error {
	if src == nil {
		return errors.New("marketplace source is nil")
	}
	switch src.Source {
	case "github", "git", "directory":
		return nil
	case "":
		return errors.New("marketplace source is empty")
	default:
		return fmt.Errorf("unsupported marketplace source %q", src.Source)
	}
}
