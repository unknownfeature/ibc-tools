package relayer

import (
	"context"
	"fmt"
	"github.com/cosmos/cosmos-sdk/codec"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"main/relayer/client"
	"main/relayer/client/paths"
	"main/utils"
	"sync"
	"time"
)

type Relayer struct {
	source  *client.ChainClient
	dest    *client.ChainClient
	codec   *codec.ProtoCodec
	path    *paths.Path
	context context.Context
	version string
}

type Props struct {
	SourceProvider *cosmos.CosmosProvider
	DestProvider   *cosmos.CosmosProvider
	Path           *paths.Path
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

	respCb := func(resp *provider.RelayerTxResponse) {
		r.path.Source().SetChanId(utils.ParseChannelIDFromEvents(resp.Events))
		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel init")
	}
	r.source.SendMessage(r.context, msg, "init channel", respCb)

}

func (r *Relayer) ChanOpenTry() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.source.GetChainStateForHeight(r.source.Height())
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

	respCb := func(resp *provider.RelayerTxResponse) {
		r.path.Dest().SetChanId(utils.ParseChannelIDFromEvents(resp.Events))
		r.updateChains(resp.Height, r.source, r.dest)
		fmt.Println("channel tried")
	}
	r.dest.MaybePrependUpdateClientAndSend(r.source.Height(), r.source.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanOpenAck() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.dest.GetChainStateForHeight(r.dest.Height())
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

	respCb := func(resp *provider.RelayerTxResponse) {
		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel acked")
	}
	r.source.MaybePrependUpdateClientAndSend(r.dest.Height(), r.dest.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanOpenConfirm() {
	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.source.GetChainStateForHeight(r.source.Height())
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

	respCb := func(resp *provider.RelayerTxResponse) {
		r.updateChains(resp.Height, r.source, r.dest)
		fmt.Println("channel confirmed")
	}
	r.dest.MaybePrependUpdateClientAndSend(r.source.Height(), r.source.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeTry() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.source.GetChainStateForHeight(r.source.Height())
	})

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeTry{
				PortId:                        r.path.Dest().Port(),
				ChannelId:                     r.path.Dest().ChanId(),
				ProposedUpgradeConnectionHops: []string{r.path.Dest().ConnId()}, // we leave same ch for this use case
				CounterpartyUpgradeFields:     chainStateFuture.Get().UpgradeProofData().Val().Fields,
				CounterpartyUpgradeSequence:   chainStateFuture.Get().ChanProofData().Val().UpgradeSequence,
				ProofChannel:                  chainStateFuture.Get().ChanProofData().Proof(),
				ProofUpgrade:                  chainStateFuture.Get().UpgradeProofData().Proof(),
				ProofHeight:                   chainStateFuture.Get().ChanProofData().Height(),
				Signer:                        r.dest.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {
		r.path.Dest().SetUpgrade(true)
		r.path.Source().SetUpgrade(true)
		r.updateChains(resp.Height, r.source, r.dest)
		fmt.Println("upgrade tried acked")
	}
	r.dest.MaybePrependUpdateClientAndSend(r.source.Height(), r.source.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeAck() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.dest.GetChainStateForHeight(r.dest.Height())
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

	respCb := func(resp *provider.RelayerTxResponse) {
		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel upgrade acked")
	}

	r.source.MaybePrependUpdateClientAndSend(r.dest.Height(), r.dest.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeConfirm() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.source.GetChainStateForHeight(r.source.Height())
	})

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeConfirm{
				PortId:                   r.path.Dest().Port(),
				ChannelId:                r.path.Dest().ChanId(),
				CounterpartyChannelState: chainStateFuture.Get().ChanProofData().Val().State,
				CounterpartyUpgrade:      *chainStateFuture.Get().UpgradeProofData().Val(),
				ProofChannel:             chainStateFuture.Get().ChanProofData().Proof(),
				ProofUpgrade:             chainStateFuture.Get().UpgradeProofData().Proof(),
				ProofHeight:              chainStateFuture.Get().ChanProofData().Height(),
				Signer:                   r.dest.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {
		r.updateChains(resp.Height, r.source, r.dest)
		fmt.Println("upgrade confirmed")
	}

	r.dest.MaybePrependUpdateClientAndSend(r.source.Height(), r.source.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeOpen() {

	chainStateFuture := utils.NewFuture[*client.ChainState](func() *client.ChainState {
		return r.dest.GetChainStateForHeight(r.dest.Height())
	})

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeOpen{
				PortId:                      r.path.Source().Port(),
				ChannelId:                   r.path.Source().ChanId(),
				CounterpartyChannelState:    chainStateFuture.Get().ChanProofData().Val().State,
				CounterpartyUpgradeSequence: chainStateFuture.Get().ChanProofData().Val().UpgradeSequence,
				ProofChannel:                chainStateFuture.Get().ChanProofData().Proof(),
				ProofHeight:                 chainStateFuture.Get().ChanProofData().Height(),

				Signer: r.source.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {
		r.path.Dest().SetUpgrade(false)
		r.path.Source().SetUpgrade(false)
		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel upgrade opened")
	}

	r.source.MaybePrependUpdateClientAndSend(r.dest.Height(), r.dest.IBCHeader, msgSupplier.Get, respCb)

}
