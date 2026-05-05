package main

import (
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
	input d.PrepareSyncIn,
	ctx *component.ComponentContainer,
) (*d.PrepareSyncOut, error) {
	res := ctx.Run("which", "ob")
	if !res.Ok {
		ctx.Logger.Log("No Obsidian headless detected, installing now.")
		res := ctx.Run("npm", "i", "-g", "obsidian-headless")
		if !res.Ok {
			ctx.Logger.Log("Could not install Obsidian headless. Make sure Node and NPM are installed. Error: %s", res.Error)
			return nil, errors.New("Couldn't install.")
		}
	}
	ctx.Logger.Log("Found Obsidian!")

	vaultDir := path.Join(ctx.StorageDir, "vaults", input.VaultName)
	_, err := os.Stat(vaultDir)
	if errors.Is(err, os.ErrNotExist) {
		os.MkdirAll(vaultDir, 0755)
	}

	activated := false

	for !activated {
		activate := ctx.Run("ob", "sync-setup", "--vault", input.VaultName, "--password", input.EncyptionKey, "--path", vaultDir)
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
				return nil, errors.New("Couldn't activate.")
			}
		} else {
			break
		}
	}

	ctx.Logger.Log("Sync setup at '%s'", vaultDir)
	stateLock.Lock()
	vaults[input.VaultName] = vaultDir
	vaultLocks[input.VaultName] = &sync.Mutex{}
	stateLock.Unlock()

	return &d.PrepareSyncOut{
		Path: vaultDir,
	}, nil
}

func doSync(
	input d.SyncIn,
	ctx *component.ComponentContainer,
) (*d.SyncOut, error) {
	stateLock.RLock()
	vaultPath, ok := vaults[input.VaultName]
	vaultLock := vaultLocks[input.VaultName]
	stateLock.RUnlock()

	if !ok {
		return nil, errors.New("Vault not registered, make sure to prepare it.")
	}

	vaultLock.Lock()
	defer vaultLock.Unlock()

	res := ctx.RunInDirectory(vaultPath, "ob", "sync")

	return &d.SyncOut{
		Path:   vaultPath,
		Output: res.Output,
		Error:  fmt.Sprintf("%s (code %d)", res.Error, res.Code),
		Ok:     res.Ok,
	}, nil
}

func syncDaemon(
	in d.SyncDaemonIn,
	ctx *component.ComponentContainer,
) (*d.SyncDaemonOut, error) {
	stateLock.RLock()
	vaultPath, ok := vaults[in.VaultName]
	vaultLock := vaultLocks[in.VaultName]
	stateLock.RUnlock()
	if !ok {
		return nil, errors.New("Vault not registered, make sure to prepare it.")
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
	return &d.SyncDaemonOut{}, nil
}

func main() {
	component.CreateComponent(
		d.Manifest,
		component.Mount(d.PrepareSync, prepareSync),
		component.Mount(d.Sync, doSync),
		component.Mount(d.SyncDaemon, syncDaemon),
	).Start()
}
