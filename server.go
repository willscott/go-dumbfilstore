package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/go-state-types/exitcode"
	"github.com/filecoin-project/lotus/api"
	lotusclient "github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/chain/types"
	lotuscliutil "github.com/filecoin-project/lotus/cli/util"
	"github.com/filecoin-project/lotus/lib/rpcenc"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"
)

var syntheticAddress, _ = address.NewIDAddress(88)

type StorageHandler struct {
	api      api.FullNode
	wallet   api.Wallet
	isMiner  bool
	listener net.Listener

	storage *filestore
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
	return syntheticAddress, nil
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

func (sh *StorageHandler) StateMarketBalance(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MarketBalance, error) {
	zero := big.NewInt(0)
	return api.MarketBalance{
		Escrow: zero,
		Locked: zero,
	}, nil
}

func (sh *StorageHandler) StateAccountKey(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	return sh.api.StateAccountKey(ctx, addr, tsk)
}

func (sh *StorageHandler) StateDealProviderCollateralBounds(ctx context.Context, size abi.PaddedPieceSize, verified bool, tsk types.TipSetKey) (api.DealCollateralBounds, error) {
	return api.DealCollateralBounds{
		Min: big.NewInt(0),
		Max: big.Mul(big.NewIntUnsigned(2_000_000_000), big.NewIntUnsigned(1_000_000_000_000_000_000)),
	}, nil
}

func (sh *StorageHandler) MpoolPending(ctx context.Context, tsk types.TipSetKey) ([]*types.SignedMessage, error) {
	return []*types.SignedMessage{}, nil
}

func (sh *StorageHandler) StateMinerInfo(ctx context.Context, addr address.Address, tsk types.TipSetKey) (api.MinerInfo, error) {
	if addr == syntheticAddress || addr.Empty() {
		return api.MinerInfo{
			Worker: syntheticAddress,
		}, nil
	}
	return sh.api.StateMinerInfo(ctx, addr, tsk)
}

func (sh *StorageHandler) StateGetActor(ctx context.Context, actor address.Address, tsk types.TipSetKey) (*types.Actor, error) {
	return nil, nil
}

func (sh *StorageHandler) StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error) {
	return sh.api.StateLookupID(ctx, addr, tsk)
}

func (sh *StorageHandler) StateCall(ctx context.Context, msg *types.Message, tsk types.TipSetKey) (*api.InvocResult, error) {
	return &api.InvocResult{
		MsgCid: msg.Cid(),
		Msg:    msg,
		MsgRct: &types.MessageReceipt{
			ExitCode: exitcode.Ok,
		},
		GasCost: api.MsgGasCost{Message: msg.Cid(), GasUsed: abi.NewTokenAmount(0), TotalCost: abi.NewTokenAmount(0)},
	}, nil
}

func (sh *StorageHandler) StateWaitMsg(ctx context.Context, msg cid.Cid, confidence uint64) (*api.MsgLookup, error) {
	ml := &api.MsgLookup{
		Message: msg,
		Receipt: types.MessageReceipt{
			ExitCode: exitcode.Ok,
		},
	}
	return ml, nil
}

func (sh *StorageHandler) MpoolPushMessage(ctx context.Context, msg *types.Message, spec *api.MessageSendSpec) (*types.SignedMessage, error) {
	mb, err := msg.ToStorageBlock()
	if err != nil {
		return nil, err
	}
	sig, err := sh.wallet.WalletSign(ctx, msg.From, mb.Cid().Bytes(), api.MsgMeta{
		Type:  api.MTChainMsg,
		Extra: mb.RawData(),
	})
	if err != nil {
		return nil, err
	}
	return &types.SignedMessage{
		Message:   *msg,
		Signature: *sig,
	}, nil
}

/* storage proxy */
func (sh *StorageHandler) WorkerJobs(ctx context.Context) (map[uuid.UUID][]storiface.WorkerJob, error) {
	return map[uuid.UUID][]storiface.WorkerJob{}, nil
}

func (sh *StorageHandler) WalletNew(ctx context.Context, kt types.KeyType) (address.Address, error) {
	return sh.wallet.WalletNew(ctx, kt)
}
func (sh *StorageHandler) WalletHas(ctx context.Context, a address.Address) (bool, error) {
	return sh.wallet.WalletHas(ctx, a)
}
func (sh *StorageHandler) WalletList(ctx context.Context) ([]address.Address, error) {
	return sh.wallet.WalletList(ctx)
}

func (sh *StorageHandler) WalletSign(ctx context.Context, signer address.Address, toSign []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	return sh.wallet.WalletSign(ctx, signer, toSign, meta)
}

func (sh *StorageHandler) WalletExport(ctx context.Context, a address.Address) (*types.KeyInfo, error) {
	return sh.wallet.WalletExport(ctx, a)
}
func (sh *StorageHandler) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	return sh.wallet.WalletImport(ctx, ki)
}
func (sh *StorageHandler) WalletDelete(ctx context.Context, a address.Address) error {
	return sh.wallet.WalletDelete(ctx, a)
}

func Serve(ctx *cli.Context) error {
	store, err := NewStore(ctx.String("root"))
	if err != nil {
		return err
	}
	readerHandler, readerServerOpt := rpcenc.ReaderParamDecoder()
	fullServer := jsonrpc.NewServer(readerServerOpt)
	minerServer := jsonrpc.NewServer(readerServerOpt)

	ainfo := lotuscliutil.ParseApiInfo(ctx.String("api"))
	addr, err := ainfo.DialArgs("v1")
	if err != nil {
		return err
	}
	lapi, closer, err := lotusclient.NewFullNodeRPCV1(ctx.Context, addr, nil)
	if err != nil {
		return err
	}
	defer closer()
	var wallet api.Wallet
	if ctx.String("wallet") == "internal" {
		_, err := os.Stat(".wallet")
		if errors.Is(err, os.ErrNotExist) {
			makeWallet(".wallet")
		} else if err != nil {
			return err
		}
		wapi, err := loadWallet(".wallet")
		if err != nil {
			return err
		}
		wallet = wapi
	} else {
		winfo := lotuscliutil.ParseApiInfo(ctx.String("wallet"))
		addr, err := winfo.DialArgs("v1")
		if err != nil {
			return err
		}
		wapi, closer, err := lotusclient.NewWalletRPCV0(ctx.Context, addr, nil)
		if err != nil {
			return err
		}
		defer closer()
		wallet = wapi
	}
	fullHandler := &StorageHandler{lapi, wallet, false, nil, store}
	minerHandler := &StorageHandler{lapi, wallet, true, nil, store}

	fullServer.Register("Filecoin", fullHandler)
	minerServer.Register("Filecoin", minerHandler)

	server := http.Server{}
	mux := http.NewServeMux()
	mux.Handle("/rpc/v1", fullServer)
	mux.Handle("/rpc/v0", minerServer)
	mux.Handle("/rpc/streams/v0/push/", readerHandler)
	mux.Handle("/sector/", store.retrieveHandler())
	server.Handler = logRequest(mux)

	listenStr := ctx.String("listen")
	listener, err := net.Listen("tcp", listenStr)
	if err != nil {
		return err
	}
	fullHandler.listener = listener
	minerHandler.listener = listener
	return server.Serve(listener)
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}
