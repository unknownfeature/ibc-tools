package relayer

import (
	"context"
	"fmt"
	"github.com/cosmos/cosmos-sdk/codec"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"main/relayer/client"
	"main/utils"
	"sync"
)

type Relayer struct {
	source               *client.ChainClient
	dest                 *client.ChainClient
	codec                *codec.ProtoCodec
	path                 *client.Path
	context              context.Context
	version              string
	sourceUpdatedHeights *sync.Map
	destUpdatedHeights   *sync.Map
}

type Props struct {
	SourceProvider *cosmos.CosmosProvider
	DestProvider   *cosmos.CosmosProvider
	Path           *client.Path
	Version        string
}

func NewRelayer(ctx context.Context, cdc *codec.ProtoCodec, props *Props) *Relayer {
	r := &Relayer{
		source:  client.NewChainClient(ctx, cdc, props.SourceProvider, props.Path.Source()),
		dest:    client.NewChainClient(ctx, cdc, props.DestProvider, props.Path.Dest()),
		codec:   cdc,
		path:    props.Path,
		context: ctx,
		version: props.Version,
	}
	return r
}
func (r *Relayer) updateChains(height int64, source, dest *client.ChainClient) {
	go func() { source.MaybeUpdateClient(height, dest.IBCHeader) }()
	go func() { dest.MaybeUpdateChainState(height) }()
}

func (r *Relayer) ChanOpenInit() {
	msg := cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenInit{
		PortId: r.path.Source().Port(),
		Channel: chantypes.Channel{
			State:    chantypes.INIT,
			Ordering: chantypes.UNORDERED,
			Counterparty: chantypes.Counterparty{
				PortId:    r.path.Dest().Port(),
				ChannelId: "",
			},
			ConnectionHops: []string{r.path.Source().ConnId()},
			Version:        r.version,
		},
		Signer: r.source.Address(),
	}, nil)
	resp := r.source.SendMessage(r.context, msg, "init channel")
	r.path.Source().SetChanId(utils.ParseChannelIDFromEvents(resp.Events))
	r.updateChains(resp.Height, r.dest, r.source)
	fmt.Println("channel init")
}

func (r *Relayer) ChanOpenTry() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.source.GetChainStateForHeight(r.path.Source().Height())
	})

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenTry{
				PortId:              r.path.Dest().Port(),
				ProofInit:           chainStateFuture.Get().ChanProofData().Proof(),
				ProofHeight:         chainStateFuture.Get().ChanProofData().Height(),
				CounterpartyVersion: r.version,
				Channel: chantypes.Channel{State: chantypes.TRYOPEN,
					Ordering: chantypes.UNORDERED,
					Counterparty: chantypes.Counterparty{PortId: r.path.Source().Port(),
						ChannelId: r.path.Source().ChanId()},
					ConnectionHops: []string{r.path.Dest().ConnId()},
					Version:        r.version},
				Signer: r.dest.Address(),
			}, nil)
		})

	resp := r.dest.MaybePrependUpdateClientAndSend(r.path.Source().Height(), r.source.IBCHeader, msgSupplier.Get)

	r.path.Dest().SetChanId(utils.ParseChannelIDFromEvents(resp.Events))
	r.updateChains(resp.Height, r.source, r.dest)
	fmt.Println("channel tried")

}

func (r *Relayer) ChanOpenAck() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.dest.GetChainStateForHeight(r.path.Dest().Height())
	})

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenAck{
				PortId:                r.path.Source().Port(),
				ChannelId:             r.path.Source().ChanId(),
				CounterpartyChannelId: r.path.Dest().ChanId(),
				CounterpartyVersion:   chainStateFuture.Get().ChanProofData().Val().Version,
				ProofTry:              chainStateFuture.Get().ChanProofData().Proof(),
				ProofHeight:           chainStateFuture.Get().ChanProofData().Height(),

				Signer: r.source.Address(),
			}, nil)
		})

	resp := r.source.MaybePrependUpdateClientAndSend(r.path.Dest().Height(), r.dest.IBCHeader, msgSupplier.Get)
	r.updateChains(resp.Height, r.dest, r.source)
	fmt.Println("channel acked")

}

func (r *Relayer) ChanOpenConfirm() {
	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.source.GetChainStateForHeight(r.path.Source().Height())
	})

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenConfirm{
				PortId:      r.path.Dest().Port(),
				ChannelId:   r.path.Dest().ChanId(),
				ProofAck:    chainStateFuture.Get().ChanProofData().Proof(),
				ProofHeight: chainStateFuture.Get().ChanProofData().Height(),

				Signer: r.dest.Address(),
			}, nil)
		})

	resp := r.dest.MaybePrependUpdateClientAndSend(r.path.Source().Height(), r.source.IBCHeader, msgSupplier.Get)
	r.updateChains(resp.Height, r.source, r.dest)
	fmt.Println("channel confirmed")

}

func (r *Relayer) ChanUpgradeTry() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.source.GetChainStateForHeight(r.path.Source().Height())
	})

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeTry{
				PortId:                        r.path.Dest().Port(),
				ChannelId:                     r.path.Dest().ChanId(),
				ProposedUpgradeConnectionHops: []string{r.path.Dest().ConnId()}, // same ch
				CounterpartyUpgradeFields:     chainStateFuture.Get().UpgradeProofData().Val().Fields,
				CounterpartyUpgradeSequence:   chainStateFuture.Get().ChanProofData().Val().UpgradeSequence,
				ProofChannel:                  chainStateFuture.Get().ChanProofData().Proof(),
				ProofUpgrade:                  chainStateFuture.Get().UpgradeProofData().Proof(),
				ProofHeight:                   chainStateFuture.Get().ChanProofData().Height(),
				Signer:                        r.dest.Address(),
			}, nil)
		})

	resp := r.dest.MaybePrependUpdateClientAndSend(r.path.Source().Height(), r.source.IBCHeader, msgSupplier.Get)
	r.updateChains(resp.Height, r.source, r.dest)
	fmt.Println("upgrade tried acked")

}

func (r *Relayer) ChanUpgradeAck() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.dest.GetChainStateForHeight(r.path.Dest().Height())
	})

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeAck{
				PortId:              r.path.Source().Port(),
				ChannelId:           r.path.Source().ChanId(),
				CounterpartyUpgrade: *chainStateFuture.Get().UpgradeProofData().Val(),
				ProofChannel:        chainStateFuture.Get().ChanProofData().Proof(),
				ProofUpgrade:        chainStateFuture.Get().UpgradeProofData().Proof(),
				ProofHeight:         chainStateFuture.Get().ChanProofData().Height(),

				Signer: r.source.Address(),
			}, nil)
		})

	resp := r.source.MaybePrependUpdateClientAndSend(r.path.Dest().Height(), r.dest.IBCHeader, msgSupplier.Get)
	r.updateChains(resp.Height, r.dest, r.source)
	fmt.Println("channel upgrade acked")

}
