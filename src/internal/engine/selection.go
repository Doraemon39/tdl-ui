package engine

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/pkg/tmessage"
)

// 对话类型（与 ref/tdl/app/chat 一致），services 层复用。
const (
	DialogPrivate    = "private"
	DialogGroup      = "group"
	DialogSupergroup = "supergroup"
	DialogChannel    = "channel"
)

// Selection 按「对话 + 消息 ID 集合」直接指定下载内容，
// 无需 t.me 链接（私聊 / 普通群组没有消息链接也可下载）。
type Selection struct {
	DialogID   int64  `json:"dialogId"`
	DialogType string `json:"dialogType"`
	MessageIDs []int  `json:"messageIds"`
}

// ResolveDialogPeer 按对话类型解析输入 peer（依赖对话列表落盘的 access hash）。
func ResolveDialogPeer(ctx context.Context, manager *peers.Manager, id int64, typ string) (tg.InputPeerClass, error) {
	if manager == nil || id <= 0 {
		return nil, errors.New("无效对话")
	}
	switch typ {
	case DialogPrivate:
		self, err := manager.Self(ctx)
		if err != nil {
			return nil, err
		}
		if self.ID() == id {
			return &tg.InputPeerSelf{}, nil
		}
		u, err := manager.ResolveUserID(ctx, id)
		if err != nil {
			return nil, err
		}
		return u.InputPeer(), nil
	case DialogChannel, DialogSupergroup:
		ch, err := manager.ResolveChannelID(ctx, id)
		if err != nil {
			return nil, err
		}
		return ch.InputPeer(), nil
	case DialogGroup:
		c, err := manager.ResolveChatID(ctx, id)
		if err != nil {
			return nil, err
		}
		return c.InputPeer(), nil
	default:
		return nil, errors.New("缺少有效对话类型，请刷新对话列表")
	}
}

// validateSelections 任务创建时的选集预检：格式错误立即反馈。
func validateSelections(sels []Selection) error {
	for _, sel := range sels {
		if sel.DialogID <= 0 {
			return errors.New("选集缺少对话 ID")
		}
		switch sel.DialogType {
		case DialogPrivate, DialogGroup, DialogSupergroup, DialogChannel:
		default:
			return errors.New("选集对话类型无效")
		}
		for _, id := range sel.MessageIDs {
			if id <= 0 || int64(id) > 2147483647 {
				return errors.New("消息 ID 必须是正 int32")
			}
		}
		if len(sel.MessageIDs) == 0 {
			return errors.Errorf("对话 %d 的选集缺少消息 ID", sel.DialogID)
		}
	}
	return nil
}

// selectionsToDialogs 把选集解析为下载迭代器所需的 tmessage.Dialog。
func selectionsToDialogs(ctx context.Context, manager *peers.Manager, sels []Selection) ([]*tmessage.Dialog, error) {
	out := make([]*tmessage.Dialog, 0, len(sels))
	for _, sel := range sels {
		p, err := ResolveDialogPeer(ctx, manager, sel.DialogID, sel.DialogType)
		if err != nil {
			return nil, errors.Wrapf(err, "解析对话 %d 失败", sel.DialogID)
		}
		out = append(out, &tmessage.Dialog{Peer: p, Messages: sel.MessageIDs})
	}
	return out, nil
}

// PeerNamespace is the underlying MTProto peer class. Supergroups and channels
// share a peer class; ordinary groups and users have separate ID spaces.
func PeerNamespace(typ string) string {
	switch typ {
	case DialogPrivate, DialogGroup, DialogChannel:
		return typ
	case DialogSupergroup:
		return DialogChannel
	default:
		return ""
	}
}
