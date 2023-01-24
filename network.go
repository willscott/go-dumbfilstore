package main

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
)

func (sh *StorageHandler) NetProtectAdd(ctx context.Context, acl []peer.ID) error {
	return nil
}

func (sh *StorageHandler) NetProtectRemove(ctx context.Context, acl []peer.ID) error {
	return nil
}
