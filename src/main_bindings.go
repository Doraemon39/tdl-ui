//go:build bindings

package main

import (
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"tdl-ui/internal/services"
)

// Binding generation must never initialize real sessions or user data.
func main() {
	_ = wails.Run(&options.App{Bind: []interface{}{&services.AuthService{}, &services.ChatService{}, &services.DownloadService{}, &services.ScriptService{}, &services.SettingsService{}, &services.LogService{}}})
}
