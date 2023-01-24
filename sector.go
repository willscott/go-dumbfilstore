package main

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	stbig "github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"
	"github.com/filecoin-project/lotus/api"
	cminer "github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
	"github.com/ipfs/go-cid"
)

func (sh *StorageHandler) SectorAddPieceToAny(ctx context.Context, size abi.UnpaddedPieceSize, r storiface.Data, d api.PieceDealInfo) (api.SectorOffset, error) {
	id, err := sh.storage.Add(r, d)
	if err != nil {
		return api.SectorOffset{
			Sector: abi.SectorNumber(id),
			Offset: 0,
		}, err
	}
	return api.SectorOffset{
		Sector: abi.SectorNumber(id),
		Offset: 0,
	}, nil
}

func (sh *StorageHandler) SectorsSummary(ctx context.Context) (map[api.SectorState]int, error) {
	sh.storage.l.RLock()
	defer sh.storage.l.RUnlock()

	return map[api.SectorState]int{
		// We have a sector - allows import of offline deals
		api.SectorState("PreCommit1"): 1,
		api.SectorState("WaitSeed"):   1,
		// We have something available
		api.SectorState("Proving"): int(sh.storage.i.N),
	}, nil
}

func (sh *StorageHandler) SectorsStatus(ctx context.Context, sid abi.SectorNumber, showOnChainInfo bool) (api.SectorInfo, error) {
	sh.storage.l.RLock()
	defer sh.storage.l.RUnlock()

	md := sh.storage.GetMeta(uint64(sid))
	if sh.storage.i.N > uint64(sid) {
		dpc, _ := md.DealProposal.Cid()
		zero := stbig.NewInt(0)

		return api.SectorInfo{
			SectorID: sid,
			State:    api.SectorState("Proving"),
			CommD:    &md.DealProposal.PieceCID,
			CommR:    &md.DealProposal.PieceCID,
			Proof:    []byte{},
			Deals: []abi.DealID{
				md.DealID,
			},
			Pieces: []api.SectorPiece{
				{
					Piece: abi.PieceInfo{
						Size:     0,
						PieceCID: md.DealProposal.PieceCID,
					},
					DealInfo: md,
				},
			},
			Ticket:               api.SealTicket{Value: abi.SealRandomness{}, Epoch: 0},
			Seed:                 api.SealSeed{Value: abi.InteractiveSealRandomness{}, Epoch: 0},
			PreCommitMsg:         &dpc,
			CommitMsg:            &dpc,
			Retries:              0,
			ToUpgrade:            false,
			ReplicaUpdateMessage: &dpc,
			LastErr:              "",
			Log:                  []api.SectorLog{},
			SealProof:            0,
			Activation:           md.DealProposal.StartEpoch,
			Expiration:           md.DealProposal.EndEpoch,
			DealWeight:           abi.DealWeight(zero),
			VerifiedDealWeight:   abi.DealWeight(zero),
			InitialPledge:        abi.TokenAmount(zero),
			OnTime:               md.DealProposal.EndEpoch,
			Early:                0,
		}, nil
	}

	//todo?
	return api.SectorInfo{}, nil
}

func (sh *StorageHandler) SectorsListInStates(ctx context.Context, ss []api.SectorState) ([]abi.SectorNumber, error) {
	for _, state := range ss {
		if string(state) == "Proving" {
			return make([]abi.SectorNumber, 1), nil
		}
	}
	sn := make([]abi.SectorNumber, 0)
	return sn, nil
}

func (sh *StorageHandler) StateSectorExpiration(ctx context.Context, addr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*cminer.SectorExpiration, error) {
	sh.storage.l.RLock()
	defer sh.storage.l.RUnlock()

	md, ok := sh.storage.i.Metadata[uint64(n)]

	if !ok {
		return nil, nil
	}

	return &cminer.SectorExpiration{
		OnTime: md.DealProposal.EndEpoch,
	}, nil
}

func (sh *StorageHandler) StateSectorGetInfo(ctx context.Context, maddr address.Address, n abi.SectorNumber, tsk types.TipSetKey) (*miner.SectorOnChainInfo, error) {
	sh.storage.l.RLock()
	defer sh.storage.l.RUnlock()

	md, ok := sh.storage.i.Metadata[uint64(n)]

	if !ok {
		return nil, nil
	}

	soci := &miner.SectorOnChainInfo{
		SectorNumber: n,
		DealIDs:      []abi.DealID{md.DealID},
		Activation:   md.DealProposal.StartEpoch,
		Expiration:   md.DealProposal.EndEpoch,
	}
	return soci, nil
}

func (sh *StorageHandler) StorageFindSector(ctx context.Context, sector abi.SectorID, ft storiface.SectorFileType, ssize abi.SectorSize, allowFetch bool) ([]storiface.SectorStorageInfo, error) {
	myAddr := sh.listener.Addr()

	return []storiface.SectorStorageInfo{
		{
			ID:       storiface.ID(fmt.Sprintf("sector-%d", sector.Number)),
			URLs:     []string{fmt.Sprintf("http://%s/sector/%d", myAddr.String(), sector.Number)},
			BaseURLs: []string{},
			Weight:   0,
			CanSeal:  false,
			CanStore: true,
			Primary:  true,
		},
	}, nil
}

func (sh *StorageHandler) StateMarketStorageDeal(ctx context.Context, dealId abi.DealID, tsk types.TipSetKey) (*api.MarketDeal, error) {
	sh.storage.l.RLock()
	defer sh.storage.l.RUnlock()

	md := &api.MarketDeal{
		Proposal: market.DealProposal{},
		State:    market.DealState{},
	}

	for _, di := range sh.storage.i.Metadata {
		if di.DealID == dealId {
			md.Proposal = *di.DealProposal
			md.State.SectorStartEpoch = di.DealProposal.StartEpoch
		}
	}

	return md, nil
}

func (sh *StorageHandler) SectorsUnsealPiece(ctx context.Context, sector storiface.SectorRef, offset storiface.UnpaddedByteIndex, size abi.UnpaddedPieceSize, randomness abi.SealRandomness, commd *cid.Cid) error {
	return nil
}
