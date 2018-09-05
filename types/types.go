package types

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/CortexFoundation/CortexTheseus/common"
	"github.com/CortexFoundation/CortexTheseus/common/hexutil"
	metaTypes "github.com/CortexFoundation/CortexTheseus/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	opCommon      = 0
	opCreateModel = 1
	opCreateInput = 2
	opNoInput     = 3
)

var (
	errWrongOpCode = errors.New("unexpected opCode")
)

// Transaction ... Tx struct
type Transaction struct {
	Price     *big.Int        `json:"gasPrice" gencodec:"required"`
	Amount    *big.Int        `json:"value"    gencodec:"required"`
	GasLimit  uint64          `json:"gas"      gencodec:"required"`
	Payload   []byte          `json:"input"    gencodec:"required"`
	From      *common.Address `json:"from"     gencodec:"required"`
	Recipient *common.Address `json:"to"       rlp:"nil"` // nil means contract creation
	Hash      *common.Hash    `json:"hash"     gencodec:"required"`
	Receipt   *TxReceipt      `json:"receipt"  rlp:"nil"`
}

// Op ...
func (t *Transaction) Op() (op int) {
	op = opCommon
	if len(t.Payload) >= 2 {
		op = (int(t.Payload[0]) << 8) + int(t.Payload[1])
		if op > 3 {
			op = opNoInput
		}
	} else if len(t.Payload) == 0 {
		op = opNoInput
	}
	return
}

// Data ...
func (t *Transaction) Data() []byte {
	if len(t.Payload) >= 2 {
		return t.Payload[2:]
	}
	return []byte{}
}

func (t *Transaction) noPayload() bool {
	return len(t.Payload) == 0
}

// IsFlowControl ...
func (t *Transaction) IsFlowControl() bool {
	return t.noPayload() && t.Amount.Uint64() == 0
}

func getInfohashFromURI(uri string) (*metainfo.Hash, error) {
	m, err := metainfo.ParseMagnetURI(uri)
	if err != nil {
		return nil, err
	}
	return &m.InfoHash, err
}

func getDisplayNameFromURI(uri string) (string, error) {
	m, err := metainfo.ParseMagnetURI(uri)
	if err != nil {
		return "", err
	}
	return m.DisplayName, nil
}

// Parse ...
func (t *Transaction) Parse() *FileMeta {
	if t.Op() == opCreateInput {
		var meta metaTypes.InputMeta
		var AuthorAddress common.Address
		AuthorAddress.SetBytes(meta.AuthorAddress.Bytes())
		rlp.Decode(bytes.NewReader(t.Data()), &meta)
		return &FileMeta{
			&AuthorAddress,
			meta.URI,
			meta.RawSize,
			meta.BlockNum.Uint64(),
		}
	} else if t.Op() == opCreateModel {
		var meta metaTypes.ModelMeta
		var AuthorAddress common.Address
		AuthorAddress.SetBytes(meta.AuthorAddress.Bytes())
		rlp.Decode(bytes.NewReader(t.Data()), &meta)
		return &FileMeta{
			&AuthorAddress,
			meta.URI,
			meta.RawSize,
			meta.BlockNum.Uint64(),
		}
	} else {
		return nil
	}
}

type transactionMarshaling struct {
	Price    *hexutil.Big
	Amount   *hexutil.Big
	GasLimit hexutil.Uint64
	Payload  hexutil.Bytes
}

// Block ... block struct
type Block struct {
	Number     uint64        `json:"number"           gencodec:"required"`
	Hash       common.Hash   `json:"Hash"             gencodec:"required"`
	ParentHash common.Hash   `json:"parentHash"       gencodec:"required"`
	Txs        []Transaction `json:"Transactions"     gencodec:"required"`
}

type blockMarshaling struct {
	Number hexutil.Uint64
}

// TxReceipt ...
type TxReceipt struct {
	// Contract Address
	ContractAddr *common.Address `json:"ContractAddress"  gencodec:"required"`
	// Transaction Hash
	TxHash *common.Hash `json:"TransactionHash"  gencodec:"required"`
}

// FileMeta ...
type FileMeta struct {
	// Author Address
	AuthorAddr *common.Address
	// Download URI, should be in magnetURI format
	URI string
	// The raw size of the file counted in bytes
	RawSize  uint64
	BlockNum uint64
}

func (m* FileMeta) InfoHash() (*metainfo.Hash) {
	if h, err := getInfohashFromURI(m.URI); err != nil {
		return nil
	} else {
		return h
	}
}

func (m* FileMeta) DisplayName() (string) {
	if dn, err := getDisplayNameFromURI(m.URI); err != nil {
		return ""
	} else {
		return dn
	}
}
