package core

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/GoPlugin/web3rpcproxy/internal/common"
	"github.com/GoPlugin/web3rpcproxy/internal/core/endpoint"
	"github.com/GoPlugin/web3rpcproxy/internal/core/reqctx"
	"github.com/GoPlugin/web3rpcproxy/internal/core/rpc"
	"github.com/GoPlugin/web3rpcproxy/utils"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/google/uuid"
)

type Client interface {
	Request(ctx context.Context, rc reqctx.Reqctxs, endpoint []*endpoint.Endpoint, jsonrpcs []rpc.SealedJSONRPC) (results []rpc.JSONRPCResulter, err error)
}

func NewClient(ecf *endpoint.ClientFactory) Client {
	return &client{
		ecf: ecf,
	}
}

type client struct {
	ecf *endpoint.ClientFactory
}

func (c *client) Request(ctx context.Context, rc reqctx.Reqctxs, endpoints []*endpoint.Endpoint, jsonrpcs []rpc.SealedJSONRPC) (results []rpc.JSONRPCResulter, err error) {
	if rc.Options().AttemptStrategy() == reqctx.Same {
		endpoints = endpoints[:1]
	}

	var (
		sChainId = fmt.Sprint(rc.ChainID())
		methods  = getMethods(jsonrpcs)
		p        = rc.Profile()
		l        = len(endpoints)
		timeout  = rc.Options().Timeout().Milliseconds()
		_timeout = int64(math.Max(float64(timeout/int64(l)), 500))
	)

	if _timeout > timeout {
		_timeout = timeout
	}

	rc.Logger().Debug().Msgf("endpoints count: %d methods: %s", l, methods)

	for i := 1; i <= rc.Options().Attempts(); i++ {
		var (
			reqId    = uuid.NewString()
			endpoint = endpoints[(i-1)%l]
			_client  = c.ecf.GetClient(endpoint)
			now      = time.Now()
		)

		if _client == nil {
			if l <= 1 {
				break
			}
			continue
		}

		url := endpoint.Url().String()
		p.Requests = append(p.Requests, common.RequestProfile{
			ReqID:     reqId,
			Timestamp: now.UnixMilli(),
			Url:       url,
			Methods:   methods,
		})
		profile := common.ResponseProfile{
			ReqID: reqId,
		}

		if endpoint.Health() {
			results, err = _client.Call(ctx, jsonrpcs, &profile)
		} else {
			_ctx, cancel := context.WithTimeout(ctx, time.Duration(_timeout)*time.Millisecond)
			results, err = _client.Call(_ctx, jsonrpcs, &profile)
			cancel()
		}

		profile.Respond, p.Responses = true, append(p.Responses, profile)

		utils.EndpointDurations.WithLabelValues(sChainId, url).Observe(float64(profile.Duration) / 1000.0)
		utils.TotalEndpoints.WithLabelValues(sChainId, url, strconv.Itoa(profile.Status)).Inc()
		rc.Logger().Debug().Str("req-id", reqId).Msgf("%d/#%d call: %s %d %dms", rc.Options().Attempts(), i, url, profile.Status, profile.Duration)

		if err == nil && results != nil && !slice.Some(results, func(_ int, item rpc.JSONRPCResulter) bool { return item.Type() == rpc.JSONRPC_ERROR }) {
			break
		}
		if e, ok := err.(common.HTTPErrors); ok && e.QueryStatus() == common.Timeout {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	if len(results) <= 0 {
		return nil, common.InternalServerError("All endpoints are unavailable")
	}

	return results, nil
}

func getMethods(jsonrpcs []rpc.SealedJSONRPC) []string {
	methods := slice.Map(jsonrpcs, func(i int, jsonrpc rpc.SealedJSONRPC) string {
		return jsonrpc.Method
	})
	return methods
}
