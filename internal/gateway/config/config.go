package config

import (
	"flag"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	port := flag.String("port", ":8080", "server port")
	flag.Parse()

	if envPort := os.Getenv("PORT"); envPort != "" {
		*port = ":" + envPort
	}

	return &Config{
		Port: *port,
	}, nil
}
