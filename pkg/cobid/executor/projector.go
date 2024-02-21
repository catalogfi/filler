package executor

import (
	"encoding/json"
	"net/http"
)

const MempoolBlocksFeesURL = "https://mempool.space/api/v1/fees/mempool-blocks"

type ProjectedBlock struct {
	FeeRange []float64
}

type BlockProjector struct {
	client *http.Client
	url    string
}

func NewMempoolProjector() *BlockProjector {
	return &BlockProjector{
		client: new(http.Client),
		url:    MempoolBlocksFeesURL,
	}
}

func (projector *BlockProjector) NextBlocks() ([]ProjectedBlock, error) {
	resp, err := projector.client.Get(projector.url)
	if err != nil {
		return nil, err
	}
	var blocks []ProjectedBlock
	if err := json.NewDecoder(resp.Body).Decode(&blocks); err != nil {
		return nil, err
	}

	return blocks, nil
}
