package downloader

import (
	"encoding/binary"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

var (
	knownHash   = common.Hash{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	unknownHash = common.Hash{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
)

func createHashes(start, amount int) (hashes []common.Hash) {
	hashes = make([]common.Hash, amount+1)
	hashes[len(hashes)-1] = knownHash

	for i := range hashes[:len(hashes)-1] {
		binary.BigEndian.PutUint64(hashes[i][:8], uint64(i+2))
	}

	return
}

func createBlock(i int, prevHash, hash common.Hash) *types.Block {
	header := &types.Header{Number: big.NewInt(int64(i))}
	block := types.NewBlockWithHeader(header)
	block.HeaderHash = hash
	block.ParentHeaderHash = prevHash
	return block
}

func createBlocksFromHashes(hashes []common.Hash) map[common.Hash]*types.Block {
	blocks := make(map[common.Hash]*types.Block)

	for i, hash := range hashes {
		blocks[hash] = createBlock(len(hashes)-i, knownHash, hash)
	}

	return blocks
}

type downloadTester struct {
	downloader *Downloader

	hashes []common.Hash                // Chain of hashes simulating
	blocks map[common.Hash]*types.Block // Blocks associated with the hashes
	chain  []common.Hash                // Block-chain being constructed

	t            *testing.T
	pcount       int
	done         chan bool
	activePeerId string
}

func newTester(t *testing.T, hashes []common.Hash, blocks map[common.Hash]*types.Block) *downloadTester {
	tester := &downloadTester{
		t: t,

		hashes: hashes,
		blocks: blocks,
		chain:  []common.Hash{knownHash},

		done: make(chan bool),
	}
	var mux event.TypeMux
	downloader := New(&mux, tester.hasBlock, tester.getBlock)
	tester.downloader = downloader

	return tester
}

// sync is a simple wrapper around the downloader to start synchronisation and
// block until it returns
func (dl *downloadTester) sync(peerId string, head common.Hash) error {
	dl.activePeerId = peerId
	return dl.downloader.Synchronise(peerId, head)
}

// syncTake is starts synchronising with a remote peer, but concurrently it also
// starts fetching blocks that the downloader retrieved. IT blocks until both go
// routines terminate.
func (dl *downloadTester) syncTake(peerId string, head common.Hash) (types.Blocks, error) {
	// Start a block collector to take blocks as they become available
	done := make(chan struct{})
	took := []*types.Block{}
	go func() {
		for running := true; running; {
			select {
			case <-done:
				running = false
			default:
				time.Sleep(time.Millisecond)
			}
			// Take a batch of blocks and accumulate
			took = append(took, dl.downloader.TakeBlocks()...)
		}
		done <- struct{}{}
	}()
	// Start the downloading, sync the taker and return
	err := dl.sync(peerId, head)

	done <- struct{}{}
	<-done

	return took, err
}

func (dl *downloadTester) insertBlocks(blocks types.Blocks) {
	for _, block := range blocks {
		dl.chain = append(dl.chain, block.Hash())
	}
}

func (dl *downloadTester) hasBlock(hash common.Hash) bool {
	for _, h := range dl.chain {
		if h == hash {
			return true
		}
	}
	return false
}

func (dl *downloadTester) getBlock(hash common.Hash) *types.Block {
	return dl.blocks[knownHash]
}

func (dl *downloadTester) getHashes(hash common.Hash) error {
	dl.downloader.DeliverHashes(dl.activePeerId, dl.hashes)
	return nil
}

func (dl *downloadTester) getBlocks(id string) func([]common.Hash) error {
	return func(hashes []common.Hash) error {
		blocks := make([]*types.Block, 0, len(hashes))
		for _, hash := range hashes {
			if block, ok := dl.blocks[hash]; ok {
				blocks = append(blocks, block)
			}
		}
		go dl.downloader.DeliverBlocks(id, blocks)

		return nil
	}
}

func (dl *downloadTester) newPeer(id string, td *big.Int, hash common.Hash) {
	dl.pcount++

	dl.downloader.RegisterPeer(id, hash, dl.getHashes, dl.getBlocks(id))
}

func (dl *downloadTester) badBlocksPeer(id string, td *big.Int, hash common.Hash) {
	dl.pcount++

	// This bad peer never returns any blocks
	dl.downloader.RegisterPeer(id, hash, dl.getHashes, func([]common.Hash) error {
		return nil
	})
}

func TestDownload(t *testing.T) {
	minDesiredPeerCount = 4
	blockTtl = 1 * time.Second

	targetBlocks := 1000
	hashes := createHashes(0, targetBlocks)
	blocks := createBlocksFromHashes(hashes)
	tester := newTester(t, hashes, blocks)

	tester.newPeer("peer1", big.NewInt(10000), hashes[0])
	tester.newPeer("peer2", big.NewInt(0), common.Hash{})
	tester.badBlocksPeer("peer3", big.NewInt(0), common.Hash{})
	tester.badBlocksPeer("peer4", big.NewInt(0), common.Hash{})
	tester.activePeerId = "peer1"

	err := tester.sync("peer1", hashes[0])
	if err != nil {
		t.Error("download error", err)
	}

	inqueue := len(tester.downloader.queue.blockCache)
	if inqueue != targetBlocks {
		t.Error("expected", targetBlocks, "have", inqueue)
	}
}

func TestMissing(t *testing.T) {
	targetBlocks := 1000
	hashes := createHashes(0, 1000)
	extraHashes := createHashes(1001, 1003)
	blocks := createBlocksFromHashes(append(extraHashes, hashes...))
	tester := newTester(t, hashes, blocks)

	tester.newPeer("peer1", big.NewInt(10000), hashes[len(hashes)-1])

	hashes = append(extraHashes, hashes[:len(hashes)-1]...)
	tester.newPeer("peer2", big.NewInt(0), common.Hash{})

	err := tester.sync("peer1", hashes[0])
	if err != nil {
		t.Error("download error", err)
	}

	inqueue := len(tester.downloader.queue.blockCache)
	if inqueue != targetBlocks {
		t.Error("expected", targetBlocks, "have", inqueue)
	}
}

func TestTaking(t *testing.T) {
	minDesiredPeerCount = 4
	blockTtl = 1 * time.Second

	targetBlocks := 1000
	hashes := createHashes(0, targetBlocks)
	blocks := createBlocksFromHashes(hashes)
	tester := newTester(t, hashes, blocks)

	tester.newPeer("peer1", big.NewInt(10000), hashes[0])
	tester.newPeer("peer2", big.NewInt(0), common.Hash{})
	tester.badBlocksPeer("peer3", big.NewInt(0), common.Hash{})
	tester.badBlocksPeer("peer4", big.NewInt(0), common.Hash{})

	err := tester.sync("peer1", hashes[0])
	if err != nil {
		t.Error("download error", err)
	}
	bs := tester.downloader.TakeBlocks()
	if len(bs) != targetBlocks {
		t.Error("retrieved block mismatch: have %v, want %v", len(bs), targetBlocks)
	}
}

func TestInactiveDownloader(t *testing.T) {
	targetBlocks := 1000
	hashes := createHashes(0, targetBlocks)
	blocks := createBlocksFromHashSet(createHashSet(hashes))
	tester := newTester(t, hashes, nil)

	err := tester.downloader.DeliverHashes("bad peer 001", hashes)
	if err != errNoSyncActive {
		t.Error("expected no sync error, got", err)
	}

	err = tester.downloader.DeliverBlocks("bad peer 001", blocks)
	if err != errNoSyncActive {
		t.Error("expected no sync error, got", err)
	}
}

func TestCancel(t *testing.T) {
	minDesiredPeerCount = 4
	blockTtl = 1 * time.Second

	targetBlocks := 1000
	hashes := createHashes(0, targetBlocks)
	blocks := createBlocksFromHashes(hashes)
	tester := newTester(t, hashes, blocks)

	tester.newPeer("peer1", big.NewInt(10000), hashes[0])

	err := tester.sync("peer1", hashes[0])
	if err != nil {
		t.Error("download error", err)
	}

	if !tester.downloader.Cancel() {
		t.Error("cancel operation unsuccessfull")
	}

	hashSize, blockSize := tester.downloader.queue.Size()
	if hashSize > 0 || blockSize > 0 {
		t.Error("block (", blockSize, ") or hash (", hashSize, ") not 0")
	}
}

func TestThrottling(t *testing.T) {
	minDesiredPeerCount = 4
	blockTtl = 1 * time.Second

	targetBlocks := 16 * blockCacheLimit
	hashes := createHashes(0, targetBlocks)
	blocks := createBlocksFromHashes(hashes)
	tester := newTester(t, hashes, blocks)

	tester.newPeer("peer1", big.NewInt(10000), hashes[0])
	tester.newPeer("peer2", big.NewInt(0), common.Hash{})
	tester.badBlocksPeer("peer3", big.NewInt(0), common.Hash{})
	tester.badBlocksPeer("peer4", big.NewInt(0), common.Hash{})

	// Concurrently download and take the blocks
	took, err := tester.syncTake("peer1", hashes[0])
	if err != nil {
		t.Fatalf("failed to synchronise blocks: %v", err)
	}
	if len(took) != targetBlocks {
		t.Fatalf("downloaded block mismatch: have %v, want %v", len(took), targetBlocks)
	}
}

// Tests that if a peer returns an invalid chain with a block pointing to a non-
// existing parent, it is correctly detected and handled.
func TestNonExistingParentAttack(t *testing.T) {
	// Forge a single-link chain with a forged header
	hashes := createHashes(0, 1)
	blocks := createBlocksFromHashes(hashes)

	forged := blocks[hashes[0]]
	forged.ParentHeaderHash = unknownHash

	// Try and sync with the malicious node and check that it fails
	tester := newTester(t, hashes, blocks)
	tester.newPeer("attack", big.NewInt(10000), hashes[0])
	if err := tester.sync("attack", hashes[0]); err != nil {
		t.Fatalf("failed to synchronise blocks: %v", err)
	}
	bs := tester.downloader.TakeBlocks()
	if len(bs) != 1 {
		t.Fatalf("retrieved block mismatch: have %v, want %v", len(bs), 1)
	}
	if tester.hasBlock(bs[0].ParentHash()) {
		t.Fatalf("tester knows about the unknown hash")
	}
	tester.downloader.Cancel()

	// Reconstruct a valid chain, and try to synchronize with it
	forged.ParentHeaderHash = knownHash
	tester.newPeer("valid", big.NewInt(20000), hashes[0])
	if err := tester.sync("valid", hashes[0]); err != nil {
		t.Fatalf("failed to synchronise blocks: %v", err)
	}
	bs = tester.downloader.TakeBlocks()
	if len(bs) != 1 {
		t.Fatalf("retrieved block mismatch: have %v, want %v", len(bs), 1)
	}
	if !tester.hasBlock(bs[0].ParentHash()) {
		t.Fatalf("tester doesn't know about the origin hash")
	}
}

// Tests that if a malicious peers keeps sending us repeating hashes, we don't
// loop indefinitely.
func TestRepeatingHashAttack(t *testing.T) {
	// Create a valid chain, but drop the last link
	hashes := createHashes(0, blockCacheLimit)
	blocks := createBlocksFromHashes(hashes)
	forged := hashes[:len(hashes)-1]

	// Try and sync with the malicious node
	tester := newTester(t, forged, blocks)
	tester.newPeer("attack", big.NewInt(10000), forged[0])

	errc := make(chan error)
	go func() {
		errc <- tester.sync("attack", hashes[0])
	}()

	// Make sure that syncing returns and does so with a failure
	select {
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("synchronisation blocked")
	case err := <-errc:
		if err == nil {
			t.Fatalf("synchronisation succeeded")
		}
	}
	// Ensure that a valid chain can still pass sync
	tester.hashes = hashes
	tester.newPeer("valid", big.NewInt(20000), hashes[0])
	if err := tester.sync("valid", hashes[0]); err != nil {
		t.Fatalf("failed to synchronise blocks: %v", err)
	}
}

// Tests that if a malicious peers returns a non-existent block hash, it should
// eventually time out and the sync reattempted.
func TestNonExistingBlockAttack(t *testing.T) {
	// Create a valid chain, but forge the last link
	hashes := createHashes(0, blockCacheLimit)
	blocks := createBlocksFromHashes(hashes)
	origin := hashes[len(hashes)/2]

	hashes[len(hashes)/2] = unknownHash

	// Try and sync with the malicious node and check that it fails
	tester := newTester(t, hashes, blocks)
	tester.newPeer("attack", big.NewInt(10000), hashes[0])
	if err := tester.sync("attack", hashes[0]); err != errPeersUnavailable {
		t.Fatalf("synchronisation error mismatch: have %v, want %v", err, errPeersUnavailable)
	}
	// Ensure that a valid chain can still pass sync
	hashes[len(hashes)/2] = origin
	tester.newPeer("valid", big.NewInt(20000), hashes[0])
	if err := tester.sync("valid", hashes[0]); err != nil {
		t.Fatalf("failed to synchronise blocks: %v", err)
	}
}

// Tests that if a malicious peer is returning hashes in a weird order, that the
// sync throttler doesn't choke on them waiting for the valid blocks.
func TestInvalidHashOrderAttack(t *testing.T) {
	// Create a valid long chain, but reverse some hashes within
	hashes := createHashes(0, 4*blockCacheLimit)
	blocks := createBlocksFromHashes(hashes)

	reverse := make([]common.Hash, len(hashes))
	copy(reverse, hashes)

	for i := len(hashes) / 4; i < 2*len(hashes)/4; i++ {
		reverse[i], reverse[len(hashes)-i-1] = reverse[len(hashes)-i-1], reverse[i]
	}

	// Try and sync with the malicious node and check that it fails
	tester := newTester(t, reverse, blocks)
	tester.newPeer("attack", big.NewInt(10000), reverse[0])
	if _, err := tester.syncTake("attack", reverse[0]); err != ErrInvalidChain {
		t.Fatalf("synchronisation error mismatch: have %v, want %v", err, ErrInvalidChain)
	}
	// Ensure that a valid chain can still pass sync
	tester.hashes = hashes
	tester.newPeer("valid", big.NewInt(20000), hashes[0])
	if _, err := tester.syncTake("valid", hashes[0]); err != nil {
		t.Fatalf("failed to synchronise blocks: %v", err)
	}
}
