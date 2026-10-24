package services

import (
	"path/filepath"
	"strconv"
	"tdl-ui/internal/config"
	"tdl-ui/internal/engine"
)

func cacheOwner(cfg *config.Manager) int64 {
	if cfg == nil {
		return 0
	}
	return cfg.Get().LoggedInUserID
}
func cacheMediaKind(kind, typ string, owner int64) string {
	return filepath.Join(kind, strconv.FormatInt(owner, 10), engine.PeerNamespace(typ))
}
func videoDiskNamespace(meta videoMeta) string {
	return filepath.Join(strconv.FormatInt(meta.AccountID, 10), engine.PeerNamespace(meta.DialogType))
}
