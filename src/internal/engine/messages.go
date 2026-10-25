package engine

import (
	"context"
	"fmt"
	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
	"github.com/iyear/tdl/core/util/tutil"
	"github.com/iyear/tdl/pkg/tmessage"
)

func MediaIdentity(m *tg.Message) string {
	if m == nil {
		return ""
	}
	switch media := m.Media.(type) {
	case *tg.MessageMediaDocument:
		if doc, ok := media.Document.(*tg.Document); ok {
			return fmt.Sprintf("document:%d", doc.ID)
		}
	case *tg.MessageMediaPhoto:
		if photo, ok := media.Photo.(*tg.Photo); ok {
			return fmt.Sprintf("photo:%d", photo.ID)
		}
	}
	return ""
}

func resolveMessageLinks(ctx context.Context, manager *peers.Manager, urls []string) ([]*tmessage.Dialog, map[string]bool, error) {
	out := []*tmessage.Dialog{}
	single := map[string]bool{}
	for _, raw := range urls {
		link, err := parseMessageLink(raw)
		if err != nil {
			return nil, nil, err
		}
		var from peers.Peer
		if link.ChannelID > 0 {
			from, err = manager.ResolveChannelID(ctx, link.ChannelID)
		} else {
			from, err = manager.Resolve(ctx, "@"+link.Username)
		}
		if err != nil {
			return nil, nil, err
		}
		msgID := link.MessageID
		if link.CommentID > 0 {
			discussion, err := manager.API().MessagesGetDiscussionMessage(ctx, &tg.MessagesGetDiscussionMessageRequest{Peer: from.InputPeer(), MsgID: msgID})
			if err != nil {
				return nil, nil, errors.Wrap(err, "解析评论对应讨论组失败")
			}
			if err := manager.Apply(ctx, discussion.Users, discussion.Chats); err != nil {
				return nil, nil, err
			}
			var target peers.Peer
			rootID := 0
			for _, mc := range discussion.Messages {
				m, ok := mc.(*tg.Message)
				if !ok {
					continue
				}
				p, ok := m.PeerID.(*tg.PeerChannel)
				if !ok {
					continue
				}
				if p.ChannelID != from.ID() {
					target, err = manager.ResolveChannelID(ctx, p.ChannelID)
					rootID = m.ID
					break
				}
			}
			if err != nil {
				return nil, nil, err
			}
			if target == nil {
				return nil, nil, errors.New("未找到评论对应的讨论组")
			}
			messages, err := getPeerMessages(ctx, manager.API(), target.InputPeer(), []int{link.CommentID}, target.ID())
			if err != nil {
				return nil, nil, err
			}
			valid := false
			for _, m := range messages {
				if messageInDiscussion(m, rootID) {
					valid = true
				}
			}
			if !valid {
				return nil, nil, errors.New("评论消息不属于指定帖子的讨论串")
			}
			from, msgID = target, link.CommentID
		}
		out = append(out, &tmessage.Dialog{Peer: from.InputPeer(), Messages: []int{msgID}})
		if link.Single {
			single[resumeKey(from.ID(), msgID, peerDialogType(from))] = true
		}
	}
	return mergeDialogRanges(out), single, nil
}

func messageInDiscussion(m *tg.Message, rootID int) bool {
	if m == nil || rootID <= 0 {
		return false
	}
	if m.ID == rootID {
		return true
	}
	h, ok := m.ReplyTo.(*tg.MessageReplyHeader)
	return ok && (h.ReplyToTopID == rootID || h.ReplyToMsgID == rootID)
}

func getPeerMessages(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, ids []int, selfIDs ...int64) ([]*tg.Message, error) {
	input := make([]tg.InputMessageClass, 0, len(ids))
	for _, id := range ids {
		input = append(input, &tg.InputMessageID{ID: id})
	}
	var result tg.MessagesMessagesClass
	var err error
	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		result, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{Channel: &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash}, ID: input})
	} else {
		result, err = api.MessagesGetMessages(ctx, input)
	}
	if err != nil {
		return nil, err
	}
	modified, ok := result.AsModified()
	if !ok {
		return nil, errors.New("媒体查询未返回消息内容")
	}
	out := []*tg.Message{}
	if _, self := peer.(*tg.InputPeerSelf); self && len(selfIDs) > 0 {
		peer = &tg.InputPeerUser{UserID: selfIDs[0]}
	}
	for _, mc := range modified.GetMessages() {
		if m, ok := mc.(*tg.Message); ok {
			if messageBelongsToPeer(m, peer) {
				out = append(out, m)
			}
		}
	}
	return out, nil
}

func messageBelongsToPeer(m *tg.Message, peer tg.InputPeerClass) bool {
	switch p := peer.(type) {
	case *tg.InputPeerUser:
		other, ok := m.PeerID.(*tg.PeerUser)
		return ok && other.UserID == p.UserID
	case *tg.InputPeerChat:
		other, ok := m.PeerID.(*tg.PeerChat)
		return ok && other.ChatID == p.ChatID
	case *tg.InputPeerChannel:
		other, ok := m.PeerID.(*tg.PeerChannel)
		return ok && other.ChannelID == p.ChannelID
	default:
		return false
	}
}

func (i *iter) prepareLocal(ctx context.Context) error {
	i.duplicateCounts = map[string]int{}
	if i.opts.Manifest != nil {
		for _, d := range i.dialogs {
			p, err := i.manager.FromInputPeer(ctx, d.Peer)
			if err != nil {
				return err
			}
			if !i.opts.Manifest.MatchesSource(p.ID(), peerDialogType(p)) {
				return errors.New("清单不属于此任务的来源对话")
			}
		}
	}
	return ctx.Err()
}

func (i *iter) manifestMessage(ctx context.Context, from peers.Peer, id int, batch []int) (*tg.Message, error) {
	key := resumeKey(from.ID(), id, peerDialogType(from))
	if m, ok := i.messageCache[key]; ok {
		return m, nil
	}
	if i.missingMessages[key] {
		return nil, tutil.ErrMessageDeleted
	}
	if len(batch) == 0 {
		batch = []int{id}
	}
	messages, err := getPeerMessages(ctx, i.pool.Default(ctx), from.InputPeer(), batch, from.ID())
	if err != nil {
		return nil, err
	}
	i.messageCache = map[string]*tg.Message{}
	i.missingMessages = map[string]bool{}
	for _, n := range batch {
		i.missingMessages[resumeKey(from.ID(), n, peerDialogType(from))] = true
	}
	for _, m := range messages {
		k := resumeKey(from.ID(), m.ID, peerDialogType(from))
		i.messageCache[k] = m
		delete(i.missingMessages, k)
	}
	if m, ok := i.messageCache[key]; ok {
		return m, nil
	}
	return nil, tutil.ErrMessageDeleted
}
