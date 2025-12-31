package key

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

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
