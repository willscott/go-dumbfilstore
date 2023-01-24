package main

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

func (sh *StorageHandler) StorageAttach(context.Context, storiface.StorageInfo, fsutil.FsStat) error {
	return nil
}

func (sh *StorageHandler) StorageReportHealth(context.Context, storiface.ID, storiface.HealthReport) error {
	return nil
}

func (sh *StorageHandler) StorageLock(ctx context.Context, sector abi.SectorID, read storiface.SectorFileType, write storiface.SectorFileType) error {
	return nil
}
