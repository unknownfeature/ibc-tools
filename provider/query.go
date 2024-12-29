package provider

import (
	"context"
	connectiontypes "github.com/cosmos/ibc-go/v8/modules/core/03-connection/types"
	chantypes "github.com/cosmos/ibc-go/v8/modules/core/04-channel/types"

	"github.com/cosmos/relayer/v2/relayer/chains/cosmos"
	"main/utils"
)

func QueryConnection(ctx context.Context, cp *cosmos.CosmosProvider, connectionId string) *connectiontypes.ConnectionEnd {
	latestHeight, err := cp.QueryLatestHeight(ctx)
	utils.HandleError(err)
	resp, err := cp.QueryConnection(ctx, latestHeight, connectionId)
	utils.HandleError(err)
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

func QueryChannel(ctx context.Context, cp *cosmos.CosmosProvider, port, chanId string) (*chantypes.Channel, int64) {
	height, err := cp.QueryLatestHeight(ctx)
	utils.HandleError(err)
	res, err := cp.QueryChannel(ctx, height, chanId, port)
	utils.HandleError(err)

	return res.Channel, height
}
