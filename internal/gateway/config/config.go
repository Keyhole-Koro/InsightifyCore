package config

import (
	"flag"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port string
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

	return &Config{
		Port: *port,
	}, nil
}
