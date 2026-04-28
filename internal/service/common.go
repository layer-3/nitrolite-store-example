package service

import (
	"github.com/layer-3/nitrolite-store-example/internal/nitrolite"
)

type clientProvider interface {
	Client() nitrolite.Client
	Health() nitrolite.Health
}

func activeClient(provider clientProvider) (nitrolite.Client, string, error) {
	client := provider.Client()
	health := provider.Health()
	signerAddress := health.SignerAddress
	if client != nil && signerAddress == "" {
		signerAddress = client.GetUserAddress()
	}
	if client == nil || !health.Connected {
		return nil, signerAddress, ErrUnavailable
	}
	return client, signerAddress, nil
}
