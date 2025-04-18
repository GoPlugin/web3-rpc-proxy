package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strconv"
	"time"

	"github.com/GoPlugin/web3rpcproxy/internal/common"
	"github.com/GoPlugin/web3rpcproxy/internal/core"
	"github.com/GoPlugin/web3rpcproxy/internal/core/endpoint"
	"github.com/GoPlugin/web3rpcproxy/internal/core/reqctx"
	"github.com/GoPlugin/web3rpcproxy/internal/core/rpc"
	"github.com/GoPlugin/web3rpcproxy/utils"
	"github.com/GoPlugin/web3rpcproxy/utils/config"
	"github.com/GoPlugin/web3rpcproxy/utils/helpers"
	"github.com/allegro/bigcache"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/rs/zerolog"
)

type CacheEntry struct {
	V          any
	T          int64
	compressed bool
}
type agentServiceConfig struct {
	CacheMethods      map[string]string
	MaxEntryCacheSize int
	DisableCache      bool
}

// AgentService
type agentService struct {
	logger     zerolog.Logger
	client     core.Client
	es         endpoint.Selector
	jrpcSchema *rpc.JSONRPCSchema
	cache      *bigcache.BigCache
	config     *agentServiceConfig
}

// define interface of IAgentService
//
//go:generate mockgen -destination=agent_service_mock.go -package=service . AgentService
type AgentService interface {
	Call(ctx context.Context, reqctx reqctx.Reqctxs, endpoints []*endpoint.Endpoint) ([]byte, error)
}

func nearestPowerOfTwo(n uint) uint {
	if n == 0 {
		return 1
	}

	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++

	return n
}

// init AgentService
func NewAgentService(
	logger zerolog.Logger,
	config *config.Conf,
	jrpcSchema *rpc.JSONRPCSchema,
	client core.Client,
	endpointService EndpointService,
) AgentService {
	logger = logger.With().Str("name", "agent_service").Logger()

	existExpiryConfig := config.Exists("cache.results.expiry_durations")
	_config := &agentServiceConfig{
		DisableCache:      config.Bool("cache.results.disable", false) || !existExpiryConfig,
		MaxEntryCacheSize: 512 * 1024, // 512KB
	}

	if existExpiryConfig {
		expiryConfig := map[string]string{}
		config.Unmarshal("cache.results.expiry_durations", &expiryConfig)
		_config.CacheMethods = expiryConfig
	}

	// default cache size is 512MB
	totalCacheSize := config.Int("cache.results.size", 512*1024*1024)

	shards := int(nearestPowerOfTwo(uint(len(endpointService.Chains()))))
	// must have 8MB size pre shard
	if totalCacheSize/shards < 8 {
		shards = int(nearestPowerOfTwo(uint(totalCacheSize / 8)))
	}

	_cacheConfig := bigcache.Config{
		// number of shards (must be a power of 2)
		Shards: shards,

		// time after which entry can be evicted
		LifeWindow: 15 * time.Minute,

		// Interval between removing expired entries (clean up).
		// If set to <= 0 then no action is p4erformed.
		// Setting to < 1 second is counterproductive — bigcache has a one second resolution.
		CleanWindow: 15 * time.Minute,

		// rps * lifeWindow, used only in initial memory allocation
		// MaxEntriesInWindow: 1000 * 10 * 60,

		// max entry size in bytes, used only in initial memory allocation
		MaxEntrySize: _config.MaxEntryCacheSize,

		// prints information about additional memory allocation
		// Verbose: true,

		// cache will not allocate more memory than this limit, value in MB
		// if value is reached then the oldest entries can be overridden for the new ones
		// 0 value means no size limit
		HardMaxCacheSize: totalCacheSize / 1024 / 1024,
	}
	config.Unmarshal("agent.bigcache", &_cacheConfig)

	cache, initErr := bigcache.NewBigCache(_cacheConfig)
	if initErr != nil {
		log.Fatal(initErr)
	}

	logger.Info().Msgf("Cache size: %d MB", _cacheConfig.HardMaxCacheSize)

	service := agentService{
		config:     _config,
		client:     client,
		logger:     logger,
		jrpcSchema: jrpcSchema,
		cache:      cache,
		es:         endpoint.NewSelector(),
	}

	return service
}

func (a agentService) Call(ctx context.Context, rc reqctx.Reqctxs, endpoints []*endpoint.Endpoint) ([]byte, error) {
	jsonrpcs, isBatchCall, err := rpc.UnmarshalJSONRPCs(*rc.Body())
	if err != nil {
		return nil, common.BadRequestError(err.Error(), err)
	}
	if len(jsonrpcs) == 0 {
		if isBatchCall {
			return rpc.MarshalJSONRPCResults([]rpc.SealedJSONRPCResult{})
		}
		return rpc.MarshalJSONRPCResults(rpc.SealedJSONRPCResult{})
	}

	for i := range jsonrpcs {
		if err := a.jrpcSchema.ValidateRequest(jsonrpcs[i].Method(), jsonrpcs[i].Raw()); err != nil {
			return nil, common.BadRequestError(err.Error(), err)
		}
	}

	dispatch := func(data []rpc.JSONRPCer) (any, error) {
		if len(data) == 0 {
			if isBatchCall {
				return []rpc.SealedJSONRPCResult{}, nil
			}
			return rpc.SealedJSONRPCResult{}, nil
		}

		results, err := a.call(ctx, rc, endpoints, data)

		if err != nil {
			return nil, err
		}

		if !isBatchCall || (len(results) == 1 && results[0].Error != nil) {
			return results[0], nil
		}

		return results, nil
	}

	handle := func(_jsonrpcs []rpc.JSONRPCer) ([]byte, error) {
		results, err := dispatch(_jsonrpcs)

		if err != nil {
			return nil, err
		}

		return rpc.MarshalJSONRPCResults(results)
	}

	if a.config.DisableCache || !rc.Options().Caches() {
		return handle(jsonrpcs)
	}

	var (
		chainId   = rc.ChainID()
		mapping   = map[string][]int{}
		_jsonrpcs = []rpc.JSONRPCer{}
		results   = make([]rpc.SealedJSONRPCResult, len(jsonrpcs))
	)

	for i := 0; i < len(jsonrpcs); i++ {
		var v any
		if ok, ttl := _WithCache(a.config.CacheMethods, jsonrpcs[i]); ok {
			key, entry := _CacheKey(chainId, jsonrpcs[i]), &CacheEntry{}
			err := _GetCache(a.cache, key, entry)
			if err == nil {
				if time.UnixMilli(entry.T).Add(ttl).After(time.Now()) {
					if entry.compressed {
						if _v, err := helpers.Decompress(entry.V.([]byte)); err != nil {
							rc.Logger().Warn().Err(err).Msgf("Failed to compress cache %s", jsonrpcs[i].Method())
						} else if err = json.Unmarshal(_v, &v); err != nil {
							rc.Logger().Warn().Err(err).Msgf("Failed to unmarshal cache %s", jsonrpcs[i].Method())
						}
					} else {
						v = entry.V
					}

					if jsonrpcs[i].Method() == "eth_blockNumber" {
						endpoint := slices.MaxFunc(endpoints, func(a *endpoint.Endpoint, b *endpoint.Endpoint) int {
							return int(b.BlockNumber() - a.BlockNumber())
						})
						if height := endpoint.BlockNumber(); height > 0 {
							if v == nil {
								v = height
							} else if n, err := strconv.ParseUint(v.(string), 16, 64); err == nil {
								v = slices.Max([]uint64{height, n})
							}
						}
					}
				} else {
					go a.cache.Delete(key)
				}
			}
		}

		appName := "unknown"
		if rc.App() != nil {
			appName = rc.App().Name
		}

		if v != nil {
			results[i] = jsonrpcs[i].MakeResult(v, nil)
			utils.TotalCaches.WithLabelValues(fmt.Sprint(chainId), appName, jsonrpcs[i].Method(), "mem").Inc()
		} else {
			_jsonrpcs = append(_jsonrpcs, jsonrpcs[i])
			utils.TotalCaches.WithLabelValues(fmt.Sprint(chainId), appName, jsonrpcs[i].Method(), "miss").Inc()

			id := fmt.Sprint(jsonrpcs[i].Raw()["id"])
			if mapping[id] == nil {
				mapping[id] = []int{}
			}
			mapping[id] = append(mapping[id], i)
		}
	}

	if len(_jsonrpcs) <= 0 {
		if isBatchCall {
			return rpc.MarshalJSONRPCResults(results)
		} else if len(results) > 0 {
			return rpc.MarshalJSONRPCResults(results[0])
		}
	}

	data, err := dispatch(_jsonrpcs)

	if err != nil {
		return nil, err
	}

	if _results, ok := data.([]rpc.SealedJSONRPCResult); ok {
		for i := range _results {
			indexes := mapping[fmt.Sprint(_results[i].ID)]

			for _, index := range indexes {
				// 如果已经有缓存结果，则跳过
				if results[index].Result != nil {
					continue
				}
				results[index] = _results[i]
			}
		}

		return rpc.MarshalJSONRPCResults(results)
	}

	return rpc.MarshalJSONRPCResults(data)
}

func (a agentService) call(ctx context.Context, rc reqctx.Reqctxs, endpoints []*endpoint.Endpoint, jsonrpcs []rpc.JSONRPCer) (results []rpc.SealedJSONRPCResult, err error) {
	chainId := rc.ChainID()
	_endpoints, ok := a.es.Select(ctx, rc, endpoints, jsonrpcs)
	if !ok || len(_endpoints) <= 0 {
		a.logger.Error().Msgf("%d No available endpoints", chainId)
		return nil, common.InternalServerError("No available endpoints")
	}

	var (
		prefix    = helpers.Short(rc.ReqID())
		_jsonrpcs = []rpc.SealedJSONRPC{}
	)
	for i := range jsonrpcs {
		_jsonrpc := jsonrpcs[i].Seal()
		_jsonrpc.ID += prefix + _jsonrpc.ID
		_jsonrpcs = append(_jsonrpcs, _jsonrpc)
	}

	_results, err := a.client.Request(ctx, rc, _endpoints, _jsonrpcs)

	if err != nil {
		return nil, err
	}

	results = make([]rpc.SealedJSONRPCResult, len(_results))
	for i := range _results {
		if j := slices.IndexFunc(_jsonrpcs, func(_jsonrpc rpc.SealedJSONRPC) bool {
			return _jsonrpc.ID == _results[i].ID()
		}); j >= 0 {
			results[i] = jsonrpcs[j].MakeResult(_results[i].Result(), _results[i].Error())
		} else {
			results[i] = rpc.SealedJSONRPCResult{}

			if _results[i].ID() != "" {
				results[i].ID = _results[i].ID()
			}
			if _results[i].Version() != "" {
				results[i].Version = _results[i].Version()
			}
			if _results[i].Result() != nil {
				results[i].Result = _results[i].Result()
			}
			if _results[i].Error() != nil {
				results[i].Error = _results[i].Error()
			}
		}
	}

	if !a.config.DisableCache {
		for i := range results {
			if jsonrpc, ok := slice.Find(jsonrpcs, func(_ int, jsonrpc rpc.JSONRPCer) bool {
				return jsonrpc.Raw()["id"] == results[i].ID
			}); ok && _results[i].Type() == rpc.JSONRPC_RESPONSE {
				if ok, _ := _WithCache(a.config.CacheMethods, *jsonrpc); ok {
					key := _CacheKey(chainId, *jsonrpc)
					if data, err := json.Marshal(results[i].Result); err == nil {
						if len(data) > a.config.MaxEntryCacheSize {
							go func(k string, v []byte) {
								defer func() {
									if err := recover(); err != nil {
										a.logger.Error().Interface("error", err).Msg("Failed to set cache result")
									}
								}()

								if compressed, err := helpers.Compress(v); err != nil {
									a.logger.Error().Err(err).Msg("Failed to compress")
								} else {
									// skip set cache, data is bigger than cache size after compression
									if len(compressed) > a.config.MaxEntryCacheSize {
										return
									}
									v = compressed
								}

								if err := _SetCache(a.cache, k, &CacheEntry{V: v, T: time.Now().UnixMilli(), compressed: true}); err != nil {
									a.logger.Error().Err(err).Msg("Cache set error")
								}
							}(key, data)
						} else {
							if err := _SetCache(a.cache, key, &CacheEntry{V: results[i].Result, T: time.Now().UnixMilli()}); err != nil {
								a.logger.Error().Err(err).Msg("Cache set error")
							}
						}

						a.logger.Debug().Msgf("Cache capacity: %d, len: %d", a.cache.Capacity(), a.cache.Len())
					}
				}
			}
		}
	}

	return results, nil
}
