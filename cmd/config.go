/*
Package cmd includes relayer commands
Copyright © 2020 Jack Zampolin jack.zampolin@gmail.com

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/unknownfeature/ibc-tools/relayer"
	"github.com/unknownfeature/ibc-tools/relayer/chains/cosmos"
	"github.com/unknownfeature/ibc-tools/relayer/provider"
	"gopkg.in/yaml.v3"
	"reflect"
)

// Wrapped converts the Config struct into a ConfigOutputWrapper struct
func (c *Config) Wrapped() *ConfigOutputWrapper {
	providers := make(ProviderConfigs)
	for _, chain := range c.Chains {
		pcfgw := &ProviderConfigWrapper{
			Type:  chain.ChainProvider.Type(),
			Value: chain.ChainProvider.ProviderConfig(),
		}
		providers[chain.ChainProvider.ChainName()] = pcfgw
	}
	return &ConfigOutputWrapper{Global: c.Global, ProviderConfigs: providers}
}

// Config represents the config file for the relayer
type Config struct {
	Global GlobalConfig   `yaml:"global" json:"global"`
	Chains relayer.Chains `yaml:"chains" json:"chains"`
}

// ConfigOutputWrapper is an intermediary type for writing the config to disk and stdout
type ConfigOutputWrapper struct {
	Global          GlobalConfig    `yaml:"global" json:"global"`
	ProviderConfigs ProviderConfigs `yaml:"chains" json:"chains"`
}

// ConfigInputWrapper is an intermediary type for parsing the config.yaml file
type ConfigInputWrapper struct {
	Global          GlobalConfig                          `yaml:"global"`
	ProviderConfigs map[string]*ProviderConfigYAMLWrapper `yaml:"chains"`
}

type ProviderConfigs map[string]*ProviderConfigWrapper

// ProviderConfigWrapper is an intermediary type for parsing arbitrary ProviderConfigs from json files and writing to json/yaml files
type ProviderConfigWrapper struct {
	Type  string                  `yaml:"type"  json:"type"`
	Value provider.ProviderConfig `yaml:"value" json:"value"`
}

// ProviderConfigYAMLWrapper is an intermediary type for parsing arbitrary ProviderConfigs from yaml files
type ProviderConfigYAMLWrapper struct {
	Type  string `yaml:"type"`
	Value any    `yaml:"-"`
}

// UnmarshalJSON adds support for unmarshalling data from an arbitrary ProviderConfig
// NOTE: Add new ProviderConfig types in the map here with the key set equal to the type of ChainProvider (e.g. cosmos, substrate, etc.)
func (pcw *ProviderConfigWrapper) UnmarshalJSON(data []byte) error {
	customTypes := map[string]reflect.Type{
		"cosmos": reflect.TypeOf(cosmos.CosmosProviderConfig{}),
	}
	val, err := UnmarshalJSONProviderConfig(data, customTypes)
	if err != nil {
		return err
	}
	pc := val.(provider.ProviderConfig)
	pcw.Value = pc
	return nil
}

// UnmarshalJSONProviderConfig contains the custom unmarshalling logic for ProviderConfig structs
func UnmarshalJSONProviderConfig(data []byte, customTypes map[string]reflect.Type) (any, error) {
	m := map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	typeName, ok := m["type"].(string)
	if !ok {
		return nil, errors.New("cannot find type")
	}

	var provCfg provider.ProviderConfig
	if ty, found := customTypes[typeName]; found {
		provCfg = reflect.New(ty).Interface().(provider.ProviderConfig)
	}

	valueBytes, err := json.Marshal(m["value"])
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(valueBytes, &provCfg); err != nil {
		return nil, err
	}

	return provCfg, nil
}

// UnmarshalYAML adds support for unmarshalling data from arbitrary ProviderConfig entries found in the config file
// NOTE: Add logic for new ProviderConfig types in a switch case here
func (iw *ProviderConfigYAMLWrapper) UnmarshalYAML(n *yaml.Node) error {
	type inputWrapper ProviderConfigYAMLWrapper
	type T struct {
		*inputWrapper `yaml:",inline"`
		Wrapper       yaml.Node `yaml:"value"`
	}

	obj := &T{inputWrapper: (*inputWrapper)(iw)}
	if err := n.Decode(obj); err != nil {
		return err
	}

	switch iw.Type {
	case "cosmos":
		iw.Value = new(cosmos.CosmosProviderConfig)
	default:
		return fmt.Errorf("%s is an invalid chain type, check your config file", iw.Type)
	}

	return obj.Wrapper.Decode(iw.Value)
}

// MustYAML returns the yaml string representation of the Paths
func (c Config) MustYAML() []byte {
	out, err := yaml.Marshal(c)
	if err != nil {
		panic(err)
	}
	return out
}

func defaultConfigYAML(memo string) []byte {
	return DefaultConfig(memo).MustYAML()
}

func DefaultConfig(memo string) *Config {
	return &Config{
		Global: newDefaultGlobalConfig(memo),
		Chains: make(relayer.Chains),
	}
}

// GlobalConfig describes any global relayer settings
type GlobalConfig struct {
	ApiListenPort     string `yaml:"api-listen-addr,omitempty" json:"api-listen-addr,omitempty"`
	DebugListenPort   string `yaml:"debug-listen-addr" json:"debug-listen-addr"`
	MetricsListenPort string `yaml:"metrics-listen-addr" json:"metrics-listen-addr"`
	Timeout           string `yaml:"timeout" json:"timeout"`
	Memo              string `yaml:"memo" json:"memo"`
	LightCacheSize    int    `yaml:"light-cache-size" json:"light-cache-size"`
	LogLevel          string `yaml:"log-level" json:"log-level"`
	ICS20MemoLimit    int    `yaml:"ics20-memo-limit" json:"ics20-memo-limit"`
	MaxReceiverSize   int    `yaml:"max-receiver-size" json:"max-receiver-size"`
}

// newDefaultGlobalConfig returns a global config with defaults set
func newDefaultGlobalConfig(memo string) GlobalConfig {
	return GlobalConfig{
		ApiListenPort:     "",
		DebugListenPort:   "127.0.0.1:5183",
		MetricsListenPort: "127.0.0.1:5184",
		Timeout:           "10s",
		LightCacheSize:    20,
		Memo:              memo,
		LogLevel:          "info",
		MaxReceiverSize:   150,
	}
}

// AddChain adds an additional chain to the config
func (c *Config) AddChain(chain *relayer.Chain) (err error) {
	chainId := chain.ChainProvider.ChainId()
	if chainId == "" {
		return errors.New("chain ID cannot be empty")
	}
	chn, err := c.Chains.Get(chainId)
	if chn != nil || err == nil {
		return fmt.Errorf("chain with ID %s already exists in config", chainId)
	}
	c.Chains[chain.ChainProvider.ChainName()] = chain
	return nil
}

func checkPathConflict(pathID, fieldName, oldP, newP string) (err error) {
	if oldP != "" && oldP != newP {
		return fmt.Errorf(
			"path with ID %s and conflicting %s (%s) already exists",
			pathID, fieldName, oldP,
		)
	}
	return nil
}

// DeleteChain modifies c in-place to remove any chains that have the given name.
func (c *Config) DeleteChain(chain string) {
	delete(c.Chains, chain)
}
