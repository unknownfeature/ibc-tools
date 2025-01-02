package chain

import (
	"context"
	connectiontypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"
	"main/funcs"

	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
)

func QueryConnection(ctx context.Context, cp *cosmos.CosmosProvider, connectionId string) *connectiontypes.ConnectionEnd {
	latestHeight, err := cp.QueryLatestHeight(ctx)
	funcs.HandleError(err)
	resp, err := cp.QueryConnection(ctx, latestHeight, connectionId)
	funcs.HandleError(err)
	return resp.Connection
}
func QueryChannels(ctx context.Context, cp *cosmos.CosmosProvider, nextKey []byte) ([]*chantypes.IdentifiedChannel, error) {
	p := cosmos.DefaultPageRequest()
	p.Key = nextKey
	res, _, err := cp.QueryChannelsPaginated(ctx, p)

	if err != nil {
		return nil, err
	}

	return res, nil
}

func QueryChannel(ctx context.Context, cp *cosmos.CosmosProvider, port, chanId string) *chantypes.Channel {
	height, err := cp.QueryLatestHeight(ctx)
	funcs.HandleError(err)
	res, err := cp.QueryChannel(ctx, height, chanId, port)
	funcs.HandleError(err)

	return res.Channel
}
