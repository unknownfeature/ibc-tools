package state

import (
	"context"
	"github.com/cosmos/cosmos-sdk/codec"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	connectiontypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v8/modules/core/24-host"
	tmclient "github.com/cosmos/ibc-go/v8/modules/light-clients/07-tendermint"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"main/relayer/client/paths"
	"main/utils"
	"math"
)

var defaultHeightOffset int64 = 10

func readTmProofFactory[T any](ctx context.Context, chainProvider *cosmos.CosmosProvider, keySupplier utils.Supplier[[]byte], transformer utils.Function[[]byte, *T]) utils.Function[int64, ProofData[T]] {
	return func(height int64) ProofData[T] {
		var val, proof []byte
		var err error
		var proofHeight clienttypes.Height
		for val, proof, proofHeight, err = chainProvider.QueryTendermintProof(ctx, height+1, keySupplier()); err != nil; {
			val, proof, proofHeight, err = chainProvider.QueryTendermintProof(ctx, height+1, keySupplier())
		}

		return ProofData[T]{val: transformer(val), proof: proof, height: proofHeight}
	}
}

func maxHeight(one, two *int64) *int64 {
	if one == nil {
		return two
	}
	if two == nil {
		return one
	}
	res := int64(math.Max(float64(*one), float64(*two)))
	return &res
}

func latestBlockSupplier[K any](theMap utils.ConcurrentMap[int64, ProofData[K]]) utils.Supplier[utils.Optional[int64]] {
	return func() utils.Optional[int64] {
		return theMap.ComputeOnKeys(maxHeight)
	}
}
func newStateCache[V any](topNumOfBlocks int64, latestBlockSupplier utils.Supplier[utils.Optional[int64]]) *utils.ExpiringConcurrentMap[int64, ProofData[V]] {
	return utils.NewExpiringConcurrentMap[int64, ProofData[V]](func(e utils.Entry[int64, ProofData[V]], _ int) bool {
		maxBlock := latestBlockSupplier()
		return maxBlock.IsPresent() && *maxBlock.Get()-topNumOfBlocks > e.Key

	})
}

type ChainState struct {
	channelState    *State[chantypes.Channel]
	upgradeState    *State[chantypes.Upgrade]
	clientState     *State[tmclient.ClientState]
	consensusState  *State[tmclient.ConsensusState]
	connectionState *State[connectiontypes.ConnectionEnd]

	chanProofData     *ProofData[chantypes.Channel]
	upgradeProofData  *ProofData[chantypes.Upgrade]
	latestClientState *tmclient.ClientState
}

func NewChainState(ctx context.Context, cdc codec.Codec, chainProvider *cosmos.CosmosProvider, end paths.PathEnd) {
	chanStateKeeper := NewStateKeeper[chantypes.Channel](ctx, chainProvider, host.ChannelKey(end.Port(), end.ChanId()), func(bytes []byte) *chantypes.Channel {
		theChannel := chantypes.Channel{}
		utils.HandleError(cdc.Unmarshal(bytes, &theChannel))
		return &theChannel
	})
}

func (cs *ChainState) ChanProofData() *ProofData[chantypes.Channel] {
	return cs.chanProofData
}

func (cs *ChainState) Channel(height int64) *ProofData[chantypes.Channel] {
	return cs.channelState.Get(height)
}
func (cs *ChainState) UpgradeProofData() *ProofData[chantypes.Upgrade] {
	return cs.upgradeProofData
}

func (cs *ChainState) LatestClientState() *tmclient.ClientState {
	return cs.latestClientState
}

type ProofData[T any] struct {
	proof  []byte
	height clienttypes.Height
	val    *T
}

func (d *ProofData[T]) Proof() []byte {
	return d.proof
}

func (d *ProofData[T]) Height() clienttypes.Height {
	return d.height
}

func (d *ProofData[T]) Val() *T {
	return d.val
}

type State[T any] struct {
	stateChache  *utils.ConcurrentMap[int64, ProofData[T]]
	newStateFunc utils.Function[int64, ProofData[T]]
}

func NewStateKeeper[T any](ctx context.Context, chainProvider *cosmos.CosmosProvider, keySupplier utils.Supplier[[]byte], transformer utils.Function[[]byte, *T]) *State[T] {
	return &State[T]{
		stateChache:  newStateCache[T](defaultHeightOffset, chainProvider.QueryLatestHeight),
		newStateFunc: readTmProofFactory(ctx, chainProvider, keySupplier, transformer),
	}
}

func (sk State[T]) Get(height int64) *ProofData[T] {
	return sk.stateChache.ComputeIfAbsent(height, sk.newStateFunc).Get()
}
