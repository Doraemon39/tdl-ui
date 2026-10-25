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

func TestDiscussionMembershipIncludesRootWithoutAcceptingOtherThreads(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message *tg.Message
		want    bool
	}{
		{"root", &tg.Message{ID: 10}, true},
		{"direct reply", &tg.Message{ID: 11, ReplyTo: &tg.MessageReplyHeader{ReplyToMsgID: 10}}, true},
		{"nested reply", &tg.Message{ID: 12, ReplyTo: &tg.MessageReplyHeader{ReplyToMsgID: 11, ReplyToTopID: 10}}, true},
		{"other thread", &tg.Message{ID: 13, ReplyTo: &tg.MessageReplyHeader{ReplyToMsgID: 20, ReplyToTopID: 20}}, false},
		{"unrelated message", &tg.Message{ID: 14}, false},
		{"missing message", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := messageInDiscussion(tc.message, 10); got != tc.want {
				t.Fatalf("membership=%v, want %v", got, tc.want)
			}
		})
	}
}
