package signing

import "testing"

const (
	testPrivateKey = "0x4c0883a69102937d6231471b5dbb6204fe5129617082792ae468d01a3f362318"
	testAddress    = "0x2c7536e3605d9c16a7a3d7b1898e529396a65c23"
)

func TestNewEnvSigner(t *testing.T) {
	t.Parallel()

	signer, err := NewEnvSigner(testPrivateKey)
	if err != nil {
		t.Fatalf("NewEnvSigner() error = %v", err)
	}

	if signer.Address() != testAddress {
		t.Fatalf("Address() = %s, want %s", signer.Address(), testAddress)
	}
	if signer.TxSigner() == nil {
		t.Fatal("TxSigner() = nil")
	}
	if signer.StateSigner() == nil {
		t.Fatal("StateSigner() = nil")
	}
}
