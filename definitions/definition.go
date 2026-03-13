package definitions

import "github.com/jacksonzamorano/strata/component"

var Manifest = component.ComponentManifest{
	Name:    "obsidian-headless",
	Version: "1.1.7",
}

type EncryptionType string

const (
	EncryptionTypeStandard EncryptionType = "standard"
	EncryptionTypeE2EE     EncryptionType = "e2ee"
)

type PrepareSyncIn struct {
	VaultName    string
	Encryption   EncryptionType
	EncyptionKey string
}
type PrepareSyncOut struct {
	Path string
}

type SyncIn struct {
	VaultName string
}
type SyncOut struct {
	Path   string
	Output string
	Error  string
	Ok     bool
}

type SyncDaemonIn struct {
	VaultName string
}
type SyncDaemonOut struct {}

var PrepareSync = component.Define[PrepareSyncIn, PrepareSyncOut](Manifest, "prepare-sync")
var Sync = component.Define[SyncIn, SyncOut](Manifest, "sync")
var SyncDaemon = component.Define[SyncDaemonIn, SyncDaemonOut](Manifest, "sync-daemon")
