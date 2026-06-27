package core

// FNV-1a 64-bit basis/prime — the shared primitives for every alloc-free layer-hash
// fold in the codebase (core's elevationLayerHash, render's layersHash/foldLayer, and
// shop's inventory fingerprint). Kept here, not beside any one caller, so the hashing
// contract has one obvious home.
const (
	FNVOffset64 = uint64(1469598103934665603)
	FNVPrime64  = uint64(1099511628211)
)

// FoldLayerRow folds one row's bytes into FNV-1a digest h with a row separator
// so ragged splits can't collide. Allocation-free; the one shared row-fold used
// by every layer-hash fold (here and render's foldLayer).
func FoldLayerRow(h uint64, row string) uint64 {
	for i := 0; i < len(row); i++ {
		h = (h ^ uint64(row[i])) * FNVPrime64
	}
	return (h ^ 0xff) * FNVPrime64 // row separator
}
