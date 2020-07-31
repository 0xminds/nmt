package nmt_test

import (
	"bytes"
	"crypto"
	_ "crypto/sha256"
	"reflect"
	"testing"

	"github.com/lazyledger/nmt"
	"github.com/lazyledger/nmt/defaulthasher"
	"github.com/lazyledger/nmt/namespace"
)

const (
	LeafPrefix = defaulthasher.LeafPrefix
	NodePrefix = defaulthasher.NodePrefix
)

func TestFromNamespaceAndData(t *testing.T) {
	tests := []struct {
		name      string
		namespace []byte
		data      []byte
		want      *namespace.PrefixedData
	}{
		0: {"simple case", []byte("namespace1"), []byte("data1"), namespace.NewPrefixedData(10, append([]byte("namespace1"), []byte("data1")...))},
		1: {"simpler case", []byte("1"), []byte("d"), namespace.NewPrefixedData(1, append([]byte("1"), []byte("d")...))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := namespace.PrefixedDataFrom(tt.namespace, tt.data); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PrefixedDataFrom() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNamespacedMerkleTree_Push(t *testing.T) {
	tests := []struct {
		name    string
		data    namespace.PrefixedData
		wantErr bool
	}{
		{"1st push: always OK", *namespace.PrefixedDataFrom([]byte{0, 0, 0}, []byte("dummy data")), false},
		{"push with same namespace: OK", *namespace.PrefixedDataFrom([]byte{0, 0, 0}, []byte("dummy data")), false},
		{"push with greater namespace: OK", *namespace.PrefixedDataFrom([]byte{0, 0, 1}, []byte("dummy data")), false},
		{"push with smaller namespace: Err", *namespace.PrefixedDataFrom([]byte{0, 0, 0}, []byte("dummy data")), true},
		{"push with same namespace: Ok", *namespace.PrefixedDataFrom([]byte{0, 0, 1}, []byte("dummy data")), false},
		{"push with greater namespace: Ok", *namespace.PrefixedDataFrom([]byte{1, 0, 0}, []byte("dummy data")), false},
		{"push with smaller namespace: Err", *namespace.PrefixedDataFrom([]byte{0, 0, 1}, []byte("dummy data")), true},
		{"push with smaller namespace: Err", *namespace.PrefixedDataFrom([]byte{0, 0, 0}, []byte("dummy data")), true},
		{"push with smaller namespace: Err", *namespace.PrefixedDataFrom([]byte{0, 1, 0}, []byte("dummy data")), true},
		{"push with same as last namespace: OK", *namespace.PrefixedDataFrom([]byte{1, 0, 0}, []byte("dummy data")), false},
		{"push with greater as last namespace: OK", *namespace.PrefixedDataFrom([]byte{1, 1, 0}, []byte("dummy data")), false},
		// note this tests for another kind of error: ErrMismatchedNamespaceSize
		{"push with wrong namespace size: Err", *namespace.PrefixedDataFrom([]byte{1, 1, 0, 0}, []byte("dummy data")), true},
	}
	n := nmt.New(defaulthasher.New(3, crypto.SHA256))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := n.Push(tt.data); (err != nil) != tt.wantErr {
				t.Errorf("Push() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNamespacedMerkleTreeRoot(t *testing.T) {
	// does some sanity checks on root computation
	// TODO: add in more realistic test-vectors
	zeroNs := []byte{0, 0, 0}
	onesNS := []byte{1, 1, 1}
	leaf := []byte("leaf1")
	leafHash := sum(crypto.SHA256, []byte{LeafPrefix}, leaf)
	zeroFlaggedLeaf := append(append(zeroNs, zeroNs...), leafHash...)
	oneFlaggedLeaf := append(append(onesNS, onesNS...), leafHash...)
	twoZeroLeafsRoot := sum(crypto.SHA256, []byte{NodePrefix}, zeroFlaggedLeaf, zeroFlaggedLeaf)
	diffNSLeafsRoot := sum(crypto.SHA256, []byte{NodePrefix}, zeroFlaggedLeaf, oneFlaggedLeaf)
	emptyRoot := bytes.Repeat([]byte{0}, crypto.SHA256.Size())

	tests := []struct {
		name       string
		nidLen     int
		pushedData []namespace.PrefixedData
		wantMinNs  namespace.ID
		wantMaxNs  namespace.ID
		wantRoot   []byte
	}{
		// default empty root according to base case:
		// https://github.com/lazyledger/lazyledger-specs/blob/master/specs/data_structures.md#namespace-merkle-tree
		{"Empty", 3, nil, zeroNs, zeroNs, emptyRoot},
		{"One leaf", 3, []namespace.PrefixedData{*namespace.PrefixedDataFrom(zeroNs, leaf)}, zeroNs, zeroNs, leafHash},
		{"Two leafs", 3, []namespace.PrefixedData{*namespace.PrefixedDataFrom(zeroNs, leaf), *namespace.PrefixedDataFrom(zeroNs, leaf)}, zeroNs, zeroNs, twoZeroLeafsRoot},
		{"Two leafs diff namespaces", 3, []namespace.PrefixedData{*namespace.PrefixedDataFrom(zeroNs, leaf), *namespace.PrefixedDataFrom(onesNS, leaf)}, zeroNs, onesNS, diffNSLeafsRoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := nmt.New(defaulthasher.New(tt.nidLen, crypto.SHA256))
			for _, d := range tt.pushedData {
				if err := n.Push(d); err != nil {
					t.Errorf("Push() error = %v, expected no error", err)
				}
			}
			gotMinNs, gotMaxNs, gotRoot := n.Root()
			if !reflect.DeepEqual(gotMinNs, tt.wantMinNs) {
				t.Errorf("Root() gotMinNs = %v, want %v", gotMinNs, tt.wantMinNs)
			}
			if !reflect.DeepEqual(gotMaxNs, tt.wantMaxNs) {
				t.Errorf("Root() gotMaxNs = %v, want %v", gotMaxNs, tt.wantMaxNs)
			}
			if !reflect.DeepEqual(gotRoot, tt.wantRoot) {
				t.Errorf("Root() gotRoot = %v, want %v", gotRoot, tt.wantRoot)
			}
		})
	}
}

func TestNamespacedMerkleTree_ProveNamespace_Ranges(t *testing.T) {
	tests := []struct {
		name           string
		nidLen         int
		pushData       []namespace.PrefixedData
		proveNID       namespace.ID
		wantProofStart int
		wantProofEnd   int
		wantFound      bool
	}{
		{"found", 1,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(1, []byte("0_data"))}, []byte("0"),
			0, 1, true},
		{"not found", 1,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(1, []byte("0_data"))}, []byte("1"),
			0, 0, false},
		{"two leafs and found", 1,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(1, []byte("0_data")), *namespace.NewPrefixedData(1, []byte("1_data"))}, []byte("1"),
			1, 2, true},
		{"two leafs and found", 1,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(1, []byte("0_data")), *namespace.NewPrefixedData(1, []byte("0_data"))}, []byte("1"),
			0, 0, false},
		{"three leafs and found", 1,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(1, []byte("0_data")), *namespace.NewPrefixedData(1, []byte("0_data")), *namespace.NewPrefixedData(1, []byte("1_data"))}, []byte("1"),
			2, 3, true},
		{"three leafs and not found but with range", 2,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(2, []byte("00_data")), *namespace.NewPrefixedData(2, []byte("00_data")), *namespace.NewPrefixedData(2, []byte("11_data"))}, []byte("01"),
			2, 3, false},
		{"three leafs and not found but within range", 2,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(2, []byte("00_data")), *namespace.NewPrefixedData(2, []byte("00_data")), *namespace.NewPrefixedData(2, []byte("11_data"))}, []byte("01"),
			2, 3, false},
		{"4 leafs and not found but within range", 2,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(2, []byte("00_data")), *namespace.NewPrefixedData(2, []byte("00_data")), *namespace.NewPrefixedData(2, []byte("11_data")), *namespace.NewPrefixedData(2, []byte("11_data"))}, []byte("01"),
			2, 3, false},
		// In the cases (nID < minNID) or (maxNID < nID) we do not generate any proof
		// and the (minNS, maxNs, root) should be indication enough that nID is not in that range.
		{"4 leafs, not found and nID < minNID", 2,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(2, []byte("01_data")), *namespace.NewPrefixedData(2, []byte("01_data")), *namespace.NewPrefixedData(2, []byte("01_data")), *namespace.NewPrefixedData(2, []byte("11_data"))}, []byte("00"),
			0, 0, false},
		{"4 leafs, not found and nID > maxNID ", 2,
			[]namespace.PrefixedData{*namespace.NewPrefixedData(2, []byte("00_data")), *namespace.NewPrefixedData(2, []byte("00_data")), *namespace.NewPrefixedData(2, []byte("01_data")), *namespace.NewPrefixedData(2, []byte("01_data"))}, []byte("11"),
			0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := nmt.New(defaulthasher.New(tt.nidLen, crypto.SHA256))
			for _, d := range tt.pushData {
				err := n.Push(d)
				if err != nil {
					t.Fatalf("invalid test case: %v", tt.name)
				}
			}
			gotProofStart, gotProofEnd, _, gotFoundLeafs, gotLeafHashes := n.ProveNamespace(tt.proveNID)
			if gotProofStart != tt.wantProofStart {
				t.Errorf("ProveNamespace() gotProofStart = %v, want %v", gotProofStart, tt.wantProofStart)
			}
			if gotProofEnd != tt.wantProofEnd {
				t.Errorf("ProveNamespace() gotProofEnd = %v, want %v", gotProofEnd, tt.wantProofEnd)
			}
			gotFound := gotFoundLeafs != nil && gotLeafHashes == nil
			if gotFound != tt.wantFound {
				t.Errorf("ProveNamespace() gotFound = %v, wantFound = %v ", gotFound, tt.wantFound)
			}
		})
	}
}

func sum(hash crypto.Hash, data ...[]byte) []byte {
	h := hash.New()
	for _, d := range data {
		//nolint:errcheck
		h.Write(d)
	}

	return h.Sum(nil)
}
