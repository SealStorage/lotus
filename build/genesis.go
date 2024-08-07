package build

import (
	"embed"
	"path"

	logging "github.com/ipfs/go-log/v2"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

// moved from now-defunct build/paramfetch.go
var log = logging.Logger("build")

//go:embed genesis
var genesisfs embed.FS

func MaybeGenesis() []byte {
	genBytes, err := genesisfs.ReadFile(path.Join("genesis", buildconstants.GenesisFile))
	if err != nil {
		log.Warnf("loading built-in genesis: %s", err)
		return nil
	}
	return genBytes
}
