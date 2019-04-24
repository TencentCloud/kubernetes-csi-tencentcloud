package util

import (
	"sync"
)

// RequestItem is the interface required to manage in flight requests.
type RequestItem interface {
	// The CSI data types are generated using a protobuf.
	// The generated structures are guaranteed to implement the Stringer interface.
	// Example: https://github.com/container-storage-interface/spec/blob/master/lib/go/csi/csi.pb.go#L3508
	// We can use the generated string as the key of our internal inflight database of requests.
	String() string
}

// Idempotent is a struct used to manage in flight requests.
type Idempotent struct {
	mux     *sync.Mutex
	idemMap map[string]bool
}

// NewIdempotent instanciates a Idempotent structures.
func NewIdempotent() *Idempotent {
	return &Idempotent{
		mux:     &sync.Mutex{},
		idemMap: make(map[string]bool),
	}
}

func (idem *Idempotent) Insert(entry RequestItem) bool {
	idem.mux.Lock()
	defer idem.mux.Unlock()

	hash := entry.String()

	_, ok := idem.idemMap[hash]
	if ok {
		return false
	}

	idem.idemMap[hash] = true
	return true
}

func (idem *Idempotent) Delete(h RequestItem) {
	idem.mux.Lock()
	defer idem.mux.Unlock()

	delete(idem.idemMap, h.String())
}
