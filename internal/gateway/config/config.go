package config

import (
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port     string
	Env      string
	Artifact ArtifactConfig
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

	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		env = "local"
	}

	return &Config{
		Port:     *port,
		Env:      env,
		Artifact: loadArtifactConfig(env),
	}, nil
}

func loadArtifactConfig(env string) ArtifactConfig {
	endpoint := resolveArtifactEndpoint(env)
	return ArtifactConfig{
		Enabled:   strings.EqualFold(strings.TrimSpace(env), "local") || endpoint != "",
		Endpoint:  endpoint,
		Region:    firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_REGION")), "us-east-1"),
		AccessKey: firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_ACCESS_KEY")), strings.TrimSpace(os.Getenv("MINIO_ROOT_USER"))),
		SecretKey: firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_SECRET_KEY")), strings.TrimSpace(os.Getenv("MINIO_ROOT_PASSWORD"))),
		Bucket:    firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_BUCKET")), "insightify-artifacts"),
		UseSSL:    resolveArtifactUseSSL(env),
	}
}

func resolveArtifactEndpoint(env string) string {
	if strings.EqualFold(strings.TrimSpace(env), "local") {
		return firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_MINIO_ENDPOINT")), "minio:9000")
	}
	return strings.TrimSpace(os.Getenv("ARTIFACT_S3_ENDPOINT"))
}

func resolveArtifactUseSSL(env string) bool {
	if strings.EqualFold(strings.TrimSpace(env), "local") {
		return false
	}
	raw := strings.TrimSpace(os.Getenv("ARTIFACT_S3_USE_SSL"))
	if raw == "" {
		return true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
