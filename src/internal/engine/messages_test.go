package engine

import (
	"github.com/gotd/td/tg"
	"testing"
)

func TestMessageLookupCannotCrossPeerIDSpaces(t *testing.T) {
	m := &tg.Message{ID: 1, PeerID: &tg.PeerUser{UserID: 7}}
	if !messageBelongsToPeer(m, &tg.InputPeerUser{UserID: 7}) {
		t.Fatal("matching private source rejected")
	}
	for _, p := range []tg.InputPeerClass{&tg.InputPeerUser{UserID: 8}, &tg.InputPeerChat{ChatID: 7}, &tg.InputPeerChannel{ChannelID: 7}, &tg.InputPeerSelf{}} {
		if messageBelongsToPeer(m, p) {
			t.Fatal("message attributed to wrong peer")
		}
	}
}
