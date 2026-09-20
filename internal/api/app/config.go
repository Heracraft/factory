// Package app assembles the api process from its environment
// (05-control-plane-api.md §5.1, §5.14): the HTTP app, the gRPC app with
// the ops driver and background loops, or both in one process for
// development and tests.
package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is read from the environment Coolify injects.
type Config struct {
	Mode           string // http | grpc | all
	Listen         string
	GRPCListen     string
	InternalListen string
	MetricsListen  string
	Migrate        bool

	DatabaseURL     string
	LogtoIssuer     string
	LogtoM2MID      string
	LogtoM2MSecret  string
	APIResource     string
	KeyVaultURL     string
	KeyVaultKeyName string
	BlobAccountURL  string
	BlobContainer   string
	GRPCServerCert  string
	GRPCServerKey   string
	GRPCServerNames []string
	GatewayHost     string
	GatewayPort     int
	ResendAPIKey    string
	NotifyFrom      string
	DashboardURL    string
	BaseRef         string
	ReplicaID       string
	// Dev enables the in-memory Key Vault and self-issued certificates;
	// it is refused unless REPOSE_DEV=1 and never in a container with a
	// KEYVAULT_URL.
	Dev bool
}

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// FromEnv reads the configuration.
func FromEnv() (Config, error) {
	c := Config{
		Mode:            env("API_MODE", "all"),
		Listen:          env("API_LISTEN", ":8080"),
		GRPCListen:      env("API_GRPC_LISTEN", ":8443"),
		InternalListen:  env("API_INTERNAL_LISTEN", ":8444"),
		MetricsListen:   env("API_METRICS_LISTEN", ":9103"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		LogtoIssuer:     strings.TrimRight(os.Getenv("LOGTO_ISSUER"), "/"),
		LogtoM2MID:      os.Getenv("LOGTO_M2M_CLIENT_ID"),
		LogtoM2MSecret:  os.Getenv("LOGTO_M2M_CLIENT_SECRET"),
		APIResource:     env("API_RESOURCE", "https://api.repose.herakraft.co"),
		KeyVaultURL:     os.Getenv("KEYVAULT_URL"),
		KeyVaultKeyName: env("KEYVAULT_KEY_NAME", "repose-dek-kek"),
		BlobAccountURL:  os.Getenv("BLOB_ACCOUNT_URL"),
		BlobContainer:   env("BLOB_CONTAINER", "repose-snapshots"),
		GRPCServerCert:  os.Getenv("GRPC_SERVER_CERT"),
		GRPCServerKey:   os.Getenv("GRPC_SERVER_KEY"),
		GatewayHost:     env("GATEWAY_HOST", "ssh.repose.herakraft.co"),
		ResendAPIKey:    os.Getenv("RESEND_API_KEY"),
		NotifyFrom:      env("NOTIFY_FROM", "repose <notify@repose.herakraft.co>"),
		DashboardURL:    env("DASHBOARD_URL", "https://repose.herakraft.co"),
		BaseRef:         os.Getenv("BASE_REF"),
		ReplicaID:       env("REPLICA_ID", ""),
		Dev:             os.Getenv("REPOSE_DEV") == "1",
	}
	if names := os.Getenv("GRPC_SERVER_NAMES"); names != "" {
		c.GRPCServerNames = strings.Split(names, ",")
	}
	port, err := strconv.Atoi(env("GATEWAY_PORT", "22"))
	if err != nil {
		return c, errors.New("GATEWAY_PORT is not a number")
	}
	c.GatewayPort = port
	if c.ReplicaID == "" {
		h, err := os.Hostname()
		if err != nil || h == "" {
			h = "replica"
		}
		c.ReplicaID = h
	}
	return c, c.Validate()
}

// Validate checks what every mode needs.
func (c Config) Validate() error {
	switch c.Mode {
	case "http", "grpc", "all":
	default:
		return fmt.Errorf("API_MODE must be http, grpc or all (got %q)", c.Mode)
	}
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.KeyVaultURL == "" && !c.Dev {
		return errors.New("KEYVAULT_URL is required (or REPOSE_DEV=1 for the in-memory key vault)")
	}
	if c.Mode != "grpc" && c.LogtoIssuer == "" && !c.Dev {
		return errors.New("LOGTO_ISSUER is required for the http app")
	}
	if c.Mode != "grpc" && (c.LogtoM2MID == "" || c.LogtoM2MSecret == "") && !c.Dev {
		return errors.New("LOGTO_M2M_CLIENT_ID and LOGTO_M2M_CLIENT_SECRET are required for the http app")
	}
	if (c.GRPCServerCert == "") != (c.GRPCServerKey == "") {
		return errors.New("GRPC_SERVER_CERT and GRPC_SERVER_KEY go together")
	}
	if c.Mode != "http" && c.GRPCServerCert == "" && !c.Dev {
		return errors.New("GRPC_SERVER_CERT and GRPC_SERVER_KEY are required for the grpc app")
	}
	return nil
}
