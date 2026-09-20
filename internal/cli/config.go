package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const defaultAPIURL = "https://api.repose.herakraft.co/v1"
const defaultLogtoIssuer = "https://auth.herakraft.co"
const apiResource = "https://api.repose.herakraft.co"

// Config is config.toml (docs/interfaces/cli-config.md).
type Config struct {
	APIURL       string   `toml:"api_url"`
	Gateway      string   `toml:"gateway"`
	DefaultClass string   `toml:"default_class"`
	DefaultAgent string   `toml:"default_agent"`
	SyncExclude  []string `toml:"sync.exclude"`
	LogtoIssuer  string   `toml:"logto_issuer"`
}

func defaultConfig() Config {
	return Config{
		APIURL:       defaultAPIURL,
		DefaultClass: "large",
		DefaultAgent: "claude",
		LogtoIssuer:  defaultLogtoIssuer,
	}
}

func configPath(dir string) string { return filepath.Join(dir, "config.toml") }

func loadConfig(dir string) (Config, error) {
	cfg := defaultConfig()
	b, err := os.ReadFile(configPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if _, err := toml.Decode(string(b), &cfg); err != nil {
		return cfg, err
	}
	if cfg.APIURL == "" {
		cfg.APIURL = defaultAPIURL
	}
	if cfg.LogtoIssuer == "" {
		cfg.LogtoIssuer = defaultLogtoIssuer
	}
	return cfg, nil
}

// Credentials is credentials.json. On macOS the refresh token lives in the
// keychain and this file holds only the issuer; RefreshToken is empty on
// disk there (07-cli.md §9: "cat credentials.json on macOS shows no
// refresh token").
type Credentials struct {
	RefreshToken string    `json:"refresh_token,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	LogtoIssuer  string    `json:"logto_issuer"`
}

func credentialsPath(dir string) string { return filepath.Join(dir, "credentials.json") }

func loadCredentials(dir string) (Credentials, bool, error) {
	b, err := os.ReadFile(credentialsPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return Credentials{}, false, nil
		}
		return Credentials{}, false, err
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return Credentials{}, false, err
	}
	if c.RefreshToken == "" {
		if tok, ok := keychainGet(c.LogtoIssuer); ok {
			c.RefreshToken = tok
		}
	}
	return c, true, nil
}

func saveCredentials(dir string, c Credentials) error {
	onDisk := c
	if keychainAvailable() {
		if err := keychainSet(c.LogtoIssuer, c.RefreshToken); err != nil {
			return err
		}
		onDisk.RefreshToken = ""
	}
	b, err := json.MarshalIndent(onDisk, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(credentialsPath(dir), b, 0o600)
}

func deleteCredentials(dir string) error {
	if keychainAvailable() {
		if c, ok, _ := loadCredentials(dir); ok {
			_ = keychainDelete(c.LogtoIssuer)
		}
	}
	err := os.Remove(credentialsPath(dir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ProjectsCache is projects.json: a local cache of (user, remote) -> project
// and directory -> project for --name projects, regenerable from GET
// /projects.
type ProjectsCache struct {
	ByRemote map[string]CachedProject `json:"-"` // top-level keys, merged into MarshalJSON
	ByDir    map[string]string        `json:"by_dir"`
}

// CachedProject is one entry of ProjectsCache.ByRemote.
type CachedProject struct {
	ProjectID string `json:"project_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
}

func newProjectsCache() ProjectsCache {
	return ProjectsCache{ByRemote: map[string]CachedProject{}, ByDir: map[string]string{}}
}

func projectsPath(dir string) string { return filepath.Join(dir, "projects.json") }

func (c ProjectsCache) MarshalJSON() ([]byte, error) {
	m := map[string]any{"by_dir": c.ByDir}
	for k, v := range c.ByRemote {
		m[k] = v
	}
	return json.Marshal(m)
}

func (c *ProjectsCache) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*c = newProjectsCache()
	for k, v := range raw {
		if k == "by_dir" {
			if err := json.Unmarshal(v, &c.ByDir); err != nil {
				return err
			}
			continue
		}
		var p CachedProject
		if err := json.Unmarshal(v, &p); err != nil {
			return err
		}
		c.ByRemote[k] = p
	}
	return nil
}

func loadProjectsCache(dir string) (ProjectsCache, error) {
	b, err := os.ReadFile(projectsPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return newProjectsCache(), nil
		}
		return newProjectsCache(), err
	}
	c := newProjectsCache()
	if err := json.Unmarshal(b, &c); err != nil {
		return newProjectsCache(), err
	}
	return c, nil
}

func saveProjectsCache(dir string, c ProjectsCache) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(projectsPath(dir), b, 0o600)
}
