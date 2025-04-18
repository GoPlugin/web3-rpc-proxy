package common

import "github.com/GoPlugin/web3rpcproxy/utils/general/names"

type QueryStatus string

const (
	Success   QueryStatus = "success"
	Fail      QueryStatus = "fail"
	Timeout   QueryStatus = "timeout"
	Intercept QueryStatus = "intercept"
	Reject    QueryStatus = "reject"
	Error     QueryStatus = "error"
)

type OptionsProfile = struct {
	SpecifiedUpstreamTypes []string      `json:"specifiedUpstreamTypes,omitempty"`
	ForceUpstreamType      string        `json:"forceUpstreamType,omitempty"`
	Timeout                names.Seconds `json:"timeout,omitempty"`

	BeforeBlocksUseScanApi int `json:"beforeBlocksUseScanApi,omitempty"`
	BeforeBlocksUseActive  int `json:"beforeBlocksUseActive,omitempty"`

	MaxRetryCount int  `json:"maxRetryCount,omitempty"`
	UseCache      bool `json:"useCache,omitempty"`
	UseScanApi    bool `json:"useScanApi,omitempty"`

	EthCallUseFullNode bool `json:"ethCallUseFullNode,omitempty"`
}

type RequestProfile = struct {
	Methods   []string           `json:"methods"`
	ReqID     names.UUIDv4       `json:"reqId"`
	Url       string             `json:"url"`
	Timestamp names.Milliseconds `json:"timestamp"`
}

type ResponseProfile = struct {
	ReqID    names.UUIDv4       `json:"reqId"`
	Error    string             `json:"error,omitempty"`
	Message  string             `json:"message,omitempty"`
	Code     string             `json:"code,omitempty"`
	Duration names.Milliseconds `json:"duration"`
	Traffic  names.Bytes        `json:"traffic"`
	Status   int                `json:"status"`
	Respond  bool               `json:"respond"`
}

type QueryProfile = struct {
	Options OptionsProfile `json:"options"`

	Requests []RequestProfile `json:"requests"`

	Responses []ResponseProfile `json:"responses"`

	ID        names.UUIDv4 `json:"id"`
	Href      names.Url    `json:"href"`
	Method    string       `json:"method"`
	IP        string       `json:"ip"`
	IPCountry string       `json:"ipCountry"`

	Status QueryStatus `json:"status"`

	AppID   uint64 `json:"appId"`
	ChainID uint64 `json:"chainId"`

	Starttime names.Milliseconds `json:"startTime"`
	Endtime   names.Milliseconds `json:"endTime"`
}
