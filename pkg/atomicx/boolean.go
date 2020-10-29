package atomicx

import "sync/atomic"

type Bool uint32

func NewBool(val bool) *Bool {
    b := new(Bool)
    b.Set(val)
    return b
}

func (b *Bool) Set(val bool) {
    if val {
        atomic.StoreUint32((*uint32)(b), 1)
    } else {
        atomic.StoreUint32((*uint32)(b), 0)
    }
}

func (b *Bool) T() bool {
    return atomic.LoadUint32((*uint32)(b)) == 1
}

func (b *Bool) F() bool {
    return atomic.LoadUint32((*uint32)(b)) == 0
}
