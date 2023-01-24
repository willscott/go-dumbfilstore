package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sync"
	"sync/atomic"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/storage/sealer/fr32"
	"github.com/filecoin-project/lotus/storage/sealer/tarutil"
	"github.com/gorilla/mux"
)

type filestore struct {
	root string
	i    index
	l    sync.RWMutex
}

// the serialized structure inside of the index file.
type index struct {
	N        uint64
	Metadata map[uint64]api.PieceDealInfo
}

func NewStore(root string) (*filestore, error) {
	_, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(root, 0770); err != nil {
			return nil, fmt.Errorf("could not mk store at %s: %w", root, err)
		}
	} else if err != nil {
		return nil, err
	}

	i := index{0, make(map[uint64]api.PieceDealInfo)}
	if _, err := os.Stat(path.Join(root, "index")); err == nil {
		idx, err := os.ReadFile(path.Join(root, "index"))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(idx, &i); err != nil {
			return nil, err
		}
	} else {
		b, _ := json.Marshal(i)
		if err := os.WriteFile(path.Join(root, "index"), b, 0660); err != nil {
			return nil, err
		}
	}

	return &filestore{root, i, sync.RWMutex{}}, nil
}

func (f *filestore) saveIndex() error {
	fi, err := os.OpenFile(path.Join(f.root, "index"), os.O_TRUNC|os.O_WRONLY, 0660)
	if err != nil {
		return err
	}
	defer fi.Close()
	b, _ := json.Marshal(f.i)
	if _, err := fi.Write(b); err != nil {
		return err
	}
	return nil
}

func (f *filestore) Add(r io.Reader, md api.PieceDealInfo) (uint64, error) {
	f.l.Lock()
	alloc := atomic.AddUint64(&f.i.N, 1) - 1
	f.i.Metadata[alloc] = md
	f.saveIndex()
	f.l.Unlock()

	fi, err := os.OpenFile(path.Join(f.root, fmt.Sprintf("%d.sector", alloc)), os.O_CREATE|os.O_WRONLY, 0660)
	if err != nil {
		return 0, err
	}
	defer fi.Close()
	if _, err := io.Copy(fi, r); err != nil {
		return 0, err
	}

	return alloc, nil
}

func (f *filestore) GetMeta(n uint64) *api.PieceDealInfo {
	pdi, ok := f.i.Metadata[n]
	if !ok {
		return nil
	}
	return &pdi
}

func (f *filestore) retrieveHandler() http.Handler {
	mux := mux.NewRouter()

	mux.HandleFunc("/{type}/{id}/{spt}/allocated/{offset}/{size}", f.hasAllocated).Methods("GET")
	mux.HandleFunc("/{type}/{id}", f.get).Methods("GET")
	return mux
}

func (f *filestore) hasAllocated(w http.ResponseWriter, r *http.Request) {
	// parallels storage/paths/http_handler.go 'remoteGetAllocated'
	vars := mux.Vars(r)

	id_str := vars["id"]
	var id uint64
	if _, err := fmt.Sscanf(id_str, "%d", &id); err != nil {
		w.WriteHeader(500)
		return
	}

	f.l.RLock()
	_, ok := f.i.Metadata[id]
	f.l.RUnlock()

	if ok {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
}

var CopyBuf = 1 << 20

func (f *filestore) get(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	id_str := vars["id"]
	var id uint64
	if _, err := fmt.Sscanf(id_str, "%d", &id); err != nil {
		w.WriteHeader(500)
		return
	}

	f.l.RLock()
	md, ok := f.i.Metadata[id]
	f.l.RUnlock()

	if ok {
		p := path.Join(f.root, fmt.Sprintf("%d.sector", id))

		stat, err := os.Stat(p)
		if err != nil {
			w.WriteHeader(500)
			return
		}

		if stat.IsDir() {
			if _, has := r.Header["Range"]; has {
				fmt.Println("Range not supported on directories")
				w.WriteHeader(500)
				return
			}

			w.Header().Set("Content-Type", "application/x-tar")
			w.WriteHeader(200)

			err := tarutil.TarDirectory(p, w, make([]byte, CopyBuf))
			if err != nil {
				fmt.Printf("send tar: %+v\n", err)
				return
			}
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
			// will do a ranged read over the file at the given path if the caller has asked for a ranged read in the request headers.

			f, err := os.Open(p)
			if err != nil {
				w.WriteHeader(500)
				return
			}
			defer f.Close()

			paddedSize := md.DealProposal.PieceSize

			ssr := paddedReaderAt{f, [128]byte{}, 0, int64(paddedSize)}

			http.ServeContent(w, r, p, stat.ModTime(), &ssr)
		}
		return
	}
	w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
}

type paddedReaderAt struct {
	internal io.ReaderAt
	scratch  [128]byte
	cursor   int64
	totalLen int64
}

func (p *paddedReaderAt) Read(b []byte) (int, error) {
	// we need to fr32 pad it to undo https://github.com/filecoin-project/lotus/blob/4682e8f326ceb9c0bf0c287c0add176394789152/storage/sealer/piece_provider.go#L100

	if len(b) < 128 {
		return 0, io.ErrShortBuffer
	}

	r, err := p.internal.ReadAt(p.scratch[:127], p.cursor)
	// Assumes len(in)%127==0 and len(out)%128==0
	fr32.Pad(p.scratch[:127], b[:128])
	p.cursor += int64(r)

	if err == io.EOF {
		if p.cursor >= p.totalLen {
			return r, io.EOF
		}
		return r, nil
	} else if err != nil {
		return r, err
	} else {
		return 128, nil
	}
}

func (p *paddedReaderAt) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekStart {
		p.cursor = offset
		return p.cursor, nil
	}
	if whence == io.SeekEnd {
		p.cursor = p.totalLen + offset
		return p.cursor, nil
	}
	p.cursor = p.cursor + offset
	return p.cursor, nil
}
