package network

import "github.com/anviod/edgeCore/internal/model"

// iprouteBackend applies runtime configuration only; edgeCore DB remains the source of truth on boot.
type iprouteBackend struct{}

func (b *iprouteBackend) Type() BackendType { return BackendIPRoute }

func (b *iprouteBackend) ApplyInterfaceConfig(_ model.NetworkInterface) error {
	return nil
}

func (b *iprouteBackend) ApplyStaticRoute(_ model.StaticRoute) error {
	return nil
}
