// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lightstepreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/lightstepreceiver"

import (
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
)

const (
	protoHTTP = "protocols::http"
)

type HTTPConfig struct {
	*confighttp.HTTPServerSettings `mapstructure:",squash"`
}

// Protocols is the configuration for the supported protocols.
type Protocols struct {
	HTTP *HTTPConfig `mapstructure:"http"`
}

// Config defines configuration for the Lightstep receiver.
type Config struct {
	// Protocols is the configuration for the supported protocols, currently HTTP.
	Protocols `mapstructure:"protocols"`
}

var _ component.Config = (*Config)(nil)

// Validate checks the receiver configuration is valid
func (cfg *Config) Validate() error {
	if cfg.HTTP == nil {
		 return errors.New("must specify at least one protocol when using the Lightstep receiver")
	}
	return nil
}

// Unmarshal a confmap.Conf into the config struct.
func (cfg *Config) Unmarshal(conf *confmap.Conf) error {
	// first load the config normally
	err := conf.Unmarshal(cfg, confmap.WithErrorUnused())
	if err != nil {
		return err
	}

	if !conf.IsSet(protoHTTP) {
		cfg.HTTP = nil
	}
	return nil
}
