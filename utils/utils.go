package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/ssh"
)

type Env struct {
	value map[string]string
	mu    sync.Mutex
}

var env *Env

func init() {
	env = &Env{value: map[string]string{}}
}

func GenerateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	seededRand := mathrand.New(mathrand.NewSource(time.Now().UnixNano() + int64(mathrand.Intn(9999))))
	var result strings.Builder
	for i := 0; i < length; i++ {
		randomIndex := seededRand.Intn(len(charset))
		result.WriteString(string(charset[randomIndex]))
	}
	return result.String()
}

func Getenv(key, defaultValue string) string {
	env.mu.Lock()
	defer env.mu.Unlock()
	if val, ok := env.value[key]; ok {
		return val
	}

	if os.Getenv("HOSTNAME") == "" {
		err := godotenv.Load(".env")
		if err != nil {
			log.Fatalf("Error loading .env file: %s", err)
		}
	}

	val := os.Getenv(key)
	if val == "" {
		val = defaultValue
	}
	env.value[key] = val

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

func GenerateSSHKeyIfNotExist(keyPath string) error {
	if _, err := os.Stat(keyPath); err == nil {
		log.Printf("SSH key already exists at %s", keyPath)
		return nil
	}

	log.Printf("SSH key not found at %s, generating new key pair...", keyPath)

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	privateKeyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer privateKeyFile.Close()

	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return err
	}

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}

	pubKeyPath := keyPath + ".pub"
	pubKeyFile, err := os.OpenFile(pubKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer pubKeyFile.Close()

	_, err = pubKeyFile.Write(ssh.MarshalAuthorizedKey(publicKey))
	if err != nil {
		return err
	}

	log.Printf("SSH key pair generated successfully at %s and %s", keyPath, pubKeyPath)
	return nil
}
