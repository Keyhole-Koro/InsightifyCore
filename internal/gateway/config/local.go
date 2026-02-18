package config

import (
	"os"
	"strings"
)

func localConfig() Config {
	return Config{
		DatabaseURL: "postgres://insightify:insightify@postgres:5432/insightify?sslmode=disable",
		Artifact: ArtifactConfig{
			Enabled:   true,
			Endpoint:  firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_MINIO_ENDPOINT")), "minio:9000"),
			Region:    firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_REGION")), "us-east-1"),
			AccessKey: firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_ACCESS_KEY")), "insightify"),
			SecretKey: firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_SECRET_KEY")), "insightify123"),
			Bucket:    firstNonEmpty(strings.TrimSpace(os.Getenv("ARTIFACT_S3_BUCKET")), "insightify-artifacts"),
			UseSSL:    false,
		},
		Interaction: InteractionConfig{
			ConversationArtifactPath: firstNonEmpty(
				strings.TrimSpace(os.Getenv("INTERACTION_CONVERSATION_ARTIFACT_PATH")),
				"interaction/conversation_history.json",
			),
		},
	}
}
