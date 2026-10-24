package engine

import (
	"github.com/go-faster/errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type messageLink struct {
	Username  string
	ChannelID int64
	MessageID int
	ThreadID  int
	CommentID int
	Single    bool
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)

func positiveID(value string, bits int) (int64, error) {
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, errors.New("消息 ID 必须是正整数数字")
		}
	}
	n, err := strconv.ParseInt(value, 10, bits)
	if err != nil || n <= 0 {
		return 0, errors.New("消息 ID 必须是范围内的正整数数字")
	}
	return n, nil
}

// parseMessageLink implements the message-link subset of Telegram deep links.
// Thread IDs are navigation context, comment IDs are discussion-group messages,
// and single suppresses album expansion. No invite/login/bot action is executed.
func parseMessageLink(raw string) (messageLink, error) {
	var out messageLink
	if strings.HasPrefix(strings.ToLower(raw), "tg:") && !strings.HasPrefix(strings.ToLower(raw), "tg://") {
		raw = "tg://" + raw[3:]
	}
	if !strings.Contains(raw, ":") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return out, err
	}
	if u.User != nil || u.Port() != "" || u.Opaque != "" {
		return out, errors.New("不支持带凭证、端口或不透明路径的链接")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return out, err
	}
	for key, values := range q {
		if len(values) != 1 {
			return out, errors.Errorf("链接参数 %s 重复", key)
		}
	}
	var post, thread string
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
		if host != "t.me" && host != "telegram.me" && host != "telegram.dog" {
			return out, errors.New("仅支持 Telegram 的 t.me / telegram.me / telegram.dog 消息链接")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) > 0 && parts[0] == "c" {
			if len(parts) != 3 && len(parts) != 4 {
				return out, errors.New("私有频道链接必须包含频道 ID 和消息 ID")
			}
			out.ChannelID, err = positiveID(parts[1], 64)
			if err != nil {
				return out, err
			}
			post = parts[len(parts)-1]
			if len(parts) == 4 {
				thread = parts[2]
			}
		} else {
			if len(parts) != 2 && len(parts) != 3 {
				return out, errors.New("链接缺少消息 ID 或包含多余路径")
			}
			out.Username = parts[0]
			post = parts[len(parts)-1]
			if len(parts) == 3 {
				thread = parts[1]
			}
		}
	case "tg":
		if u.Path != "" && u.Path != "/" {
			return out, errors.New("无效 Telegram 消息链接路径")
		}
		switch strings.ToLower(u.Host) {
		case "resolve":
			out.Username = q.Get("domain")
		case "privatepost":
			out.ChannelID, err = positiveID(q.Get("channel"), 64)
			if err != nil {
				return out, err
			}
		default:
			return out, errors.New("仅支持 tg://resolve 和 tg://privatepost 消息链接")
		}
		post = q.Get("post")
	default:
		return out, errors.New("消息链接必须使用 https:// 或 tg://")
	}
	if out.ChannelID == 0 && !usernamePattern.MatchString(out.Username) {
		return out, errors.New("Telegram 用户名无效")
	}
	id, err := positiveID(post, 32)
	if err != nil {
		return out, err
	}
	out.MessageID = int(id)
	if value, ok := q["thread"]; ok {
		if _, err := positiveID(value[0], 32); err != nil {
			return out, err
		}
		if thread != "" && thread != value[0] {
			return out, errors.New("话题 ID 参数冲突")
		}
		thread = value[0]
	}
	if thread != "" {
		n, err := positiveID(thread, 32)
		if err != nil {
			return out, err
		}
		out.ThreadID = int(n)
	}
	if value, ok := q["comment"]; ok {
		n, err := positiveID(value[0], 32)
		if err != nil {
			return out, err
		}
		out.CommentID = int(n)
	}
	_, out.Single = q["single"]
	return out, nil
}

func checkMessageLink(raw string) error { _, err := parseMessageLink(raw); return err }

func NormalizeURLs(raw []string) ([]string, error) {
	seen := make(map[string]bool)
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if err := checkMessageLink(value); err != nil {
			return nil, errors.Wrap(err, "消息链接无效")
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("至少需要一条消息链接")
	}
	return out, nil
}
