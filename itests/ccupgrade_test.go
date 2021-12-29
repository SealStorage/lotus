package itests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/itests/kit"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCCUpgrade(t *testing.T) {
	kit.QuietMiningLogs()

	for _, height := range []abi.ChainEpoch{
		-1,  // before
		162, // while sealing
		560, // after upgrade deal
	} {
		height := height // make linters happy by copying
		t.Run(fmt.Sprintf("upgrade-%d", height), func(t *testing.T) {
			runTestCCUpgrade(t, height)
		})
	}
}

func runTestCCUpgrade(t *testing.T, upgradeHeight abi.ChainEpoch) *kit.TestFullNode {
	ctx := context.Background()
	blockTime := 5 * time.Millisecond

	client, miner, ens := kit.EnsembleMinimal(t, kit.GenesisNetworkVersion(network.Version15))
	ens.InterconnectAll().BeginMining(blockTime)

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		t.Fatal(err)
	}

	CCUpgrade := abi.SectorNumber(kit.DefaultPresealsPerBootstrapMiner + 1)
	fmt.Printf("CCUpgrade: %d\n", CCUpgrade)

	// wait for deadline 0 to pass so that committing starts after post on preseals
	// this gives max time for post to complete minimizing chances of timeout
	waitForDeadline(ctx, t, 1, client, maddr)
	miner.PledgeSectors(ctx, 1, 0, nil)

	sl, err := miner.SectorsList(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")
	require.Equal(t, CCUpgrade, sl[0], "unexpected sector number")
	{
		si, err := client.StateSectorGetInfo(ctx, maddr, CCUpgrade, types.EmptyTSK)
		require.NoError(t, err)
		require.Less(t, 50000, int(si.Expiration))
	}

	waitForSectorActive(t, ctx, CCUpgrade, client, maddr)

	err = miner.SectorMarkForUpgrade(ctx, sl[0], true)
	require.NoError(t, err)

	sl, err = miner.SectorsList(ctx)
	require.NoError(t, err)
	require.Len(t, sl, 1, "expected 1 sector")

	dh := kit.NewDealHarness(t, client, miner, miner)
	deal, res, inPath := dh.MakeOnlineDeal(ctx, kit.MakeFullDealParams{
		Rseed:                        6,
		SuspendUntilCryptoeconStable: true,
	})
	outPath := dh.PerformRetrieval(context.Background(), deal, res.Root, false)
	kit.AssertFilesEqual(t, inPath, outPath)

	status, err := miner.SectorsStatus(ctx, CCUpgrade, true)
	assert.Equal(t, 1, len(status.Deals))
	return client
}

func waitForDeadline(ctx context.Context, t *testing.T, waitIdx uint64, node *kit.TestFullNode, maddr address.Address) {
	for {
		ts, err := node.ChainHead(ctx)
		require.NoError(t, err)
		dl, err := node.StateMinerProvingDeadline(ctx, maddr, ts.Key())
		require.NoError(t, err)
		if dl.Index == waitIdx {
			return
		}
	}
}

func waitForSectorActive(t *testing.T, ctx context.Context, sn abi.SectorNumber, node *kit.TestFullNode, maddr address.Address) {
	for {
		active, err := node.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		require.NoError(t, err)
		for _, si := range active {
			if si.SectorNumber == sn {
				fmt.Printf("ACTIVE\n")
				return
			}
		}

		time.Sleep(time.Second)
	}
}

func TestCCUpgradeAndPoSt(t *testing.T) {
	kit.QuietMiningLogs()
	t.Run("upgrade and then post", func(t *testing.T) {
		ctx := context.Background()
		n := runTestCCUpgrade(t, 100)
		ts, err := n.ChainHead(ctx)
		require.NoError(t, err)
		start := ts.Height()
		// wait for a full proving period
		n.WaitTillChain(ctx, func(ts *types.TipSet) bool {
			if ts.Height() > start+abi.ChainEpoch(2880) {
				return true
			}
			return false
		})
	})
}
