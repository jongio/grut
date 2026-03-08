package config

// AppName is the canonical application name (constant).
const AppName = "grut"

// AppVersion is the application version, overridden at build time via ldflags:
//
//	-X github.com/jongio/grut/internal/config.AppVersion=x.y.z
var AppVersion = "dev"
