package config

import (
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	Env         string
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

	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		env = "local"
	}

	return &Config{
		Port:        *port,
		Env:         env,
		Artifact:    loadArtifactConfig(env),
		Interaction: loadInteractionConfig(),
	}, nil
}

func loadArtifactConfig(env string) ArtifactConfig {
	if strings.EqualFold(strings.TrimSpace(env), "local") {
		return ArtifactConfig{
			Enabled:   true,
			Endpoint:  firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_MINIO_ENDPOINT")), "minio:9000"),
			Region:    firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_REGION")), "us-east-1"),
			AccessKey: firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_ACCESS_KEY")), "insightify"),
			SecretKey: firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_SECRET_KEY")), "insightify123"),
			Bucket:    firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_BUCKET")), "insightify-artifacts"),
			UseSSL:    false,
		}
	}

	endpoint := resolveArtifactEndpoint(env)
	return ArtifactConfig{
		Enabled:   endpoint != "",
		Endpoint:  endpoint,
		Region:    firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_REGION")), "us-east-1"),
		AccessKey: strings.TrimSpace(os.Getenv("ARTIFACT_S3_ACCESS_KEY")),
		SecretKey: strings.TrimSpace(os.Getenv("ARTIFACT_S3_SECRET_KEY")),
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

func loadInteractionConfig() InteractionConfig {
	return InteractionConfig{
		ConversationArtifactPath: firstNonEmpty(
			strings.TrimSpace(os.Getenv("INTERACTION_CONVERSATION_ARTIFACT_PATH")),
			"interaction/conversation_history.json",
		),
	}
}
