package shared

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/GoPlugin/web3rpcproxy/utils/config"
	kYaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"gopkg.in/yaml.v2"
)

var logger = log.New(os.Stderr, "conf ", log.Ldate|log.Ltime)

const KoanfEndpointsToken = "endpoints"
const KoanfEtcdStaticConfigToken = "etcd.setup.config.file"
const KoanfEtcdEndpointsConfigToken = "etcd.endpoints.config.file"
const KoanfEtcdJSONRPCSchemaToken = "etcd.jsonrpc.schema.file"

func NewConfInstance(etcd *clientv3.Client) *config.Conf {
	k := koanf.New(".")
	conf := &config.Conf{Koanf: k}

	var source = "local"
	defer func() {
		config.LoadEndpointChains(conf, KoanfEndpointsToken)
		logger.Printf(conf.Sprint())
		logger.Printf("Load %s config!", source)
	}()

	if _, err := os.Stat("config/default.yaml"); err != nil {
		logger.Printf("Error read default config: %v", err)
	} else if err := conf.Load(file.Provider("config/default.yaml"), kYaml.Parser()); err != nil {
		logger.Printf("Error loading defautl config: %v", err)
	}
	if _, err := os.Stat("config/local.yaml"); err != nil {
		logger.Printf("Error read local config: %v", err)
	} else if err := conf.Load(file.Provider("config/local.yaml"), kYaml.Parser()); err != nil {
		logger.Printf("Error load local config: %v", err)
	}

	if err := conf.Load(env.ProviderWithValue("WEB3RPCPROXY_", ".", func(s string, v string) (string, interface{}) {
		// Strip out the MYVAR_ prefix and lowercase and get the key wjhile also replacing
		// the _ character with . in the key (koanf delimeter).
		key := strings.Replace(strings.ToLower(strings.TrimPrefix(s, "WEB3RPCPROXY_")), "_", ".", -1)

		// If there is a space in the value, split the value into a slice by the space.
		if strings.Contains(v, " ") {
			return key, strings.Split(v, " ")
		}

		// Otherwise, return the plain string.
		return key, v
	}), nil); err != nil {
		logger.Printf("Error load env: %v", err)
	}

	if etcd == nil {
		return conf
	}

	if conf.Exists(KoanfEtcdStaticConfigToken) {
		if resp, err := etcd.Get(context.Background(), conf.String(KoanfEtcdStaticConfigToken)); err != nil {
			logger.Printf("Error read etcd config %v", err)
		} else if len(resp.Kvs) < 1 {
			logger.Printf("ETCD got empty value!")
		} else if err := conf.Load(rawbytes.Provider(resp.Kvs[0].Value), kYaml.Parser()); err != nil {
			logger.Printf("Error load etcd config: %v", err)
		} else {
			source = "etcd"
		}
	}

	if conf.Exists(KoanfEtcdEndpointsConfigToken) {
		if resp, err := etcd.Get(context.Background(), conf.String(KoanfEtcdEndpointsConfigToken)); err != nil {
			logger.Printf("Error read etcd endpoints config %v", err)
		} else if len(resp.Kvs) < 1 {
			logger.Printf("ETCD got empty value!")
		} else {
			var (
				val = []any{}
				err = yaml.Unmarshal(resp.Kvs[0].Value, &val)
			)
			if err != nil {
				logger.Printf("Error load etcd endpoints config: %v", err)
			} else {
				conf.Set(KoanfEndpointsToken, val)
			}
		}
	}

	if conf.Bool("jsonrpc.enable_validation", false) && conf.Exists(KoanfEtcdJSONRPCSchemaToken) {
		resp, err := etcd.Get(context.Background(), conf.String(KoanfEtcdJSONRPCSchemaToken))
		if err != nil {
			logger.Printf("Error read etcd jsonrpc schema %v", err)
		} else if len(resp.Kvs) > 0 && len(resp.Kvs[0].Value) > 0 {
			conf.Set("jsonrpc.schema", resp.Kvs[0].Value)
		}
	} else if conf.Bool("jsonrpc.enable_validation", false) {
		b, err := os.ReadFile("config/ethereum-openrpc.json")
		if err != nil {
			log.Printf("Error read local jsonrpc schema: %v", err)
		} else {
			conf.Set("jsonrpc.schema", b)
		}
	}

	return conf
}
