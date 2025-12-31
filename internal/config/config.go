package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

func init() {
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(".env"); err != nil {
			log.Printf("Warning: Failed to load .env file: %s", err)
		}
	}
}

func Getenv(key, defaultValue string) string {
	val := os.Getenv(key)
	if val == "" {
		val = defaultValue
	}

	return val
}

func GetBufferSize() int {
	sizeStr := Getenv("BUFFER_SIZE", "32768")
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size < 4096 || size > 1048576 {
		return 32768
	}
	return size
}
