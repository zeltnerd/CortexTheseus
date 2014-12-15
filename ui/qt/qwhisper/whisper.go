package qwhisper

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethutil"
	"github.com/ethereum/go-ethereum/whisper"
	"gopkg.in/qml.v1"
)

func fromHex(s string) []byte {
	if len(s) > 1 {
		return ethutil.Hex2Bytes(s[2:])
	}
	return nil
}
func toHex(b []byte) string { return "0x" + ethutil.Bytes2Hex(b) }

type Whisper struct {
	*whisper.Whisper
	view qml.Object
}

func New(w *whisper.Whisper) *Whisper {
	return &Whisper{w, nil}
}

func (self *Whisper) SetView(view qml.Object) {
	self.view = view
}

func (self *Whisper) Post(data string, to, from string, topics []string, pow, ttl uint32) {
	msg := whisper.NewMessage(fromHex(data))
	envelope, err := msg.Seal(time.Duration(pow), whisper.Opts{
		Ttl:    time.Duration(ttl),
		To:     crypto.ToECDSAPub(fromHex(to)),
		From:   crypto.ToECDSA(fromHex(from)),
		Topics: whisper.TopicsFromString(topics),
	})
	if err != nil {
		fmt.Println(err)
		// handle error
		return
	}

	if err := self.Whisper.Send(envelope); err != nil {
		fmt.Println(err)
		// handle error
		return
	}
}

func (self *Whisper) NewIdentity() string {
	return toHex(self.Whisper.NewIdentity().D.Bytes())
}

func (self *Whisper) HasIdentify(key string) bool {
	return self.Whisper.HasIdentity(crypto.ToECDSA(fromHex(key)))
}

func (self *Whisper) Watch(opts map[string]interface{}) {
	filter := filterFromMap(opts)
	filter.Fn = func(msg *whisper.Message) {
		// TODO POST TO QT WINDOW
	}
	self.Whisper.Watch(filter)
}

func filterFromMap(opts map[string]interface{}) (f whisper.Filter) {
	if to, ok := opts["to"].(string); ok {
		f.To = crypto.ToECDSA(fromHex(to))
	}
	if from, ok := opts["from"].(string); ok {
		f.From = crypto.ToECDSAPub(fromHex(from))
	}

	return
}
