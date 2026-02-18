package config

import (
	"flag"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type AppEnv string

const (
	AppEnvLocal AppEnv = "local"
	AppEnvStage AppEnv = "stage"
	AppEnvProd  AppEnv = "prod"
)

type Config struct {
	Port        string
	Env         AppEnv
	DatabaseURL string
	Artifact    ArtifactConfig
	Interaction InteractionConfig
}

type ArtifactConfig struct {
	Enabled   bool
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

func (c ArtifactConfig) CanUseS3() bool {
	if !c.Enabled {
		return false
	}
	return strings.TrimSpace(c.Endpoint) != "" &&
		strings.TrimSpace(c.AccessKey) != "" &&
		strings.TrimSpace(c.SecretKey) != "" &&
		strings.TrimSpace(c.Bucket) != ""
}

type InteractionConfig struct {
	ConversationArtifactPath string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	port := flag.String("port", ":8081", "server port")
	flag.Parse()

	if envPort := os.Getenv("PORT"); envPort != "" {
		if strings.HasPrefix(envPort, ":") {
			*port = envPort
		} else {
			*port = ":" + envPort
		}
	}

	env := normalizeAppEnv(os.Getenv("APP_ENV"))
	cfg := configForEnv(env)
	cfg.Port = *port
	cfg.Env = env

	return &cfg, nil
}

func normalizeAppEnv(raw string) AppEnv {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(AppEnvLocal):
		return AppEnvLocal
	case string(AppEnvStage):
		return AppEnvStage
	case string(AppEnvProd):
		return AppEnvProd
	default:
		return AppEnvLocal
	}
}

func configForEnv(env AppEnv) Config {
	switch env {
	case AppEnvLocal:
		return localConfig()
	case AppEnvStage:
		return stageConfig()
	case AppEnvProd:
		return prodConfig()
	default:
		return localConfig()
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
