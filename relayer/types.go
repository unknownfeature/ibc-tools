package relayer

import (
	"context"
	"fmt"
	"github.com/cosmos/cosmos-sdk/codec"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"github.com/cosmos/relayer/v2/relayer/provider"
	"main/concurrent"
	"main/funcs"
	"main/relayer/client"
	"main/relayer/client/paths"
	"main/relayer/client/state"
	"main/utils"
)

func LatestHeightLoaderFactory(ctx context.Context, cosmosProvider *cosmos.CosmosProvider) funcs.Supplier[int64] {
	factory := func(channel chan int64) funcs.Supplier[error] {
		return func() error {
			hdr, err := cosmosProvider.QueryLatestHeight(ctx)
			if err != nil {
				fmt.Println("error", err.Error())
				return err
			}
			channel <- hdr
			return nil
		}
	}
	return funcs.RetriableSupplier[int64](factory)
}

type Relayer struct {
	source  *client.ChainClient
	dest    *client.ChainClient
	codec   *codec.ProtoCodec
	path    *paths.Path
	context context.Context
}

type Props struct {
	SourceChain *cosmos.CosmosProvider
	DestChain   *cosmos.CosmosProvider
	Path        *paths.Path
}

func NewRelayer(ctx context.Context, cdc *codec.ProtoCodec, props *Props, sourceHeight, destHeight *concurrent.Future[int64]) *Relayer {

	r := &Relayer{
		source:  client.NewChainClient(ctx, cdc, props.SourceChain, props.Path.Source(), sourceHeight, destHeight),
		dest:    client.NewChainClient(ctx, cdc, props.DestChain, props.Path.Dest(), destHeight, sourceHeight),
		codec:   cdc,
		path:    props.Path,
		context: ctx,
	}
	fmt.Println("created relayer")
	r.updateChains(destHeight.Get(), r.source, r.dest)
	r.updateChains(sourceHeight.Get(), r.dest, r.source)

	return r
}

func (r *Relayer) CloneForPath(otherPath *paths.Path) *Relayer {
	return &Relayer{
		source:  r.source.CloneForPathEnd(otherPath.Source()),
		dest:    r.dest.CloneForPathEnd(otherPath.Dest()),
		codec:   r.codec,
		path:    otherPath,
		context: r.context,
	}
}
func (r *Relayer) updateChains(height int64, source, dest *client.ChainClient) {
	go source.MaybeUpdateClient(height, dest.IBCHeader)

}

func (r *Relayer) ChanOpenInit(version string) string {

	msgSupplier := concurrent.SupplyAsync[provider.RelayerMessage](
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
					Version:        version,
				},
				Signer: r.source.Address(),
			}, nil)

		})
	respCb := func(resp *provider.RelayerTxResponse, state *state.ChainStateManager) {
		r.path.Source().SetChanId(utils.ParseChannelIDFromEvents(resp.Events))
		r.updateChains(resp.Height, r.dest, r.source)
		state.LoadChannel(resp.Height)
		fmt.Println("channel init")
	}
	r.source.MaybePrependUpdateClientAndSend(r.dest.Height(), r.dest.IBCHeader, msgSupplier.Get, respCb)
	return r.path.Source().ChanId()
}

func (r *Relayer) ChanOpenTry() string {

	chainState := r.source.ChannelState()

	msgSupplier := concurrent.SupplyAsync[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.ChannelStateSupplier()
			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenTry{
				PortId:              r.path.Dest().Port(),
				ProofInit:           getChannelState().Proof(),
				ProofHeight:         getChannelState().Height(),
				CounterpartyVersion: getChannelState().Val().Version,
				Channel: chantypes.Channel{State: chantypes.TRYOPEN,
					Ordering: chantypes.UNORDERED,
					Counterparty: chantypes.Counterparty{PortId: r.path.Source().Port(),
						ChannelId: r.path.Source().ChanId()},
					ConnectionHops: []string{r.path.Dest().ConnId()},
					Version:        getChannelState().Val().Version},
				Signer: r.dest.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse, state *state.ChainStateManager) {
		r.path.Dest().SetChanId(utils.ParseChannelIDFromEvents(resp.Events))
		r.updateChains(resp.Height, r.source, r.dest)
		state.LoadChannel(resp.Height)
		fmt.Println("channel tried")

	}

	r.dest.MaybePrependUpdateClientAndSend(chainState.Height(), r.source.IBCHeader, msgSupplier.Get, respCb)
	return r.path.Dest().ChanId()

}

func (r *Relayer) ChanOpenAck() {

	chainState := r.dest.ChannelState()

	msgSupplier := concurrent.SupplyAsync[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.ChannelStateSupplier()

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

	respCb := func(resp *provider.RelayerTxResponse, state *state.ChainStateManager) {
		r.updateChains(resp.Height, r.dest, r.source)
		state.LoadChannel(resp.Height)
		fmt.Println("channel acked")
	}
	r.source.MaybePrependUpdateClientAndSend(chainState.Height(), r.dest.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanOpenConfirm() {
	chainState := r.source.ChannelState()

	msgSupplier := concurrent.SupplyAsync[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.ChannelStateSupplier()

			return cosmos.NewCosmosMessage(&chantypes.MsgChannelOpenConfirm{
				PortId:      r.path.Dest().Port(),
				ChannelId:   r.path.Dest().ChanId(),
				ProofAck:    getChannelState().Proof(),
				ProofHeight: getChannelState().Height(),

				Signer: r.dest.Address(),
			}, nil)
		})

	respCb := func(resp *provider.RelayerTxResponse, state *state.ChainStateManager) {
		r.updateChains(resp.Height, r.source, r.dest)
		state.LoadChannel(resp.Height)
		fmt.Println("channel confirmed")

	}
	r.dest.MaybePrependUpdateClientAndSend(chainState.Height(), r.source.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeTry() {

	chainState := r.source.ChannelUpgradeState()

	msgSupplier := concurrent.SupplyAsync[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.ChannelStateSupplier()
			getUpgradeState := chainState.UpgradeStateSupplier()
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

	respCb := func(resp *provider.RelayerTxResponse, state *state.ChainStateManager) {

		r.updateChains(resp.Height, r.source, r.dest)
		state.LatestChannelUpgrade()
		fmt.Println("upgrade tried acked")

	}
	r.dest.MaybePrependUpdateClientAndSend(chainState.Height(), r.source.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeAck() {

	chainState := r.dest.ChannelUpgradeState()

	msgSupplier := concurrent.SupplyAsync[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.ChannelStateSupplier()
			getUpgradeState := chainState.UpgradeStateSupplier()
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

	respCb := func(resp *provider.RelayerTxResponse, state *state.ChainStateManager) {
		r.updateChains(resp.Height, r.dest, r.source)
		state.LoadChannelUpgrade(resp.Height)
		fmt.Println("channel upgrade acked")

	}

	r.source.MaybePrependUpdateClientAndSend(chainState.Height(), r.dest.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeConfirm() {

	chainState := r.source.ChannelUpgradeState()

	msgSupplier := concurrent.SupplyAsync[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.ChannelStateSupplier()
			getUpgradeState := chainState.UpgradeStateSupplier()
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

	respCb := func(resp *provider.RelayerTxResponse, state *state.ChainStateManager) {
		r.updateChains(resp.Height, r.source, r.dest)
		state.LoadChannelUpgrade(resp.Height)
		fmt.Println("upgrade confirmed")

	}

	r.dest.MaybePrependUpdateClientAndSend(chainState.Height(), r.source.IBCHeader, msgSupplier.Get, respCb)

}

func (r *Relayer) ChanUpgradeOpen() {
	chainState := r.dest.ChannelUpgradeState()

	msgSupplier := concurrent.SupplyAsync[provider.RelayerMessage](
		func() provider.RelayerMessage {
			getChannelState := chainState.ChannelStateSupplier()
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

	respCb := func(resp *provider.RelayerTxResponse, state *state.ChainStateManager) {

		r.updateChains(resp.Height, r.dest, r.source)
		fmt.Println("channel upgrade opened")

	}

	r.source.MaybePrependUpdateClientAndSend(chainState.Height(), r.dest.IBCHeader, msgSupplier.Get, respCb)

}
