package signing

import (
	"fmt"

	"github.com/layer-3/nitrolite/pkg/core"
	"github.com/layer-3/nitrolite/pkg/sign"
)

// Signer exposes the two signer forms the example service needs.
type Signer interface {
	StateSigner() core.ChannelSigner
	TxSigner() sign.Signer
	Address() string
}

type envSigner struct {
	stateSigner core.ChannelSigner
	txSigner    sign.Signer
	address     string
}

// NewEnvSigner builds the demo signer from a hex private key.
func NewEnvSigner(privateKeyHex string) (Signer, error) {
	txSigner, err := sign.NewEthereumRawSigner(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx signer: %w", err)
	}

	msgSigner, err := sign.NewEthereumMsgSignerFromRaw(txSigner)
	if err != nil {
		return nil, fmt.Errorf("failed to create message signer: %w", err)
	}

	stateSigner, err := core.NewChannelDefaultSigner(msgSigner)
	if err != nil {
		return nil, fmt.Errorf("failed to create channel signer: %w", err)
	}

	return &envSigner{
		stateSigner: stateSigner,
		txSigner:    txSigner,
		address:     txSigner.PublicKey().Address().String(),
	}, nil
}

func (s *envSigner) StateSigner() core.ChannelSigner {
	return s.stateSigner
}

func (s *envSigner) TxSigner() sign.Signer {
	return s.txSigner
}

func (s *envSigner) Address() string {
	return s.address
}
