package main

import (
	"context"
	"net"
	"net/http"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	lotusclient "github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/chain/types"
	lotuscliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/urfave/cli/v2"
)

type StorageHandler struct {
	api     api.FullNode
	isMiner bool
}

func (sh *StorageHandler) Version(ctx context.Context) (api.APIVersion, error) {
	if sh.isMiner {
		// v0 miner API expected.
		return api.APIVersion{
			Version:    "",
			APIVersion: api.MinerAPIVersion0,
		}, nil
	}
	return sh.api.Version(ctx)
}

func (sh *StorageHandler) ActorAddress(ctx context.Context) (address.Address, error) {
	return address.NewFromString("")
}

func (sh *StorageHandler) SyncState(ctx context.Context) (*api.SyncState, error) {
	base, err := sh.api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	return &api.SyncState{
		ActiveSyncs: []api.ActiveSync{
			{
				Base:   base,
				Target: base,
				Stage:  api.StageSyncComplete,
				Height: base.Height(),
			},
		},
		VMApplied: 0,
	}, nil
}

func (sh *StorageHandler) ChainNotify(ctx context.Context) (<-chan []*api.HeadChange, error) {
	return sh.api.ChainNotify(ctx)
}

func (sh *StorageHandler) ChainHead(ctx context.Context) (*types.TipSet, error) {
	return sh.api.ChainHead(ctx)
}

func Serve(ctx *cli.Context) error {
	fullServer := jsonrpc.NewServer()
	minerServer := jsonrpc.NewServer()

	ainfo := lotuscliutil.ParseApiInfo(ctx.String("api"))
	addr, err := ainfo.DialArgs("v1")
	if err != nil {
		return err
	}
	api, closer, err := lotusclient.NewFullNodeRPCV1(ctx.Context, addr, nil)
	if err != nil {
		return err
	}
	defer closer()
	fullHandler := &StorageHandler{api, false}

	fullServer.Register("Filecoin", fullHandler)
	minerServer.Register("Filecoin", &StorageHandler{api, true})

	server := http.Server{}
	mux := http.NewServeMux()
	mux.Handle("/rpc/v1", fullServer)
	mux.Handle("/rpc/v0", minerServer)
	server.Handler = mux

	listenStr := ctx.String("listen")
	listener, err := net.Listen("tcp", listenStr)
	if err != nil {
		return err
	}
	return server.Serve(listener)
}
