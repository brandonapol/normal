package audit

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

type Signer interface {
	KeyID() string
	Sign(message []byte) ([]byte, error)
	PublicKey() []byte
}

type SoftwareSigner struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	keyID   string
}

func NewSoftwareSigner() (*SoftwareSigner, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating signing key: %w", err)
	}
	return newSoftwareSigner(public, private), nil
}

func NewSoftwareSignerFromSeed(seed []byte) (*SoftwareSigner, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("a signing seed must be %d bytes, got %d", ed25519.SeedSize, len(seed))
	}
	private := ed25519.NewKeyFromSeed(seed)
	public, ok := private.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("unexpected public key type")
	}
	return newSoftwareSigner(public, private), nil
}

func newSoftwareSigner(public ed25519.PublicKey, private ed25519.PrivateKey) *SoftwareSigner {
	sum := sha256.Sum256(public)
	return &SoftwareSigner{private: private, public: public, keyID: hex.EncodeToString(sum[:8])}
}

func (s *SoftwareSigner) KeyID() string { return s.keyID }

func (s *SoftwareSigner) PublicKey() []byte {
	out := make([]byte, len(s.public))
	copy(out, s.public)
	return out
}

func (s *SoftwareSigner) Sign(message []byte) ([]byte, error) {
	if s.private == nil {
		return nil, fmt.Errorf("signer has no key")
	}
	return ed25519.Sign(s.private, message), nil
}

func KeyIDFor(publicKey []byte) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:8])
}

func VerifySignature(publicKey, message []byte, signature string) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("public key must be %d bytes, got %d", ed25519.PublicKeySize, len(publicKey))
	}
	raw, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("signature is not valid base64: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), message, raw) {
		return fmt.Errorf("signature does not match this key")
	}
	return nil
}

func EncodeSignature(signature []byte) string {
	return base64.StdEncoding.EncodeToString(signature)
}
