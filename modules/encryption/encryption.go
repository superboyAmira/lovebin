package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

// Encryption interface for dependency injection
type Encryption interface {
	Encrypt(data []byte, password string) ([]byte, []byte, error) // returns encrypted data and salt
	Decrypt(encryptedData []byte, salt []byte, password string) ([]byte, error)
	GenerateKey() ([]byte, error)
}

type encryptionImpl struct {
	iterations int
}

// Config holds encryption configuration
type Config struct {
	Iterations int // PBKDF2 iterations
}

// Init initializes the encryption module
func Init(cfg Config) Encryption {
	iterations := cfg.Iterations
	if iterations == 0 {
		iterations = 100000 // default
	}
	return &encryptionImpl{iterations: iterations}
}

func (e *encryptionImpl) Encrypt(data []byte, password string) ([]byte, []byte, error) {
	// Generate salt
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, nil, err
	}

	// Derive key from password
	key := pbkdf2.Key([]byte(password), salt, e.iterations, 32, sha256.New)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return ciphertext, salt, nil
}

func (e *encryptionImpl) Decrypt(encryptedData []byte, salt []byte, password string) ([]byte, error) {
	// Derive key from password
	key := pbkdf2.Key([]byte(password), salt, e.iterations, 32, sha256.New)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := encryptedData[:nonceSize], encryptedData[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func (e *encryptionImpl) GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

// GenerateURLKey generates a URL-safe key for paste IDs
func GenerateURLKey() (string, error) {
	key := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(key), nil
}
