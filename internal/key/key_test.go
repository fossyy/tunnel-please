package key

import (
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateSSHKeyIfNotExist(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string) string
		mockSetup func() func()
		wantErr   bool
		errStr    string
		verify    func(t *testing.T, keyPath string)
	}{
		{
			name: "GenerateNewKey",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "id_rsa")
			},
			verify: func(t *testing.T, keyPath string) {
				pubKeyPath := keyPath + ".pub"
				if _, err := os.Stat(keyPath); os.IsNotExist(err) {
					t.Errorf("Private key file not created")
				}
				if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
					t.Errorf("Public key file not created")
				}
				privateKeyBytes, err := os.ReadFile(keyPath)
				if err != nil {
					t.Fatalf("Failed to read private key: %v", err)
				}
				if _, err = ssh.ParseRawPrivateKey(privateKeyBytes); err != nil {
					t.Errorf("Failed to parse private key: %v", err)
				}
				publicKeyBytes, err := os.ReadFile(pubKeyPath)
				if err != nil {
					t.Fatalf("Failed to read public key: %v", err)
				}
				if _, _, _, _, err = ssh.ParseAuthorizedKey(publicKeyBytes); err != nil {
					t.Errorf("Failed to parse public key: %v", err)
				}
			},
		},
		{
			name: "DoNotOverwriteExistingKey",
			setup: func(t *testing.T, tempDir string) string {
				keyPath := filepath.Join(tempDir, "existing_id_rsa")
				dummyPrivate := "dummy private"
				dummyPublic := "dummy public"
				if err := os.WriteFile(keyPath, []byte(dummyPrivate), 0600); err != nil {
					t.Fatalf("Failed to create dummy private key: %v", err)
				}
				if err := os.WriteFile(keyPath+".pub", []byte(dummyPublic), 0644); err != nil {
					t.Fatalf("Failed to create dummy public key: %v", err)
				}
				return keyPath
			},
			verify: func(t *testing.T, keyPath string) {
				gotPrivate, _ := os.ReadFile(keyPath)
				if string(gotPrivate) != "dummy private" {
					t.Errorf("Private key was overwritten")
				}
				gotPublic, _ := os.ReadFile(keyPath + ".pub")
				if string(gotPublic) != "dummy public" {
					t.Errorf("Public key was overwritten")
				}
			},
		},
		{
			name: "CreateNestedDirectories",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "nested", "dir", "id_rsa")
			},
			verify: func(t *testing.T, keyPath string) {
				if _, err := os.Stat(keyPath); os.IsNotExist(err) {
					t.Errorf("Private key file not created in nested directory")
				}
			},
		},
		{
			name: "FailureMkdirAll",
			setup: func(t *testing.T, tempDir string) string {
				dirPath := filepath.Join(tempDir, "file_as_dir")
				if err := os.WriteFile(dirPath, []byte("not a dir"), 0644); err != nil {
					t.Fatalf("Failed to create file: %v", err)
				}
				return filepath.Join(dirPath, "id_rsa")
			},
			wantErr: true,
		},
		{
			name: "PrivateExistsPublicMissing",
			setup: func(t *testing.T, tempDir string) string {
				keyPath := filepath.Join(tempDir, "partial_id_rsa")
				if err := os.WriteFile(keyPath, []byte("private"), 0600); err != nil {
					t.Fatalf("Failed to create private key: %v", err)
				}
				return keyPath
			},
			verify: func(t *testing.T, keyPath string) {
				if _, err := os.Stat(keyPath + ".pub"); !os.IsNotExist(err) {
					t.Errorf("Public key should NOT have been created if private key existed")
				}
			},
		},
		{
			name: "FailureRSAGenerateKey",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "fail_rsa")
			},
			mockSetup: func() func() {
				old := rsaGenerateKey
				rsaGenerateKey = func(random io.Reader, bits int) (*rsa.PrivateKey, error) {
					return nil, errors.New("rsa error")
				}
				return func() { rsaGenerateKey = old }
			},
			wantErr: true,
			errStr:  "rsa error",
		},
		{
			name: "FailureOpenFilePrivate",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "fail_open_private")
			},
			mockSetup: func() func() {
				old := osOpenFile
				osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
					return nil, errors.New("open error")
				}
				return func() { osOpenFile = old }
			},
			wantErr: true,
			errStr:  "open error",
		},
		{
			name: "FailurePemEncode",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "fail_pem")
			},
			mockSetup: func() func() {
				old := pemEncode
				pemEncode = func(out io.Writer, b *pem.Block) error {
					return errors.New("pem error")
				}
				return func() { pemEncode = old }
			},
			wantErr: true,
			errStr:  "pem error",
		},
		{
			name: "FailureSSHNewPublicKey",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "fail_ssh")
			},
			mockSetup: func() func() {
				old := sshNewPublicKey
				sshNewPublicKey = func(key interface{}) (ssh.PublicKey, error) {
					return nil, errors.New("ssh error")
				}
				return func() { sshNewPublicKey = old }
			},
			wantErr: true,
			errStr:  "ssh error",
		},
		{
			name: "FailureOpenFilePublic",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "fail_open_public")
			},
			mockSetup: func() func() {
				old := osOpenFile
				osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
					if filepath.Ext(name) == ".pub" {
						return nil, errors.New("open pub error")
					}
					return os.OpenFile(name, flag, perm)
				}
				return func() { osOpenFile = old }
			},
			wantErr: true,
			errStr:  "open pub error",
		},
		{
			name: "FailurePubKeyWrite",
			setup: func(t *testing.T, tempDir string) string {
				return filepath.Join(tempDir, "fail_write")
			},
			mockSetup: func() func() {
				old := pubKeyWrite
				pubKeyWrite = func(w io.Writer, data []byte) (int, error) {
					return 0, errors.New("write error")
				}
				return func() { pubKeyWrite = old }
			},
			wantErr: true,
			errStr:  "write error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPath := tt.setup(t, tempDir)
			if tt.mockSetup != nil {
				cleanup := tt.mockSetup()
				defer cleanup()
			}

			err := GenerateSSHKeyIfNotExist(keyPath)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateSSHKeyIfNotExist() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errStr != "" && err != nil && err.Error() != tt.errStr {
				t.Errorf("GenerateSSHKeyIfNotExist() error = %v, wantErrStr %v", err, tt.errStr)
			}

			if tt.verify != nil {
				tt.verify(t, keyPath)
			}
		})
	}
}
