package hamt

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	block "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"
)

type mockBlocks struct {
	data map[cid.Cid]block.Block
}

func newMockBlocks() *mockBlocks {
	return &mockBlocks{make(map[cid.Cid]block.Block)}
}

func (mb *mockBlocks) Get(c cid.Cid) (block.Block, error) {
	d, ok := mb.data[c]
	if ok {
		return d, nil
	}
	return nil, fmt.Errorf("Not Found")
}

func (mb *mockBlocks) Put(b block.Block) error {
	mb.data[b.Cid()] = b
	return nil
}

func randString() string {
	buf := make([]byte, 18)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

func randValue() []byte {
	buf := make([]byte, 30)
	rand.Read(buf)
	return buf
}

func dotGraph(n *Node) {
	fmt.Println("digraph foo {")
	name := 0
	dotGraphRec(n, &name)
	fmt.Println("}")
}

var identityHash = func(k []byte) []byte {
	res := make([]byte, 32)
	copy(res, k)
	return res
}

var shortIdentityHash = func(k []byte) []byte {
	res := make([]byte, 16)
	copy(res, k)
	return res
}

var murmurHash = hash

func TestCanonicalStructure(t *testing.T) {
	hash = identityHash
	defer func() {
		hash = murmurHash
	}()
	addAndRemoveKeys(t, defaultBitWidth, []string{"K"}, []string{"B"}, "0171a0e402208683c5cd09bc6c1df93d100bee677d7a6bbe8db0b340361866e3fb20fb0a981e")

	addAndRemoveKeys(t, defaultBitWidth, []string{"K0", "K1", "KAA1", "KAA2", "KAA3"}, []string{"KAA4"}, "0171a0e40220e2a9e53c77d146010b60f2be9b3ba423c0db4efea06e66bd87e072671c8ef411")
}

func TestCanonicalStructureAlternateBitWidth(t *testing.T) {
	hash = identityHash
	defer func() {
		hash = murmurHash
	}()
	addAndRemoveKeys(t, 7, []string{"K"}, []string{"B"}, "0171a0e40220c4ac32c9bb0dbec96b290d68b1b1fc6e1ddfe33f99420b4b46a078255d997db8")
	addAndRemoveKeys(t, 7, []string{"K0", "K1", "KAA1", "KAA2", "KAA3"}, []string{"KAA4"}, "0171a0e4022094833c20da84ad6e18a603a47aa143e3393171d45786eddc5b182ae647dafd64")
	addAndRemoveKeys(t, 6, []string{"K"}, []string{"B"}, "0171a0e40220b45f48552b1b802fafcb79b417c4d2972ea42cd24600eaf9a0d1314c7d46c214")
	addAndRemoveKeys(t, 6, []string{"K0", "K1", "KAA1", "KAA2", "KAA3"}, []string{"KAA4"}, "0171a0e40220c84814bb7fdbb71a17ac24b0eb110a38e4e79c93fccaa6d87fa9e5aa771bb453")
	addAndRemoveKeys(t, 5, []string{"K"}, []string{"B"}, "0171a0e402209a00d457b7d5d398a225fa837125db401a5eabdf4833352aed48dd28dc6eca56")
	addAndRemoveKeys(t, 5, []string{"K0", "K1", "KAA1", "KAA2", "KAA3"}, []string{"KAA4"}, "0171a0e40220c5f39f53c67de67dbf8a058b699fb1e4673d78a5f6a0dc59583f9a175db234e3")
}
func TestOverflow(t *testing.T) {
	hash = identityHash
	defer func() {
		hash = murmurHash
	}()
	keys := make([]string, 4)
	for i := range keys {
		keys[i] = strings.Repeat("A", 32) + fmt.Sprintf("%d", i)
	}

	cs := cbor.NewCborStore(newMockBlocks())
	n := NewNode(cs)
	for _, k := range keys[:3] {
		if err := n.Set(context.Background(), k, "foobar"); err != nil {
			t.Error(err)
		}
	}

	// Try forcing the depth beyond 32
	if err := n.Set(context.Background(), keys[3], "bad"); err != ErrMaxDepth {
		t.Errorf("expected error %q, got %q", ErrMaxDepth, err)
	}

	// Force _to_ max depth.
	if err := n.Set(context.Background(), keys[3][1:], "bad"); err != nil {
		t.Error(err)
	}

	// Now, try fetching with a shorter hash function.
	hash = shortIdentityHash
	if err := n.Find(context.Background(), keys[0], nil); err != ErrMaxDepth {
		t.Errorf("expected error %q, got %q", ErrMaxDepth, err)
	}
}

func addAndRemoveKeys(t *testing.T, bitWidth int, keys []string, extraKeys []string, expected string) {
	ctx := context.Background()
	vals := make(map[string]int)
	for i := 0; i < len(keys); i++ {
		s := keys[i]
		vals[s] = i
	}

	cs := cbor.NewCborStore(newMockBlocks())
	begn := NewNode(cs, UseTreeBitWidth(bitWidth))
	for _, k := range keys {
		if err := begn.Set(ctx, k, vals[k]); err != nil {
			t.Fatal(err)
		}
	}

	fmt.Println("start flush")
	bef := time.Now()
	if err := begn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	fmt.Println("flush took: ", time.Since(bef))
	c, err := cs.Put(ctx, begn)
	if err != nil {
		t.Fatal(err)
	}

	var n Node
	if err := cs.Get(ctx, c, &n); err != nil {
		t.Fatal(err)
	}
	n.store = cs
	n.bitWidth = bitWidth
	for k, v := range vals {
		var out int
		err := n.Find(ctx, k, &out)
		if err != nil {
			t.Fatalf("should have found the thing (err: %s)", err)
		}
		if out != v {
			t.Fatalf("got wrong value after value change: %x != %x", out, v)
		}
	}

	// create second hamt by adding and deleting the extra keys
	for i := 0; i < len(extraKeys); i++ {
		begn.Set(ctx, extraKeys[i], i)
	}
	for i := 0; i < len(extraKeys); i++ {
		if err := begn.Delete(ctx, extraKeys[i]); err != nil {
			t.Fatal(err)
		}
	}

	if err := begn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c2, err := cs.Put(ctx, begn)
	if err != nil {
		t.Fatal(err)
	}

	var n2 Node
	if err := cs.Get(ctx, c2, &n2); err != nil {
		t.Fatal(err)
	}
	n2.store = cs
	n2.bitWidth = bitWidth
	if !nodesEqual(t, cs, &n, &n2) {
		t.Fatal("nodes should be equal")
	}

	require.Equal(t, expected, hex.EncodeToString(c2.Bytes()))
}

func dotGraphRec(n *Node, name *int) {
	cur := *name
	for _, p := range n.Pointers {
		if p.isShard() {
			*name++
			fmt.Printf("\tn%d -> n%d;\n", cur, *name)
			nd, err := p.loadChild(context.Background(), n.store, n.bitWidth)
			if err != nil {
				panic(err)
			}

			dotGraphRec(nd, name)
		} else {
			var names []string
			for _, pt := range p.KVs {
				names = append(names, string(pt.Key))
			}
			fmt.Printf("\tn%d -> n%s;\n", cur, strings.Join(names, "_"))
		}
	}
}

type hamtStats struct {
	totalNodes int
	totalKvs   int
	counts     map[int]int
}

func stats(n *Node) *hamtStats {
	st := &hamtStats{counts: make(map[int]int)}
	statsrec(n, st)
	return st
}

func statsrec(n *Node, st *hamtStats) {
	st.totalNodes++
	for _, p := range n.Pointers {
		if p.isShard() {
			nd, err := p.loadChild(context.Background(), n.store, n.bitWidth)
			if err != nil {
				panic(err)
			}

			statsrec(nd, st)
		} else {
			st.totalKvs += len(p.KVs)
			st.counts[len(p.KVs)]++
		}
	}
}

func TestHash(t *testing.T) {
	h1 := hash([]byte("abcd"))
	h2 := hash([]byte("abce"))
	if h1[0] == h2[0] && h1[1] == h2[1] && h1[3] == h2[3] {
		t.Fatal("Hash should give different strings different hash prefixes")
	}
}

func TestBasic(t *testing.T) {
	ctx := context.Background()
	cs := cbor.NewCborStore(newMockBlocks())
	begn := NewNode(cs)

	val := []byte("cat dog bear")
	if err := begn.Set(ctx, "foo", val); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 1000; i++ {
		if err := begn.Set(ctx, randString(), randValue()); err != nil {
			t.Fatal(err)
		}
	}

	if err := begn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := cs.Put(ctx, begn)
	if err != nil {
		t.Fatal(err)
	}

	n, err := LoadNode(ctx, cs, c)
	if err != nil {
		t.Fatal(err)
	}

	var out []byte
	if err := n.Find(ctx, "foo", &out); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(out, val) {
		t.Fatal("out bytes were wrong: ", out)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	cs := cbor.NewCborStore(newMockBlocks())
	begn := NewNode(cs, UseTreeBitWidth(5))

	if err := begn.Set(ctx, "foo", []byte("cat dog bear")); err != nil {
		t.Fatal(err)
	}
	if err := begn.Set(ctx, "bar", []byte("cat dog")); err != nil {
		t.Fatal(err)
	}
	if err := begn.Set(ctx, "baz", []byte("cat")); err != nil {
		t.Fatal(err)
	}

	if err := begn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := cs.Put(ctx, begn)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, "0171a0e402204c4cec750f4e5fc0df61e5a6b6f430d45e6d42108824492658ccd480a4f86aef", hex.EncodeToString(c.Bytes()))

	n, err := LoadNode(ctx, cs, c)
	if err != nil {
		t.Fatal(err)
	}

	if err := n.Delete(ctx, "foo"); err != nil {
		t.Fatal(err)
	}

	var out []byte
	if err := n.Find(ctx, "foo", &out); err == nil {
		t.Fatal("shouldnt have found object")
	}

	if err := n.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c, err = cs.Put(ctx, n)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, "0171a0e40220f8889d65614928ee8fd0a1fc27fb94357751ce95e99260b16b8789455eb7d212", hex.EncodeToString(c.Bytes()))
}

func TestSerDeleteMany(t *testing.T) {
	ctx := context.Background()
	cs := cbor.NewCborStore(newMockBlocks())

	n := NewNode(cs, UseTreeBitWidth(5))

	for i := 0; i < 200; i++ {
		if err := n.Set(ctx, strconv.Itoa(i), i); err != nil {
			t.Fatal(err)
		}
	}

	if err := n.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := cs.Put(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "0171a0e402207c660382de99c174ce39517bdbd28f3967801aebbd9795f0591e226d93e2f010", hex.EncodeToString(c.Bytes()))

	for i := 200; i < 400; i++ {
		if err := n.Set(ctx, strconv.Itoa(i), i); err != nil {
			t.Fatal(err)
		}
	}

	if err := n.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c, err = cs.Put(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "0171a0e40220dba161623db24093bd90e00c3d185bae8468f8d3e81f01f112b3afe47e603fd1", hex.EncodeToString(c.Bytes()))

	for i := 200; i < 400; i++ {
		if err := n.Delete(ctx, strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}

	if err := n.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	c, err = cs.Put(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "0171a0e402207c660382de99c174ce39517bdbd28f3967801aebbd9795f0591e226d93e2f010", hex.EncodeToString(c.Bytes()))
}

func TestSetGet(t *testing.T) {
	ctx := context.Background()
	vals := make(map[string][]byte)
	var keys []string
	for i := 0; i < 200; i++ {
		s := randString()
		vals[s] = randValue()
		keys = append(keys, s)
	}

	cs := cbor.NewCborStore(newMockBlocks())
	begn := NewNode(cs)
	for _, k := range keys {
		if err := begn.Set(ctx, k, vals[k]); err != nil {
			t.Fatal(err)
		}
	}

	size, err := begn.checkSize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mapsize := 0
	for k, v := range vals {
		mapsize += (len(k) + len(v))
	}
	fmt.Printf("Total size is: %d, size of keys+vals: %d, overhead: %.2f\n", size, mapsize, float64(size)/float64(mapsize))
	fmt.Println(stats(begn))

	fmt.Println("start flush")
	bef := time.Now()
	if err := begn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	fmt.Println("flush took: ", time.Since(bef))
	c, err := cs.Put(ctx, begn)
	if err != nil {
		t.Fatal(err)
	}

	var n Node
	if err := cs.Get(ctx, c, &n); err != nil {
		t.Fatal(err)
	}
	n.store = cs
	n.bitWidth = defaultBitWidth
	bef = time.Now()
	//for k, v := range vals {
	for _, k := range keys {
		v := vals[k]
		var out []byte
		if err := n.Find(ctx, k, &out); err != nil {
			t.Fatal("should have found the thing: ", err)
		}
		if !bytes.Equal(out, v) {
			t.Fatal("got wrong value")
		}
	}
	fmt.Println("finds took: ", time.Since(bef))

	for i := 0; i < 100; i++ {
		err := n.Find(ctx, randString(), nil)
		if err != ErrNotFound {
			t.Fatal("should have gotten ErrNotFound, instead got: ", err)
		}
	}

	for k := range vals {
		next := randValue()
		n.Set(ctx, k, next)
		vals[k] = next
	}

	for k, v := range vals {
		var out []byte
		err := n.Find(ctx, k, &out)
		if err != nil {
			t.Fatal("should have found the thing")
		}
		if !bytes.Equal(out, v) {
			t.Fatal("got wrong value after value change")
		}
	}

	for i := 0; i < 100; i++ {
		err := n.Delete(ctx, randString())
		if err != ErrNotFound {
			t.Fatal("should have gotten ErrNotFound, instead got: ", err)
		}
	}

	for _, k := range keys {
		if err := n.Delete(ctx, k); err != nil {
			t.Fatal(err)
		}
		if err := n.Find(ctx, k, nil); err != ErrNotFound {
			t.Fatal("Expected ErrNotFound, got: ", err)
		}
	}
}

func nodesEqual(t *testing.T, store cbor.IpldStore, n1, n2 *Node) bool {
	ctx := context.Background()
	err := n1.Flush(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n1Cid, err := store.Put(ctx, n1)
	if err != nil {
		t.Fatal(err)
	}
	err = n2.Flush(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n2Cid, err := store.Put(ctx, n2)
	if err != nil {
		t.Fatal(err)
	}
	return n1Cid.Equals(n2Cid)
}

func TestReloadEmpty(t *testing.T) {
	ctx := context.Background()
	cs := cbor.NewCborStore(newMockBlocks())

	n := NewNode(cs)
	c, err := cs.Put(ctx, n)
	if err != nil {
		t.Fatal(err)
	}

	on, err := LoadNode(ctx, cs, c)
	if err != nil {
		t.Fatal(err)
	}

	if err := on.Set(ctx, "foo", "bar"); err != nil {
		t.Fatal(err)
	}
}

func TestCopy(t *testing.T) {
	ctx := context.Background()
	cs := cbor.NewCborStore(newMockBlocks())

	n := NewNode(cs)
	nc := n.Copy()
	if !nodesEqual(t, cs, n, nc) {
		t.Fatal("nodes should be equal")
	}
	n.Set(ctx, "key", []byte{0x01})
	if nodesEqual(t, cs, n, nc) {
		t.Fatal("nodes should not be equal -- we set a key on n")
	}
	nc = n.Copy()
	nc.Set(ctx, "key2", []byte{0x02})
	if nodesEqual(t, cs, n, nc) {
		t.Fatal("nodes should not be equal -- we set a key on nc")
	}
	n = nc.Copy()
	if !nodesEqual(t, cs, n, nc) {
		t.Fatal("nodes should be equal")
	}
}

func TestCopyCopiesNilSlices(t *testing.T) {
	cs := cbor.NewCborStore(newMockBlocks())

	n := NewNode(cs)
	pointer := &Pointer{}
	n.Pointers = append(n.Pointers, pointer)

	if n.Pointers[0].KVs != nil {
		t.Fatal("Expected uninitialize slice to be nil")
	}

	nc := n.Copy()

	if nc.Pointers[0].KVs != nil {
		t.Fatal("Expected copied nil slices to be nil")
	}
}

func TestCopyWithoutFlush(t *testing.T) {
	ctx := context.Background()
	cs := cbor.NewCborStore(newMockBlocks())

	count := 200
	n := NewNode(cs)
	for i := 0; i < count; i++ {
		n.Set(ctx, fmt.Sprintf("key%d", i), []byte{byte(i)})
	}

	n.Flush(ctx)

	for i := 0; i < count; i++ {
		n.Set(ctx, fmt.Sprintf("key%d", i), []byte{byte(count + i)})
	}

	nc := n.Copy()

	for i := 0; i < count; i++ {
		key := fmt.Sprintf("key%d", i)

		var val []byte
		if err := n.Find(ctx, key, &val); err != nil {
			t.Fatalf("should have found key %s in original", key)
		}

		var valCopy []byte
		if err := nc.Find(ctx, key, &valCopy); err != nil {
			t.Fatalf("should have found key %s in copy", key)
		}

		if val[0] != valCopy[0] {
			t.Fatalf("copy does not equal original (%d != %d)", valCopy[0], val[0])
		}
	}
}

func TestValueLinking(t *testing.T) {
	ctx := context.Background()
	cs := cbor.NewCborStore(newMockBlocks())

	thingy1 := map[string]string{"cat": "dog"}
	c1, err := cs.Put(ctx, thingy1)
	if err != nil {
		t.Fatal(err)
	}

	thingy2 := map[string]interface{}{
		"one": c1,
		"foo": "bar",
	}

	n := NewNode(cs)

	if err := n.Set(ctx, "cat", thingy2); err != nil {
		t.Fatal(err)
	}

	tcid, err := cs.Put(ctx, n)
	if err != nil {
		t.Fatal(err)
	}

	blk, err := cs.Blocks.Get(tcid)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("BLOCK DATA: %x\n", blk.RawData())
	nd, err := cbor.DecodeBlock(blk)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("thingy1", c1)
	fmt.Println(nd.Links()[0])
}
