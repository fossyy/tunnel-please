package key

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

var (
	rsaGenerateKey  = rsa.GenerateKey
	pemEncode       = pem.Encode
	sshNewPublicKey = func(key interface{}) (ssh.PublicKey, error) {
		return ssh.NewPublicKey(key)
	}
	pubKeyWrite = func(w io.Writer, data []byte) (int, error) {
		return w.Write(data)
	}
	osOpenFile = os.OpenFile
)

func GenerateSSHKeyIfNotExist(keyPath string) error {
	if _, err := os.Stat(keyPath); err == nil {
		log.Printf("SSH key already exists at %s", keyPath)
		return nil
	}

	log.Printf("SSH key not found at %s, generating new key pair...", keyPath)

	privateKey, err := rsaGenerateKey(rand.Reader, 4096)
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

	privateKeyFile, err := osOpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer privateKeyFile.Close()

	if err := pemEncode(privateKeyFile, privateKeyPEM); err != nil {
		return err
	}

	publicKey, err := sshNewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return err
	}

	pubKeyPath := keyPath + ".pub"
	pubKeyFile, err := osOpenFile(pubKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer pubKeyFile.Close()

	_, err = pubKeyWrite(pubKeyFile, ssh.MarshalAuthorizedKey(publicKey))
	if err != nil {
		return err
	}

	log.Printf("SSH key pair generated successfully at %s and %s", keyPath, pubKeyPath)
	return nil
}
