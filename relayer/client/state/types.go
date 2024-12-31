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
)

var defaultHeightOffset int64 = 10

func readTmProofFactory[T any](ctx context.Context, chainProvider *cosmos.CosmosProvider, key []byte, transformer utils.Function[[]byte, *T]) utils.Function[int64, ProofData[T]] {
	return func(height int64) ProofData[T] {
		var val, proof []byte
		var err error
		var proofHeight clienttypes.Height
		for val, proof, proofHeight, err = chainProvider.QueryTendermintProof(ctx, height+1, key); err != nil; {
			val, proof, proofHeight, err = chainProvider.QueryTendermintProof(ctx, height+1, key)
		}

		return ProofData[T]{val: transformer(val), proof: proof, height: proofHeight}
	}
}

type ChainState struct {
	chanStateKeeper       *StateKeeper[chantypes.Channel]
	upgradeStateKeeper    *StateKeeper[chantypes.Upgrade]
	clientStateKeeper     *StateKeeper[tmclient.ClientState]
	consensusStateKeeper  *StateKeeper[tmclient.ConsensusState]
	connectionStateKeeper *StateKeeper[connectiontypes.ConnectionEnd]

	enabledKeepers []*StateKeeper[any]

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
	return cs.chanStateKeeper.Get(height)
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

type StateKeeper[T any] struct {
	perHeightState *utils.ConcurrentTTLMap[int64, ProofData[T]]
	newStateFunc   utils.Function[int64, ProofData[T]]
}

func NewStateKeeper[T any](ctx context.Context, chainProvider *cosmos.CosmosProvider, key []byte, transformer utils.Function[[]byte, *T]) *StateKeeper[T] {
	return &StateKeeper[T]{
		perHeightState: utils.NewMapWithExpirationPredicate[int64, ProofData[T]](
			func(i, j int64) int { return int(i - j) },
			func(i utils.MapItem[ProofData[T]], topKey int64) bool {
				return int64(i.Val.height.RevisionHeight) < topKey-defaultHeightOffset
			},
		),
		newStateFunc: readTmProofFactory(ctx, chainProvider, key, transformer),
	}
}

func (sk StateKeeper[T]) Get(height int64) *ProofData[T] {
	return sk.perHeightState.ComputeIfAbsent(height, sk.newStateFunc).Get()
}
