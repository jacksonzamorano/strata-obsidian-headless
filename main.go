package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"

	d "github.com/jacksonzamorano/strata-obsidian-headless/definitions"
	"github.com/jacksonzamorano/strata/component"
)

var stateLock sync.RWMutex
var vaults map[string]string = map[string]string{}
var vaultLocks map[string]*sync.Mutex = map[string]*sync.Mutex{}

func prepareSync(
	input *component.ComponentInput[d.PrepareSyncIn, d.PrepareSyncOut],
	ctx *component.ComponentContainer,
) *component.ComponentReturn[d.PrepareSyncOut] {
	res := ctx.Run("which", "ob")
	if !res.Ok {
		ctx.Logger.Log("No Obsidian headless detected, installing now.")
		res := ctx.Run("npm", "i", "-g", "obsidian-headless")
		if !res.Ok {
			ctx.Logger.Log("Could not install Obsidian headless. Make sure Node and NPM are installed. Error: %s", res.Error)
			return input.Error("Couldn't install.")
		}
	}
	ctx.Logger.Log("Found Obsidian!")

	vaultDir := path.Join(ctx.StorageDir, "vaults", input.Body.VaultName)
	_, err := os.Stat(vaultDir)
	if errors.Is(err, os.ErrNotExist) {
		os.MkdirAll(vaultDir, 0755)
	}

	activated := false

	for !activated {
		activate := ctx.Run("ob", "sync-setup", "--vault", input.Body.VaultName, "--password", input.Body.EncyptionKey, "--path", vaultDir)
		if !activate.Ok {
			if activate.Code == 2 {
				username, _ := ctx.RequestSecret("Obsidian email")
				password, _ := ctx.RequestSecret("Obsidian password")
				tfa, _ := ctx.RequestSecret("Obsidian TFA")

				res := ctx.Run("ob", "login", "--email", username, "--password", password, "--mfa", tfa)
				if !res.Ok {
					ctx.Logger.Log("Login failed: %s", res.Error)
				}
			} else {
				ctx.Logger.Log("Could not setup Obsidian, got %s", activate.Error)
				return input.Error("Couldn't activate.")
			}
		} else {
			break
		}
	}

	ctx.Logger.Log("Sync setup at '%s'", vaultDir)
	stateLock.Lock()
	vaults[input.Body.VaultName] = vaultDir
	vaultLocks[input.Body.VaultName] = &sync.Mutex{}
	stateLock.Unlock()

	return input.Return(d.PrepareSyncOut{
		Path: vaultDir,
	})
}

func doSync(
	input *component.ComponentInput[d.SyncIn, d.SyncOut],
	ctx *component.ComponentContainer,
) *component.ComponentReturn[d.SyncOut] {
	stateLock.RLock()
	vaultPath, ok := vaults[input.Body.VaultName]
	vaultLock := vaultLocks[input.Body.VaultName]
	stateLock.RUnlock()

	if !ok {
		return input.Error("Vault not registered, make sure to prepare it.")
	}

	vaultLock.Lock()
	defer vaultLock.Unlock()

	res := ctx.RunInDirectory(vaultPath, "ob", "sync")

	return input.Return(d.SyncOut{
		Path:   vaultPath,
		Output: res.Output,
		Error:  fmt.Sprintf("%s (code %d)", res.Error, res.Code),
		Ok:     res.Ok,
	})
}

func syncDaemon(
	in *component.ComponentInput[d.SyncDaemonIn, d.SyncDaemonOut],
	ctx *component.ComponentContainer,
) *component.ComponentReturn[d.SyncDaemonOut] {
	stateLock.RLock()
	vaultPath, ok := vaults[in.Body.VaultName]
	vaultLock := vaultLocks[in.Body.VaultName]
	stateLock.RUnlock()
	if !ok {
		return in.Error("Vault not registered, make sure to prepare it.")
	}

	vaultLock.Lock()
	cfg := component.ComponentDaemonConfig{
		WorkingDirectory: vaultPath,
		Program:          "ob",
		Args:             []string{"sync", "--continuous"},
		Exited: func(r component.ComponentExecuteResponse) {
			if !r.Ok {
				ctx.Logger.Log("Obsidian sync exited with error: %s", r.Error)
				vaultLock.Unlock()
			}
		},
	}
	ctx.StartDaemonInDirectory(cfg)
	return in.Return(d.SyncDaemonOut{})
}

func main() {
	component.CreateComponent(
		d.Manifest,
		component.Mount(d.PrepareSync, prepareSync),
		component.Mount(d.Sync, doSync),
		component.Mount(d.SyncDaemon, syncDaemon),
	).Start()
}
