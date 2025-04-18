package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/GoPlugin/web3rpcproxy/internal/common"
	"github.com/GoPlugin/web3rpcproxy/internal/core/rpc"
	"github.com/GoPlugin/web3rpcproxy/utils/helpers"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type websocketClientConfig struct {
	Transport     *http.Transport
	JSONRPCSchema *rpc.JSONRPCSchema
}

type websocketClient struct {
	logger   zerolog.Logger
	endpoint *Endpoint
	conn     *websocket.Conn
	sessions sync.Map
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	config   *websocketClientConfig
}

func NewWebSocketClient(endpoint *Endpoint, config *websocketClientConfig) Client {
	url := endpoint.Url().String()
	logger := zerolog.New(os.Stderr).With().Timestamp().Str("name", "web socket endpoint").Str("url", url).Logger()
	if url == "" {
		return nil
	}

	e := &websocketClient{
		endpoint: endpoint,
		logger:   logger,
		config:   config,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e.conn = e.connect(ctx)
	if e.conn == nil {
		e = nil
		return nil
	}

	return e
}

func getJSONResultKey(data []rpc.JSONRPCResulter) string {
	ids := slice.Map(data, func(i int, result rpc.JSONRPCResulter) string {
		return result.ID()
	})
	slice.Sort(ids)
	return helpers.Short(slice.Join(ids, ""))
}

func background(ctx context.Context, logger zerolog.Logger, conn *websocket.Conn, sessions *sync.Map) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error().Interface("error", err).Msg("Failed to revceive message")
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				logger.Warn().Msgf("Error reading message: %v", err)
				if websocket.IsCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseInternalServerErr) || err == websocket.ErrCloseSent || err == io.ErrUnexpectedEOF {
					return
				}
			}

			if messageType == websocket.TextMessage {
				results, isBatchResult, err := rpc.UnmarshalJSONRPCResults(message)
				if err != nil {
					logger.Warn().Msgf("Failed to unmarshal message: %s", message)
					continue
				}

				if !isBatchResult && len(results) == 1 && results[0].Type() == rpc.JSONRPC_ERROR {
					sessions.Range(func(key, value interface{}) bool {
						if c := value.(chan []rpc.JSONRPCResulter); c != nil {
							c <- results
							return false
						}
						return true
					})
				}

				key := getJSONResultKey(results)
				if c, ok := sessions.Load(key); ok {
					c.(chan []rpc.JSONRPCResulter) <- results
				}
			}
		}
	}
}

func (e *websocketClient) connect(ctx context.Context) *websocket.Conn {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	if _headers := e.endpoint.Headers(); _headers != nil {
		for key, value := range _headers {
			headers.Set(key, value)
		}
	}

	dialer := websocket.Dialer{
		EnableCompression: true,
	}
	if e.config.Transport != nil {
		dialer.TLSClientConfig = e.config.Transport.TLSClientConfig.Clone()
	}

	_connect := func(ctx context.Context) *websocket.Conn {
		var (
			duration int64 = 0
			health   bool  = false
		)

		defer func() {
			ops := []Attributer{
				WithAttr(Health, health),
				WithAttr(LastUpdateTime, time.Now()),
			}
			if duration > 0 {
				ops = append(ops, WithAttr(Duration, float64(duration)))
			}
			e.endpoint.Update(ops...)
		}()

		now := time.Now()
		conn, _, err := dialer.DialContext(ctx, e.endpoint.Url().String(), headers)
		if err != nil {
			e.logger.Error().Msgf("Error creating connection: %v", err)
			return nil
		}

		duration = time.Since(now).Milliseconds()
		health = true

		_ctx, _cancel := context.WithCancel(context.Background())
		e.ctx = _ctx
		e.cancel = _cancel
		go background(e.ctx, e.logger, conn, &e.sessions)

		return conn
	}

	conn := _connect(ctx)

	if conn == nil {
		return nil
	}

	conn.SetCloseHandler(func(code int, text string) error {
		e.logger.Warn().Msgf("Closing connection code %d and text %s", code, text)

		e.endpoint.Update(
			WithAttr(Health, false),
			WithAttr(LastUpdateTime, time.Now()),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		oldConn, oldCancel := e.conn, e.cancel
		e.conn = _connect(ctx)
		cancel()

		message := websocket.FormatCloseMessage(code, "")
		oldConn.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))
		time.AfterFunc(time.Second, func() {
			oldConn.Close()
			oldCancel()
		})
		return nil
	})

	return conn
}

func (e *websocketClient) Close() error {
	message := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	e.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))
	time.AfterFunc(time.Second, func() {
		e.conn.Close()
		e.cancel()
	})
	return nil
}

func isBrokenPipeError(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Op == "write" && netErr.Err.Error() == "write: broken pipe" {
			return true
		}
	}
	return false
}

func (e *websocketClient) request(ctx context.Context, key string, b []byte) ([]rpc.JSONRPCResulter, common.HTTPErrors) {
	c := make(chan []rpc.JSONRPCResulter)
	e.sessions.Store(key, c)
	defer func() {
		if c, ok := e.sessions.Load(key); ok {
			e.sessions.Delete(key)
			close(c.(chan []rpc.JSONRPCResulter))
		}
		_EndpointGauge(e.endpoint).Dec()
	}()

	_EndpointGauge(e.endpoint).Inc()
	e.mu.Lock()
	err := e.conn.WriteMessage(websocket.TextMessage, b)
	e.mu.Unlock()

	if err != nil {
		e.logger.Warn().Msgf("Creating request %s %s", e.endpoint.Url(), b)
		e.logger.Error().Msgf("Error creating request: %v", err)

		if isBrokenPipeError(err) {
			_ctx, cancel := context.WithTimeoutCause(context.Background(), time.Second, nil)
			e.mu.Lock()
			conn := e.connect(_ctx)
			if conn != nil {
				e.conn = conn
			}
			e.mu.Unlock()
			cancel()
		}

		if _err, ok := err.(*net.OpError); ok {
			return nil, common.UpstreamServerError("Error creating request", _err.Err)
		} else {
			return nil, common.UpstreamServerError("Error creating request", err)
		}
	}

	select {
	case results, ok := <-c:
		if !ok {
			return nil, common.UpstreamServerError("Error connection to endpoint", nil)
		}
		return results, nil
	case <-ctx.Done():
		return nil, common.TimeoutError("context deadline exceeded")
	}
}

func getJSONRPCKey(data []rpc.SealedJSONRPC) string {
	ids := slice.Map(data, func(i int, jsonrpc rpc.SealedJSONRPC) string {
		return fmt.Sprint(jsonrpc.ID)
	})
	slice.Sort(ids)
	return helpers.Short(slice.Join(ids, ""))
}

func (e *websocketClient) Call(ctx context.Context, data []rpc.SealedJSONRPC, profiles ...*common.ResponseProfile) (results []rpc.JSONRPCResulter, err error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, common.InternalServerError("Marshalling request failed", err)
	}

	var profile = &common.ResponseProfile{}
	if len(profiles) > 0 {
		profile = profiles[0]
	}

	now, key := time.Now(), getJSONRPCKey(data)
	results, err = e.request(ctx, key, b)
	profile.Duration = time.Since(now).Milliseconds()

	defer updateMetrics(e.endpoint, profile)

	if err != nil {
		switch err.Error() {
		case "Error connection to endpoint":
			profile.Code = "connection_error"
		case "Error creating request":
			profile.Code = "request_error"
		}
		profile.Error = err.Error()
		return nil, err
	}

	body, err := json.Marshal(results)
	if err == nil {
		profile.Status = 200
		profile.Traffic = len(body)
	}

	if len(results) > 0 {
		if r := results[len(results)-1]; r.Type() == rpc.JSONRPC_ERROR {
			recordingErrorResult(profile, r)
			return results, nil
		}
	}

	// maybe is single result
	if len(results) != len(data) {
		return results, nil
	}

	// maybe includes normal results, should be validate by schema
	if e.config.JSONRPCSchema != nil {
		if err := validateResults(e.logger, e.config.JSONRPCSchema, profile, data, results); err != nil {
			return nil, common.UpstreamServerError("Validating response failed", err)
		}
	}

	return results, nil
}
