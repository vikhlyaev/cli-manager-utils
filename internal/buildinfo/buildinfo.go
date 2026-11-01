// Package buildinfo хранит сведения о сборке. Version подставляется при сборке:
//
//	go build -ldflags "-X cli-manager/internal/buildinfo.Version=v1.1.0"
package buildinfo

// Version — версия приложения; "dev" для сборок без ldflags (go run).
var Version = "dev"
