package main

import (
	"errors"
	"os"
	"path"
	"sync"

	d "github.com/jacksonzamorano/strata-obsidian-headless/definitions"
	"github.com/jacksonzamorano/strata/component"
)

var stateLock sync.RWMutex
var vaults map[string]string = map[string]string{}

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
	defer stateLock.RUnlock()

	vaultPath, ok := vaults[input.Body.VaultName]
	if !ok {
		return input.Error("Vault not registered, make sure to preapare it.")
	}

	ctx.Run("ob", "sync", "--path", vaultPath)

	return input.Return(d.SyncOut{
		Path: vaultPath,
	})
}

func main() {
	component.CreateComponent(
		d.Manifest,
		component.Mount(d.PrepareSync, prepareSync),
		component.Mount(d.Sync, doSync),
	).Start()
}
