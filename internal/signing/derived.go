package signing

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
)

// NewStoreAppSigner returns the configured store app signer or derives a stable
// secondary signer from the demo private key for local/demo use.
func NewStoreAppSigner(configuredPrivateKey string, demoPrivateKey string) (Signer, error) {
	if trimmed := strings.TrimSpace(configuredPrivateKey); trimmed != "" {
		return NewEnvSigner(trimmed)
	}

	seed := strings.TrimSpace(strings.TrimPrefix(demoPrivateKey, "0x"))
	if seed == "" {
		return nil, fmt.Errorf("missing demo private key seed")
	}

	for i := 0; i < 8; i++ {
		hash := crypto.Keccak256([]byte(fmt.Sprintf("nitrolite-store-app:%s:%d", seed, i)))
		candidate := "0x" + hex.EncodeToString(hash)
		signer, err := NewEnvSigner(candidate)
		if err == nil {
			return signer, nil
		}
	}

	return nil, fmt.Errorf("failed to derive store app signer")
}
