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
	"main/relayer/client/state"
	"main/utils"
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
	go source.MaybeUpdateClient(height, dest.IBCHeader)

}

func (r *Relayer) ChanOpenInit(_ *state.ForHeightBuilder) *state.ForHeightBuilder {

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenInit{
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

		})
	respCb := func(resp *provider.RelayerTxResponse) {
		r.path.Source().SetChanId(utils.ParseChannelIDFromEvents(resp.Events))
		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel init")
	}
	return r.source.MaybePrependUpdateClientAndSend(r.dest.IBCHeader, msgSupplier.Get, respCb).WithChannelState()

}

func (r *Relayer) ChanOpenTry(cpStateBuilder *state.ForHeightBuilder) *state.ForHeightBuilder {

	chainState := cpStateBuilder.WithChannelState().Build()
	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.Channel()
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenTry{
				PortId:              r.path.Dest().Port(),
				ProofInit:           getChannelState().Proof(),
				ProofHeight:         getChannelState().Height(),
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

	return r.dest.MaybePrependUpdateClientAndSend(r.source.IBCHeader, msgSupplier.Get, respCb).WithChannelState()

}

func (r *Relayer) ChanOpenAck(cpStateBuilder *state.ForHeightBuilder) *state.ForHeightBuilder {

	chainState := cpStateBuilder.WithChannelState().Build()

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.Channel()

			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenAck{
				PortId:                r.path.Source().Port(),
				ChannelId:             r.path.Source().ChanId(),
				CounterpartyChannelId: r.path.Dest().ChanId(),
				CounterpartyVersion:   getChannelState().Val().Version,
				ProofTry:              getChannelState().Proof(),
				ProofHeight:           getChannelState().Height(),

				Signer: r.source.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {
		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel acked")
	}
	return r.source.MaybePrependUpdateClientAndSend(r.dest.IBCHeader, msgSupplier.Get, respCb).WithChannelState()

}

func (r *Relayer) ChanOpenConfirm(cpStateBuilder *state.ForHeightBuilder) *state.ForHeightBuilder {
	chainState := cpStateBuilder.WithChannelState().Build()

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.Channel()

			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenConfirm{
				PortId:      r.path.Dest().Port(),
				ChannelId:   r.path.Dest().ChanId(),
				ProofAck:    getChannelState().Proof(),
				ProofHeight: getChannelState().Height(),

				Signer: r.dest.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {
		r.updateChains(resp.Height, r.source, r.dest)
		fmt.Println("channel confirmed")
	}
	return r.dest.MaybePrependUpdateClientAndSend(r.source.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeTry(cpStateBuilder *state.ForHeightBuilder) *state.ForHeightBuilder {

	chainState := cpStateBuilder.WithChannelState().WithUpgradeState().Build()

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.Channel()
			getUpgradeState := chainState.Upgrade()
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeTry{
				PortId:                        r.path.Dest().Port(),
				ChannelId:                     r.path.Dest().ChanId(),
				ProposedUpgradeConnectionHops: []string{r.path.Dest().ConnId()}, // we leave same ch for this use case
				CounterpartyUpgradeFields:     getUpgradeState().Val().Fields,
				CounterpartyUpgradeSequence:   getChannelState().Val().UpgradeSequence,
				ProofChannel:                  getChannelState().Proof(),
				ProofUpgrade:                  getUpgradeState().Proof(),
				ProofHeight:                   getChannelState().Height(),
				Signer:                        r.dest.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {

		r.updateChains(resp.Height, r.source, r.dest)
		fmt.Println("upgrade tried acked")
	}
	return r.dest.MaybePrependUpdateClientAndSend(r.source.IBCHeader, msgSupplier.Get, respCb).WithChannelState().WithUpgradeState()

}

func (r *Relayer) ChanUpgradeAck(cpStateBuilder *state.ForHeightBuilder) *state.ForHeightBuilder {

	chainState := cpStateBuilder.WithChannelState().WithUpgradeState().Build()

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.Channel()
			getUpgradeState := chainState.Upgrade()
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeAck{
				PortId:              r.path.Source().Port(),
				ChannelId:           r.path.Source().ChanId(),
				CounterpartyUpgrade: *getUpgradeState().Val(),
				ProofChannel:        getChannelState().Proof(),
				ProofUpgrade:        getUpgradeState().Proof(),
				ProofHeight:         getChannelState().Height(),

				Signer: r.source.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {
		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel upgrade acked")
	}

	return r.source.MaybePrependUpdateClientAndSend(r.dest.IBCHeader, msgSupplier.Get, respCb).WithChannelState().WithUpgradeState()

}

func (r *Relayer) ChanUpgradeConfirm(cpStateBuilder *state.ForHeightBuilder) *state.ForHeightBuilder {

	chainState := cpStateBuilder.WithChannelState().WithUpgradeState().Build()

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.Channel()
			getUpgradeState := chainState.Upgrade()
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeConfirm{
				PortId:                   r.path.Dest().Port(),
				ChannelId:                r.path.Dest().ChanId(),
				CounterpartyChannelState: getChannelState().Val().State,
				CounterpartyUpgrade:      *getUpgradeState().Val(),
				ProofChannel:             getChannelState().Proof(),
				ProofUpgrade:             getUpgradeState().Proof(),
				ProofHeight:              getChannelState().Height(),
				Signer:                   r.dest.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {
		r.updateChains(resp.Height, r.source, r.dest)
		fmt.Println("upgrade confirmed")
	}

	return r.dest.MaybePrependUpdateClientAndSend(r.source.IBCHeader, msgSupplier.Get, respCb).WithChannelState()

}

func (r *Relayer) ChanUpgradeOpen(cpStateBuilder *state.ForHeightBuilder) *state.ForHeightBuilder {

	chainState := cpStateBuilder.WithChannelState().Build()

	msgSupplier := utils.NewFuture[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.Channel()
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelUpgradeOpen{
				PortId:                      r.path.Source().Port(),
				ChannelId:                   r.path.Source().ChanId(),
				CounterpartyChannelState:    getChannelState().Val().State,
				CounterpartyUpgradeSequence: getChannelState().Val().UpgradeSequence,
				ProofChannel:                getChannelState().Proof(),
				ProofHeight:                 getChannelState().Height(),

				Signer: r.source.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse) {

		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel upgrade opened")
	}

	return r.source.MaybePrependUpdateClientAndSend(r.dest.IBCHeader, msgSupplier.Get, respCb)

}
